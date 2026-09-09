// Package auth is how somebody signs up, signs in and signs out.
//
// Who may register is asked at the moment somebody asks to, not when the
// process started: the answer comes from settings an administrator changes,
// and a site that opens registration should not need a restart before anybody
// can use it.
package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/session"
)

// Policy answers what the site currently allows. The settings package
// implements it; auth does not read settings itself, so that "who may
// register" stays one decision in one place.
type Policy interface {
	// RegistrationOpen says whether anybody may sign up.
	RegistrationOpen() bool
	// AllowedEmailDomains restricts sign-up when it is not empty.
	AllowedEmailDomains() []string
	// MinPasswordLength and MaxPasswordLength bound a new password.
	MinPasswordLength() int
	MaxPasswordLength() int
	// EnabledProviders lists the identity providers offered, for the sign-in
	// page to draw buttons for.
	EnabledProviders() []Provider
}

// Provider is an identity provider offered on the sign-in page.
type Provider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Service is the auth API.
type Service struct {
	users    *users.Store
	sessions *session.Store
	policy   Policy
}

// New builds the auth service.
func New(userStore *users.Store, sessions *session.Store, policy Policy) *Service {
	return &Service{users: userStore, sessions: sessions, policy: policy}
}

// --- what the sign-up page needs to draw itself -----------------------------

// StatusResponse is what a client is told about signing up before it offers
// the form.
type StatusResponse struct {
	// Open says whether the form should be offered at all.
	Open bool `json:"open"`
	// FirstUser says this is a site nobody has claimed: whoever registers
	// becomes the administrator, and the form says so.
	FirstUser bool `json:"firstUser"`
	// AllowedDomains is shown when sign-up is restricted.
	AllowedDomains []string `json:"allowedDomains"`
	// Providers are the identity providers to offer alongside the form.
	Providers []Provider `json:"providers"`
	// MinPasswordLength is enforced here and mirrored in the form.
	MinPasswordLength int `json:"minPasswordLength"`
}

// Status answers what the sign-up page needs.
func (s *Service) Status(w http.ResponseWriter, r *http.Request) error {
	claimable, err := s.claimable(r.Context())
	if err != nil {
		return err
	}
	domains := s.policy.AllowedEmailDomains()
	if domains == nil {
		domains = []string{}
	}
	providers := s.policy.EnabledProviders()
	if providers == nil {
		providers = []Provider{}
	}
	return httpapi.JSON(w, http.StatusOK, StatusResponse{
		// A site nobody has claimed is always open, however registration is
		// configured: somebody has to be able to set it up.
		Open:              claimable || s.policy.RegistrationOpen(),
		FirstUser:         claimable,
		AllowedDomains:    domains,
		Providers:         providers,
		MinPasswordLength: s.policy.MinPasswordLength(),
	})
}

// --- registering ------------------------------------------------------------

// RegisterRequest is the sign-up form.
type RegisterRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
}

// SessionResponse is what signing up or in answers with.
type SessionResponse struct {
	User *PublicUser `json:"user"`
	// Redirect is where the client should go next.
	Redirect string `json:"redirect"`
}

// PublicUser is the part of an account a client may see about itself.
type PublicUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	IsAdmin   bool   `json:"isAdmin"`
}

func publicUser(u *users.User) *PublicUser {
	if u == nil {
		return nil
	}
	return &PublicUser{
		ID:        u.ID.Hex(),
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		IsAdmin:   u.IsAdmin,
	}
}

// Register creates an account and signs the person in.
//
// Signing them in rather than sending them back to a sign-in form: somebody
// who just chose a password is holding the session that chose it.
func (s *Service) Register(w http.ResponseWriter, r *http.Request) error {
	var in RegisterRequest
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}

	email, err := s.validEmail(in.Email)
	if err != nil {
		return err
	}
	if err := s.validPassword(in.Password); err != nil {
		return err
	}

	claimable, err := s.claimable(r.Context())
	if err != nil {
		return err
	}
	if !claimable {
		if !s.policy.RegistrationOpen() {
			return apierr.Forbidden.WithMessage("Sign up is closed on this site.")
		}
		if !s.domainAllowed(email) {
			return apierr.Forbidden.
				WithField("email").
				WithMessage("Sign up is limited to these email domains: " +
					strings.Join(s.policy.AllowedEmailDomains(), ", "))
		}
	}

	user, err := s.users.Create(r.Context(), users.NewUser{
		Email:     email,
		Password:  in.Password,
		FirstName: in.FirstName,
		LastName:  in.LastName,
	})
	if errors.Is(err, users.ErrEmailTaken) {
		return apierr.Conflict.WithField("email").
			WithMessage("There is already an account with that email address.")
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	if claimable {
		if err := s.claim(r.Context(), user); err != nil {
			return apierr.Internal.WithCause(err)
		}
	}

	return s.signIn(w, r, user)
}

// --- signing in and out -----------------------------------------------------

// LoginRequest is the sign-in form.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login checks a password and starts a session.
func (s *Service) Login(w http.ResponseWriter, r *http.Request) error {
	var in LoginRequest
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	if in.Email == "" || in.Password == "" {
		return apierr.BadRequest.WithMessage("An email address and a password are required.")
	}

	user, err := s.users.Authenticate(r.Context(), in.Email, in.Password)
	if errors.Is(err, users.ErrWrongPassword) {
		// One message for a wrong password and for an address nobody holds:
		// telling them apart is how an account list is enumerated.
		return apierr.Unauthorised.WithMessage("That email address and password do not match an account.")
	}
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	if err := s.users.RecordLogin(r.Context(), user.ID, clientIP(r)); err != nil {
		// Worth a line in the log, not worth refusing the sign-in.
		_ = err
	}
	return s.signIn(w, r, user)
}

