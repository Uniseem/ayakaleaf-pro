package settings

import (
	"net/http"
	"sort"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/apierr"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/httpapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// The personal dictionary: the words somebody has told the spell checker to
// stop underlining.
//
// Kept on the user rather than on the project, because it is a fact about the
// person's vocabulary -- their own name, the terms of their field -- and not
// about any one document.

// maxLearnedWord is a limit on one word, not on the list.
//
// Hunspell is given these words as a dictionary, and a "word" of unbounded
// length is a way to make it do unbounded work on every spell check.
const maxLearnedWord = 100

// maxLearnedWords caps the list, for the same reason.
const maxLearnedWords = 10000

type learnRequest struct {
	Word string `json:"word"`
}

// LearnedWords answers with this person's dictionary.
func (s *UserService) LearnedWords(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}
	words, err := s.readLearnedWords(r, user.ID)
	if err != nil {
		return err
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"words": words})
}

// Learn adds a word to it.
func (s *UserService) Learn(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}

	var in learnRequest
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	word := strings.TrimSpace(in.Word)
	if word == "" || len([]rune(word)) > maxLearnedWord {
		return apierr.BadRequest.WithField("word").
			WithMessage("That is not a word this dictionary can hold.")
	}

	words, err := s.readLearnedWords(r, user.ID)
	if err != nil {
		return err
	}
	for _, held := range words {
		if held == word {
			// Already known. Answering with the list rather than an error: the
			// caller wants the word in the dictionary, and it is.
			return httpapi.JSON(w, http.StatusOK, map[string]any{"words": words})
		}
	}
	if len(words) >= maxLearnedWords {
		return apierr.BadRequest.WithField("word").
			WithMessage("This dictionary is full.")
	}

	if _, err := s.users.UpdateByID(r.Context(), user.ID,
		bson.M{"$addToSet": bson.M{"learnedWords": word}}); err != nil {
		return apierr.Internal.WithCause(err)
	}

	words = append(words, word)
	sort.Strings(words)
	return httpapi.JSON(w, http.StatusOK, map[string]any{"words": words})
}

// Unlearn takes a word out again.
func (s *UserService) Unlearn(w http.ResponseWriter, r *http.Request) error {
	user, err := httpapi.RequireUser(r.Context())
	if err != nil {
		return err
	}

	var in learnRequest
	if err := httpapi.Decode(r, &in); err != nil {
		return err
	}
	word := strings.TrimSpace(in.Word)
	if word == "" {
		return apierr.BadRequest.WithField("word").
			WithMessage("That is not a word this dictionary can hold.")
	}

	if _, err := s.users.UpdateByID(r.Context(), user.ID,
		bson.M{"$pull": bson.M{"learnedWords": word}}); err != nil {
		return apierr.Internal.WithCause(err)
	}

	words, err := s.readLearnedWords(r, user.ID)
	if err != nil {
		return err
	}
	return httpapi.JSON(w, http.StatusOK, map[string]any{"words": words})
}

func (s *UserService) readLearnedWords(r *http.Request, id bson.ObjectID) ([]string, error) {
	var held struct {
		LearnedWords []string `bson:"learnedWords"`
	}
	if err := s.users.FindOne(r.Context(), bson.M{"_id": id}).Decode(&held); err != nil {
		return nil, apierr.Internal.WithCause(err)
	}
	if held.LearnedWords == nil {
		// An empty list rather than null: the client puts this straight into a
		// Set, and a null would have to be guarded at every use.
		return []string{}, nil
	}
	sort.Strings(held.LearnedWords)
	return held.LearnedWords, nil
}
