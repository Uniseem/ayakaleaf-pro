package oauth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/session"
)

// Settings is what the handlers need to know about how the site is set up.
// The settings package satisfies it.
type Settings interface {
	ProviderConfig(providerID string) (Config, bool)
	MayCreateAccounts() bool
	SiteURL() string
}

// SignIn starts a session for a user the handlers identified.
type SignIn func(w http.ResponseWriter, r *http.Request, user *users.User) error

// Service is the OAuth API.
type Service struct {
	users    *users.Store
	sessions *session.Store
	settings Settings
	signIn   SignIn
}

// NewService builds it.
func NewService(userStore *users.Store, sessions *session.Store, settings Settings, signIn SignIn) *Service {
	return &Service{users: userStore, sessions: sessions, settings: settings, signIn: signIn}
}

// Start sends somebody to their provider.
//
// Whether this is a sign-in or a link is decided by whether they are already
// signed in. There is nothing else the button could mean, and reading it as a
// sign-in would hand them somebody else's account if that identity happened to
// belong to another one.
func (s *Service) Start(w http.ResponseWriter, r *http.Request) error {
	provider, config, err := s.provider(r)
	if err != nil {
		return err
	}

	state, err := NewState()
	if err != nil {
		return apierr.Internal.WithCause(err)
	}

	sess := httpapi.SessionFrom(r.Context())
	sessionID := httpapi.SessionIDFrom(r.Context())
	if sess == nil {
		sess = &session.Session{}
	}
	if sess.Extra == nil {
		sess.Extra = map[string]jsonRaw{}
	}
	sess.Extra["oauthState"] = jsonString(state)
	sess.Extra["oauthProvider"] = jsonString(provider.ID)
	if httpapi.UserFrom(r.Context()) != nil {
		sess.Extra["oauthIntent"] = jsonString("link")
	} else {
		delete(sess.Extra, "oauthIntent")
	}

	if sessionID == "" {
		// Somebody signing in has no session yet, and the state has to live
		// somewhere until they come back.
		if sessionID, err = s.sessions.Create(r.Context(), w, sess); err != nil {
			return apierr.Internal.WithCause(err)
		}
	} else if err := s.sessions.Save(r.Context(), sessionID, sess); err != nil {
		return apierr.Internal.WithCause(err)
	}

	http.Redirect(w, r, AuthorizeURL(provider, config, state), http.StatusFound)
	return nil
}

// Callback is where the provider sends people back to.
//
// It answers with a redirect rather than JSON: a browser arrives here by
// following the provider's redirect, not by fetching.
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) error {
	provider, config, err := s.provider(r)
	if err != nil {
		return err
	}

	sess := httpapi.SessionFrom(r.Context())
	sessionID := httpapi.SessionIDFrom(r.Context())
	if sess == nil {
		return s.fail(w, r, "That sign-in did not complete. Try again.")
	}

	expected := readString(sess.Extra, "oauthState")
	intent := readString(sess.Extra, "oauthIntent")
	// Whatever happens next, the state is spent.
	delete(sess.Extra, "oauthState")
	delete(sess.Extra, "oauthProvider")
	delete(sess.Extra, "oauthIntent")
	if sessionID != "" {
		_ = s.sessions.Save(r.Context(), sessionID, sess)
	}

	if state := r.URL.Query().Get("state"); state == "" || state != expected {
		return s.fail(w, r, "That sign-in did not complete. Try again.")
	}
	if refused := r.URL.Query().Get("error"); refused != "" {
		return s.fail(w, r, "That sign-in was cancelled.")
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		return s.fail(w, r, "That sign-in did not complete. Try again.")
	}

	identity, err := Exchange(r.Context(), DefaultClient, provider, config, code)
	if err != nil {
		return s.fail(w, r, "That sign-in did not complete. Try again.")
	}

	if intent == "link" {
		return s.link(w, r, identity)
	}

	user, _, err := Resolve(r.Context(), s.users, identity, s.settings.MayCreateAccounts())
	if err != nil {
		return s.fail(w, r, capitalise(err.Error())+".")
	}
	// signIn writes JSON, which a redirect-driven arrival cannot use, so the
	// session is made here and the browser is sent on its way.
	if err := s.startSession(w, r, user); err != nil {
		return err
	}
	http.Redirect(w, r, "/projects", http.StatusFound)
	return nil
}

