package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/mailer"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Forgetting a password.

// How long a reset link lasts. An hour is the usual answer: long enough to
// find the message, short enough that a forwarded mailbox is not a standing
// key to the account.
const resetLifetime = time.Hour

// resetToken is a live reset request.
//
// The token is stored as a hash, not as itself. Anybody who can read this
// collection -- a backup, a log, a compromised database -- would otherwise
// hold a working key to every account that had asked for a reset.
type resetToken struct {
	ID        bson.ObjectID `bson:"_id"`
	UserID    bson.ObjectID `bson:"user_id"`
	Hash      string        `bson:"hash"`
	CreatedAt time.Time     `bson:"createdAt"`
	ExpiresAt time.Time     `bson:"expiresAt"`
}

// EnsureResetIndexes creates what the reset flow relies on.
func (s *Service) EnsureResetIndexes(ctx context.Context) error {
	if s.resets == nil {
		return nil
	}
	_, err := s.resets.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "hash", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("hash_unique"),
		},
		{
			// An unused request stops being a way in.
			Keys:    bson.D{{Key: "expiresAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetName("expiresAt_1"),
		},
	})
	return err
}

// RequestReset starts a password reset.
//
// It answers the same way whether or not the address has an account. Telling
// them apart turns this endpoint into a way to ask whether somebody is a user
// here, which is the one thing it must not be.
func (s *Service) RequestReset(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email string `json:"email"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))

	// Whether a reset can be delivered at all is not about this address, so it
	// is answered before looking anybody up.
	if s.mail == nil || !s.mail.Configured() {
		return apierr.BadRequest.WithMessage(
			"This site has no mail server, so it cannot send a reset link. " +
				"Ask an administrator to set your password.")
	}

	if email != "" && strings.Contains(email, "@") {
		if user, err := s.users.ByAnyEmail(r.Context(), email); err == nil {
			if err := s.sendReset(r.Context(), user.ID, user.Email); err != nil {
				return apierr.Internal.WithCause(err).
					WithMessage("The reset email could not be sent.")
			}
		}
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"sent":    true,
		"message": "If there is an account for that address, a reset link is on its way.",
	})
}

func (s *Service) sendReset(ctx context.Context, userID bson.ObjectID, email string) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := strings.ToLower(
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))

	sum := sha256.Sum256([]byte(token))
	now := time.Now().UTC()

	// Any earlier request for this account stops working: asking again should
	// not leave two keys outstanding.
	if _, err := s.resets.DeleteMany(ctx, bson.M{"user_id": userID}); err != nil {
		return err
	}
	if _, err := s.resets.InsertOne(ctx, resetToken{
		ID:        bson.NewObjectID(),
		UserID:    userID,
		Hash:      hex.EncodeToString(sum[:]),
		CreatedAt: now,
		ExpiresAt: now.Add(resetLifetime),
	}); err != nil {
		return err
	}

	link := s.mail.Link("/password/set?token=" + token)
	return s.mail.Send(ctx, mailer.Message{
		To:      email,
		Subject: "Reset your password",
		Text: "Somebody asked to reset the password for this account.\n\n" +
			link + "\n\n" +
			"The link works once and expires in an hour.\n\n" +
			"If it was not you, nothing has changed and you can ignore this.\n",
	})
}

// SetPasswordFromToken finishes a reset.
func (s *Service) SetPasswordFromToken(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Token) == "" {
		return apierr.BadRequest.WithField("token").
			WithMessage("That reset link is not valid.")
	}
	if err := s.validPassword(in.Password); err != nil {
		return err
	}

	sum := sha256.Sum256([]byte(strings.TrimSpace(in.Token)))
	hash := hex.EncodeToString(sum[:])

	// Found and removed in one step, so a link cannot be used twice even if
	// two requests arrive at the same moment.
	var held resetToken
	err := s.resets.FindOneAndDelete(r.Context(), bson.M{"hash": hash}).Decode(&held)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return apierr.BadRequest.WithField("token").
			WithMessage("That reset link is not valid, or has already been used.")
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if time.Now().After(held.ExpiresAt) {
		return apierr.BadRequest.WithField("token").
			WithMessage("That reset link has expired. Ask for another.")
	}

	if err := s.users.SetPassword(r.Context(), held.UserID, in.Password); err != nil {
		return apierr.Internal.WithCause(err)
	}

	// Every other session for this account is ended. A reset is what somebody
	// does when they think their password is known to somebody else, and
	// leaving that person signed in elsewhere is the one outcome it must not
	// have.
	if s.sessions != nil {
		if err := s.sessions.RemoveAllForUser(r.Context(), held.UserID.Hex()); err != nil {
			// Not fatal: the password is already changed, and saying otherwise
			// would send them round the loop again for no gain.
			_ = err
		}
	}

	return httpapi.NoContent(w)
}

// Sessions lists the sessions this account has open.
func (s *Service) Sessions(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	open, err := s.sessions.Sessions(r.Context(), user.ID.Hex())
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	current := httpapi.SessionIDFrom(r.Context())
	out := make([]map[string]any, 0, len(open))
	for _, each := range open {
		out = append(out, map[string]any{
			"ipAddress":        each.IPAddress,
			"sessionCreatedAt": each.CreatedAt,
			"isCurrent":        each.ID == current,
		})
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// ClearSessions ends every session but the one asking.
func (s *Service) ClearSessions(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	// The session doing the asking is kept: somebody clearing their other
	// sessions did not ask to be signed out of the machine they are at.
	if err := s.sessions.RemoveAllForUser(
		r.Context(), user.ID.Hex(), httpapi.SessionIDFrom(r.Context()),
	); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}
