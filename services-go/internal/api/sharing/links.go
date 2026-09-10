package sharing

// Link sharing, and the two things that only an owner may do to a membership:
// send an invitation again, and hand the project over.
//
// A sharing link is a credential in a URL. That shapes everything here: the
// tokens are generated once and read back only by the owner, following a link
// puts the follower on their own access list so that turning sharing off can
// take that whole group out again, and the fragment carries a hash of the
// token so a browser can tell a truncated paste from a wrong one without the
// server ever seeing the difference.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/mailer"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// The shape of the tokens, which is the original's: a read-only token is
// twelve letters, and a read-write one is a run of digits followed by twelve
// letters. The digits are what make a read-write link recognisable as one.
const (
	tokenLetters      = "bcdfghjkmnpqrstvwxyz"
	tokenLetterCount  = 12
	tokenPrefixDigits = 10
)

// Tokens answers the project's sharing links, making them if it has none.
//
// Owner-only, and deliberately not part of the sharing list: a token admits
// whoever holds it, so handing one to a collaborator would let them widen
// access past what the owner granted them.
func (s *Service) Tokens(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.administrable(r)
	if err != nil {
		return err
	}

	tokens := project.Tokens
	if tokens == nil || tokens.ReadOnly == "" || tokens.ReadAndWrite == "" {
		made, err := newTokens()
		if err != nil {
			return apierr.Internal.WithCause(err)
		}
		if err := s.projects.SetTokens(r.Context(), project.ID, made); err != nil {
			return apierr.Internal.WithCause(err)
		}
		tokens = &made
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"readOnly":               tokens.ReadOnly,
		"readOnlyHashPrefix":     hashPrefix(tokens.ReadOnly),
		"readAndWrite":           tokens.ReadAndWrite,
		"readAndWriteHashPrefix": hashPrefix(tokens.ReadAndWrite),
		"readAndWritePrefix":     tokens.ReadAndWritePrefix,
	})
}

// RedeemToken admits somebody who followed a sharing link.
//
// No access check, for the same reason accepting an invitation has none: not
// having access is the whole reason the request exists, and holding a token
// for a project whose sharing is switched on is what authorises it.
func (s *Service) RedeemToken(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		return apierr.NotFound
	}

	project, privilege, err := s.projects.ByToken(r.Context(), token)
	if errors.Is(err, projects.ErrNotFound) {
		return apierr.NotFound.WithMessage("That link is no longer valid.")
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	// A token that still admits people after the owner has turned sharing off
	// is the revocation not having happened.
	if project.PublicAccessLevel != "tokenBased" {
		return apierr.NotFound.WithMessage("That link is no longer valid.")
	}

	// Already in by some other route: say so rather than adding a second,
	// weaker claim on the same project.
	if project.AccessFor(user.ID) != projects.AccessNone {
		return httpapi.JSON(w, http.StatusOK, map[string]any{
			"projectId": project.ID.Hex(),
			"privilege": privilege,
		})
	}

	if err := s.projects.GrantTokenAccess(r.Context(), project.ID, user.ID, privilege); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"projectId": project.ID.Hex(),
		"privilege": privilege,
	})
}

// ResendInvite sends an unaccepted invitation again.
//
// The same token, not a new one: the first mail may yet arrive, and issuing a
// second credential would mean two live invitations for one address. Only the
// expiry moves, so that a resend is worth something when the original is close
// to running out.
func (s *Service) ResendInvite(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.administrable(r)
	if err != nil {
		return err
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("inviteId"))
	if err != nil {
		return apierr.NotFound
	}

	var invite Invite
	err = s.invites.FindOne(r.Context(),
		bson.M{"_id": id, "projectId": project.ID}).Decode(&invite)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return apierr.NotFound
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	expires := time.Now().UTC().Add(inviteLifetime)
	if _, err := s.invites.UpdateByID(r.Context(), invite.ID, bson.M{
		"$set": bson.M{"expiresAt": expires},
	}); err != nil {
		return apierr.Internal.WithCause(err)
	}
	invite.ExpiresAt = expires

	path := "/project/invite/" + invite.Token
	sent := false
	if s.mail != nil && s.mail.Configured() {
		who := user.DisplayName()
		body := who + " has invited you to " + project.Name + `.

` + s.mail.Link(path) + `

The link works once and expires in 30 days.
`
		err := s.mail.Send(r.Context(), mailer.Message{
			To:      invite.Email,
			Subject: who + " shared a project with you",
			Text:    body,
		})
		sent = err == nil
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"invite": invite,
		"link":   path,
		"sent":   sent,
	})
}

// TransferOwnership hands the project to one of its members.
//
// Only to somebody who already has access: making a stranger the owner of a
// project by address would be an invitation that skips the invitation, and the
// person on the other end never agreed to own anything.
func (s *Service) TransferOwnership(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.administrable(r)
	if err != nil {
		return err
	}
	var in struct {
		UserID string `json:"userId"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	newOwner, err := bson.ObjectIDFromHex(strings.TrimSpace(in.UserID))
	if err != nil {
		return apierr.BadRequest.WithField("userId").
			WithMessage("That is not somebody on this project.")
	}
	if newOwner == project.OwnerRef {
		return apierr.BadRequest.WithField("userId").
			WithMessage("They already own this project.")
	}
	if project.AccessFor(newOwner) == projects.AccessNone {
		return apierr.BadRequest.WithField("userId").
			WithMessage("They do not have access to this project.")
	}

	if err := s.projects.SetOwner(r.Context(), project.ID, newOwner, user.ID); err != nil {
		return apierr.Internal.WithCause(err)
	}

	// Told, not asked: somebody who now owns a project should hear it from the
	// system rather than notice it.
	if s.mail != nil && s.mail.Configured() {
		if recipient, err := s.users.ByID(r.Context(), newOwner); err == nil {
			_ = s.mail.Send(r.Context(), mailer.Message{
				To:      recipient.Email,
				Subject: user.DisplayName() + " made you the owner of " + project.Name,
				Text: user.DisplayName() + " has made you the owner of " + project.Name + `.

` + s.mail.Link("/project/"+project.ID.Hex()) + `
`,
			})
		}
	}

	return httpapi.NoContent(w)
}

// newTokens makes a fresh pair.
func newTokens() (projects.Tokens, error) {
	readOnly, err := tokenLettersOnly()
	if err != nil {
		return projects.Tokens{}, err
	}
	prefix, err := tokenDigits()
	if err != nil {
		return projects.Tokens{}, err
	}
	suffix, err := tokenLettersOnly()
	if err != nil {
		return projects.Tokens{}, err
	}
	return projects.Tokens{
		ReadOnly:           readOnly,
		ReadAndWrite:       prefix + suffix,
		ReadAndWritePrefix: prefix,
	}, nil
}

func tokenLettersOnly() (string, error) {
	out := make([]byte, tokenLetterCount)
	max := big.NewInt(int64(len(tokenLetters)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = tokenLetters[n.Int64()]
	}
	return string(out), nil
}

func tokenDigits() (string, error) {
	out := make([]byte, tokenPrefixDigits)
	max := big.NewInt(10)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = byte('0' + n.Int64())
	}
	return string(out), nil
}

// hashPrefix is what a sharing link carries after the "#".
//
// A fragment never reaches the server, so this is only ever checked in the
// browser: it lets the page tell "you pasted half the link" from "this link
// has been revoked" without asking, and asking would mean sending a token that
// may not be one.
func hashPrefix(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:6]
}
