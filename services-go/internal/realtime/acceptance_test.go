package realtime

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/redis/go-redis/v9"
)

// These tests drive the whole service the way the editor does: a signed
// session cookie, a socket.io handshake, a websocket, and the rpcs the
// frontend makes. They need a real Redis, because the pub/sub fan-out is the
// half that unit tests cannot reach.
//
// Set REALTIME_TEST_REDIS to the address of a throwaway Redis to run them.
func testRedisAddr(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("REALTIME_TEST_REDIS")
	if addr == "" {
		t.Skip("set REALTIME_TEST_REDIS=host:port to run the acceptance tests")
	}
	return addr
}

const (
	testSecret    = "test-session-secret"
	testProjectID = "507f1f77bcf86cd799439011"
	testDocID     = "507f191e810c19729de860ea"
	testUserID    = "5f2a0a1b1c9d440000a1b2c3"
)

// harness is one running service with the collaborators it talks to.
type harness struct {
	service *Service
	server  *httptest.Server
	redis   *redis.Client
	cancel  context.CancelFunc

	// joinResponse is what the fake web returns; tests change it before
	// connecting.
	privilegeLevel   string
	isRestrictedUser bool
	doc              map[string]any
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	addr := testRedisAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	h := &harness{
		cancel:         cancel,
		privilegeLevel: "readAndWrite",
		doc: map[string]any{
			"lines":   []string{"\\documentclass{article}", "café"},
			"version": 42,
			"ranges":  map[string]any{},
			"ops":     []any{},
		},
	}

	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/join") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"project":          map[string]any{"_id": testProjectID, "name": "Test Project"},
			"privilegeLevel":   h.privilegeLevel,
			"isRestrictedUser": h.isRestrictedUser,
			"isInvitedMember":  !h.isRestrictedUser,
		})
	}))
	t.Cleanup(web.Close)

	du := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(h.doc)
	}))
	t.Cleanup(du.Close)

	// Each run uses its own Redis database so a failed test cannot poison the
	// next one.
	newClient := func() *redis.Client {
		c := redis.NewClient(&redis.Options{Addr: addr, DB: 9})
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	h.redis = newClient()
	if err := h.redis.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("cannot reach the test redis: %v", err)
	}

	h.service = New(ctx, Deps{
		Log: slog.New(slog.DiscardHandler),
		Config: Config{
			CookieName:                  "overleaf.sid",
			SessionSecrets:              []string{testSecret},
			MaxUpdateSize:               7 * 1024 * 1024,
			PendingUpdateListShardCount: 10,
			BehindProxy:                 true,
			TrustedProxyIPs:             "loopback",
		},
		Web:             NewWebClient(web.URL, "overleaf", "password"),
		DocumentUpdater: NewDocumentUpdaterClient(du.URL),
		Sessions:        NewSessionStore(newClient(), "overleaf.sid", []string{testSecret}, ""),
		PubSubRedis:     newClient(),
		RealtimeRedis:   h.redis,
		DocUpdaterRedis: h.redis,
	})
	t.Cleanup(h.service.IO().Close)

	h.server = httptest.NewServer(h.service.Handler("", nil))
	t.Cleanup(h.server.Close)
	return h
}

// signCookie produces the cookie cookie-parser would accept.
func signCookie(sessionID string) string {
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(sessionID))
	sig := strings.TrimRight(base64.StdEncoding.EncodeToString(mac.Sum(nil)), "=")
	return "s:" + sessionID + "." + sig
}

// seedSession writes a session the way connect-redis does.
func (h *harness) seedSession(t *testing.T, sessionID string, session map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("encoding session: %v", err)
	}
	if err := h.redis.Set(context.Background(), "sess:"+sessionID, encoded, time.Hour).Err(); err != nil {
		t.Fatalf("seeding session: %v", err)
	}
}

// client is a socket.io 0.9 client, enough of one to drive the editor's rpcs.
type client struct {
	t     *testing.T
	ws    *websocket.Conn
	acks  int
	inbox []Packet
}

// Packet mirrors what the tests need out of a decoded frame.
type Packet struct {
	Type int
	ID   string
	Name string
	Args []json.RawMessage
	Raw  string
}

