// Package settings holds what an administrator changes from the admin pages.
//
// It is read constantly and written rarely, so the whole document is kept in
// memory and swapped on change rather than fetched per request. A change made
// in one process reaches the others over Redis; a process that misses the
// message is stale only until the next one, and the stored document stays the
// truth.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/mailer"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/auth"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/githubsync"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/oauth"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// documentID is the single document every setting lives in.
const documentID = "site"

// changeChannel is how one process tells the others a setting changed.
const changeChannel = "site-settings:changed"

// Values is the stored settings, as typed as this service needs them. Anything
// it does not model is kept in Raw so a write never drops what another version
// wrote.
type Values struct {
	SiteURL string `json:"siteUrl,omitempty"`
	AppName string `json:"appName,omitempty"`

	AllowPublicRegistration  *bool  `json:"allowPublicRegistration,omitempty"`
	RegistrationEmailDomains string `json:"registrationEmailDomains,omitempty"`
	EmailConfirmation        string `json:"emailConfirmation,omitempty"`

	EmailFromAddress   string `json:"emailFromAddress,omitempty"`
	EmailReplyTo       string `json:"emailReplyTo,omitempty"`
	EmailSmtpHost      string `json:"emailSmtpHost,omitempty"`
	EmailSmtpPort      any    `json:"emailSmtpPort,omitempty"`
	EmailSmtpUser      string `json:"emailSmtpUser,omitempty"`
	EmailSmtpPass      string `json:"emailSmtpPass,omitempty"`
	EmailSmtpSecure    *bool  `json:"emailSmtpSecure,omitempty"`
	EmailSmtpIgnoreTLS *bool  `json:"emailSmtpIgnoreTLS,omitempty"`

	PasswordMinLength any `json:"passwordMinLength,omitempty"`
	PasswordMaxLength any `json:"passwordMaxLength,omitempty"`

	CompileTimeout any    `json:"compileTimeout,omitempty"`
	TexLiveImages  string `json:"texLiveImages,omitempty"`

	GitBridgeEnabled *bool `json:"gitBridgeEnabled,omitempty"`

	GitHubSyncEnabled      *bool  `json:"githubSyncEnabled,omitempty"`
	GitHubSyncClientID     string `json:"githubSyncClientId,omitempty"`
	GitHubSyncClientSecret string `json:"githubSyncClientSecret,omitempty"`

	GoogleClientID     string `json:"googleClientId,omitempty"`
	GoogleClientSecret string `json:"googleClientSecret,omitempty"`
	GitHubClientID     string `json:"githubClientId,omitempty"`
	GitHubClientSecret string `json:"githubClientSecret,omitempty"`
}

// Store reads and writes the settings document.
type Store struct {
	collection *mongo.Collection
	redis      *redis.Client
	log        *slog.Logger

	// current is swapped whole, so a reader never sees a half-updated set.
	current atomic.Pointer[snapshot]
}

type snapshot struct {
	values Values
	raw    map[string]json.RawMessage
}

// NewStore builds a Store. Load must be called before it is read.
func NewStore(db *mongo.Database, client *redis.Client, log *slog.Logger) *Store {
	s := &Store{collection: db.Collection("siteSettings"), redis: client, log: log}
	s.current.Store(&snapshot{})
	return s
}

// Load reads the settings document.
func (s *Store) Load(ctx context.Context) error {
	var doc struct {
		Values map[string]json.RawMessage `bson:"values"`
	}
	err := s.collection.FindOne(ctx, bson.M{"_id": documentID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		s.current.Store(&snapshot{raw: map[string]json.RawMessage{}})
		return nil
	}
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(doc.Values)
	if err != nil {
		return err
	}
	var values Values
	if err := json.Unmarshal(encoded, &values); err != nil {
		return err
	}
	s.current.Store(&snapshot{values: values, raw: doc.Values})
	return nil
}

// Watch keeps this process in step with the others until the context ends.
func (s *Store) Watch(ctx context.Context) {
	if s.redis == nil {
		return
	}
	sub := s.redis.Subscribe(ctx, changeChannel)
	go func() {
		defer func() { _ = sub.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub.Channel():
				if !ok {
					return
				}
				_ = msg
				reload, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := s.Load(reload); err != nil {
					s.log.Warn("could not reload the site settings", slog.Any("err", err))
				}
				cancel()
			}
		}
	}()
}

