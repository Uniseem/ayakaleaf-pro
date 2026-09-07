package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/oid"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/socketio"
)

const (
	// serverPingInterval is how often the server probes a websocket client, so
	// a connection that has silently died is noticed from this side too.
	serverPingInterval = 15 * time.Second
	// eventQueueDepth bounds the per-connection backlog. Beyond it the read
	// loop blocks, which is the honest signal that this instance cannot keep
	// up -- dropping events would corrupt a document instead.
	eventQueueDepth = 256
)

var hostname, _ = os.Hostname()

// queuedEvent is one client event waiting for the connection's worker.
type queuedEvent struct {
	ev socketio.Event
	cc *clientContext
}

// OnConnect authenticates a new connection and joins it to its project.
func (s *Service) OnConnect(c *socketio.Conn) error {
	ctx := context.Background()
	cc := &clientContext{PublicID: newPublicID(), docs: map[string]bool{}}
	s.setContext(c, cc)

	if s.shutDownInProgress.Load() {
		// This instance is draining. Telling the client to retry sends it to
		// another one instead of leaving it on a socket about to close.
		_ = c.Emit("connectionRejected", map[string]any{"message": "retry"})
		c.Close()
		return nil
	}

	query := c.Request.URL.Query()
	projectID := query.Get("projectId")
	isDebugging := query.Get("debugging") != ""

	session, _, err := s.store.Get(ctx, c.Request)
	if err != nil {
		if errors.Is(err, ErrMissingSession) {
			s.log.Warn("invalid session", slog.String("client", c.ID))
			_ = c.Emit("connectionRejected", map[string]any{"message": "invalid session"})
		} else {
			s.log.Error("error when client connected", slog.String("client", c.ID),
				slog.String("err", err.Error()))
			_ = c.Emit("connectionRejected", map[string]any{"message": "error"})
		}
		c.Close()
		return nil
	}

	if !isDebugging && !oid.IsValid(projectID) {
		_ = c.Emit("connectionRejected", map[string]any{
			"message": "missing/bad ?projectId=... query flag on handshake",
		})
		c.Close()
		return nil
	}

	user := session.Identity()
	if user == nil {
		// Not signed in: the only way in is a link-sharing token this session
		// picked up when it opened the project over HTTP.
		user = &User{
			ID:                   "anonymous-user",
			AnonymousAccessToken: session.AnonTokenAccess[projectID],
		}
	}

	cc.mu.Lock()
	cc.user = user
	cc.remoteIP = s.addr.RemoteIP(c.Request)
	cc.userAgent = c.Request.Header.Get("User-Agent")
	cc.isDebugging = isDebugging
	if isDebugging {
		cc.connectedAt = time.Now()
	}
	cc.mu.Unlock()

	info := []any{
		slog.String("user", user.ID), slog.String("project", projectID),
		slog.String("transport", c.Transport), slog.String("publicId", cc.PublicID),
		slog.String("client", c.ID), slog.Bool("isDebugging", isDebugging),
	}
	if isDebugging {
		s.log.Info("client connected", info...)
	} else {
		s.log.Debug("client connected", info...)
	}

	// Events are handled by one worker per connection, so they take effect in
	// the order the client sent them -- an out-of-order applyOtUpdate would
	// corrupt the document -- while the read loop stays free to answer
	// heartbeats.
	queue := make(chan queuedEvent, eventQueueDepth)
	s.setQueue(c, queue)
	go s.serveEvents(c, queue)

	// The client asks for server pings by handshake flag, and they are only
	// meaningful on a real socket: a polling transport has no connection to
	// keep alive.
	if query.Get("esh") != "" && query.Get("ssp") != "" && c.Transport == "websocket" {
		go s.serverPingLoop(c, cc)
	}

	if !isDebugging {
		// Joining is asynchronous, as it is in the Node service: it makes an
		// HTTP call to web, and the connection must keep reading meanwhile.
		go func() {
			outcome, err := s.JoinProject(ctx, c, cc, projectID)
			if err != nil {
				s.logHandlerError(err, c, cc, "joinProject")
				_ = c.Emit("connectionRejected", serializeError(err))
				c.Close()
				return
			}
			if outcome == nil {
				return // the client went away while joining
			}
			_ = c.Emit("joinProjectResponse", outcome)
		}()
	}
	return nil
}

