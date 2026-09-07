package socketio

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// pollTimeout is how long a GET is held open before it is answered with a
// noop. The client reissues immediately, which keeps proxies from closing an
// idle connection themselves.
const pollTimeout = 20 * time.Second

// idlePollTimeout is how long a polling client may go without making a
// request before it is treated as gone. A live client reissues its GET the
// moment the previous one returns, so anything past a few poll timeouts is a
// client that has left.
const idlePollTimeout = 3 * pollTimeout

// pollingConn is a Conn served over xhr-polling rather than a websocket.
//
// The editor falls back to polling when a websocket cannot be established,
// which is a real case behind corporate proxies. Without it those users simply
// cannot open a project, and the failure looks like the editor being broken
// rather than a transport being unavailable.
type pollingConn struct {
	conn *Conn

	mu      sync.Mutex
	pending []string
	// lastSeen is when the client last made a request. A polling client that
	// vanishes sends nothing to notice, so the only way to disconnect it is to
	// stop hearing from it.
	lastSeen time.Time
	// waiting is signalled when a frame arrives while a GET is parked.
	waiting chan struct{}
}

func newPollingConn(c *Conn) *pollingConn {
	return &pollingConn{conn: c, waiting: make(chan struct{}, 1), lastSeen: time.Now()}
}

// touch records that the client is still there.
func (p *pollingConn) touch() {
	p.mu.Lock()
	p.lastSeen = time.Now()
	p.mu.Unlock()
}

// idleFor reports how long it has been since the client last asked.
func (p *pollingConn) idleFor(now time.Time) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return now.Sub(p.lastSeen)
}

// queue adds a frame to be delivered on the next poll.
func (p *pollingConn) queue(frame string) {
	p.mu.Lock()
	p.pending = append(p.pending, frame)
	p.mu.Unlock()
	select {
	case p.waiting <- struct{}{}:
	default:
	}
}

// take removes everything queued.
func (p *pollingConn) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	frames := p.pending
	p.pending = nil
	return frames
}

// serveGET answers a long poll, waiting for a frame until the timeout.
func (p *pollingConn) serveGET(w http.ResponseWriter, r *http.Request) {
	if frames := p.take(); len(frames) > 0 {
		writePolling(w, r, EncodePayload(frames))
		return
	}

	timer := time.NewTimer(pollTimeout)
	defer timer.Stop()
	select {
	case <-p.waiting:
		frames := p.take()
		if len(frames) == 0 {
			frames = []string{Encode(Packet{Type: PacketNoop})}
		}
		writePolling(w, r, EncodePayload(frames))
	case <-timer.C:
		// Nothing to say; a noop closes this poll so the client opens the next.
		writePolling(w, r, Encode(Packet{Type: PacketNoop}))
	case <-p.conn.closed:
		// Deliver whatever was queued before the close: the last frames are
		// usually the reason for it, such as connectionRejected.
		writePolling(w, r, EncodePayload(append(p.take(),
			Encode(Packet{Type: PacketDisconnect}))))
	case <-r.Context().Done():
	}
}

// servePOST reads frames the client sent.
func (p *pollingConn) servePOST(w http.ResponseWriter, r *http.Request, dispatch func(Packet)) {
	// The same ceiling as the websocket transport: an oversized update has to
	// reach the service to be answered with an error the client understands.
	body, err := io.ReadAll(io.LimitReader(r.Body, DefaultReadLimit))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// The client waits for this before sending anything else, so it is
	// answered before the frames are handled.
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("X-XSS-Protection", "0")
	}
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Header().Set("Content-Length", "1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("1"))

	for _, frame := range DecodePayload(string(body)) {
		packet, err := Decode(frame)
		if err != nil {
			continue
		}
		dispatch(packet)
	}
}

func writePolling(w http.ResponseWriter, r *http.Request, body string) {
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Header().Set("Connection", "Keep-Alive")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// servePolling handles both halves of the xhr-polling transport for one
// session, creating the connection on the first request.
func (s *Server) servePolling(w http.ResponseWriter, r *http.Request, sessionID string) {
	s.mu.Lock()
	pc, live := s.polling[sessionID]
	if !live {
		p, handshaken := s.sessions[sessionID]
		if !handshaken {
			s.mu.Unlock()
			http.Error(w, Encode(Packet{
				Type: PacketError,
				Data: itoa(ReasonClientNotHandshaken) + "+" + itoa(AdviceReconnect),
			}), http.StatusBadRequest)
			return
		}
		delete(s.sessions, sessionID)
		s.mu.Unlock()

		c := s.newConn(sessionID, p.request, "xhr-polling", nil)
		pc = newPollingConn(c)
		s.mu.Lock()
		s.polling[sessionID] = pc
		s.mu.Unlock()

		// Frames written by the handler are queued for the next poll rather
		// than pushed down a socket.
		go func() {
			for {
				select {
				case frame := <-c.send:
					pc.queue(frame)
				case <-c.closed:
					// Move anything still in flight into the poll queue, so a
					// client that asks once more still learns why it was cut
					// off.
					for drained := true; drained; {
						select {
						case frame := <-c.send:
							pc.queue(frame)
						default:
							drained = false
						}
					}
					s.mu.Lock()
					delete(s.polling, sessionID)
					s.mu.Unlock()
					// As in the websocket path: the handler needs the rooms
					// intact to unsubscribe their channels.
					s.handler.OnDisconnect(c)
					s.forget(c)
					return
				}
			}
		}()

		pc.queue(Encode(Packet{Type: PacketConnect}))
		if err := s.handler.OnConnect(c); err != nil {
			pc.queue(Encode(Packet{Type: PacketError, Data: itoa(ReasonUnauthorized)}))
			c.Close()
		}
	} else {
		s.mu.Unlock()
	}

	pc.touch()

	switch r.Method {
	case http.MethodPost:
		pc.servePOST(w, r, func(p Packet) {
			switch p.Type {
			case PacketHeartbeat:
				pc.queue(Encode(Packet{Type: PacketHeartbeat}))
			case PacketDisconnect:
				pc.conn.Close()
			case PacketEvent:
				s.handler.OnEvent(pc.conn, Event{Name: p.Name, Args: p.Args, ID: p.ID})
			}
		})
	case http.MethodGet:
		pc.serveGET(w, r)
	case http.MethodOptions:
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "content-type")
		}
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// sweep drops abandoned handshakes and polling sessions.
//
// A websocket announces its own departure, but a polling client that closes
// its browser simply stops asking. Without this its connection would stay in
// its rooms forever, holding the Redis subscriptions open and leaving a ghost
// in every collaborator list.
func (s *Server) sweep(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			s.sweepLocked()
			idle := make([]*pollingConn, 0)
			for _, pc := range s.polling {
				if pc.idleFor(now) > idlePollTimeout {
					idle = append(idle, pc)
				}
			}
			s.mu.Unlock()

			for _, pc := range idle {
				s.log.Info("closing idle polling connection",
					slog.String("client", pc.conn.ID))
				pc.conn.Close()
			}
		}
	}
}

// Close stops the background sweeper. A Server normally lives as long as the
// process, so this exists for tests and for a clean shutdown.
func (s *Server) Close() {
	s.closeOnce.Do(func() { close(s.done) })
}