// Set writes settings and tells the other processes.
func (s *Store) Set(ctx context.Context, changes map[string]any, by string) error {
	update := bson.M{}
	for key, value := range changes {
		update["values."+key] = value
	}
	if len(update) == 0 {
		return nil
	}
	update["updatedBy"] = by
	_, err := s.collection.UpdateOne(ctx,
		bson.M{"_id": documentID},
		bson.M{"$set": update, "$currentDate": bson.M{"updatedAt": true}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return err
	}
	if err := s.Load(ctx); err != nil {
		return err
	}
	if s.redis != nil {
		_ = s.redis.Publish(ctx, changeChannel, strconv.FormatInt(time.Now().Unix(), 10)).Err()
	}
	return nil
}

// Values returns the settings as they stand.
func (s *Store) Values() Values { return s.current.Load().values }

// Raw returns every stored setting, including ones this service does not
// model. The admin API answers with this.
func (s *Store) Raw() map[string]json.RawMessage {
	raw := s.current.Load().raw
	out := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	return out
}

// --- the auth.Policy this store satisfies -----------------------------------

// RegistrationOpen says whether anybody may sign up. Unset means open: a site
// that has just been installed should be usable without first being told to
// allow the person installing it.
func (s *Store) RegistrationOpen() bool {
	v := s.Values().AllowPublicRegistration
	return v == nil || *v
}

// AllowedEmailDomains restricts sign-up when it is not empty.
func (s *Store) AllowedEmailDomains() []string {
	raw := strings.TrimSpace(s.Values().RegistrationEmailDomains)
	if raw == "" {
		return nil
	}
	var domains []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(entry), "@"))
		if entry != "" {
			domains = append(domains, entry)
		}
	}
	return domains
}

// MinPasswordLength is the shortest password accepted.
func (s *Store) MinPasswordLength() int { return intOr(s.Values().PasswordMinLength, 8) }

// MaxPasswordLength is the longest. bcrypt truncates past 72 bytes, so a
// larger value would be a promise the hash cannot keep.
func (s *Store) MaxPasswordLength() int {
	max := intOr(s.Values().PasswordMaxLength, 72)
	if max > 72 {
		return 72
	}
	return max
}

// EnabledProviders lists the identity providers an administrator configured. A
// provider with no client id is left out, which is what hides its button.
func (s *Store) EnabledProviders() []auth.Provider {
	values := s.Values()
	var providers []auth.Provider
	if values.GoogleClientID != "" && values.GoogleClientSecret != "" {
		providers = append(providers, auth.Provider{ID: "google", Name: "Google", Path: "/api/auth/google"})
	}
	if values.GitHubClientID != "" && values.GitHubClientSecret != "" {
		providers = append(providers, auth.Provider{ID: "github", Name: "GitHub", Path: "/api/auth/github"})
	}
	return providers
}

// intOr reads a number that may have been stored as a number or as the string
// a form sent.
func intOr(value any, fallback int) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

// --- the compile.Limits this store satisfies --------------------------------

// CompileTimeout is how long one compile may run, in seconds.
func (s *Store) CompileTimeout() int {
	timeout := intOr(s.Values().CompileTimeout, 180)
	if timeout <= 0 {
		return 180
	}
	return timeout
}

// DefaultImageName is the TeX Live image a project compiles in when it has not
// chosen one.
//
// The first of the images the administrator listed, because that list is
// written newest first and a project that has never been told otherwise should
// get the current TeX Live rather than whichever one happens to sort first. An
// empty answer is fine: it leaves the choice to the compiler's own default,
// which is what a deployment that never touched this setting wants.
func (s *Store) DefaultImageName() string {
	for _, image := range strings.Split(s.Values().TexLiveImages, ",") {
		if image = strings.TrimSpace(image); image != "" {
			return image
		}
	}
	return os.Getenv("TEX_LIVE_DOCKER_IMAGE")
}

