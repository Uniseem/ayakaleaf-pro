package socketio

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

// pollTimeout is how long a GET is held open before it is answered with a
// noop. The client reissues immediately, which keeps proxies from closing an
// idle connection themselves.
const pollTimeout = 20 * time.Second

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
	// waiting is signalled when a frame arrives while a GET is parked.
	waiting chan struct{}
}

func newPollingConn(c *Conn) *pollingConn {
	return &pollingConn{conn: c, waiting: make(chan struct{}, 1)}
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
		writePolling(w, r, Encode(Packet{Type: PacketDisconnect}))
	case <-r.Context().Done():
	}
}

// servePOST reads frames the client sent.
func (p *pollingConn) servePOST(w http.ResponseWriter, r *http.Request, dispatch func(Packet)) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
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

		c := &Conn{
			ID:      sessionID,
			Request: p.request,
			send:    make(chan string, 64),
			closed:  make(chan struct{}),
			log:     s.log,
		}
		pc = newPollingConn(c)
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
					s.mu.Lock()
					delete(s.polling, sessionID)
					s.mu.Unlock()
					s.handler.OnDisconnect(c)
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

	switch r.Method {
	case http.MethodPost:
		pc.servePOST(w, r, func(p Packet) {
			switch p.Type {
			case PacketHeartbeat:
				pc.queue(Encode(Packet{Type: PacketHeartbeat}))
			case PacketDisconnect:
				pc.conn.Close()
			case PacketEvent:
				s.handler.OnEvent(pc.conn, p.Name, p.Args)
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

// closeIdlePolling drops polling sessions whose client has stopped asking.
func (s *Server) closeIdlePolling(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			s.sweepLocked()
			s.mu.Unlock()
			s.log.Debug("swept expired socket.io handshakes")
		}
	}
}