// link attaches an identity to the account already signed in.
func (s *Service) link(w http.ResponseWriter, r *http.Request, identity *Identity) error {
	user := httpapi.UserFrom(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return nil
	}

	holder, err := s.users.ByThirdPartyIdentity(r.Context(), identity.ProviderID, identity.ExternalUserID)
	if err == nil && holder.ID != user.ID {
		// Moving it would quietly lock the other account out of that provider.
		return s.failTo(w, r, "/account",
			"That "+identity.ProviderID+" account is already linked to another account here.")
	}
	if err != nil && !errors.Is(err, users.ErrNotFound) {
		return apierr.Internal.WithCause(err)
	}

	if err := s.users.LinkIdentity(r.Context(), user.ID, users.ThirdPartyIdentifier{
		ProviderID:     identity.ProviderID,
		ExternalUserID: identity.ExternalUserID,
	}); err != nil {
		return apierr.Internal.WithCause(err)
	}
	http.Redirect(w, r, "/account", http.StatusFound)
	return nil
}

// Unlink detaches a provider from the account signed in.
func (s *Service) Unlink(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	providerID := r.PathValue("provider")
	if _, known := Providers[providerID]; !known {
		return apierr.NotFound
	}

	remaining := 0
	held := false
	for _, identity := range user.ThirdPartyIdentifiers {
		if identity.ProviderID == providerID {
			held = true
			continue
		}
		remaining++
	}
	if !held {
		return apierr.NotFound.WithMessage("That provider is not linked.")
	}
	if remaining == 0 && user.HashedPassword == "" {
		return apierr.Forbidden.WithMessage(
			"That is the only way you can sign in. Set a password first, or link another provider.")
	}

	if err := s.users.UnlinkIdentity(r.Context(), user.ID, providerID); err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.NoContent(w)
}

// Linked answers which providers are configured and which this account holds.
func (s *Service) Linked(w http.ResponseWriter, r *http.Request) error {
	user := httpapi.UserFrom(r.Context())

	type linkedProvider struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Path   string `json:"path"`
		Linked bool   `json:"linked"`
	}
	answer := []linkedProvider{}
	for _, id := range []string{"google", "github"} {
		provider := Providers[id]
		if _, configured := s.settings.ProviderConfig(id); !configured {
			continue
		}
		linked := false
		if user != nil {
			for _, identity := range user.ThirdPartyIdentifiers {
				if identity.ProviderID == id {
					linked = true
					break
				}
			}
		}
		answer = append(answer, linkedProvider{
			ID: id, Name: provider.Name, Path: "/api/auth/" + id, Linked: linked,
		})
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"providers": answer})
}

// --- helpers ----------------------------------------------------------------

func (s *Service) provider(r *http.Request) (Provider, Config, error) {
	id := r.PathValue("provider")
	provider, known := Providers[id]
	if !known {
		return Provider{}, Config{}, apierr.NotFound
	}
	config, configured := s.settings.ProviderConfig(id)
	if !configured {
		return Provider{}, Config{}, apierr.NotFound.
			WithMessage("Signing in with " + provider.Name + " is not set up on this site.")
	}
	return provider, config, nil
}

func (s *Service) startSession(w http.ResponseWriter, r *http.Request, user *users.User) error {
	if old := httpapi.SessionIDFrom(r.Context()); old != "" {
		_ = s.sessions.Destroy(r.Context(), w, old)
	}
	_, err := s.sessions.Create(r.Context(), w, &session.Session{
		Passport: &session.Passport{User: &session.User{
			ID:        user.ID.Hex(),
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Email:     user.Email,
			IsAdmin:   user.IsAdmin,
		}},
	})
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return nil
}

// fail sends somebody back to the sign-in page with something to read.
//
// The message travels in the query string and the page shows it as text: it is
// written here, never by the provider, so there is nothing a link can put on
// that page.
func (s *Service) fail(w http.ResponseWriter, r *http.Request, message string) error {
	return s.failTo(w, r, "/login", message)
}

func (s *Service) failTo(w http.ResponseWriter, r *http.Request, path, message string) error {
	http.Redirect(w, r, path+"?error="+urlEncode(message), http.StatusFound)
	return nil
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
