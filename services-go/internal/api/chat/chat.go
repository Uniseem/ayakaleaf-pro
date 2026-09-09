// Package chat is the public face of the chat service.
//
// The chat service itself has no idea who may read a project: it stores
// messages against a project id and answers anybody who asks. It is reachable
// only from inside the deployment for exactly that reason, and this is the
// part that decides whether the person asking is allowed.
//
// It also fills in who wrote each message. Chat stores a user id, because a
// name copied into a message would be the name that person had at the time;
// resolving it here means a message shows who wrote it now.
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// maxMessageLength matches the chat service's own limit, so that a message too
// long is refused here with this API's error rather than the internal one's.
const maxMessageLength = 10 * 1024

// defaultLimit is how many messages are sent when none is asked for.
const defaultLimit = 50

// maxLimit caps what one request may ask for.
const maxLimit = 200

// Service answers the chat endpoints.
type Service struct {
	projects *projects.Store
	users    *users.Store
	baseURL  string
	http     *http.Client
}

// New builds it.
func New(projectStore *projects.Store, userStore *users.Store, baseURL string) *Service {
	return &Service{
		projects: projectStore,
		users:    userStore,
		baseURL:  strings.TrimRight(baseURL, "/"),
		http:     &http.Client{Timeout: 20 * time.Second},
	}
}

// storedMessage is what the chat service answers with.
type storedMessage struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Timestamp float64 `json:"timestamp"`
	UserID    string  `json:"user_id"`
}

// person is who wrote a message, as this API reports it.
type person struct {
	ID        string `json:"id"`
	Email     string `json:"email,omitempty"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
}

// message is a message with its author resolved.
type message struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Timestamp float64 `json:"timestamp"`
	User      person  `json:"user"`
}

// List answers with a project's messages, newest first.
func (s *Service) List(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}

	query := "limit=" + strconv.Itoa(limitFrom(r))
	if before := r.URL.Query().Get("before"); before != "" {
		if _, err := strconv.ParseInt(before, 10, 64); err != nil {
			return apierr.BadRequest.WithField("before").
				WithMessage("That is not a time.")
		}
		query += "&before=" + before
	}

	var stored []storedMessage
	if err := s.call(r.Context(), http.MethodGet,
		fmt.Sprintf("/project/%s/messages?%s", project.ID.Hex(), query),
		nil, &stored); err != nil {
		return err
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"messages": s.withAuthors(r.Context(), stored),
	})
}

// Send adds a message to a project.
func (s *Service) Send(w http.ResponseWriter, r *http.Request) error {
	// Reading a project is not enough to write in its chat: somebody given a
	// read-only link is being shown the project, not invited into it.
	project, user, err := s.writable(r)
	if err != nil {
		return err
	}

	var in struct {
		Content string `json:"content"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return apierr.BadRequest.WithField("content").
			WithMessage("A message cannot be empty.")
	}
	if len(content) > maxMessageLength {
		return apierr.BadRequest.WithField("content").
			WithMessage("That message is too long.")
	}

	body, err := json.Marshal(map[string]any{
		"user_id": user.ID.Hex(),
		"content": content,
	})
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	var stored storedMessage
	if err := s.call(r.Context(), http.MethodPost,
		fmt.Sprintf("/project/%s/messages", project.ID.Hex()),
		body, &stored); err != nil {
		return err
	}

	written := s.withAuthors(r.Context(), []storedMessage{stored})
	if len(written) == 0 {
		return apierr.Internal.WithMessage("That message was stored but cannot be read back.")
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{"message": written[0]})
}

// withAuthors resolves the user behind each message.
//
// One lookup per distinct author rather than per message: a conversation is
// usually a few people saying many things.
func (s *Service) withAuthors(ctx context.Context, stored []storedMessage) []message {
	found := map[string]person{}
	out := make([]message, 0, len(stored))

	for _, each := range stored {
		who, known := found[each.UserID]
		if !known {
			who = person{ID: each.UserID}
			if id, err := bson.ObjectIDFromHex(each.UserID); err == nil {
				if user, err := s.users.ByID(ctx, id); err == nil {
					who.Email = user.Email
					who.FirstName = user.FirstName
					who.LastName = user.LastName
				}
				// A message from somebody whose account has gone keeps the
				// message: deleting an account should not silently rewrite a
				// conversation other people were part of.
			}
			found[each.UserID] = who
		}
		out = append(out, message{
			ID:        each.ID,
			Content:   each.Content,
			Timestamp: each.Timestamp,
			User:      who,
		})
	}
	return out
}

// call sends a request to the chat service and reads its answer.
func (s *Service) call(ctx context.Context, method, path string, body []byte, into any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reader)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := s.http.Do(request)
	if err != nil {
		return apierr.Internal.WithCause(err).
			WithMessage("Chat is not answering at the moment.")
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 400 {
		return apierr.Internal.WithCause(
			fmt.Errorf("chat answered %d", response.StatusCode))
	}
	if into == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return nil
}

func limitFrom(r *http.Request) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return defaultLimit
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultLimit
	}
	if value > maxLimit {
		return maxLimit
	}
	return value
}

func (s *Service) readable(r *http.Request) (*projects.Project, *users.User, error) {
	return s.project(r, false)
}

func (s *Service) writable(r *http.Request) (*projects.Project, *users.User, error) {
	return s.project(r, true)
}

func (s *Service) project(r *http.Request, needWrite bool) (*projects.Project, *users.User, error) {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return nil, nil, err
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		return nil, nil, apierr.NotFound
	}
	project, access, err := s.projects.Get(r.Context(), id, user.ID)
	if errors.Is(err, projects.ErrNotFound) {
		return nil, nil, apierr.NotFound
	}
	if err != nil {
		return nil, nil, apierr.Internal.WithCause(err)
	}
	if needWrite && !access.CanWrite() {
		return nil, nil, apierr.Forbidden.
			WithMessage("You have read-only access to this project.")
	}
	return project, user, nil
}
