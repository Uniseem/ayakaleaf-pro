// Package users owns the user document: reading it, creating it, and checking
// a password against it.
//
// Nothing above this package touches the users collection directly. That is
// what keeps "how a user is stored" a decision this package can change, and it
// is the first place the shape of the rewrite differs from what it replaces --
// in the Node service the same document is read from thirty places.
package users

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// Cost is bcrypt's work factor. It matches what the Node service used, so a
// password hashed by either is readable by both for as long as both exist.
const Cost = 12

var (
	// ErrNotFound is returned when no user matches.
	ErrNotFound = errors.New("no such user")
	// ErrEmailTaken is returned when an address already belongs to somebody.
	ErrEmailTaken = errors.New("that email address is already registered")
	// ErrWrongPassword is returned when a password does not match the stored
	// hash. It is deliberately indistinguishable from ErrNotFound to callers
	// that sign people in.
	ErrWrongPassword = errors.New("wrong password")
)

// Email is one address on an account.
type Email struct {
	Email          string     `bson:"email" json:"email"`
	CreatedAt      time.Time  `bson:"createdAt,omitempty" json:"createdAt,omitempty"`
	ConfirmedAt    *time.Time `bson:"confirmedAt,omitempty" json:"confirmedAt,omitempty"`
	ReversedHostname string   `bson:"reversedHostname,omitempty" json:"-"`
}

// ThirdPartyIdentifier links an account to an identity at Google, GitHub or
// another provider.
type ThirdPartyIdentifier struct {
	ProviderID     string `bson:"providerId" json:"providerId"`
	ExternalUserID string `bson:"externalUserId" json:"externalUserId"`
	ExternalData   any    `bson:"externalData,omitempty" json:"externalData,omitempty"`
}

// User is an account.
type User struct {
	ID             bson.ObjectID `bson:"_id" json:"_id"`
	Email          string        `bson:"email" json:"email"`
	Emails         []Email       `bson:"emails,omitempty" json:"emails,omitempty"`
	FirstName      string        `bson:"first_name,omitempty" json:"first_name,omitempty"`
	LastName       string        `bson:"last_name,omitempty" json:"last_name,omitempty"`
	HashedPassword string        `bson:"hashedPassword,omitempty" json:"-"`
	IsAdmin        bool          `bson:"isAdmin,omitempty" json:"isAdmin,omitempty"`
	SignUpDate     time.Time     `bson:"signUpDate,omitempty" json:"signUpDate,omitempty"`
	LastLoggedIn   *time.Time    `bson:"lastLoggedIn,omitempty" json:"lastLoggedIn,omitempty"`
	LastLoginIP    string        `bson:"lastLoginIp,omitempty" json:"-"`
	LoginCount     int           `bson:"loginCount,omitempty" json:"-"`
	AnalyticsID    string        `bson:"analyticsId,omitempty" json:"-"`

	ThirdPartyIdentifiers []ThirdPartyIdentifier `bson:"thirdPartyIdentifiers,omitempty" json:"thirdPartyIdentifiers,omitempty"`

	Features map[string]any `bson:"features,omitempty" json:"features,omitempty"`
	Refered  bool           `bson:"refered,omitempty" json:"-"`
}

// DisplayName is what to call somebody in the interface.
func (u *User) DisplayName() string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name != "" {
		return name
	}
	return u.Email
}

// Store is the users collection.
type Store struct {
	users *mongo.Collection
}

// NewStore builds a Store over a database.
func NewStore(db *mongo.Database) *Store {
	return &Store{users: db.Collection("users")}
}

// EnsureIndexes creates what this package relies on being unique or fast.
//
// The email index is unique because two accounts with one address is a
// corruption nothing above can recover from, and a constraint in the database
// is the only place that holds under two writers.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.users.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("email_1"),
		},
		{
			Keys:    bson.D{{Key: "emails.email", Value: 1}},
			Options: options.Index().SetName("emails.email_1"),
		},
		{
			Keys: bson.D{
				{Key: "thirdPartyIdentifiers.providerId", Value: 1},
				{Key: "thirdPartyIdentifiers.externalUserId", Value: 1},
			},
			Options: options.Index().SetName("thirdPartyIdentifiers_1"),
		},
	})
	// An index that already exists with different options is not worth failing
	// startup over; the collection predates this service.
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		var cmdErr mongo.CommandError
		if errors.As(err, &cmdErr) && (cmdErr.Code == 85 || cmdErr.Code == 86) {
			return nil
		}
		return err
	}
	return nil
}

// ByID reads one user.
func (s *Store) ByID(ctx context.Context, id bson.ObjectID) (*User, error) {
	var user User
	err := s.users.FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// ByEmail reads one user by their primary address.
func (s *Store) ByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := s.users.FindOne(ctx, bson.M{"email": Normalise(email)}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// ByAnyEmail reads one user by any address they hold, primary or secondary.
func (s *Store) ByAnyEmail(ctx context.Context, email string) (*User, error) {
	normalised := Normalise(email)
	var user User
	err := s.users.FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"email": normalised},
			{"emails.email": normalised},
		},
	}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// ByThirdPartyIdentity reads the user holding an external identity.
