// Package oauth signs people in with Google or GitHub, and attaches either to
// an account that already exists.
//
// One person may hold a password, a Google account and a GitHub account, and
// they are meant to be one account here, not three. What makes them one is the
// verified email address: an identity whose address already belongs to
// somebody is attached to that account rather than starting a new one.
// Anything else silently hands people a second, empty account the first time
// they use a different button.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/users"
)

// Identity is what a provider told us about somebody.
type Identity struct {
	ProviderID     string
	ExternalUserID string
	Email          string
	FirstName      string
	LastName       string
}

// Provider is one identity provider.
type Provider struct {
	ID   string
	Name string
	// AuthorizeURL and TokenURL are the two ends of the code exchange.
	AuthorizeURL string
	TokenURL     string
	Scopes       []string
	// Profile turns an access token into an identity.
	Profile func(ctx context.Context, client *http.Client, token string) (*Identity, error)
}

// Providers, as data. Adding a third is adding an entry.
var Providers = map[string]Provider{
	"google": {
		ID:           "google",
		Name:         "Google",
		AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Scopes:       []string{"openid", "email", "profile"},
		Profile:      googleProfile,
	},
	"github": {
		ID:           "github",
		Name:         "GitHub",
		AuthorizeURL: "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		Scopes:       []string{"read:user", "user:email"},
		Profile:      githubProfile,
	},
}

// Config is what one provider needs to be usable.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// ErrNotConfigured is returned for a provider an administrator has not set up.
var ErrNotConfigured = errors.New("that provider is not configured")

// AuthorizeURL is where to send somebody to sign in.
func AuthorizeURL(provider Provider, config Config, state string) string {
	query := url.Values{}
	query.Set("client_id", config.ClientID)
	query.Set("redirect_uri", config.RedirectURL)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(provider.Scopes, " "))
	query.Set("state", state)
	return provider.AuthorizeURL + "?" + query.Encode()
}

// Exchange turns the code a provider sent back into an identity.
func Exchange(
	ctx context.Context,
	client *http.Client,
	provider Provider,
	config Config,
	code string,
) (*Identity, error) {
	form := url.Values{}
	form.Set("client_id", config.ClientID)
	form.Set("client_secret", config.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", config.RedirectURL)
	form.Set("grant_type", "authorization_code")

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s refused the code exchange: %s", provider.Name, response.Status)
	}

	var token struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, err
	}
	if token.Error != "" {
		return nil, fmt.Errorf("%s refused the code exchange: %s", provider.Name, token.Error)
	}
	if token.AccessToken == "" {
		return nil, fmt.Errorf("%s returned no access token", provider.Name)
	}

	identity, err := provider.Profile(ctx, client, token.AccessToken)
	if err != nil {
		return nil, err
	}
	identity.ProviderID = provider.ID
	return identity, nil
}

// NewState makes the value that ties a callback to the request that started
// it. Without it a callback is a request anybody can make.
func NewState() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// --- the profiles -----------------------------------------------------------

func googleProfile(ctx context.Context, client *http.Client, token string) (*Identity, error) {
	var profile struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
	}
	if err := get(ctx, client, "https://openidconnect.googleapis.com/v1/userinfo", token, &profile); err != nil {
		return nil, err
	}
	if profile.Sub == "" {
		return nil, errors.New("Google did not identify the account")
	}
	identity := &Identity{
		ExternalUserID: profile.Sub,
		FirstName:      profile.GivenName,
		LastName:       profile.FamilyName,
	}
	// An unverified address is not evidence of anything: attaching an account
	// on the strength of one would let somebody claim an address by asserting
	// it at their identity provider.
	if profile.EmailVerified {
		identity.Email = profile.Email
	}
	return identity, nil
}

func githubProfile(ctx context.Context, client *http.Client, token string) (*Identity, error) {
	var profile struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := get(ctx, client, "https://api.github.com/user", token, &profile); err != nil {
		return nil, err
	}
	if profile.ID == 0 {
		return nil, errors.New("GitHub did not identify the account")
	}

	email := profile.Email
	if email == "" {
		// GitHub leaves the address out of the profile when it is private, so
		// the verified one is asked for separately.
		var addresses []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := get(ctx, client, "https://api.github.com/user/emails", token, &addresses); err == nil {
			for _, candidate := range addresses {
				if candidate.Primary && candidate.Verified {
					email = candidate.Email
					break
				}
				if candidate.Verified && email == "" {
					email = candidate.Email
				}
			}
		}
	}

	first, last, _ := strings.Cut(strings.TrimSpace(profile.Name), " ")
	if first == "" {
		first = profile.Login
	}
	return &Identity{
		ExternalUserID: fmt.Sprintf("%d", profile.ID),
		Email:          email,
		FirstName:      first,
		LastName:       last,
	}, nil
}

func get(ctx context.Context, client *http.Client, endpoint, token string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "ayakaleaf")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", endpoint, response.Status)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(into)
}

// DefaultClient is what the exchanges use. A timeout, because a provider that
// hangs must not hold a request open until somebody gives up.
var DefaultClient = &http.Client{Timeout: 15 * time.Second}

// Resolve finds or makes the account an identity belongs to.
//
// Returned with linked=false when it made one, so the caller can tell somebody
// that a new account was created rather than that they signed in.
func Resolve(
	ctx context.Context,
	store *users.Store,
	identity *Identity,
	mayCreate bool,
) (user *users.User, created bool, err error) {
	if identity.ExternalUserID == "" {
		return nil, false, errors.New("that provider did not identify the account")
	}

	// Already attached: this is simply a sign-in.
	existing, err := store.ByThirdPartyIdentity(ctx, identity.ProviderID, identity.ExternalUserID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, users.ErrNotFound) {
		return nil, false, err
	}

	if identity.Email == "" {
		return nil, false, errors.New(
			"that provider did not give us a verified email address. Add one there, or sign in another way and link it from your account settings")
	}

	// The same person, arriving by a door they have not used before.
	owner, err := store.ByAnyEmail(ctx, identity.Email)
	if err == nil {
		if err := store.LinkIdentity(ctx, owner.ID, users.ThirdPartyIdentifier{
			ProviderID:     identity.ProviderID,
			ExternalUserID: identity.ExternalUserID,
		}); err != nil {
			return nil, false, err
		}
		return owner, false, nil
	}
	if !errors.Is(err, users.ErrNotFound) {
		return nil, false, err
	}

	if !mayCreate {
		return nil, false, errors.New(
			"this site does not create accounts from Google or GitHub. Ask an administrator for one, then link it from your account settings")
	}

	created_, err := store.Create(ctx, users.NewUser{
		Email:     identity.Email,
		FirstName: identity.FirstName,
		LastName:  identity.LastName,
		// The provider vouched for the address, so it does not need confirming
		// again by email -- which matters on a site with no mail server.
		Confirmed: true,
	})
	if err != nil {
		return nil, false, err
	}
	if err := store.LinkIdentity(ctx, created_.ID, users.ThirdPartyIdentifier{
		ProviderID:     identity.ProviderID,
		ExternalUserID: identity.ExternalUserID,
	}); err != nil {
		return nil, false, err
	}
	return created_, true, nil
}
