package settings

import (
	"context"
	"net/http"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// A person's own settings: how they like the editor, not how the site is run.

// UserService answers the per-person settings endpoints.
type UserService struct {
	users *mongo.Collection
}

// NewUserService builds it.
func NewUserService(db *mongo.Database) *UserService {
	return &UserService{users: db.Collection("users")}
}

// allowed is every setting a person may change, and what a valid value is.
//
// An allowlist rather than "write whatever arrives": these are merged into the
// user document, and without this an unknown key from a client would let
// anybody set isAdmin on themselves.
var allowed = map[string]func(any) bool{
	"mode":               oneOf("code", "visual"),
	"overallTheme":       oneOf("light", "dark"),
	"editorTheme":        isShortString,
	"fontSize":           inRange(8, 48),
	"fontFamily":         oneOf("monospace", "lucida", "opendyslexic"),
	"lineHeight":         oneOf("compact", "normal", "wide"),
	"keybindings":        oneOf("default", "vim", "emacs"),
	"autoComplete":       isBool,
	"autoPairDelimiters": isBool,
	"syntaxValidation":   isBool,
	"spellCheckLanguage": isShortString,
	"mathPreview":        isBool,
	"pdfViewer":          oneOf("pdfjs", "native"),
	"darkModePdf":        isBool,
	"showOutline":        isBool,
	"breadcrumbs":        isBool,
	"nonBlinkingCursor":  isBool,
}

// Get answers with this person's settings.
func (s *UserService) Get(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	stored, err := s.read(r.Context(), user.ID)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, stored)
}

// Set writes the settings that were sent, and only those.
//
// A partial update: two tabs open on one account would otherwise overwrite
// each other's unrelated changes with whichever saved last.
func (s *UserService) Set(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}

	var in map[string]any
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}

	changes := bson.M{}
	for key, value := range in {
		valid, known := allowed[key]
		if !known {
			return apierr.BadRequest.WithField(key).
				WithMessage("That is not a setting.")
		}
		if !valid(value) {
			return apierr.BadRequest.WithField(key).
				WithMessage("That is not a value this setting can take.")
		}
		changes["settings."+key] = value
	}

	if len(changes) > 0 {
		if _, err := s.users.UpdateByID(r.Context(), user.ID,
			bson.M{"$set": changes}); err != nil {
			return apierr.Internal.WithCause(err)
		}
	}

	stored, err := s.read(r.Context(), user.ID)
	if err != nil {
		return apierr.Internal.WithCause(err)
	}
	return httpapi.JSON(w, http.StatusOK, stored)
}

// read pulls the settings sub-document, which may not be there yet.
func (s *UserService) read(ctx context.Context, id bson.ObjectID) (map[string]any, error) {
	var held struct {
		Settings map[string]any `bson:"settings"`
	}
	err := s.users.FindOne(ctx, bson.M{"_id": id}).Decode(&held)
	if err != nil {
		return nil, err
	}
	if held.Settings == nil {
		// An account that has never changed anything has no settings document,
		// which is not an error and not an empty answer either -- the client
		// fills in its own defaults for whatever is absent.
		return map[string]any{}, nil
	}
	return held.Settings, nil
}

func oneOf(values ...string) func(any) bool {
	return func(value any) bool {
		text, ok := value.(string)
		if !ok {
			return false
		}
		for _, each := range values {
			if each == text {
				return true
			}
		}
		return false
	}
}

func isBool(value any) bool {
	_, ok := value.(bool)
	return ok
}

func isShortString(value any) bool {
	text, ok := value.(string)
	return ok && len(text) <= 64 && strings.TrimSpace(text) != ""
}

func inRange(low, high float64) func(any) bool {
	return func(value any) bool {
		// JSON numbers decode as float64 whatever they looked like.
		number, ok := value.(float64)
		return ok && number >= low && number <= high
	}
}
