package socketio

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	// heartbeatSeconds is the interval the client is told to expect; it sends
	// a heartbeat frame and the server answers with one.
	heartbeatSeconds = 30
	// closeSeconds is how long the client waits before giving up on a
	// reconnect, matching the defaults of the Node service.
	closeSeconds = 60
)

// Conn is one connected client.
type Conn struct {
	ID      string
	Request *http.Request
	// Transport is "websocket" or "xhr-polling". The Node service labels most
	// of its metrics with it and takes one behavioural decision on it (server
	// pings are websocket-only), so it has to be visible to the handler.
	Transport string

	srv    *Server
	ws     *websocket.Conn
	send   chan string
	closed chan struct{}
	once   sync.Once
	log    *slog.Logger

	// rooms is guarded by srv.mu, so a join and the count that decides whether
	// the room is newly active cannot interleave.
	rooms map[string]struct{}
}

// Emit sends an event to this client.
func (c *Conn) Emit(name string, args ...any) error {
	frame, err := EncodeEvent(name, args...)
	if err != nil {
		return err
	}
	return c.write(frame)
}

// Ack answers an event the client asked to have acknowledged.
//
// The frame is 6:::<id>+<args>, and the client applies the args to the
// callback it passed to emit(). Every rpc the editor makes is shaped this way,
// so a missing ack leaves the editor waiting rather than failing.
func (c *Conn) Ack(id string, args ...any) error {
	if id == "" {
		return nil
	}
	data := id
	if len(args) > 0 {
		encoded, err := json.Marshal(args)
		if err != nil {
			return err
		}
		data += "+" + string(encoded)
	}
	return c.write(Encode(Packet{Type: PacketAck, Data: data}))
}

func (c *Conn) write(frame string) error {
	select {
	case c.send <- frame:
		return nil
	case <-c.closed:
		return context.Canceled
	}
}

// Close ends the connection.
func (c *Conn) Close() {
	c.once.Do(func() { close(c.closed) })
}

// Disconnected reports whether the connection has ended. The Node service
// checks client.disconnected at every await point to avoid work whose result
// nobody will read.
func (c *Conn) Disconnected() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

// Done is closed when the connection ends.
func (c *Conn) Done() <-chan struct{} { return c.closed }

// Join puts the client in a room and reports how many clients were in it
// beforehand. Zero means the room has just become active.
func (c *Conn) Join(room string) int {
	return c.srv.join(c, room)
}

// Leave removes the client from a room, reporting how many remain and whether
// it was in the room at all. A client that is not in the room is left alone:
// the editor sends spurious leaveDoc calls after a reconnect.
func (c *Conn) Leave(room string) (remaining int, wasIn bool) {
	return c.srv.leave(c, room)
}

// Rooms lists the rooms the client is in.
func (c *Conn) Rooms() []string {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	rooms := make([]string, 0, len(c.rooms))
	for room := range c.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// Event is one event from a client.
type Event struct {
	Name string
	Args []json.RawMessage
	// ID is the acknowledgement id, empty when the client wants no reply.
	ID string
}

// Handler receives the lifecycle of one connection.
type Handler interface {
	// OnConnect runs after the transport is established. Returning an error
	// rejects the connection.
	OnConnect(c *Conn) error
	// OnEvent handles one client event.
	OnEvent(c *Conn, ev Event)
	// OnDisconnect runs once, after the connection ends.
	OnDisconnect(c *Conn)
}

// Server speaks socket.io 0.9 over HTTP.
type Server struct {
	handler Handler
	log     *slog.Logger

	// OfferTransports overrides what the handshake advertises. Nil offers
	// Transports.
	OfferTransports []string

	mu       sync.Mutex
	sessions map[string]*pending
	polling  map[string]*pollingConn
	conns    map[string]*Conn
	rooms    map[string]map[string]*Conn

	done      chan struct{}
	closeOnce sync.Once
}

// pending is a handshake that has not yet been upgraded to a transport.
type pending struct {
	request  *http.Request
	deadline time.Time
}

// NewServer builds a socket.io server.
func NewServer(h Handler, log *slog.Logger) *Server {
	s := &Server{
		handler:  h,
		log:      log,
		sessions: map[string]*pending{},
		polling:  map[string]*pollingConn{},
		conns:    map[string]*Conn{},
		rooms:    map[string]map[string]*Conn{},
		done:     make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.done
		cancel()
	}()
	go s.sweep(ctx)
	return s
}

// Transports are offered in the handshake, in the order the Node service
// offers them. The client tries websocket first and falls back to polling when
// it cannot be established -- which happens behind proxies that do not pass
// upgrades through.
var Transports = []string{"websocket", "xhr-polling"}

// transports returns the transports to advertise. Restricting them is how the
// polling path is exercised on demand: the client always prefers websocket
// when it is offered.
func (s *Server) transports() []string {
	if s.OfferTransports != nil {
		return s.OfferTransports
	}
	return Transports
}

func (s *Server) newConn(id string, r *http.Request, transport string, ws *websocket.Conn) *Conn {
	c := &Conn{
		ID:        id,
		Request:   r,
		Transport: transport,
		srv:       s,
		ws:        ws,
		send:      make(chan string, 64),
		closed:    make(chan struct{}),
		log:       s.log,
		rooms:     map[string]struct{}{},
	}
	s.mu.Lock()
	s.conns[id] = c
	s.mu.Unlock()
	return c
}

// forget drops a client and its room memberships once it has disconnected.
func (s *Server) forget(c *Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, c.ID)
	for room := range c.rooms {
		if members := s.rooms[room]; members != nil {
			delete(members, c.ID)
			if len(members) == 0 {
				delete(s.rooms, room)
			}
		}
	}
	c.rooms = map[string]struct{}{}
}

