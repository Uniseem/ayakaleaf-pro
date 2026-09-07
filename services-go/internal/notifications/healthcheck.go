package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const healthCheckTimeout = 5 * time.Second

// healthCheckNotification is the subset of a stored notification the check
// reads back, matching getUserNotificationsResponseSchema.
type healthCheckNotification struct {
	ID          string `json:"_id"`
	Key         string `json:"key"`
	MessageOpts string `json:"messageOpts"`
	TemplateKey string `json:"templateKey"`
	UserID      string `json:"user_id"`
}

// healthCheck exercises the service end to end over HTTP: it creates a
// notification, reads it back, and deletes it again. Going through the HTTP
// layer rather than the store is deliberate -- it is what makes this a smoke
// test of the whole service rather than of Mongo alone.
func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	userID := bson.NewObjectID()
	notificationKey := "smoke-test-notification-" + bson.NewObjectID().Hex()

	s.log.Debug("Health Check: running",
		slog.String("userId", userID.Hex()), slog.String("key", notificationKey))

	// Cleanup runs whatever the outcome. The Node version passes a string
	// where an ObjectId is required and so never deletes anything; using the
	// ObjectId here means smoke-test documents no longer accumulate.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
		defer cancel()
		if err := s.store.DeleteByUserID(ctx, userID); err != nil {
			s.log.Error("Health Check: error cleaning up notification", slog.String("err", err.Error()))
		}
	}()

	if err := s.runHealthCheck(r.Context(), userID, notificationKey); err != nil {
		s.log.Error("Health Check: error running health check", slog.String("err", err.Error()))
		sendStatus(w, http.StatusInternalServerError)
		return
	}
	sendStatus(w, http.StatusOK)
}

func (s *Server) runHealthCheck(ctx context.Context, userID bson.ObjectID, notificationKey string) error {
	userPath := "/user/" + userID.Hex()

	if err := s.call(ctx, http.MethodPost, userPath, map[string]any{
		"key":         notificationKey,
		"messageOpts": "",
		"templateKey": "f4g5",
		"user_id":     userID.Hex(),
	}, nil); err != nil {
		return fmt.Errorf("creating notification: %w", err)
	}

	var found []healthCheckNotification
	if err := s.call(ctx, http.MethodGet, userPath, nil, &found); err != nil {
		return fmt.Errorf("getting notification: %w", err)
	}

	var match *healthCheckNotification
	for i := range found {
		if found[i].Key == notificationKey && found[i].UserID == userID.Hex() {
			match = &found[i]
			break
		}
	}
	if match == nil {
		return fmt.Errorf("notification not found in response")
	}

	s.log.Debug("Health Check: doing cleanup",
		slog.String("notificationId", match.ID), slog.String("notificationKey", match.Key))

	if err := s.call(ctx, http.MethodDelete, userPath+"/notification/"+match.ID, nil, nil); err != nil {
		return fmt.Errorf("deleting notification by id: %w", err)
	}
	if err := s.call(ctx, http.MethodDelete, userPath, map[string]any{"key": match.Key}, nil); err != nil {
		return fmt.Errorf("deleting notification by key: %w", err)
	}
	return nil
}

// call performs one self-request, decoding the response into out when given.
func (s *Server) call(ctx context.Context, method, path string, body any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.selfURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: non-2xx status code %d", method, path, res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}
