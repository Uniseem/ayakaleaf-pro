// Package tokens holds the personal access tokens somebody uses instead of a
// password.
//
// One thing needs them today -- git, where the password prompt has nowhere to
// put a second factor and no way to show a consent screen. A token is better
// than the account password anyway: it can be listed, it can be revoked on its
// own, and it says what it is for.
//
// The stored shape is the one the service this replaces used, so tokens made
// before the rewrite still work and are still listed.
package tokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	// ErrNotFound is returned for a token that is not there, or not this
	// person's.
	ErrNotFound = errors.New("no such token")
	// ErrExpired is returned for a token that was real and is past its date.
	// Told apart from a token that never existed because the two mean
	// different things to the person holding one.
	ErrExpired = errors.New("token expired")
	// ErrTooMany is returned when somebody already holds as many as they may.
	ErrTooMany = errors.New("too many tokens")
)

const (
	// prefix marks a string as one of ours, so a token pasted somewhere it
	// does not belong is recognisable.
	prefix = "olp_"
	// length is the random part, matching what was issued before.
	length = 36
	// scope is what these tokens are for. One scope exists; the field is kept
	// because the tokens are stored alongside OAuth ones that have others.
	scope = "git_bridge"
	// kind separates these from tokens issued by an OAuth flow.
	kind = "personal_access_token"
	// maximum is how many one person may hold at once. Not a security limit --
	// somebody who wants more can revoke one -- but a list that cannot grow
	// without bound.
	maximum = 10
	// life is how long a token lasts before it has to be made again.
	life = 365 * 24 * time.Hour
)

// alphabet is what a token is made of: letters and digits, so it survives
// being typed into a password prompt and pasted out of a terminal.
const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Token is one token as it is stored. The secret itself is not here: only the
// hash of it, so this collection leaking does not hand anybody an account.
type Token struct {
	ID        bson.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    bson.ObjectID `bson:"user_id" json:"-"`
	Hash      string        `bson:"accessToken" json:"-"`
	Partial   string        `bson:"accessTokenPartial" json:"partial"`
	Scope     string        `bson:"scope" json:"scope"`
	Kind      string        `bson:"type" json:"-"`
	ExpiresAt time.Time     `bson:"accessTokenExpiresAt" json:"expiresAt"`
	CreatedAt time.Time     `bson:"createdAt" json:"createdAt"`
	LastUsed  *time.Time    `bson:"lastUsedAt,omitempty" json:"lastUsedAt,omitempty"`
	// Application is the OAuth application row these belong to. Nothing here
	// reads it; it is kept so the collection stays one shape.
	Application *bson.ObjectID `bson:"oauthApplication_id,omitempty" json:"-"`
}

// Store is the tokens collection.
type Store struct {
	tokens       *mongo.Collection
	applications *mongo.Collection
}

// NewStore builds one.
func NewStore(db *mongo.Database) *Store {
	return &Store{
		tokens:       db.Collection("oauthAccessTokens"),
		applications: db.Collection("oauthApplications"),
	}
}

// EnsureIndexes creates what verifying a token on every git request relies on.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.tokens.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "accessToken", Value: 1}},
			Options: options.Index().SetName("accessToken_1"),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetName("user_id_1"),
		},
	})
	if err != nil {
		var cmdErr mongo.CommandError
		if errors.As(err, &cmdErr) && (cmdErr.Code == 85 || cmdErr.Code == 86) {
			return nil
		}
		return err
	}
	return nil
}

// List is somebody's tokens, newest first.
func (s *Store) List(ctx context.Context, userID bson.ObjectID) ([]Token, error) {
	cursor, err := s.tokens.Find(ctx,
		bson.M{"user_id": userID, "type": kind},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	list := []Token{}
	for cursor.Next(ctx) {
		var token Token
		if err := cursor.Decode(&token); err != nil {
			return nil, err
		}
		list = append(list, token)
	}
	return list, cursor.Err()
}

// Create makes a token and returns it once.
//
// Once, because only the hash is kept: somebody who did not write it down has
// to make another, which is the property that makes a leaked list of tokens
// worth nothing.
func (s *Store) Create(ctx context.Context, userID bson.ObjectID) (string, *Token, error) {
	held, err := s.tokens.CountDocuments(ctx, bson.M{"user_id": userID, "type": kind})
	if err != nil {
		return "", nil, err
	}
	if held >= maximum {
		return "", nil, ErrTooMany
	}

	secret, err := randomString(length)
	if err != nil {
		return "", nil, err
	}
	plain := prefix + secret

	now := time.Now().UTC()
	token := &Token{
		ID:        bson.NewObjectID(),
		UserID:    userID,
		Hash:      hashOf(plain),
		Partial:   plain[:8],
		Scope:     scope,
		Kind:      kind,
		ExpiresAt: now.Add(life),
		CreatedAt: now,
	}
	if app, err := s.application(ctx); err == nil && app != nil {
		token.Application = app
	}
	if _, err := s.tokens.InsertOne(ctx, token); err != nil {
		return "", nil, err
	}
	return plain, token, nil
}

// Delete revokes one of somebody's tokens.
func (s *Store) Delete(ctx context.Context, id, userID bson.ObjectID) error {
	result, err := s.tokens.DeleteOne(ctx,
		bson.M{"_id": id, "user_id": userID, "type": kind})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Verify reads the token a request presented.
//
// The lookup is by hash, so a token that is close to a real one is not close
// to anything stored. The comparison after it is constant time for the same
// reason a password check is.
func (s *Store) Verify(ctx context.Context, plain string) (*Token, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return nil, ErrNotFound
	}
	hashed := hashOf(plain)

	var token Token
	err := s.tokens.FindOne(ctx, bson.M{"accessToken": hashed}).Decode(&token)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(token.Hash), []byte(hashed)) != 1 {
		return nil, ErrNotFound
	}
	if !token.ExpiresAt.IsZero() && token.ExpiresAt.Before(time.Now()) {
		return nil, ErrExpired
	}

	// When it was last used is the only thing that makes a list of tokens
	// useful for deciding which to revoke, and it is written on the way past
	// rather than being made to matter to this answer.
	now := time.Now().UTC()
	_, _ = s.tokens.UpdateByID(ctx, token.ID, bson.M{"$set": bson.M{"lastUsedAt": now}})
	token.LastUsed = &now
	return &token, nil
}

// application is the OAuth application row tokens are filed under, made if it
// is not there. Nothing in this service reads it; it exists so the collection
// keeps one shape.
func (s *Store) application(ctx context.Context) (*bson.ObjectID, error) {
	const name = "Overleaf Git Bridge"
	var existing struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err := s.applications.FindOne(ctx, bson.M{"name": name}).Decode(&existing)
	if err == nil {
		return &existing.ID, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}

	id, err := randomString(64)
	if err != nil {
		return nil, err
	}
	secret, err := randomString(32)
	if err != nil {
		return nil, err
	}
	created := bson.NewObjectID()
	_, err = s.applications.InsertOne(ctx, bson.M{
		"_id":          created,
		"id":           strings.ToLower(id),
		"clientSecret": hashOf(secret),
		"grants":       []string{"password"},
		"name":         name,
		"redirectUris": []string{},
		"scopes":       []string{scope},
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func hashOf(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func randomString(n int) (string, error) {
	limit := big.NewInt(int64(len(alphabet)))
	out := make([]byte, n)
	for i := range out {
		pick, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[pick.Int64()]
	}
	return string(out), nil
}