func (s *Server) join(c *Conn, room string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.rooms[room]
	before := len(members)
	if members == nil {
		members = map[string]*Conn{}
		s.rooms[room] = members
	}
	members[c.ID] = c
	c.rooms[room] = struct{}{}
	return before
}

func (s *Server) leave(c *Conn, room string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, in := c.rooms[room]; !in {
		return len(s.rooms[room]), false
	}
	delete(c.rooms, room)
	members := s.rooms[room]
	delete(members, c.ID)
	remaining := len(members)
	if remaining == 0 {
		delete(s.rooms, room)
	}
	return remaining, true
}

// ClientsIn lists the clients in a room.
func (s *Server) ClientsIn(room string) []*Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.rooms[room]
	clients := make([]*Conn, 0, len(members))
	for _, c := range members {
		clients = append(clients, c)
	}
	return clients
}

// CountClientsIn counts the clients in a room without building a slice; it is
// on the path of every published event.
func (s *Server) CountClientsIn(room string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rooms[room])
}

// Clients lists every connected client.
func (s *Server) Clients() []*Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	clients := make([]*Conn, 0, len(s.conns))
	for _, c := range s.conns {
		clients = append(clients, c)
	}
	return clients
}

// Client looks a client up by its session id.
func (s *Server) Client(id string) *Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns[id]
}

// CountClients reports how many clients are connected.
func (s *Server) CountClients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// Broadcast emits an event to every connected client.
func (s *Server) Broadcast(name string, args ...any) {
	frame, err := EncodeEvent(name, args...)
	if err != nil {
		s.log.Error("dropping unencodable broadcast", slog.String("event", name))
		return
	}
	for _, c := range s.Clients() {
		_ = c.write(frame)
	}
}

// ServeHTTP routes the socket.io endpoints:
//
//	GET /socket.io/1/?t=...             the handshake
//	GET /socket.io/1/websocket/<sid>    the transport upgrade
//	*   /socket.io/1/xhr-polling/<sid>  the polling transport
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/socket.io/1")
	path = strings.Trim(path, "/")

	switch {
	case path == "":
		s.handshake(w, r)
	case strings.HasPrefix(path, "websocket/"):
		s.upgrade(w, r, strings.TrimPrefix(path, "websocket/"))
	case strings.HasPrefix(path, "xhr-polling/"):
		s.servePolling(w, r, strings.TrimPrefix(path, "xhr-polling/"))
	default:
		// jsonp-polling and the flashsocket transport are not implemented.
		// Saying so plainly beats a bare 404, which the client reports as a
		// network error.
		http.Error(w, "transport not supported", http.StatusNotImplemented)
	}
}

