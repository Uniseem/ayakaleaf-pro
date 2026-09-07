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
	// reconnect, matching the Node service's defaults.
	closeSeconds = 60
)

// Conn is one connected client.
type Conn struct {
	ID      string
	Request *http.Request

	ws     *websocket.Conn
	send   chan string
	closed chan struct{}
	once   sync.Once
	log    *slog.Logger
}

// Emit sends an event to this client.
func (c *Conn) Emit(name string, args ...any) error {
	frame, err := EncodeEvent(name, args...)
	if err != nil {
		return err
	}
	return c.write(frame)
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

// Handler receives the lifecycle of one connection.
type Handler interface {
	// OnConnect runs after the transport is established. Returning an error
	// rejects the connection.
	OnConnect(c *Conn) error
	// OnEvent handles one client event.
	OnEvent(c *Conn, name string, args []json.RawMessage)
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
}

// pending is a handshake that has not yet been upgraded to a transport.
type pending struct {
	request  *http.Request
	deadline time.Time
}

// NewServer builds a socket.io server.
func NewServer(h Handler, log *slog.Logger) *Server {
	return &Server{
		handler:  h,
		log:      log,
		sessions: map[string]*pending{},
		polling:  map[string]*pollingConn{},
	}
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

// ServeHTTP routes the two socket.io endpoints:
//
//	GET /socket.io/1/?t=...              the handshake
//	GET /socket.io/1/websocket/<sid>     the transport upgrade
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
		// supported deployment, but the check is off here because a
		// reverse proxy may rewrite Host.
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.log.Info("websocket upgrade failed", slog.String("err", err.Error()))
		return
	}

	c := &Conn{
		ID:      sessionID,
		Request: p.request,
		ws:      ws,
		send:    make(chan string, 64),
		closed:  make(chan struct{}),
		log:     s.log,
	}
	s.run(c)
}

func (s *Server) run(c *Conn) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		c.Close()
		_ = c.ws.CloseNow()
		s.handler.OnDisconnect(c)
	}()

	// The writer owns the socket for writes; everything else queues frames.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case frame := <-c.send:
				if err := c.ws.Write(ctx, websocket.MessageText, []byte(frame)); err != nil {
					c.Close()
					return
				}
			case <-c.closed:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	defer wg.Wait()

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
			s.handler.OnEvent(c, p.Name, p.Args)
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
