package docstore

import (
	"encoding/json"
	"testing"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/bsonjson"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func decode(t *testing.T, s string) bson.D {
	t.Helper()
	doc, err := bsonjson.DecodeDocument([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func encode(t *testing.T, v any) string {
	t.Helper()
	out, err := json.Marshal(bsonjson.Document(v.(bson.D)))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// The acceptance suite sends "meta", while the transformation looks for
// "metadata". The round trip is therefore the identity, which is exactly why
// `doc.ranges.should.deep.equal(originalRanges)` holds in UpdatingDocsTests.
func TestRangesRoundTripLeavesMetaAlone(t *testing.T) {
	const in = `{"changes":[{"id":"507f1f77bcf86cd799439011","op":{"i":"$foo","p":3},` +
		`"meta":{"user_id":"507f191e810c19729de860ea","ts":"Sun Sep 07 2026 16:00:00 GMT+0800 (CST)"}}]}`
	ranges := JSONRangesToMongo(decode(t, in))
	if got := encode(t, ranges); got != in {
		t.Errorf("round trip:\n got %s\nwant %s", got, in)
	}
}

// With the spelling the production code actually converts, ts becomes a date
// and user_id an ObjectId -- and both still render back to strings.
func TestRangesConvertMetadata(t *testing.T) {
	const in = `{"changes":[{"id":"507f1f77bcf86cd799439011",` +
		`"metadata":{"user_id":"507f191e810c19729de860ea","ts":"2026-09-07T08:00:00.000Z"}}]}`
	ranges := JSONRangesToMongo(decode(t, in)).(bson.D)

	changes, _ := docGet(ranges, "changes")
	change := changes.(bson.A)[0].(bson.D)
	if id, _ := docGet(change, "id"); id.(bson.ObjectID).Hex() != "507f1f77bcf86cd799439011" {
		t.Errorf("change id was not converted to an ObjectId: %#v", id)
	}
	metadata, _ := docGet(change, "metadata")
	userID, _ := docGet(metadata.(bson.D), "user_id")
	if _, ok := userID.(bson.ObjectID); !ok {
		t.Errorf("user_id = %#v, want an ObjectId", userID)
	}
	if got := encode(t, ranges); got != in {
		t.Errorf("round trip:\n got %s\nwant %s", got, in)
	}
}

// The thread id on the op wins over the comment's own id, and `resolved` is
// stripped before storage.
func TestCommentIdComesFromOpThread(t *testing.T) {
	const in = `{"comments":[{"id":"507f1f77bcf86cd799439011",` +
		`"op":{"t":"507f191e810c19729de860ea","c":"hi","p":1,"resolved":true}}]}`
	ranges := JSONRangesToMongo(decode(t, in)).(bson.D)

	comments, _ := docGet(ranges, "comments")
	comment := comments.(bson.A)[0].(bson.D)
	id, _ := docGet(comment, "id")
	if id.(bson.ObjectID).Hex() != "507f191e810c19729de860ea" {
		t.Errorf("comment id = %v, want the op's thread id", id)
	}
	op, _ := docGet(comment, "op")
	if _, present := docGet(op.(bson.D), "resolved"); present {
		t.Error("resolved should be stripped before storage")
	}
	if tv, _ := docGet(op.(bson.D), "t"); tv.(bson.ObjectID).Hex() != "507f191e810c19729de860ea" {
		t.Errorf("op.t = %v, want the thread id", tv)
	}
}

func TestFixCommentIds(t *testing.T) {
	ranges := decode(t, `{"comments":[{"id":"aaa","op":{"t":"bbb"}}]}`)
	FixCommentIds(ranges)
	comments, _ := docGet(ranges, "comments")
	id, _ := docGet(comments.(bson.A)[0].(bson.D), "id")
	if id != "bbb" {
		t.Errorf("id = %v, want bbb", id)
	}
}

// A non-ObjectId id is kept as-is, matching _safeObjectId's fallback.
func TestSafeObjectIDFallback(t *testing.T) {
	ranges := JSONRangesToMongo(decode(t, `{"changes":[{"id":"not-an-object-id"}]}`)).(bson.D)
	changes, _ := docGet(ranges, "changes")
	id, _ := docGet(changes.(bson.A)[0].(bson.D), "id")
	if id != "not-an-object-id" {
		t.Errorf("id = %#v, want the original string", id)
	}
}

func TestShouldUpdateRanges(t *testing.T) {
	a := decode(t, `{"changes":[{"id":"507f1f77bcf86cd799439011","op":{"p":3}}]}`)
	b := decode(t, `{"changes":[{"id":"507f1f77bcf86cd799439011","op":{"p":3}}]}`)
	if ShouldUpdateRanges(JSONRangesToMongo(a), JSONRangesToMongo(b)) {
		t.Error("identical ranges should not need an update")
	}
	// Key order must not matter, the way lodash isEqual treats objects.
	c := decode(t, `{"changes":[{"op":{"p":3},"id":"507f1f77bcf86cd799439011"}]}`)
	if ShouldUpdateRanges(JSONRangesToMongo(a), JSONRangesToMongo(c)) {
		t.Error("key order should not count as a change")
	}
	d := decode(t, `{"changes":[{"id":"507f1f77bcf86cd799439011","op":{"p":4}}]}`)
	if !ShouldUpdateRanges(JSONRangesToMongo(a), JSONRangesToMongo(d)) {
		t.Error("a differing value should need an update")
	}
	// Absent stored ranges compare equal to an empty incoming set.
	if ShouldUpdateRanges(nil, bson.D{}) {
		t.Error("nil stored ranges should equal empty incoming ranges")
	}
	if !ShouldUpdateRanges(nil, a) {
		t.Error("nil stored ranges should differ from non-empty incoming ranges")
	}
}