func (h *harness) connect(t *testing.T, sessionID, projectID string) *client {
	t.Helper()

	// The query flags and the session cookie go on the HANDSHAKE, not on the
	// websocket upgrade: socket.io 0.9 appends the client options to the
	// handshake URL, and the server reads them off that request for the life
	// of the connection.
	handshakeURL := fmt.Sprintf("%s/socket.io/1/?t=1&projectId=%s", h.server.URL, projectID)
	req, _ := http.NewRequest(http.MethodGet, handshakeURL, nil)
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: "overleaf.sid", Value: signCookie(sessionID)})
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	socketID := strings.Split(string(body), ":")[0]

	url := fmt.Sprintf("ws%s/socket.io/1/websocket/%s",
		strings.TrimPrefix(h.server.URL, "http"), socketID)
	ws, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })

	c := &client{t: t, ws: ws}
	if p := c.read(); p.Raw != "1::" {
		t.Fatalf("first frame = %q, want the connect packet", p.Raw)
	}
	return c
}

func (c *client) read() Packet {
	c.t.Helper()
	p, err := c.tryRead()
	if err != nil {
		c.t.Fatalf("read: %v (frames so far: %v)", err, c.seen())
	}
	return p
}

func (c *client) tryRead() (Packet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, data, err := c.ws.Read(ctx)
	if err != nil {
		return Packet{}, err
	}
	p := decodeTestFrame(c.t, string(data))
	c.inbox = append(c.inbox, p)
	return p, nil
}

// seen lists the frames received so far, which is what makes a failure here
// diagnosable rather than just late.
func (c *client) seen() []string {
	frames := make([]string, 0, len(c.inbox))
	for _, p := range c.inbox {
		frames = append(frames, p.Raw)
	}
	return frames
}

// waitFor reads until an event with the given name arrives.
func (c *client) waitFor(name string) Packet {
	c.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		p, err := c.tryRead()
		if err != nil {
			c.t.Fatalf("waiting for %q: %v (frames received: %v)", name, err, c.seen())
		}
		if p.Name == name {
			return p
		}
	}
	c.t.Fatalf("never received %q (frames received: %v)", name, c.seen())
	return Packet{}
}

// emit sends an event and returns the acknowledgement.
func (c *client) emit(name string, args ...any) Packet {
	c.t.Helper()
	c.acks++
	id := fmt.Sprint(c.acks)
	encoded, err := json.Marshal(map[string]any{"name": name, "args": args})
	if err != nil {
		c.t.Fatalf("encoding %s: %v", name, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	frame := fmt.Sprintf("5:%s+::%s", id, encoded)
	if err := c.ws.Write(ctx, websocket.MessageText, []byte(frame)); err != nil {
		c.t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		p, err := c.tryRead()
		if err != nil {
			c.t.Fatalf("waiting to acknowledge %s: %v (frames received: %v)", name, err, c.seen())
		}
		if p.Type == 6 && p.ID == id {
			return p
		}
	}
	c.t.Fatalf("no acknowledgement for %s (frames received: %v)", name, c.seen())
	return Packet{}
}

func decodeTestFrame(t *testing.T, frame string) Packet {
	t.Helper()
	p := Packet{Raw: frame}
	parts := strings.SplitN(frame, ":", 4)
	if len(parts) < 3 {
		t.Fatalf("malformed frame %q", frame)
	}
	_, _ = fmt.Sscanf(parts[0], "%d", &p.Type)

	data := ""
	if len(parts) == 4 {
		data = parts[3]
	}
	switch p.Type {
	case 5: // event
		var ev struct {
			Name string            `json:"name"`
			Args []json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("malformed event %q: %v", frame, err)
		}
		p.Name, p.Args = ev.Name, ev.Args
	case 6: // ack
		id, rest, found := strings.Cut(data, "+")
		p.ID = id
		if found {
			if err := json.Unmarshal([]byte(rest), &p.Args); err != nil {
				t.Fatalf("malformed ack %q: %v", frame, err)
			}
		}
	}
	return p
}

func TestConnectionIsRejectedWithoutSession(t *testing.T) {
	h := newHarness(t)
	c := h.connect(t, "", testProjectID)
	p := c.waitFor("connectionRejected")

	var reason struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(p.Args[0], &reason); err != nil {
		t.Fatalf("decoding rejection: %v", err)
	}
	// The editor reads this exact message and sends the user to log in again.
	if reason.Message != "invalid session" {
		t.Errorf("rejection = %q, want %q", reason.Message, "invalid session")
	}
}

func TestConnectionIsRejectedWithoutProjectId(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-noproject", map[string]any{
		"user": map[string]any{"_id": testUserID},
	})
	c := h.connect(t, "sid-noproject", "")
	p := c.waitFor("connectionRejected")
	if !strings.Contains(string(p.Args[0]), "projectId") {
		t.Errorf("rejection = %s, want it to name the missing flag", p.Args[0])
	}
}