// OnEvent hands an event to the connection's worker.
func (s *Service) OnEvent(c *socketio.Conn, ev socketio.Event) {
	cc := s.context(c)
	if cc == nil {
		return
	}
	queue := s.queue(c)
	if queue == nil {
		return
	}
	select {
	case queue <- queuedEvent{ev: ev, cc: cc}:
	case <-c.Done():
	}
}

// OnDisconnect tears the connection down.
func (s *Service) OnDisconnect(c *socketio.Conn) {
	cc := s.context(c)
	s.dropQueue(c)
	if cc == nil {
		return
	}
	defer s.dropContext(c)

	if cc.isDebugging {
		s.log.Info("debug client disconnected", slog.String("client", c.ID),
			slog.Duration("duration", time.Since(cc.connectedAt)))
	}
	s.LeaveProject(context.Background(), c, cc)
}

// serveEvents runs one connection's events in order until it disconnects.
func (s *Service) serveEvents(c *socketio.Conn, queue chan queuedEvent) {
	for {
		select {
		case <-c.Done():
			return
		case item := <-queue:
			s.handleEvent(c, item.cc, item.ev)
		}
	}
}

func (s *Service) handleEvent(c *socketio.Conn, cc *clientContext, ev socketio.Event) {
	ctx := context.Background()
	switch ev.Name {
	case "joinDoc":
		s.handleJoinDoc(ctx, c, cc, ev)
	case "leaveDoc":
		s.handleLeaveDoc(ctx, c, cc, ev)
	case "applyOtUpdate":
		s.handleApplyOtUpdate(ctx, c, cc, ev)
	case "clientTracking.updatePosition":
		s.handleUpdatePosition(ctx, c, cc, ev)
	case "clientTracking.getConnectedUsers":
		// The 1s wait for other instances to answer the refresh must not hold
		// up this connection's other events.
		go s.handleGetConnectedUsers(ctx, c, cc, ev)
	case "clientPong":
		s.handleClientPong(cc, ev)
	case "debug":
		s.handleDebug(c, cc, ev)
	case "debug.getHostname":
		if s.cfg.ExposeHostname {
			_ = c.Ack(ev.ID, hostname)
		}
	}
}

func (s *Service) handleJoinDoc(ctx context.Context, c *socketio.Conn, cc *clientContext, ev socketio.Event) {
	if ev.ID == "" {
		s.invalidArguments(c, cc, "joinDoc", ev.ID)
		return
	}
	docID, fromVersion, options, ok := parseJoinDocArgs(ev.Args)
	if !ok {
		s.invalidArguments(c, cc, "joinDoc", ev.ID)
		return
	}
	if !oid.IsValid(docID) {
		s.validationError(c, cc, "joinDoc", ev.ID, invalidObjectID)
		return
	}

	result, err := s.JoinDoc(ctx, c, cc, docID, fromVersion, options)
	if err != nil {
		s.logHandlerError(err, c, cc, "joinDoc")
		_ = c.Ack(ev.ID, serializeError(err))
		return
	}
	if result == nil {
		return // the client disconnected; nobody will read the answer
	}
	_ = c.Ack(ev.ID, nil, result.Lines, result.Version, result.Ops, result.Ranges, result.Type)
}

// parseJoinDocArgs reads the variadic joinDoc signature:
//
//	docId
//	docId, fromVersion
//	docId, options
//	docId, fromVersion, options
func parseJoinDocArgs(args []json.RawMessage) (docID string, fromVersion int64, options JoinDocOptions, ok bool) {
	fromVersion = -1
	if len(args) == 0 || len(args) > 3 {
		return "", 0, options, false
	}
	if json.Unmarshal(args[0], &docID) != nil {
		return "", 0, options, false
	}
	switch len(args) {
	case 1:
		return docID, fromVersion, options, true
	case 2:
		if json.Unmarshal(args[1], &fromVersion) == nil {
			return docID, fromVersion, options, true
		}
		fromVersion = -1
		if json.Unmarshal(args[1], &options) == nil {
			return docID, fromVersion, options, true
		}
		return "", 0, options, false
	default:
		if json.Unmarshal(args[1], &fromVersion) != nil {
			return "", 0, options, false
		}
		if json.Unmarshal(args[2], &options) != nil {
			return "", 0, options, false
		}
		return docID, fromVersion, options, true
	}
}

