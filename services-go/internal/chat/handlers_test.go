package chat

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/obsv"
)

// newTestHandler builds the router with no store behind it.
//
// Every case below is rejected during request validation, before any database
// access, which is exactly the ordering the Node service has: exegesis
// validates the body, readContext validates the path parameters, and only then
// does the controller talk to Mongo.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return NewServer(nil, slog.New(slog.DiscardHandler), obsv.New("chat-test")).Handler()
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decodeStringBody reads a JSON string body, the form exegesis produces for
// res.status(400).setBody('some message').
func decodeStringBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("body %q is not a JSON string: %v", rec.Body.String(), err)
	}
	return s
}

// The cases below mirror the failure cases in
// services/chat/test/acceptance/js/SendingAMessageTests.js.
func TestSendMessageValidationFailures(t *testing.T) {
	h := newTestHandler(t)
	const projectID = "507f1f77bcf86cd799439011"
	const threadID = "507f191e810c19729de860ea"
	const userID = "507f191e810c19729de860eb"

	t.Run("malformed userId", func(t *testing.T) {
		rec := post(t, h, "/project/"+projectID+"/thread/"+threadID+"/messages",
			`{"user_id":"malformed-user","content":"content"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if got := decodeStringBody(t, rec); got != "Invalid userId" {
			t.Errorf("body = %q, want %q", got, "Invalid userId")
		}
	})

	t.Run("malformed projectId", func(t *testing.T) {
		rec := post(t, h, "/project/malformed-project/thread/"+threadID+"/messages",
			`{"user_id":"`+userID+`","content":"content"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if got := decodeStringBody(t, rec); got != "Invalid projectId" {
			t.Errorf("body = %q, want %q", got, "Invalid projectId")
		}
	})

	t.Run("malformed threadId", func(t *testing.T) {
		rec := post(t, h, "/project/"+projectID+"/thread/malformed-thread-id/messages",
			`{"user_id":"`+userID+`","content":"content"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if got := decodeStringBody(t, rec); got != "Invalid threadId" {
			t.Errorf("body = %q, want %q", got, "Invalid threadId")
		}
	})

	t.Run("null content is a schema violation", func(t *testing.T) {
		rec := post(t, h, "/project/"+projectID+"/thread/"+threadID+"/messages",
			`{"user_id":"`+userID+`","content":null}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		var body struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body %q is not an object: %v", rec.Body.String(), err)
		}
		if body.Message != "Validation errors" {
			t.Errorf("message = %q, want %q", body.Message, "Validation errors")
		}
	})

	t.Run("content over the length limit", func(t *testing.T) {
		content := strings.Repeat("-", 10*1024+1)
		rec := post(t, h, "/project/"+projectID+"/thread/"+threadID+"/messages",
			`{"user_id":"`+userID+`","content":"`+content+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if got := decodeStringBody(t, rec); got != "Content too long (> 10240 bytes)" {
			t.Errorf("body = %q, want %q", got, "Content too long (> 10240 bytes)")
		}
	})
}

// A malformed projectId must be rejected on read paths too, because
// readContext runs for every route that takes one.
func TestPathParameterValidationOnReads(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{
		"/project/malformed-project/threads",
		"/project/malformed-project/messages",
		"/project/malformed-project/resolved-thread-ids",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d, want 400", path, rec.Code)
		}
		if got := decodeStringBody(t, rec); got != "Invalid projectId" {
			t.Errorf("GET %s: body = %q, want %q", path, got, "Invalid projectId")
		}
	}
}

func TestStatusEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	newTestHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// GettingMessagesTests asserts on the decoded value, so the body must be
	// a JSON string rather than bare text.
	if got := decodeStringBody(t, rec); got != "chat is alive" {
		t.Errorf("body = %q, want %q", got, "chat is alive")
	}
}

func TestUnknownRouteReturnsExegesisNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/no/such/route", nil)
	rec := httptest.NewRecorder()
	newTestHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not an object: %v", rec.Body.String(), err)
	}
	if body["message"] != "Not found" {
		t.Errorf("message = %q, want %q", body["message"], "Not found")
	}
}

// GettingMessagesTests scrapes /metrics for a counter labelled with the route
// template, so the label has to survive exactly.
func TestMetricsExposeRouteTemplateLabel(t *testing.T) {
	h := newTestHandler(t)
	post(t, h, "/project/malformed-project/messages", `{"user_id":"x","content":"y"}`)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	var found bool
	for _, line := range strings.Split(string(body), "\n") {
		if strings.Contains(line, "timer_http_request_count") &&
			strings.Contains(line, `path="project_{projectId}_messages"`) &&
			strings.Contains(line, `method="POST"`) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no timer_http_request_count series for the messages route:\n%s", body)
	}
}