// Logout ends the session.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) error {
	id := httpapi.SessionIDFrom(r.Context())
	if err := s.sessions.Destroy(r.Context(), w, id); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, map[string]string{"redirect": "/login"})
}

// Me answers who the request is from.
func (s *Service) Me(w http.ResponseWriter, r *http.Request) error {
	user := httpapi.UserFrom(r.Context())
	if user == nil {
		return httpapi.JSON(w, http.StatusOK, map[string]any{"user": nil})
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

// SignInUser starts a session for a user who has been identified some other
// way, which is what an identity provider callback needs.
func (s *Service) SignInUser(w http.ResponseWriter, r *http.Request, user *users.User) error {
	return s.signIn(w, r, user)
}

// signIn writes the session and answers with it.
//
// A new session id, never the one the request arrived with: signing in on a
// session somebody else planted is session fixation, and a fresh identifier at
// the moment the identity changes is the defence that always works.
func (s *Service) signIn(w http.ResponseWriter, r *http.Request, user *users.User) error {
	sess := &session.Session{
		Passport: &session.Passport{User: &session.User{
			ID:        user.ID.Hex(),
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Email:     user.Email,
			IsAdmin:   user.IsAdmin,
		}},
	}

	redirect := "/projects"
	if previous := httpapi.SessionFrom(r.Context()); previous != nil && previous.Redirect != "" {
		redirect = previous.Redirect
	}
	if old := httpapi.SessionIDFrom(r.Context()); old != "" {
		_ = s.sessions.Destroy(r.Context(), w, old)
	}
	if _, err := s.sessions.Create(r.Context(), w, sess); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, SessionResponse{
		User:     publicUser(user),
		Redirect: redirect,
	})
}

// --- claiming a new site ----------------------------------------------------

// claimable says whether the site still has no administrator.
func (s *Service) claimable(ctx context.Context) (bool, error) {
	exists, err := s.users.AdminExists(ctx)
	if err != nil {
		return false, apierr.Internal.WithCause(err)
	}
	return !exists, nil
}

// claim makes the first account an administrator.
//
// Two people registering in the same second must not both get it, and no query
// over the users collection settles that: both would read no administrator and
// both would then write one. The claim is an insert of a single document with
// a fixed id, so the database decides.
func (s *Service) claim(ctx context.Context, user *users.User) error {
	taken, err := s.users.ClaimAdminSlot(ctx, user.ID)
	if err != nil {
		return err
	}
	if !taken {
		return nil
	}
	return s.users.MakeAdmin(ctx, user.ID)
}

// --- validation -------------------------------------------------------------

func (s *Service) validEmail(raw string) (string, error) {
	email := users.Normalise(raw)
	if email == "" {
		return "", apierr.BadRequest.WithField("email").WithMessage("An email address is required.")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", apierr.BadRequest.WithField("email").WithMessage("That does not look like an email address.")
	}
	if strings.Count(email, "@") != 1 {
		return "", apierr.BadRequest.WithField("email").WithMessage("That does not look like an email address.")
	}
	return email, nil
}

func (s *Service) validPassword(password string) error {
	min := s.policy.MinPasswordLength()
	max := s.policy.MaxPasswordLength()
	if len(password) < min {
		return apierr.BadRequest.WithField("password").
			WithMessage("A password needs at least " + itoa(min) + " characters.")
	}
	if max > 0 && len(password) > max {
		return apierr.BadRequest.WithField("password").
			WithMessage("A password can be at most " + itoa(max) + " characters.")
	}
	return nil
}

// domainAllowed says whether an address is in one of the allowed domains.
// A pattern beginning "*." matches that domain and anything under it.
func (s *Service) domainAllowed(email string) bool {
	patterns := s.policy.AllowedEmailDomains()
	if len(patterns) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(pattern, "@")))
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "*.") {
			base := pattern[2:]
			if domain == base || strings.HasSuffix(domain, "."+base) {
				return true
			}
			continue
		}
		if domain == pattern {
			return true
		}
	}
	return false
}

// clientIP is the address the request came from, trusting the proxy header
// only for its first entry, which is the one the edge wrote.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if comma := strings.IndexByte(forwarded, ','); comma > 0 {
			return strings.TrimSpace(forwarded[:comma])
		}
		return strings.TrimSpace(forwarded)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// ChangePassword replaces the password on the signed-in account.
//
// It lives here rather than with the other settings because it is not a
// setting: it is the credential this package exists to check, and the rule for
// what a password may be is the policy this package already holds.
func (s *Service) ChangePassword(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}

	// Proved by the old password, not by holding the session. A session left
	// open on a shared machine should not be enough to lock its owner out of
	// their own account.
	if _, err := s.users.Authenticate(r.Context(), user.Email, in.CurrentPassword); err != nil {
		return apierr.Forbidden.WithField("currentPassword").
			WithMessage("That is not your current password.")
	}
	if err := s.validPassword(in.NewPassword); err != nil {
		return err
	}
	if in.NewPassword == in.CurrentPassword {
		return apierr.BadRequest.WithField("newPassword").
			WithMessage("That is the password you already have.")
	}
	if err := s.users.SetPassword(r.Context(), user.ID, in.NewPassword); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}