func TestJoinProjectAndDoc(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-1", map[string]any{
		"passport": map[string]any{
			"user": map[string]any{
				"_id": testUserID, "first_name": "Ada", "last_name": "Lovelace",
				"email": "ada@example.com",
			},
		},
	})
	c := h.connect(t, "sid-1", testProjectID)

	joined := c.waitFor("joinProjectResponse")
	var response struct {
		PublicID         string `json:"publicId"`
		PermissionsLevel string `json:"permissionsLevel"`
		ProtocolVersion  int    `json:"protocolVersion"`
		Project          struct {
			ID string `json:"_id"`
		} `json:"project"`
	}
	if err := json.Unmarshal(joined.Args[0], &response); err != nil {
		t.Fatalf("decoding joinProjectResponse: %v", err)
	}
	if response.PermissionsLevel != "readAndWrite" {
		t.Errorf("permissionsLevel = %q", response.PermissionsLevel)
	}
	if response.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocolVersion = %d, want %d", response.ProtocolVersion, ProtocolVersion)
	}
	if response.Project.ID != testProjectID {
		t.Errorf("project = %+v", response.Project)
	}
	if !strings.HasPrefix(response.PublicID, "P.") {
		t.Errorf("publicId = %q, want the P. form", response.PublicID)
	}

	// The user must now appear in the project's connected clients, which is
	// what the collaborator list is built from.
	count, err := h.redis.SCard(context.Background(), h.service.keys.ClientsInProject(testProjectID)).Result()
	if err != nil || count != 1 {
		t.Errorf("connected clients = %d (%v), want 1", count, err)
	}

	ack := c.emit("joinDoc", testDocID, 42, map[string]any{})
	if len(ack.Args) < 6 {
		t.Fatalf("joinDoc ack has %d args, want 6", len(ack.Args))
	}
	if string(ack.Args[0]) != "null" {
		t.Fatalf("joinDoc failed: %s", ack.Args[0])
	}
	var lines []string
	if err := json.Unmarshal(ack.Args[1], &lines); err != nil {
		t.Fatalf("decoding lines: %v", err)
	}
	if lines[0] != `\documentclass{article}` {
		t.Errorf("first line = %q", lines[0])
	}
	// The accented line comes back in the two-code-unit encoding the editor
	// decodes with decodeURIComponent(escape(...)).
	if lines[1] != "cafÃ©" {
		t.Errorf("second line = %q, want the encoded form", lines[1])
	}
	var version int64
	if err := json.Unmarshal(ack.Args[2], &version); err != nil || version != 42 {
		t.Errorf("version = %d (%v), want 42", version, err)
	}
}