func (s *Store) ByThirdPartyIdentity(ctx context.Context, providerID, externalUserID string) (*User, error) {
	var user User
	err := s.users.FindOne(ctx, bson.M{
		"thirdPartyIdentifiers": bson.M{"$elemMatch": bson.M{
			"providerId":     providerID,
			"externalUserId": externalUserID,
		}},
	}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// NewUser is what Create needs to make an account.
type NewUser struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
	// Confirmed marks the address as already verified, which is what an
	// identity provider vouching for it means.
	Confirmed bool
}

// Create makes an account.
//
// The unique index does the deciding, not a read followed by a write: two
// people registering the same address in the same moment both pass a check and
// only one passes the insert.
func (s *Store) Create(ctx context.Context, in NewUser) (*User, error) {
	email := Normalise(in.Email)
	now := time.Now().UTC()

	user := &User{
		ID:         bson.NewObjectID(),
		Email:      email,
		FirstName:  in.FirstName,
		LastName:   in.LastName,
		SignUpDate: now,
		Emails: []Email{{
			Email:            email,
			CreatedAt:        now,
			ReversedHostname: reverseHostname(email),
		}},
	}
	if in.Confirmed {
		user.Emails[0].ConfirmedAt = &now
	}
	if in.Password != "" {
		hash, err := Hash(in.Password)
		if err != nil {
			return nil, err
		}
		user.HashedPassword = hash
	}

	if _, err := s.users.InsertOne(ctx, user); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return user, nil
}

// Authenticate checks a password and returns the user it belongs to.
//
// A missing user still costs a bcrypt comparison, so that the time taken does
// not say whether an address is registered.
func (s *Store) Authenticate(ctx context.Context, email, password string) (*User, error) {
	user, err := s.ByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, ErrWrongPassword
	}
	if err != nil {
		return nil, err
	}
	if user.HashedPassword == "" {
		// An account that signs in with an identity provider only.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, ErrWrongPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(password)); err != nil {
		return nil, ErrWrongPassword
	}
	return user, nil
}

// SetPassword replaces somebody's password.
func (s *Store) SetPassword(ctx context.Context, id bson.ObjectID, password string) error {
	hash, err := Hash(password)
	if err != nil {
		return err
	}
	_, err = s.users.UpdateByID(ctx, id, bson.M{"$set": bson.M{"hashedPassword": hash}})
	return err
}

// RecordLogin notes that somebody signed in.
func (s *Store) RecordLogin(ctx context.Context, id bson.ObjectID, ip string) error {
	now := time.Now().UTC()
	_, err := s.users.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"lastLoggedIn": now, "lastLoginIp": ip},
		"$inc": bson.M{"loginCount": 1},
	})
	return err
}

// AdminExists says whether the site has an administrator yet.
func (s *Store) AdminExists(ctx context.Context) (bool, error) {
	err := s.users.FindOne(ctx, bson.M{"isAdmin": true},
		options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// adminClaimID is the document whose existence means the first-administrator
// place is taken.
const adminClaimID = "first-admin-claim"

// ClaimAdminSlot takes the first-administrator place for a user, and says
// whether it got it.
//
// Two people registering in the same second must not both become the
// administrator, and no query over the users collection settles that: both
// read no administrator, and both then write one. An insert of a single
// document with a fixed id is decided by the database -- one succeeds, the
// other gets a duplicate key.
func (s *Store) ClaimAdminSlot(ctx context.Context, id bson.ObjectID) (bool, error) {
	claims := s.users.Database().Collection("siteSettings")
	_, err := claims.InsertOne(ctx, bson.M{
		"_id":       adminClaimID,
		"userId":    id.Hex(),
		"claimedAt": time.Now().UTC(),
	})
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MakeAdmin promotes an account.
func (s *Store) MakeAdmin(ctx context.Context, id bson.ObjectID) error {
	_, err := s.users.UpdateByID(ctx, id, bson.M{"$set": bson.M{"isAdmin": true}})
	return err
}

// LinkIdentity attaches an external identity to an account.
func (s *Store) LinkIdentity(ctx context.Context, id bson.ObjectID, identity ThirdPartyIdentifier) error {
	_, err := s.users.UpdateOne(ctx,
		bson.M{
			"_id": id,
			"thirdPartyIdentifiers": bson.M{"$not": bson.M{"$elemMatch": bson.M{
				"providerId": identity.ProviderID,
			}}},
		},
		bson.M{"$push": bson.M{"thirdPartyIdentifiers": identity}},
	)
	return err
}

// UnlinkIdentity detaches an external identity.
func (s *Store) UnlinkIdentity(ctx context.Context, id bson.ObjectID, providerID string) error {
	_, err := s.users.UpdateByID(ctx, id, bson.M{
		"$pull": bson.M{"thirdPartyIdentifiers": bson.M{"providerId": providerID}},
	})
	return err
}

// ConfirmEmail marks an address as verified.
func (s *Store) ConfirmEmail(ctx context.Context, id bson.ObjectID, email string) error {
	now := time.Now().UTC()
	_, err := s.users.UpdateOne(ctx,
		bson.M{"_id": id, "emails.email": Normalise(email)},
		bson.M{"$set": bson.M{"emails.$.confirmedAt": now}},
	)
	return err
}

// Hash produces a stored password.
func Hash(password string) (string, error) {
	// bcrypt truncates at 72 bytes, and silently: a longer password would have
	// its tail ignored, so it is refused rather than half honoured.
	if len(password) > 72 {
		return "", errors.New("password is too long")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), Cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Normalise puts an address in the form it is stored and compared in.
func Normalise(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// reverseHostname is the reversed domain, which is what a "who is at this
// institution" query is indexed on.
func reverseHostname(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	host := email[at+1:]
	runes := []rune(host)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// dummyHash is a real bcrypt hash of a value nobody uses, compared against so
// that a sign-in attempt for an address that does not exist takes as long as
// one for an address that does.
const dummyHash = "$2a$12$k4Xn3z1YQ9m5s8Q0nGxq3eKcQ0m0Y7bJ8hS0oZ9r5fV2u1wW3tK2i"
