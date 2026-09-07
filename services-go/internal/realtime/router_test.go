package realtime

import (
	"encoding/json"
	"net/http"
	"testing"
)

// raw builds the argument list a socket.io event carries.
func raw(values ...string) []json.RawMessage {
	args := make([]json.RawMessage, 0, len(values))
	for _, v := range values {
		args = append(args, json.RawMessage(v))
	}
	return args
}

// payload encodes an argument list the way it appears on the wire.
func payload(values ...string) json.RawMessage {
	encoded, err := json.Marshal(raw(values...))
	if err != nil {
		panic(err)
	}
	return encoded
}

// joinDoc has four call shapes because the editor has grown three times
// without changing the event name. Getting a shape wrong means a client on one
// version of the frontend silently cannot open documents.
func TestParseJoinDocArgs(t *testing.T) {
	const docID = `"507f1f77bcf86cd799439011"`

	cases := []struct {
		name        string
		args        []json.RawMessage
		wantVersion int64
		wantOptions JoinDocOptions
		wantOK      bool
	}{
		{"doc only", raw(docID), -1, JoinDocOptions{}, true},
		{"doc and version", raw(docID, `42`), 42, JoinDocOptions{}, true},
		{
			"doc and options", raw(docID, `{"encodeRanges":true}`),
			-1, JoinDocOptions{EncodeRanges: true}, true,
		},
		{
			"doc, version and options",
			raw(docID, `7`, `{"supportsHistoryOT":true}`),
			7, JoinDocOptions{SupportsHistoryOT: true}, true,
		},
		{"no arguments", nil, 0, JoinDocOptions{}, false},
		{"too many arguments", raw(docID, `1`, `{}`, `{}`), 0, JoinDocOptions{}, false},
		{"doc id is not a string", raw(`42`), 0, JoinDocOptions{}, false},
		{"second argument is neither", raw(docID, `"nonsense"`), 0, JoinDocOptions{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotDoc, gotVersion, gotOptions, ok := parseJoinDocArgs(tc.args)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if gotDoc != "507f1f77bcf86cd799439011" {
				t.Errorf("docID = %q", gotDoc)
			}
			if gotVersion != tc.wantVersion {
				t.Errorf("fromVersion = %d, want %d", gotVersion, tc.wantVersion)
			}
			if gotOptions.EncodeRanges != tc.wantOptions.EncodeRanges ||
				gotOptions.SupportsHistoryOT != tc.wantOptions.SupportsHistoryOT {
				t.Errorf("options = %+v, want %+v", gotOptions, tc.wantOptions)
			}
		})
	}
}

func TestSerializeError(t *testing.T) {
	if got := serializeError(ErrNotAuthorized); got.Message != "not authorized" {
		t.Errorf("an expected error should reach the client verbatim, got %q", got.Message)
	}
	if got := serializeError(ErrProjectNotFound); got.Code != "ProjectNotFound" {
		t.Errorf("a coded error should carry its code, got %+v", got)
	}
	// An unexpected error must not reach the client: its text could name
	// internal hosts, queries or file paths.
	got := serializeError(errServerSide{})
	if got.Message != "Something went wrong in real-time service" {
		t.Errorf("an internal error leaked to the client: %q", got.Message)
	}
}

type errServerSide struct{}

func (errServerSide) Error() string {
	return "dial tcp 10.0.0.7:27017: connection refused"
}

func TestShouldDisconnectClient(t *testing.T) {
	s := &Service{}
	user := func(id string, invited bool) *clientContext {
		return &clientContext{user: &User{ID: id}, isInvitedMember: invited}
	}

	cases := []struct {
		name string
		cc   *clientContext
		ev   editorEvent
		want bool
	}{
		{
			// web emits one argument per removed user, so the ids are the
			// payload entries themselves.
			"the removed user is disconnected",
			user("u1", true),
			editorEvent{Message: "userRemovedFromProject", Payload: payload(`"u1"`, `"u2"`)},
			true,
		},
		{
			"other users stay",
			user("u3", true),
			editorEvent{Message: "userRemovedFromProject", Payload: payload(`"u1"`, `"u2"`)},
			false,
		},
		{
			// A list packed into one argument is a different message shape and
			// is not matched, exactly as the Node service does not match it.
			"a list inside a single argument is not a match",
			user("u1", true),
			editorEvent{Message: "userRemovedFromProject", Payload: payload(`["u1","u2"]`)},
			false,
		},
		{
			"an anonymous client is never the removed user",
			&clientContext{user: &User{}},
			editorEvent{Message: "userRemovedFromProject", Payload: payload(`""`)},
			false,
		},
		{
			"a project turned private drops a link-share visitor",
			user("u1", false),
			editorEvent{
				Message: "project:publicAccessLevel:changed",
				Payload: payload(`{"newAccessLevel":"private"}`),
			},
			true,
		},
		{
			"an invited member keeps their place when a project turns private",
			user("u1", true),
			editorEvent{
				Message: "project:publicAccessLevel:changed",
				Payload: payload(`{"newAccessLevel":"private"}`),
			},
			false,
		},
		{
			"a project opened up drops nobody",
			user("u1", false),
			editorEvent{
				Message: "project:publicAccessLevel:changed",
				Payload: payload(`{"newAccessLevel":"tokenBased"}`),
			},
			false,
		},
		{
			"a changed access level reconnects that user only",
			user("u1", true),
			editorEvent{
				Message: "project:collaboratorAccessLevel:changed",
				Payload: payload(`{"userId":"u1"}`),
			},
			true,
		},
		{
			"an unrelated event disconnects nobody",
			user("u1", true),
			editorEvent{Message: "projectNameUpdated", Payload: payload(`"new name"`)},
			false,
		},
		{
			"a payload that is not an id disconnects nobody",
			user("u1", true),
			editorEvent{Message: "userRemovedFromProject", Payload: payload(`{"id":"u1"}`)},
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.shouldDisconnectClient(tc.cc, tc.ev); got != tc.want {
				t.Errorf("shouldDisconnectClient = %v, want %v", got, tc.want)
			}
		})
	}
}

