package docstore

import (
	"errors"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Doc is a document in the `docs` collection.
//
// Pointers mark the fields whose presence matters: a projection can leave any
// of them out, and the response view includes an attribute only when it is
// present, so "absent" and "zero" have to stay distinguishable.
type Doc struct {
	ID        bson.ObjectID  `bson:"_id"`
	ProjectID *bson.ObjectID `bson:"project_id,omitempty"`
	Lines     []string       `bson:"lines,omitempty"`
	Ranges    bson.D         `bson:"ranges,omitempty"`
	Rev       *int64         `bson:"rev,omitempty"`
	Version   *int64         `bson:"version,omitempty"`
	Deleted   *bool          `bson:"deleted,omitempty"`
	DeletedAt *time.Time     `bson:"deletedAt,omitempty"`
	Name      *string        `bson:"name,omitempty"`
	InS3      *bool          `bson:"inS3,omitempty"`
}

// IsDeleted reports whether the doc is soft-deleted. `deleted` is absent
// rather than false on live docs.
func (d *Doc) IsDeleted() bool { return d.Deleted != nil && *d.Deleted }

// IsArchived reports whether the doc's contents live in the persistor.
func (d *Doc) IsArchived() bool { return d.InS3 != nil && *d.InS3 }

// RevValue returns the doc's rev, or 0 when the doc is absent or rev was not
// projected.
func (d *Doc) RevValue() int64 {
	if d == nil || d.Rev == nil {
		return 0
	}
	return *d.Rev
}

// Errors mirroring services/docstore/app/js/Errors.js. The HTTP layer maps
// them to status codes exactly as the Node error handler does.
var (
	// ErrNotFound becomes a 404.
	ErrNotFound = errors.New("doc not found")
	// ErrDocModified becomes a 409.
	ErrDocModified = errors.New("doc rev has changed")
	// ErrDocVersionDecremented becomes a 409.
	ErrDocVersionDecremented = errors.New("doc version decremented")
	// ErrDocRevValue signals a lost race on the rev; updateDoc retries once
	// and unarchiving turns it into a 409.
	ErrDocRevValue = errors.New("doc rev value error")
	// ErrDocWithoutLines is raised by the raw-doc endpoint.
	ErrDocWithoutLines = errors.New("doc has no lines")
	// ErrMd5Mismatch guards archived downloads.
	ErrMd5Mismatch = errors.New("md5 mismatch when downloading doc")
)

func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }

// jsNumber renders an integer the way the Node driver serialises a JavaScript
// number: int32 when it fits, a double otherwise. Writing int64 instead works
// -- Mongo compares numerically across types -- but it leaves rev and version
// stored as Long where the Node service stores Number, and the two
// implementations are meant to be able to share one database without leaving a
// trace of which wrote a given document.
func jsNumber(v int64) any {
	if v >= math.MinInt32 && v <= math.MaxInt32 {
		return int32(v)
	}
	return float64(v)
}
