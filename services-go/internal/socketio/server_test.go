package socketio

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// testHandler records the lifecycle and answers the events the tests send.
type testHandler struct {
	connected  chan *Conn
	disconnect chan string
}

func (h *testHandler) OnConnect(c *Conn) error {
	h.connected <- c
	return c.Emit("connectionAccepted", nil, c.ID)
}

func (h *testHandler) OnEvent(c *Conn, ev Event) {
	switch ev.Name {
	case "echo":
		var text string
		if len(ev.Args) > 0 {
			_ = json.Unmarshal(ev.Args[0], &text)
		}
		_ = c.Ack(ev.ID, nil, text)
	case "join":
		var room string
		_ = json.Unmarshal(ev.Args[0], &room)
		before := c.Join(room)
		_ = c.Ack(ev.ID, nil, before)
	case "leave":
		var room string
		_ = json.Unmarshal(ev.Args[0], &room)
		remaining, wasIn := c.Leave(room)
		_ = c.Ack(ev.ID, nil, remaining, wasIn)
	case "bye":
		c.Close()
	}
}

func (h *testHandler) OnDisconnect(c *Conn) { h.disconnect <- c.ID }

// dial performs a handshake and opens the websocket transport, the way the
// editor's client does.
func dial(t *testing.T, base string) (*websocket.Conn, string) {
	t.Helper()
	res, err := http.Get(base + "/socket.io/1/?t=" + time.Now().Format("150405"))
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)

	parts := strings.Split(string(body), ":")
	if len(parts) != 4 {
		t.Fatalf("handshake body %q is not sid:heartbeat:close:transports", body)
	}
	sessionID := parts[0]

	url := "ws" + strings.TrimPrefix(base, "http") + "/socket.io/1/websocket/" + sessionID
	ws, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	return ws, sessionID
}

func readFrame(t *testing.T, ws *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}

