// Package sharing is who else may open a project.
//
// Three ways in, and they are deliberately different things:
//
//   - a member, added by address, who is in the project's own lists
//   - an invitation, for an address with no account yet, which becomes a
//     membership when it is accepted
//   - a link, which anybody holding it may use
//
// The distinction matters because removing somebody has to remove all three.
// Taking a person off the member list while a link they have still works is
// the kind of half-revocation that reads as a security fix and is not one.
package sharing

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/projects"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// How long an unaccepted invitation lasts, and how many one project may have.
const (
	inviteLifetime   = 30 * 24 * time.Hour
	maxOpenInvites   = 50
	maxCollaborators = 200
)

// Privilege is what somebody may do with a project.
type Privilege string

const (
	ReadAndWrite Privilege = "readAndWrite"
	ReadOnly     Privilege = "readOnly"
)

func (p Privilege) valid() bool {
	return p == ReadAndWrite || p == ReadOnly
}

// Invite is an offer of access to an address that has not accepted yet.
type Invite struct {
	ID        bson.ObjectID `bson:"_id" json:"id"`
	ProjectID bson.ObjectID `bson:"projectId" json:"-"`
	Email     string        `bson:"email" json:"email"`
	Privilege Privilege     `bson:"privileges" json:"privilege"`
	// Token is what an invitation link carries. It is the whole credential,
	// so it is generated here and never derived from anything guessable.
	Token     string        `bson:"token" json:"-"`
	SentBy    bson.ObjectID `bson:"sendingUserId" json:"-"`
	CreatedAt time.Time     `bson:"createdAt" json:"createdAt"`
	ExpiresAt time.Time     `bson:"expiresAt" json:"expiresAt"`
}

// member is one person who already has access.
type member struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Privilege Privilege `json:"privilege"`
	/// Owner is shown differently and cannot be removed.
	Owner bool `json:"owner"`
}

// Service answers the sharing endpoints.
type Service struct {
	projects *projects.Store
	users    *users.Store
	invites  *mongo.Collection
}

// New builds it.
func New(projectStore *projects.Store, userStore *users.Store, db *mongo.Database) *Service {
	return &Service{
		projects: projectStore,
		users:    userStore,
		invites:  db.Collection("projectInvites"),
	}
}

// EnsureIndexes creates what this package relies on.
func (s *Service) EnsureIndexes(ctx context.Context) error {
	_, err := s.invites.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "projectId", Value: 1}, {Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("project_email_unique"),
		},
		{
			Keys:    bson.D{{Key: "token", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("token_unique"),
		},
		{
			// An invitation nobody accepted stops being an open door.
			Keys:    bson.D{{Key: "expiresAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetName("expiresAt_1"),
		},
	})
	return err
}

// List answers with who has access and who has been invited.
func (s *Service) List(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.readable(r)
	if err != nil {
		return err
	}

	members := []member{}
	if owner, err := s.users.ByID(r.Context(), project.OwnerRef); err == nil {
		members = append(members, member{
			ID:        owner.ID.Hex(),
			Email:     owner.Email,
			Name:      owner.DisplayName(),
			Privilege: ReadAndWrite,
			Owner:     true,
		})
	}
	members = append(members, s.membersOf(r.Context(), project.Collaborators, ReadAndWrite)...)
	members = append(members, s.membersOf(r.Context(), project.ReadOnly, ReadOnly)...)

	// Invitations are only shown to somebody who could send one: they carry an
	// address, and a read-only collaborator has no business with that list.
	invites := []Invite{}
	if project.AccessFor(userOf(r).ID).CanAdmin() {
		cursor, err := s.invites.Find(r.Context(), bson.M{"projectId": project.ID})
		if err == nil {
			_ = cursor.All(r.Context(), &invites)
		}
	}

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"members":      members,
		"invites":      invites,
		"publicAccess": project.PublicAccessLevel,
	})
}