func (s *Service) handleLeaveDoc(ctx context.Context, c *socketio.Conn, cc *clientContext, ev socketio.Event) {
	if ev.ID == "" {
		s.invalidArguments(c, cc, "leaveDoc", ev.ID)
		return
	}
	// The client's callback occupies the second argument, so anything else
	// there means the call itself was malformed rather than its contents.
	if len(ev.Args) != 1 {
		s.invalidArguments(c, cc, "leaveDoc", ev.ID)
		return
	}
	var docID string
	if json.Unmarshal(ev.Args[0], &docID) != nil || !oid.IsValid(docID) {
		s.validationError(c, cc, "leaveDoc", ev.ID, invalidObjectID)
		return
	}
	s.LeaveDoc(ctx, c, cc, docID)
	_ = c.Ack(ev.ID, nil)
}

func (s *Service) handleApplyOtUpdate(ctx context.Context, c *socketio.Conn, cc *clientContext, ev socketio.Event) {
	if ev.ID == "" {
		s.invalidArguments(c, cc, "applyOtUpdate", ev.ID)
		return
	}
	if len(ev.Args) != 2 {
		s.invalidArguments(c, cc, "applyOtUpdate", ev.ID)
		return
	}
	var docID string
	if json.Unmarshal(ev.Args[0], &docID) != nil || !oid.IsValid(docID) {
		s.validationError(c, cc, "applyOtUpdate", ev.ID, invalidObjectID)
		return
	}
	// An update that is not an object cannot be an operation, and a client
	// sending one will keep sending them until it is disconnected.
	var update map[string]json.RawMessage
	if json.Unmarshal(ev.Args[1], &update) != nil {
		s.validationError(c, cc, "applyOtUpdate", ev.ID,
			"invalid input: expected object")
		return
	}

	if err := s.ApplyOtUpdate(ctx, c, cc, docID, update); err != nil {
		s.logHandlerError(err, c, cc, "applyOtUpdate")
		_ = c.Ack(ev.ID, serializeError(err))
		return
	}
	_ = c.Ack(ev.ID)
}

func (s *Service) handleUpdatePosition(ctx context.Context, c *socketio.Conn, cc *clientContext, ev socketio.Event) {
	var cursor map[string]any
	if len(ev.Args) == 0 || json.Unmarshal(ev.Args[0], &cursor) != nil {
		s.invalidArguments(c, cc, "clientTracking.updatePosition", ev.ID)
		return
	}
	if err := s.UpdateClientPosition(ctx, c, cc, cursor); err != nil {
		s.logHandlerError(err, c, cc, "clientTracking.updatePosition")
		// The editor passes no callback here, so an error can only be logged.
		_ = c.Ack(ev.ID, serializeError(err))
		return
	}
	_ = c.Ack(ev.ID)
}

func (s *Service) handleGetConnectedUsers(ctx context.Context, c *socketio.Conn, cc *clientContext, ev socketio.Event) {
	if ev.ID == "" {
		s.invalidArguments(c, cc, "clientTracking.getConnectedUsers", ev.ID)
		return
	}
	users, err := s.GetConnectedUsers(ctx, c, cc)
	if err != nil {
		s.logHandlerError(err, c, cc, "clientTracking.getConnectedUsers")
		_ = c.Ack(ev.ID, serializeError(err))
		return
	}
	_ = c.Ack(ev.ID, nil, users)
}

// handleClientPong records the answer to a server ping and logs the anomalies
// the Node service watches for.
func (s *Service) handleClientPong(cc *clientContext, ev socketio.Event) {
	if len(ev.Args) < 2 {
		return
	}
	var receivedPingID int64
	var sentTimestamp int64
	if json.Unmarshal(ev.Args[0], &receivedPingID) != nil ||
		json.Unmarshal(ev.Args[1], &sentTimestamp) != nil {
		return
	}
	cc.mu.Lock()
	cc.pongID = receivedPingID
	expected := cc.pingID
	cc.mu.Unlock()

	latency := time.Now().UnixMilli() - sentTimestamp
	switch {
	case receivedPingID != expected:
		s.log.Warn("received pong with wrong counter",
			slog.Int64("receivedPingId", receivedPingID), slog.Int64("pingId", expected))
	case latency > int64(serverPingLatencyThreshold/time.Millisecond):
		s.log.Warn("received pong with high latency", slog.Int64("latency", latency))
	}
}