func writeFrame(t *testing.T, ws *websocket.Conn, frame string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ws.Write(ctx, websocket.MessageText, []byte(frame)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func newTestServer(t *testing.T) (*Server, *testHandler, string) {
	t.Helper()
	h := &testHandler{connected: make(chan *Conn, 4), disconnect: make(chan string, 4)}
	srv := NewServer(h, slog.New(slog.DiscardHandler))
	t.Cleanup(srv.Close)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, h, ts.URL
}

func TestHandshakeAndConnect(t *testing.T) {
	_, _, base := newTestServer(t)
	ws, sessionID := dial(t, base)

	if frame := readFrame(t, ws); frame != "1::" {
		t.Errorf("first frame = %q, want the connect packet 1::", frame)
	}
	// The handler's own emit follows, carrying the session id it was given.
	frame := readFrame(t, ws)
	if !strings.Contains(frame, "connectionAccepted") || !strings.Contains(frame, sessionID) {
		t.Errorf("second frame = %q, want connectionAccepted with the session id", frame)
	}
}

func TestHeartbeatIsEchoed(t *testing.T) {
	_, _, base := newTestServer(t)
	ws, _ := dial(t, base)
	readFrame(t, ws) // connect
	readFrame(t, ws) // connectionAccepted

	writeFrame(t, ws, "2::")
	if frame := readFrame(t, ws); frame != "2::" {
		// A missed heartbeat makes the client tear the connection down, so
		// this is what keeps an idle editor connected.
		t.Errorf("heartbeat answered with %q, want 2::", frame)
	}
}

// Every rpc the editor makes carries an acknowledgement id and waits for the
// answer, so the exact ack framing is load-bearing.
func TestEventAcknowledgement(t *testing.T) {
	_, _, base := newTestServer(t)
	ws, _ := dial(t, base)
	readFrame(t, ws)
	readFrame(t, ws)

	writeFrame(t, ws, `5:7+::{"name":"echo","args":["hello"]}`)
	if frame := readFrame(t, ws); frame != `6:::7+[null,"hello"]` {
		t.Errorf("ack = %q, want 6:::7+[null,\"hello\"]", frame)
	}
}

func TestServerInitiatedDisconnect(t *testing.T) {
	_, _, base := newTestServer(t)
	ws, _ := dial(t, base)
	readFrame(t, ws)
	readFrame(t, ws)

	writeFrame(t, ws, `5:3+::{"name":"bye","args":[]}`)
	// bye closes the connection rather than acking. The client is told so with
	// a disconnect packet, which is what stops socket.io reconnecting.
	if frame := readFrame(t, ws); frame != "0::" {
		t.Errorf("last frame = %q, want the disconnect packet 0::", frame)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := ws.Read(ctx); err == nil {
		t.Error("the connection should have been closed by the handler")
	}
}

func TestRoomsTrackMembership(t *testing.T) {
	srv, _, base := newTestServer(t)

	first, _ := dial(t, base)
	readFrame(t, first)
	readFrame(t, first)
	writeFrame(t, first, `5:1+::{"name":"join","args":["room-a"]}`)
	if frame := readFrame(t, first); frame != `6:::1+[null,0]` {
		t.Errorf("first join reported %q, want a before-count of 0", frame)
	}

	second, _ := dial(t, base)
	readFrame(t, second)
	readFrame(t, second)
	writeFrame(t, second, `5:1+::{"name":"join","args":["room-a"]}`)
	if frame := readFrame(t, second); frame != `6:::1+[null,1]` {
		t.Errorf("second join reported %q, want a before-count of 1", frame)
	}

	if n := srv.CountClientsIn("room-a"); n != 2 {
		t.Errorf("room holds %d clients, want 2", n)
	}

	// Leaving a room the client is not in must be a no-op: the editor sends
	// spurious leaveDoc calls after a reconnect.
	writeFrame(t, second, `5:2+::{"name":"leave","args":["room-b"]}`)
	if frame := readFrame(t, second); frame != `6:::2+[null,0,false]` {
		t.Errorf("leaving an unjoined room reported %q, want wasIn=false", frame)
	}

	writeFrame(t, second, `5:3+::{"name":"leave","args":["room-a"]}`)
	if frame := readFrame(t, second); frame != `6:::3+[null,1,true]` {
		t.Errorf("leave reported %q, want one client remaining", frame)
	}
	if n := srv.CountClientsIn("room-a"); n != 1 {
		t.Errorf("room holds %d clients, want 1", n)
	}
}

// A disconnect has to take the client out of every room, or a departed client
// would keep a room alive and its Redis channel subscribed forever.
func TestDisconnectClearsRooms(t *testing.T) {
	srv, h, base := newTestServer(t)
	ws, _ := dial(t, base)
	readFrame(t, ws)
	readFrame(t, ws)
	writeFrame(t, ws, `5:1+::{"name":"join","args":["room-c"]}`)
	readFrame(t, ws)

	if n := srv.CountClientsIn("room-c"); n != 1 {
		t.Fatalf("room holds %d clients, want 1", n)
	}
	_ = ws.Close(websocket.StatusNormalClosure, "done")

	select {
	case <-h.disconnect:
	case <-time.After(5 * time.Second):
		t.Fatal("the disconnect was never reported")
	}
	if n := srv.CountClientsIn("room-c"); n != 0 {
		t.Errorf("room still holds %d clients after a disconnect", n)
	}
	if n := srv.CountClients(); n != 0 {
		t.Errorf("%d clients still registered after a disconnect", n)
	}
}

func TestBroadcastReachesEveryClient(t *testing.T) {
	srv, h, base := newTestServer(t)

	first, _ := dial(t, base)
	readFrame(t, first)
	readFrame(t, first)
	second, _ := dial(t, base)
	readFrame(t, second)
	readFrame(t, second)

	// Wait for both connections to be registered before broadcasting.
	<-h.connected
	<-h.connected

	srv.Broadcast("projectNameUpdated", "renamed")
	for _, ws := range []*websocket.Conn{first, second} {
		frame := readFrame(t, ws)
		if !strings.Contains(frame, "projectNameUpdated") {
			t.Errorf("client received %q, want the broadcast", frame)
		}
	}
}

func TestUnknownSessionIsRejected(t *testing.T) {
	_, _, base := newTestServer(t)
	url := "ws" + strings.TrimPrefix(base, "http") + "/socket.io/1/websocket/not-a-session"
	if _, _, err := websocket.Dial(context.Background(), url, nil); err == nil {
		t.Error("a websocket for an unknown session should be refused")
	}
}

func TestUnsupportedTransport(t *testing.T) {
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/socket.io/1/jsonp-polling/abc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", res.StatusCode)
	}
}