func TestApplyOtUpdateReachesDocumentUpdater(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-2", map[string]any{
		"user": map[string]any{"_id": testUserID},
	})
	c := h.connect(t, "sid-2", testProjectID)
	c.waitFor("joinProjectResponse")
	c.emit("joinDoc", testDocID, 42, map[string]any{})

	ack := c.emit("applyOtUpdate", testDocID, map[string]any{
		"doc": testDocID,
		"op":  []any{map[string]any{"i": "hello", "p": 0}},
		"v":   42,
	})
	if len(ack.Args) > 0 && string(ack.Args[0]) != "null" {
		t.Fatalf("applyOtUpdate failed: %s", ack.Args[0])
	}

	ctx := context.Background()
	queued, err := h.redis.LRange(ctx, h.service.keys.PendingUpdates(testDocID), 0, -1).Result()
	if err != nil || len(queued) != 1 {
		t.Fatalf("pending updates = %v (%v), want one", queued, err)
	}
	var update struct {
		Doc  string `json:"doc"`
		Meta struct {
			Source string `json:"source"`
			UserID string `json:"user_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(queued[0]), &update); err != nil {
		t.Fatalf("decoding queued update: %v", err)
	}
	if update.Doc != testDocID {
		t.Errorf("queued doc = %q", update.Doc)
	}
	if !strings.HasPrefix(update.Meta.Source, "P.") {
		t.Errorf("meta.source = %q, want the client public id", update.Meta.Source)
	}
	if update.Meta.UserID != testUserID {
		t.Errorf("meta.user_id = %q", update.Meta.UserID)
	}

	// The document id must also land on a dispatcher queue, or
	// document-updater never looks at the list above.
	found := false
	for shard := 0; shard < 10 && !found; shard++ {
		key := "pending-updates-list"
		if shard > 0 {
			key = fmt.Sprintf("pending-updates-list-%d", shard)
		}
		items, _ := h.redis.LRange(ctx, key, 0, -1).Result()
		for _, item := range items {
			if item == testProjectID+":"+testDocID {
				found = true
			}
		}
	}
	if !found {
		t.Error("the document was never queued on a dispatcher list")
	}
}

func TestReadOnlyUserCannotEdit(t *testing.T) {
	h := newHarness(t)
	h.privilegeLevel = "readOnly"
	h.seedSession(t, "sid-3", map[string]any{"user": map[string]any{"_id": testUserID}})
	c := h.connect(t, "sid-3", testProjectID)
	c.waitFor("joinProjectResponse")
	c.emit("joinDoc", testDocID, 42, map[string]any{})

	ack := c.emit("applyOtUpdate", testDocID, map[string]any{
		"op": []any{map[string]any{"i": "nope", "p": 0}},
		"v":  42,
	})
	var failure struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(ack.Args[0], &failure); err != nil {
		t.Fatalf("decoding error: %v", err)
	}
	if failure.Message != "not authorized" {
		t.Errorf("error = %q, want %q", failure.Message, "not authorized")
	}

	queued, _ := h.redis.LRange(context.Background(), h.service.keys.PendingUpdates(testDocID), 0, -1).Result()
	if len(queued) != 0 {
		t.Errorf("a rejected update reached document-updater: %v", queued)
	}
}

func TestAppliedOpIsForwardedToTheDocRoom(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-4", map[string]any{"user": map[string]any{"_id": testUserID}})
	c := h.connect(t, "sid-4", testProjectID)
	c.waitFor("joinProjectResponse")
	c.emit("joinDoc", testDocID, 42, map[string]any{})

	// This is what document-updater publishes once it has applied an edit made
	// by somebody else.
	payload, _ := json.Marshal(map[string]any{
		"doc_id": testDocID,
		"op": map[string]any{
			"doc": testDocID, "v": 43,
			"op":   []any{map[string]any{"i": "x", "p": 0}},
			"meta": map[string]any{"source": "P.someone-else"},
		},
	})
	if err := h.redis.Publish(context.Background(), "applied-ops", payload).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	p := c.waitFor("otUpdateApplied")
	var update struct {
		V    int64          `json:"v"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(p.Args[0], &update); err != nil {
		t.Fatalf("decoding update: %v", err)
	}
	if update.V != 43 {
		t.Errorf("version = %d, want 43", update.V)
	}
	// tsRT is an internal timestamp and must never reach a client.
	if _, leaked := update.Meta["tsRT"]; leaked {
		t.Error("the internal tsRT timestamp was sent to the client")
	}
}

func TestEditorEventIsForwardedToTheProjectRoom(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-5", map[string]any{"user": map[string]any{"_id": testUserID}})
	c := h.connect(t, "sid-5", testProjectID)
	c.waitFor("joinProjectResponse")

	payload, _ := json.Marshal(map[string]any{
		"room_id": testProjectID,
		"message": "projectNameUpdated",
		"payload": []any{"Renamed Project"},
	})
	if err := h.redis.Publish(context.Background(), "editor-events", payload).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	p := c.waitFor("projectNameUpdated")
	var name string
	if err := json.Unmarshal(p.Args[0], &name); err != nil || name != "Renamed Project" {
		t.Errorf("payload = %s (%v)", p.Args[0], err)
	}
}

// A restricted user is a link-share visitor. Events outside the pass list
// carry collaborator names and other details they must not see.
func TestRestrictedUserDoesNotReceiveHiddenEvents(t *testing.T) {
	h := newHarness(t)
	h.isRestrictedUser = true
	h.seedSession(t, "sid-6", map[string]any{"user": map[string]any{"_id": testUserID}})
	c := h.connect(t, "sid-6", testProjectID)
	c.waitFor("joinProjectResponse")

	ctx := context.Background()
	hidden, _ := json.Marshal(map[string]any{
		"room_id": testProjectID,
		"message": "new-chat-message",
		"payload": []any{map[string]any{"content": "secret"}},
	})
	allowed, _ := json.Marshal(map[string]any{
		"room_id": testProjectID,
		"message": "projectNameUpdated",
		"payload": []any{"Renamed"},
	})
	if err := h.redis.Publish(ctx, "editor-events", hidden).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := h.redis.Publish(ctx, "editor-events", allowed).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The allowed event arriving proves the hidden one was skipped rather than
	// merely slow.
	p := c.waitFor("projectNameUpdated")
	if p.Name != "projectNameUpdated" {
		t.Errorf("received %q first, want the chat message to have been withheld", p.Name)
	}
}

func TestAccessRevokedDisconnectsTheUser(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-7", map[string]any{"user": map[string]any{"_id": testUserID}})
	c := h.connect(t, "sid-7", testProjectID)
	c.waitFor("joinProjectResponse")

	payload, _ := json.Marshal(map[string]any{
		"room_id": testProjectID,
		"message": "userRemovedFromProject",
		"payload": []any{[]string{testUserID}},
	})
	if err := h.redis.Publish(context.Background(), "editor-events", payload).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	c.waitFor("project:access:revoked")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, _, err := c.ws.Read(ctx); err == nil {
		t.Error("the connection should have been closed after access was revoked")
	}
}

func TestConnectedClientsEndpoint(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-8", map[string]any{
		"user": map[string]any{"_id": testUserID, "email": "ada@example.com"},
	})
	c := h.connect(t, "sid-8", testProjectID)
	c.waitFor("joinProjectResponse")

	res, err := http.Get(h.server.URL + "/clients")
	if err != nil {
		t.Fatalf("get /clients: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	var clients []connectedClientView
	if err := json.NewDecoder(res.Body).Decode(&clients); err != nil {
		t.Fatalf("decoding /clients: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("/clients returned %d clients, want 1", len(clients))
	}
	if clients[0].ProjectID != testProjectID || clients[0].Email != "ada@example.com" {
		t.Errorf("client view = %+v", clients[0])
	}
	if len(clients[0].Rooms) == 0 {
		t.Error("the client should be in the project room")
	}
}

func TestCountConnectedClientsEndpoint(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-9", map[string]any{"user": map[string]any{"_id": testUserID}})
	c := h.connect(t, "sid-9", testProjectID)
	c.waitFor("joinProjectResponse")

	res, err := http.Get(h.server.URL + "/project/" + testProjectID + "/count-connected-clients")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	var body struct {
		N int `json:"nConnectedClients"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.N != 1 {
		t.Errorf("nConnectedClients = %d, want 1", body.N)
	}
}

// web pushes events into a project over HTTP; they must reach the room through
// Redis just as an event published by another instance would.
func TestSendMessageEndpoint(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-10", map[string]any{"user": map[string]any{"_id": testUserID}})
	c := h.connect(t, "sid-10", testProjectID)
	c.waitFor("joinProjectResponse")

	res, err := http.Post(
		h.server.URL+"/project/"+testProjectID+"/message/projectNameUpdated",
		"application/json", strings.NewReader(`"From web"`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}

	p := c.waitFor("projectNameUpdated")
	var name string
	if err := json.Unmarshal(p.Args[0], &name); err != nil || name != "From web" {
		t.Errorf("payload = %s (%v)", p.Args[0], err)
	}
}

func TestLeaveDocAndDisconnectCleanUp(t *testing.T) {
	h := newHarness(t)
	h.seedSession(t, "sid-11", map[string]any{"user": map[string]any{"_id": testUserID}})
	c := h.connect(t, "sid-11", testProjectID)
	c.waitFor("joinProjectResponse")
	c.emit("joinDoc", testDocID, 42, map[string]any{})

	if n := h.service.IO().CountClientsIn(testDocID); n != 1 {
		t.Fatalf("doc room holds %d clients, want 1", n)
	}
	c.emit("leaveDoc", testDocID)
	if n := h.service.IO().CountClientsIn(testDocID); n != 0 {
		t.Errorf("doc room still holds %d clients after leaveDoc", n)
	}

	_ = c.ws.Close(websocket.StatusNormalClosure, "bye")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if h.service.IO().CountClientsIn(testProjectID) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if n := h.service.IO().CountClientsIn(testProjectID); n != 0 {
		t.Fatalf("project room still holds %d clients after a disconnect", n)
	}

	// The user must also be gone from Redis, or they stay in the collaborator
	// list of everyone still editing.
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		n, _ := h.redis.SCard(context.Background(), h.service.keys.ClientsInProject(testProjectID)).Result()
		if n == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("the client was never removed from the project in redis")
}