// The permission an update needs is inferred from what the update does, so a
// read-only user commenting must be allowed and a read-only user editing must
// not.
func TestUpdateClassification(t *testing.T) {
	parse := func(s string) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("bad test fixture: %v", err)
		}
		return m
	}

	if !isCommentUpdate(parse(`{"op":[{"c":"hi","p":1}]}`)) {
		t.Error("an op made only of comments is a comment update")
	}
	if isCommentUpdate(parse(`{"op":[{"c":"hi","p":1},{"i":"x","p":2}]}`)) {
		t.Error("an op with an insert is not a comment update")
	}
	if isCommentUpdate(parse(`{"op":[]}`)) {
		t.Error("an empty op is not a comment update")
	}
	if isCommentUpdate(parse(`{"v":1}`)) {
		t.Error("an update with no op is not a comment update")
	}
	if !hasTrackedChanges(parse(`{"meta":{"tc":"abc"}}`)) {
		t.Error("meta.tc marks a tracked change")
	}
	if hasTrackedChanges(parse(`{"meta":{"source":"P.x"}}`)) {
		t.Error("meta without tc is not a tracked change")
	}
}

func TestAssertClientCanApplyUpdate(t *testing.T) {
	s := &Service{}
	const docID = "507f1f77bcf86cd799439011"
	client := func(level string) *clientContext {
		return &clientContext{privilegeLevel: level, docs: map[string]bool{docID: true}}
	}
	parse := func(s string) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		_ = json.Unmarshal([]byte(s), &m)
		return m
	}

	edit := parse(`{"op":[{"i":"x","p":1}]}`)
	comment := parse(`{"op":[{"c":"x","p":1}]}`)
	tracked := parse(`{"op":[{"i":"x","p":1}],"meta":{"tc":"id"}}`)

	if err := s.assertClientCanApplyUpdate(client("readOnly"), docID, edit); err == nil {
		t.Error("a read-only user must not edit")
	}
	if err := s.assertClientCanApplyUpdate(client("readOnly"), docID, comment); err != nil {
		t.Errorf("a read-only user may comment: %v", err)
	}
	if err := s.assertClientCanApplyUpdate(client("readOnly"), docID, tracked); err == nil {
		t.Error("a read-only user must not make tracked changes")
	}
	if err := s.assertClientCanApplyUpdate(client("review"), docID, tracked); err != nil {
		t.Errorf("a reviewer may make tracked changes: %v", err)
	}
	if err := s.assertClientCanApplyUpdate(client("review"), docID, edit); err == nil {
		t.Error("a reviewer must not make untracked edits")
	}
	if err := s.assertClientCanApplyUpdate(client("readAndWrite"), docID, edit); err != nil {
		t.Errorf("a writer may edit: %v", err)
	}
	// Access to the project is not access to every document in it.
	other := &clientContext{privilegeLevel: "owner", docs: map[string]bool{}}
	if err := s.assertClientCanApplyUpdate(other, docID, edit); err == nil {
		t.Error("an owner must still have joined the doc")
	}
}

