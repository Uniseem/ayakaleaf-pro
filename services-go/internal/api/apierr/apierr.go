// Package apierr is the one error shape the API answers with.
//
// Every handler returns one of these or nothing. The client has a single thing
// to parse, and the decision about what an error means to a person is made
// once, here, rather than in each handler -- which is how the service it
// replaces ended up with four different JSON error shapes on the same page.
package apierr

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Error is what a failed request answers with.
type Error struct {
	// Status is the HTTP status. Not serialised: it is the response's status.
	Status int `json:"-"`
	// Code is a stable, machine-readable name. Clients branch on this and
	// never on the message.
	Code string `json:"code"`
	// Message is for a person, already in the words they should see.
	Message string `json:"message"`
	// Field names the input that was wrong, when one was.
	Field string `json:"field,omitempty"`
	// cause is kept for the log and never sent.
	cause error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.cause }

// WithCause attaches the underlying error, for the log only.
func (e *Error) WithCause(err error) *Error {
	clone := *e
	clone.cause = err
	return &clone
}

// WithField names the input at fault.
func (e *Error) WithField(field string) *Error {
	clone := *e
	clone.Field = field
	return &clone
}

// WithMessage replaces the message shown to a person.
func (e *Error) WithMessage(message string) *Error {
	clone := *e
	clone.Message = message
	return &clone
}

// New builds an error.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// The errors the whole API shares. A handler that needs a different message
// says so with WithMessage rather than inventing a code.
var (
	BadRequest   = New(http.StatusBadRequest, "bad_request", "That request could not be understood.")
	Unauthorised = New(http.StatusUnauthorized, "unauthorised", "You need to sign in to do that.")
	Forbidden    = New(http.StatusForbidden, "forbidden", "You do not have access to that.")
	NotFound     = New(http.StatusNotFound, "not_found", "That does not exist.")
	Conflict     = New(http.StatusConflict, "conflict", "That has already been done.")
	TooMany      = New(http.StatusTooManyRequests, "too_many_requests", "Too many attempts. Wait a moment and try again.")
	Internal     = New(http.StatusInternalServerError, "internal", "Something went wrong at our end.")
)

// Write sends an error as the response.
//
// Anything that is not already one of ours is answered as an internal error
// and logged in full: a message written for a developer is not a message to
// put in front of somebody using the site.
func Write(w http.ResponseWriter, log *slog.Logger, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		apiErr = Internal.WithCause(err)
	}
	if apiErr.Status >= 500 && log != nil {
		log.Error("request failed", slog.String("code", apiErr.Code), slog.Any("err", err))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(apiErr.Status)
	_ = json.NewEncoder(w).Encode(struct {
		Error *Error `json:"error"`
	}{apiErr})
}