// --- the gitbridge.Settings this store satisfies ----------------------------

// GitEnabled says whether projects can be cloned and pushed over git.
//
// Off unless somebody turned it on, because it needs a second container that a
// deployment only has if it asked for one: answering yes without it would mean
// a clone that hangs rather than one that is refused.
func (s *Store) GitEnabled() bool {
	enabled := s.Values().GitBridgeEnabled
	return enabled != nil && *enabled
}

// --- the githubsync.Settings this store satisfies ---------------------------

// GitHubSyncEnabled says whether projects can be kept in step with GitHub
// repositories.
func (s *Store) GitHubSyncEnabled() bool {
	enabled := s.Values().GitHubSyncEnabled
	return enabled != nil && *enabled
}

// GitHubSyncCredentials is the application an administrator registered with
// GitHub for this, and whether there is one.
//
// A different application from the one that signs people in: this one asks for
// access to repositories, which is not something to ask of somebody who only
// wants to sign in.
func (s *Store) GitHubSyncCredentials() (githubsync.Credentials, bool) {
	values := s.Values()
	if values.GitHubSyncClientID == "" || values.GitHubSyncClientSecret == "" {
		return githubsync.Credentials{}, false
	}
	site := strings.TrimRight(values.SiteURL, "/")
	if site == "" {
		// Nowhere for GitHub to send anybody back to. Better to be off than to
		// send people to a page that cannot exist.
		return githubsync.Credentials{}, false
	}
	return githubsync.Credentials{
		ClientID:     values.GitHubSyncClientID,
		ClientSecret: values.GitHubSyncClientSecret,
		// Built from the site URL rather than from the request, because it has
		// to match what was registered at GitHub exactly.
		CallbackURL: site + "/api/github/callback",
	}, true
}

// --- the oauth.Settings this store satisfies --------------------------------

// ProviderConfig is what one identity provider needs to be usable, and whether
// an administrator has set it up.
func (s *Store) ProviderConfig(providerID string) (oauth.Config, bool) {
	values := s.Values()
	var id, secret string
	switch providerID {
	case "google":
		id, secret = values.GoogleClientID, values.GoogleClientSecret
	case "github":
		id, secret = values.GitHubClientID, values.GitHubClientSecret
	default:
		return oauth.Config{}, false
	}
	if id == "" || secret == "" {
		return oauth.Config{}, false
	}
	return oauth.Config{
		ClientID:     id,
		ClientSecret: secret,
		// Built from the site URL rather than from the request, because it has
		// to match what was registered at the provider exactly, and a request
		// can arrive on an address nobody registered.
		RedirectURL: strings.TrimRight(values.SiteURL, "/") + "/api/auth/" + providerID + "/callback",
	}, true
}

// MayCreateAccounts says whether signing in with a provider may make an
// account for somebody the site has never seen.
func (s *Store) MayCreateAccounts() bool {
	raw, present := s.current.Load().raw["oauthCreatesAccounts"]
	if !present {
		return true
	}
	var allowed bool
	if err := json.Unmarshal(raw, &allowed); err != nil {
		return true
	}
	return allowed
}

// SiteURL is where this site lives.
func (s *Store) SiteURL() string { return strings.TrimRight(s.Values().SiteURL, "/") }

// SMTP is the mail server, as configured.
func (s *Store) SMTP() mailer.Config {
	values := s.Values()
	return mailer.Config{
		Host:      strings.TrimSpace(values.EmailSmtpHost),
		Port:      intOr(values.EmailSmtpPort, 587),
		User:      values.EmailSmtpUser,
		Password:  values.EmailSmtpPass,
		Secure:    values.EmailSmtpSecure != nil && *values.EmailSmtpSecure,
		IgnoreTLS: values.EmailSmtpIgnoreTLS != nil && *values.EmailSmtpIgnoreTLS,
		From:      strings.TrimSpace(values.EmailFromAddress),
		ReplyTo:   strings.TrimSpace(values.EmailReplyTo),
	}
}

// AppName is what this instance calls itself.
func (s *Store) AppName() string {
	if name := strings.TrimSpace(s.Values().AppName); name != "" {
		return name
	}
	return "Overleaf"
}
