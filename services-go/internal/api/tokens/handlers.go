package tokens

import (
	"errors"
	"net/http"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Service is the tokens API: what somebody sees and does on their own account
// page.
type Service struct {
	store *Store
}

// NewService builds it.
func NewService(store *Store) *Service { return &Service{store: store} }

// List answers with somebody's tokens.
//
// Never with the tokens themselves -- they are not stored -- only with enough
// to tell one from another: the first few characters, when it was made, and
// when it was last used.
func (s *Service) List(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	list, err := s.store.List(r.Context(), user.ID)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"tokens": list})
}

// Create makes a token and answers with it.
//
// The only time it is ever sent: it is hashed before it is stored, so this
// response is the one chance to copy it, and the client says so.
func (s *Service) Create(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	plain, token, err := s.store.Create(r.Context(), user.ID)
	if errors.Is(err, ErrTooMany) {
		return apierr.BadRequest.WithMessage(
			"You already have as many tokens as an account may have. Revoke one first.")
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"token":       token,
		"accessToken": plain,
	})
}

// Delete revokes a token.
func (s *Service) Delete(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("tokenId"))
	if err != nil {
		return apierr.NotFound
	}
	if err := s.store.Delete(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierr.NotFound
		}
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}