// serverPingLatencyThreshold is the round trip beyond which a connection is
// worth reporting as degraded.
const serverPingLatencyThreshold = 5 * time.Second

func (s *Service) serverPingLoop(c *socketio.Conn, cc *clientContext) {
	ticker := time.NewTicker(serverPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.Done():
			return
		case <-ticker.C:
			cc.mu.Lock()
			if cc.pongID != cc.pingID {
				s.log.Warn("no client response to last ping", slog.String("client", c.ID),
					slog.Int64("pingId", cc.pingID), slog.Int64("pongId", cc.pongID))
			}
			cc.pingID++
			pingID := cc.pingID
			cc.mu.Unlock()
			if err := c.Emit("serverPing", pingID, time.Now().UnixMilli(), c.Transport, c.ID); err != nil {
				return
			}
		}
	}
}

func (s *Service) handleDebug(c *socketio.Conn, cc *clientContext, ev socketio.Event) {
	if ev.ID == "" {
		s.invalidArguments(c, cc, "debug", ev.ID)
		return
	}
	s.log.Info("received debug message", slog.String("publicId", cc.PublicID),
		slog.String("client", c.ID))

	var data json.RawMessage
	if len(ev.Args) > 0 {
		data = ev.Args[0]
	}
	cc.mu.RLock()
	clientInfo := map[string]any{
		"publicId":  cc.PublicID,
		"remoteIp":  cc.remoteIP,
		"userAgent": cc.userAgent,
		"connected": !c.Disconnected(),
	}
	if !cc.connectedAt.IsZero() {
		clientInfo["connectedAt"] = cc.connectedAt.UnixMilli()
	}
	cc.mu.RUnlock()

	server := map[string]any{}
	if s.cfg.ExposeHostname {
		server["hostname"] = hostname
	}
	_ = c.Ack(ev.ID, map[string]any{
		"serverTime": time.Now().UnixMilli(),
		"data":       data,
		"client":     clientInfo,
		"server":     server,
	})
}

// invalidArguments mirrors Router._handleInvalidArguments: the payload may be
// large, so it is logged at debug, and the caller is told only that its call
// was malformed.
//
// The answer matters as much as the log. A client that made a bad call is
// waiting on its callback, and saying nothing leaves it waiting forever
// instead of failing.
func (s *Service) invalidArguments(c *socketio.Conn, cc *clientContext, method, ackID string) {
	s.log.Debug("unexpected arguments", slog.String("method", method),
		slog.String("client", c.ID), slog.String("project", cc.ProjectID()))
	_ = c.Ack(ackID, serializedError{Message: ErrUnexpectedArguments.Error()})
}

// invalidObjectID is the message the schema produces for an id that is not an
// ObjectId; the client shows it as-is.
const invalidObjectID = "invalid Mongo ObjectId"

// validationError answers a call whose arguments did not validate, and
// disconnects the client shortly afterwards -- the delay gives it time to
// receive the answer first.
//
// The disconnect is deliberate: a client sending malformed ids or malformed
// updates is out of step with the server, and a fresh connection is the
// cheapest repair.
func (s *Service) validationError(c *socketio.Conn, cc *clientContext, method, ackID, message string) {
	s.log.Info("validation error", slog.String("method", method),
		slog.String("client", c.ID), slog.String("project", cc.ProjectID()),
		slog.String("err", message))
	_ = c.Ack(ackID, serializedError{Message: message})
	time.AfterFunc(100*time.Millisecond, c.Close)
}

// logHandlerError logs at the level the error deserves. The expected ones --
// a revoked permission, a client that raced itself -- are warnings; anything
// else is a fault in this service.
func (s *Service) logHandlerError(err error, c *socketio.Conn, cc *clientContext, method string) {
	attrs := []any{
		slog.String("method", method), slog.String("client", c.ID),
		slog.String("project", cc.ProjectID()), slog.String("user", cc.UserID()),
		slog.String("err", err.Error()),
	}
	var coded *codedError
	switch {
	case errors.As(err, &coded), passThroughMessages[err.Error()]:
		s.log.Warn(err.Error(), attrs...)
	default:
		s.log.Error("server side error in "+method, attrs...)
	}
}
