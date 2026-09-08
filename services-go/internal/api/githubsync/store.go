package githubsync

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// What is remembered between syncs.
//
// Two things: the token somebody's account was connected with, and where each
// project last agreed with its repository. The first is a secret and is
// encrypted; the second is not, and is the whole of what makes a sync able to
// tell a change here from a change there.

// State is where a project and its repository last agreed.
type State struct {
	ProjectID    bson.ObjectID `bson:"projectId" json:"-"`
	RepoFullName string        `bson:"repoFullName" json:"repoFullName"`
	// DefaultBranchName is the branch a sync pushes to and pulls from.
	DefaultBranchName string `bson:"defaultBranchName,omitempty" json:"defaultBranchName,omitempty"`
	// MergeStatus is clean, conflict or diverged.
	//
	// clean: the two agreed at the last sync and can be compared from there.
	// conflict: a merge needs a person, and until one happens the branch below
	// holds what could not be merged.
	// diverged: the repository was rewritten and the commit this project was
	// anchored to is no longer on the branch.
	MergeStatus string `bson:"mergeStatus" json:"mergeStatus"`
	// LastSyncCommit and LastSyncVersion are the two sides of the last
	// agreement. Everything a sync does is worked out from them.
	LastSyncCommit  string `bson:"lastSyncCommit,omitempty" json:"lastSyncCommit,omitempty"`
	LastSyncVersion int    `bson:"lastSyncVersion,omitempty" json:"lastSyncVersion,omitempty"`
	// UnmergedBranchName is where the work that could not be merged is
	// waiting, for somebody to merge it there.
	UnmergedBranchName string `bson:"unmergedBranchName,omitempty" json:"unmergedBranchName,omitempty"`
	UnmergedBranchHead string `bson:"unmergedBranchHead,omitempty" json:"unmergedBranchHead,omitempty"`
	// ConflictVersion is where the project was when the conflict happened, so
	// that changes made while it was unresolved can still be found.
	ConflictVersion int `bson:"conflictVersion,omitempty" json:"-"`
}

// The three states a project can be in.
const (
	StatusClean    = "clean"
	StatusConflict = "conflict"
	StatusDiverged = "diverged"
)

// Store holds both collections.
type Store struct {
	states      *mongo.Collection
	credentials *mongo.Collection
	// key encrypts the tokens. Derived from the deployment's own secret, so
	// there is nothing extra to configure and nothing extra to lose.
	key []byte
}

// NewStore builds one.
func NewStore(db *mongo.Database, secret string) *Store {
	key := sha256.Sum256([]byte("github-sync:" + secret))
	return &Store{
		states:      db.Collection("githubSyncProjectStates"),
		credentials: db.Collection("githubSyncUserCredentials"),
		key:         key[:],
	}
}

// EnsureIndexes creates the two uniqueness rules this depends on: one state
// per project, one token per person.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	for _, pair := range []struct {
		collection *mongo.Collection
		field      string
	}{
		{s.states, "projectId"},
		{s.credentials, "userId"},
	} {
		_, err := pair.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: pair.field, Value: 1}},
			Options: options.Index().SetUnique(true).SetName(pair.field + "_1"),
		})
		if err != nil {
			var cmdErr mongo.CommandError
			if errors.As(err, &cmdErr) && (cmdErr.Code == 85 || cmdErr.Code == 86) {
				continue
			}
			return err
		}
	}
	return nil
}

// --- tokens ----------------------------------------------------------------

// SaveToken keeps somebody's GitHub token.
func (s *Store) SaveToken(ctx context.Context, userID bson.ObjectID, token string) error {
	sealed, err := s.seal(token)
	if err != nil {
		return err
	}
	_, err = s.credentials.UpdateOne(ctx,
		bson.M{"userId": userID},
		bson.M{"$set": bson.M{"github": sealed, "updatedAt": time.Now().UTC()}},
		options.UpdateOne().SetUpsert(true))
	return err
}

// Token reads it back.
func (s *Store) Token(ctx context.Context, userID bson.ObjectID) (string, error) {
	var stored struct {
		GitHub string `bson:"github"`
	}
	err := s.credentials.FindOne(ctx, bson.M{"userId": userID}).Decode(&stored)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", ErrNoToken
	}
	if err != nil {
		return "", err
	}
	token, err := s.open(stored.GitHub)
	if err != nil {
		// Stored under a secret this process does not have -- an older
		// deployment's, or one that was rotated. Treated as not connected, so
		// the way out is to connect again rather than to be stuck.
		return "", ErrNoToken
	}
	return token, nil
}

// ForgetToken removes it.
func (s *Store) ForgetToken(ctx context.Context, userID bson.ObjectID) error {
	_, err := s.credentials.DeleteOne(ctx, bson.M{"userId": userID})
	return err
}

// --- project state ---------------------------------------------------------

// State reads where a project last agreed with its repository, or nil when it
// is not linked to one.
func (s *Store) State(ctx context.Context, projectID bson.ObjectID) (*State, error) {
	var state State
	err := s.states.FindOne(ctx, bson.M{"projectId": projectID}).Decode(&state)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveState writes it.
func (s *Store) SaveState(ctx context.Context, state *State) error {
	_, err := s.states.UpdateOne(ctx,
		bson.M{"projectId": state.ProjectID},
		bson.M{"$set": state},
		options.UpdateOne().SetUpsert(true))
	return err
}

// UpdateState changes some of it.
func (s *Store) UpdateState(ctx context.Context, projectID bson.ObjectID, changes bson.M) error {
	_, err := s.states.UpdateOne(ctx,
		bson.M{"projectId": projectID}, bson.M{"$set": changes})
	return err
}

// ForgetState unlinks a project from its repository. Nothing is deleted on
// either side: the project keeps its files and the repository keeps its
// commits, and they simply stop being told about each other.
func (s *Store) ForgetState(ctx context.Context, projectID bson.ObjectID) error {
	_, err := s.states.DeleteMany(ctx, bson.M{"projectId": projectID})
	return err
}

// --- keeping a token secret -------------------------------------------------

// prefix marks a value this process wrote, so one written by something else is
// recognisable as unreadable rather than decoded into nonsense.
const prefix = "gsv1."

func (s *Store) seal(token string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	sealed, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, sealed.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := sealed.Seal(nonce, nonce, []byte(token), nil)
	return prefix + base64.StdEncoding.EncodeToString(out), nil
}

func (s *Store) open(stored string) (string, error) {
	if len(stored) <= len(prefix) || stored[:len(prefix)] != prefix {
		return "", errors.New("not a token this process wrote")
	}
	raw, err := base64.StdEncoding.DecodeString(stored[len(prefix):])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	sealed, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < sealed.NonceSize() {
		return "", errors.New("the stored token is too short to be one")
	}
	nonce, body := raw[:sealed.NonceSize()], raw[sealed.NonceSize():]
	opened, err := sealed.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(opened), nil
}
