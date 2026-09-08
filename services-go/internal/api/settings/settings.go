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
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/auth"
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

	PasswordMinLength any `json:"passwordMinLength,omitempty"`
	PasswordMaxLength any `json:"passwordMaxLength,omitempty"`

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