// Invite offers somebody access.
//
// If the address already has an account they are added straight away: an
// invitation they would have to accept adds a step and tells them nothing they
// would not learn by opening the project.
func (s *Service) Invite(w http.ResponseWriter, r *http.Request) error {
	project, user, err := s.administrable(r)
	if err != nil {
		return err
	}

	var in struct {
		Email     string    `json:"email"`
		Privilege Privilege `json:"privilege"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || !strings.Contains(email, "@") {
		return apierr.BadRequest.WithField("email").
			WithMessage("That does not look like an email address.")
	}
	if !in.Privilege.valid() {
		in.Privilege = ReadAndWrite
	}
	if len(project.Collaborators)+len(project.ReadOnly) >= maxCollaborators {
		return apierr.BadRequest.
			WithMessage("That is as many people as one project may have.")
	}

	// Already the owner, or already in.
	if owner, err := s.users.ByID(r.Context(), project.OwnerRef); err == nil {
		if strings.EqualFold(owner.Email, email) {
			return apierr.BadRequest.WithField("email").
				WithMessage("That is the owner of this project.")
		}
	}

	if existing, err := s.users.ByAnyEmail(r.Context(), email); err == nil {
		if project.AccessFor(existing.ID) != projects.AccessNone {
			return apierr.Conflict.WithField("email").
				WithMessage("They already have access to this project.")
		}
		if err := s.addMember(r.Context(), project, existing.ID, in.Privilege); err != nil {
			return apierr.Internal.WithCause(err)
		}
		return httpapi.JSON(w, http.StatusCreated, map[string]any{
			"member": member{
				ID:        existing.ID.Hex(),
				Email:     existing.Email,
				Name:      existing.DisplayName(),
				Privilege: in.Privilege,
			},
		})
	}

	count, err := s.invites.CountDocuments(r.Context(), bson.M{"projectId": project.ID})
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if count >= maxOpenInvites {
		return apierr.BadRequest.
			WithMessage("That is as many invitations as one project may have open.")
	}

	token, err := newToken()
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	invite := Invite{
		ID:        bson.NewObjectID(),
		ProjectID: project.ID,
		Email:     email,
		Privilege: in.Privilege,
		Token:     token,
		SentBy:    user.ID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(inviteLifetime),
	}
	if _, err := s.invites.InsertOne(r.Context(), invite); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apierr.Conflict.WithField("email").
				WithMessage("They have already been invited.")
		}
		return apierr.Internal.WithCause(err)
	}

	// The link is answered once, here, and never read back: it is the whole
	// credential, and an endpoint that lists tokens is an endpoint that leaks
	// them.
	return httpapi.JSON(w, http.StatusCreated, map[string]any{
		"invite": invite,
		"link":   "/project/invite/" + token,
	})
}

// Accept turns an invitation into access.
func (s *Service) Accept(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	token := r.PathValue("token")
	if token == "" {
		return apierr.NotFound
	}

	var invite Invite
	err = s.invites.FindOne(r.Context(), bson.M{"token": token}).Decode(&invite)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return apierr.NotFound.WithMessage("That invitation is no longer valid.")
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if time.Now().After(invite.ExpiresAt) {
		return apierr.NotFound.WithMessage("That invitation has expired.")
	}

	project, _, err := s.projects.Get(r.Context(), invite.ProjectID, user.ID)
	if err != nil {
		return apierr.NotFound.WithMessage("That project no longer exists.")
	}
	if err := s.addMember(r.Context(), project, user.ID, invite.Privilege); err != nil {
		return apierr.Internal.WithCause(err)
	}
	// Spent. An invitation that still works after it has been accepted is a
	// link that adds strangers to a project every time it is forwarded.
	_, _ = s.invites.DeleteOne(r.Context(), bson.M{"_id": invite.ID})

	return httpapi.JSON(w, http.StatusOK, map[string]any{
		"projectId": invite.ProjectID.Hex(),
	})
}

// RevokeInvite withdraws an invitation that has not been accepted.
func (s *Service) RevokeInvite(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.administrable(r)
	if err != nil {
		return err
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("inviteId"))
	if err != nil {
		return apierr.NotFound
	}
	result, err := s.invites.DeleteOne(r.Context(),
		bson.M{"_id": id, "projectId": project.ID})
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	if result.DeletedCount == 0 {
		return apierr.NotFound
	}
	return httpapi.NoContent(w)
}

// SetPrivilege changes what a member may do.
func (s *Service) SetPrivilege(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.administrable(r)
	if err != nil {
		return err
	}
	memberID, err := bson.ObjectIDFromHex(r.PathValue("userId"))
	if err != nil {
		return apierr.NotFound
	}
	if memberID == project.OwnerRef {
		return apierr.BadRequest.
			WithMessage("The owner's access cannot be changed.")
	}
	var in struct {
		Privilege Privilege `json:"privilege"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if !in.Privilege.valid() {
		return apierr.BadRequest.WithField("privilege").
			WithMessage("That is not a level of access.")
	}
	if err := s.addMember(r.Context(), project, memberID, in.Privilege); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// Remove takes somebody's access away.
func (s *Service) Remove(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	project, err := s.project(r)
	if err != nil {
		return err
	}
	memberID, err := bson.ObjectIDFromHex(r.PathValue("userId"))
	if err != nil {
		return apierr.NotFound
	}

	// Anybody may remove themselves; only an owner may remove anybody else.
	// Leaving a project should not need the owner's help.
	if memberID != user.ID && !project.AccessFor(user.ID).CanAdmin() {
		return apierr.Forbidden.
			WithMessage("Only the owner can remove somebody else.")
	}
	if memberID == project.OwnerRef {
		return apierr.BadRequest.
			WithMessage("The owner cannot be removed from their own project.")
	}

	// Off every list at once, including the token ones: taking somebody off
	// the member list while a link they already used still works is not a
	// removal.
	if err := s.projects.RemoveAccess(r.Context(), project.ID, memberID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// SetPublicAccess turns link sharing on or off.
func (s *Service) SetPublicAccess(w http.ResponseWriter, r *http.Request) error {
	project, _, err := s.administrable(r)
	if err != nil {
		return err
	}
	var in struct {
		Level string `json:"level"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	switch in.Level {
	case "private", "tokenBased":
	default:
		return apierr.BadRequest.WithField("level").
			WithMessage("That is not a sharing setting.")
	}
	if err := s.projects.SetPublicAccessLevel(r.Context(), project.ID, in.Level); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

func (s *Service) membersOf(
	ctx context.Context,
	ids []bson.ObjectID,
	privilege Privilege,
) []member {
	out := []member{}
	for _, id := range ids {
		user, err := s.users.ByID(ctx, id)
		if err != nil {
			// Somebody whose account has gone is not shown, and is cleaned up
			// the next time access is written.
			continue
		}
		out = append(out, member{
			ID:        user.ID.Hex(),
			Email:     user.Email,
			Name:      user.DisplayName(),
			Privilege: privilege,
		})
	}
	return out
}

// addMember puts somebody on exactly one of the two lists.
func (s *Service) addMember(
	ctx context.Context,
	project *projects.Project,
	userID bson.ObjectID,
	privilege Privilege,
) error {
	// Removed from both first, so that changing somebody from write to read
	// does not leave them on the write list as well.
	if err := s.projects.RemoveAccess(ctx, project.ID, userID); err != nil {
		return err
	}
	return s.projects.GrantAccess(ctx, project.ID, userID, string(privilege))
}

func newToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return strings.ToLower(
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw),
	), nil
}

func userOf(r *http.Request) *users.User {
	return httpapi.UserFrom(r.Context())
}

func (s *Service) project(r *http.Request) (*projects.Project, error) {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return nil, err
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		return nil, apierr.NotFound
	}
	project, _, err := s.projects.Get(r.Context(), id, user.ID)
	if errors.Is(err, projects.ErrNotFound) {
		return nil, apierr.NotFound
	}
	if err != nil {
		return nil, apierr.Internal.WithCause(err)
	}
	return project, nil
}

func (s *Service) readable(r *http.Request) (*projects.Project, *users.User, error) {
	project, err := s.project(r)
	if err != nil {
		return nil, nil, err
	}
	return project, userOf(r), nil
}

func (s *Service) administrable(r *http.Request) (*projects.Project, *users.User, error) {
	project, err := s.project(r)
	if err != nil {
		return nil, nil, err
	}
	user := userOf(r)
	if user == nil || !project.AccessFor(user.ID).CanAdmin() {
		return nil, nil, apierr.Forbidden.
			WithMessage("Only the owner can change who has access.")
	}
	return project, user, nil
}