func TestRemoteIP(t *testing.T) {
	req := func(remote, forwarded string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if forwarded != "" {
			r.Header.Set("X-Forwarded-For", forwarded)
		}
		return r
	}

	trusting := NewAddressResolver(true, "loopback")
	if got := trusting.RemoteIP(req("127.0.0.1:5000", "203.0.113.7")); got != "203.0.113.7" {
		t.Errorf("behind a trusted proxy the header wins, got %q", got)
	}
	if got := trusting.RemoteIP(req("127.0.0.1:5000", "203.0.113.7, 127.0.0.1")); got != "203.0.113.7" {
		t.Errorf("a chain of trusted hops is walked through, got %q", got)
	}
	// The header must not be believed when the connection did not come from a
	// proxy we trust, or any client could claim any address.
	if got := trusting.RemoteIP(req("198.51.100.4:5000", "203.0.113.7")); got != "198.51.100.4" {
		t.Errorf("an untrusted peer must not set its own address, got %q", got)
	}
	if got := trusting.RemoteIP(req("127.0.0.1:5000", "")); got != "127.0.0.1" {
		t.Errorf("without a header the socket address is used, got %q", got)
	}

	direct := NewAddressResolver(false, "loopback")
	if got := direct.RemoteIP(req("127.0.0.1:5000", "203.0.113.7")); got != "127.0.0.1" {
		t.Errorf("with no proxy configured the header is ignored, got %q", got)
	}
	if got := direct.RemoteIP(nil); got != "client-handshake-missing" {
		t.Errorf("a missing request is reported, got %q", got)
	}
}

// Two schemas are in use and they are not interchangeable. Under the upstream
// one the braces are Redis Cluster hash tags, keeping every key of one project
// on the same slot; server-ce replaces the schema with an unbraced one, and a
// service writing the wrong names queues updates where document-updater is not
// listening.
func TestKeySchema(t *testing.T) {
	upstream := NewKeySchema("")
	if got := upstream.ClientsInProject("p1"); got != "clients_in_project:{p1}" {
		t.Errorf("ClientsInProject = %q", got)
	}
	if got := upstream.ConnectedUser("p1", "P.abc"); got != "connected_user:{p1}:P.abc" {
		t.Errorf("ConnectedUser = %q", got)
	}
	if got := upstream.PendingUpdates("d1"); got != "PendingUpdates:{d1}" {
		t.Errorf("PendingUpdates = %q", got)
	}

	serverCE := NewKeySchema("server-ce")
	if got := serverCE.ClientsInProject("p1"); got != "clients_in_project:p1" {
		t.Errorf("ClientsInProject = %q", got)
	}
	if got := serverCE.ConnectedUser("p1", "P.abc"); got != "connected_user:p1:P.abc" {
		t.Errorf("ConnectedUser = %q", got)
	}
	if got := serverCE.PendingUpdates("d1"); got != "PendingUpdates:d1" {
		t.Errorf("PendingUpdates = %q", got)
	}

	// server-ce overrides the other three keys and leaves this one alone, so
	// it keeps its braces under both schemas.
	for _, keys := range []KeySchema{upstream, serverCE} {
		if got := keys.ProjectNotEmptySince("p1"); got != "projectNotEmptySince:{p1}" {
			t.Errorf("ProjectNotEmptySince = %q", got)
		}
	}

	// An unrecognised name must not silently pick the wrong schema.
	if NewKeySchema("nonsense") != upstream {
		t.Error("an unknown schema name should fall back to the upstream one")
	}
}

func TestNewPublicID(t *testing.T) {
	id := newPublicID()
	// 15 random bytes in URL-safe base64 is 20 characters, matching the
	// base64id package socket.io uses.
	if len(id) != len("P.")+20 {
		t.Errorf("public id %q has length %d, want 22", id, len(id))
	}
	if id[:2] != "P." {
		t.Errorf("public id %q must be prefixed", id)
	}
	if newPublicID() == id {
		t.Error("public ids must not repeat")
	}
}

// document-updater publishes its canary probe with a bare object where every
// other message carries an argument list. A strict envelope rejects the whole
// message, and the rejection is not confined to the canary: it is the same
// decode every editor event goes through.
func TestEditorEventTolerantPayload(t *testing.T) {
	canary := `{"message":"canary-applied-op","payload":{"ack":42,"broadcast":99,` +
		`"docId":"d1","projectId":"p1","source":"P.abc"}}`
	var ev editorEvent
	if err := json.Unmarshal([]byte(canary), &ev); err != nil {
		t.Fatalf("the canary message must still decode: %v", err)
	}
	if ev.Message != "canary-applied-op" {
		t.Errorf("message = %q", ev.Message)
	}
	// It is consumed here, never forwarded, so it yields no arguments.
	if args := ev.args(); args != nil {
		t.Errorf("args = %v, want none for a payload that is not a list", args)
	}

	normal := `{"room_id":"p1","message":"projectNameUpdated","payload":["Renamed"]}`
	if err := json.Unmarshal([]byte(normal), &ev); err != nil {
		t.Fatalf("a normal message must decode: %v", err)
	}
	args := ev.args()
	if len(args) != 1 || string(args[0]) != `"Renamed"` {
		t.Errorf("args = %v, want the single argument", args)
	}
}