func (s *Server) handshake(w http.ResponseWriter, r *http.Request) {
	id, err := newSessionID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.sessions[id] = &pending{request: r.Clone(r.Context()), deadline: time.Now().Add(time.Minute)}
	s.sweepLocked()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write([]byte(Handshake(id, heartbeatSeconds, closeSeconds, s.transports())))
}

// sweepLocked drops handshakes that were never upgraded, so an abandoned
// connect attempt cannot accumulate.
func (s *Server) sweepLocked() {
	now := time.Now()
	for id, p := range s.sessions {
		if now.After(p.deadline) {
			delete(s.sessions, id)
		}
	}
}

func (s *Server) upgrade(w http.ResponseWriter, r *http.Request, sessionID string) {
	s.mu.Lock()
	p, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if !ok {
		// The client has a session id the server does not know, which happens
		// after a restart. Telling it so makes it handshake again instead of
		// retrying a dead id.
		http.Error(w, Encode(Packet{
			Type: PacketError,
			Data: itoa(ReasonClientNotHandshaken) + "+" + itoa(AdviceReconnect),
		}), http.StatusBadRequest)
		return
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The editor is served from the same origin as this service in every
		// supported deployment, but the check is off here because a reverse
		// proxy may rewrite Host.
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.log.Info("websocket upgrade failed", slog.String("err", err.Error()))
		return
	}

	s.run(s.newConn(sessionID, p.request, "websocket", ws))
}

// writeTimeout bounds a single frame write, so one stalled client cannot pin
// a goroutine indefinitely.
const writeTimeout = 30 * time.Second

func (s *Server) run(c *Conn) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Closing the connection has to interrupt the blocked read, or a client
	// the server has rejected would linger until it happened to send
	// something -- or forever.
	go func() {
		select {
		case <-c.closed:
			cancel()
		case <-ctx.Done():
		}
	}()

	// The writer owns the socket for writes; everything else queues frames.
	var wg sync.WaitGroup
	defer func() {
		// Close first, then wait: the writer parks on c.closed, so waiting for
		// it before closing would hang here forever -- and the disconnect
		// would never be reported, leaving the client in its rooms and its
		// Redis channels subscribed.
		c.Close()
		wg.Wait()
		_ = c.ws.CloseNow()
		s.forget(c)
		s.handler.OnDisconnect(c)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Writes use their own deadline rather than the connection context,
		// which is cancelled the moment the connection closes -- and the
		// frames that still need writing at that point are precisely the ones
		// explaining why (connectionRejected, otUpdateError, reconnect
		// requests).
		write := func(frame string) bool {
			writeCtx, done := context.WithTimeout(context.Background(), writeTimeout)
			defer done()
			return c.ws.Write(writeCtx, websocket.MessageText, []byte(frame)) == nil
		}
		for {
			select {
			case frame := <-c.send:
				if !write(frame) {
					c.Close()
					return
				}
			case <-c.closed:
				// Flush whatever is already queued before giving up on the
				// socket.
				for {
					select {
					case frame := <-c.send:
						if !write(frame) {
							return
						}
					default:
						return
					}
				}
			}
		}
	}()

	// The client waits for a connect packet before it considers itself open.
	if err := c.write(Encode(Packet{Type: PacketConnect})); err != nil {
		return
	}
	if err := s.handler.OnConnect(c); err != nil {
		_ = c.write(Encode(Packet{
			Type: PacketError,
			Data: itoa(ReasonUnauthorized),
		}))
		return
	}

	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		p, err := Decode(string(data))
		if err != nil {
			s.log.Info("dropping malformed frame", slog.String("err", err.Error()))
			continue
		}
		switch p.Type {
		case PacketHeartbeat:
			// The client expects the heartbeat echoed; a missed one makes it
			// tear the connection down and reconnect.
			if err := c.write(Encode(Packet{Type: PacketHeartbeat})); err != nil {
				return
			}
		case PacketDisconnect:
			return
		case PacketEvent:
			s.handler.OnEvent(c, Event{Name: p.Name, Args: p.Args, ID: p.ID})
		}
		// A handler that closed the connection ends the read loop here rather
		// than on the next failed read, so the disconnect is not delayed until
		// the client happens to send something.
		if c.Disconnected() {
			return
		}
	}
}

func newSessionID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func itoa(i int) string {
	return string(rune('0' + i))
}
