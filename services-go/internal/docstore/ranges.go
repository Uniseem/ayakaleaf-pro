package docstore

import (
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/oid"
)

// Range documents are free-form. Only the keys below are understood; every
// other key -- including "meta", which the acceptance suite sends and which
// the transformations deliberately do not touch because they look for
// "metadata" -- has to survive untouched and in place. That is why ranges are
// carried as bson.D rather than typed structs, which would silently drop
// anything not modelled.

// docGet returns the value for key, and whether it was present.
func docGet(doc bson.D, key string) (any, bool) {
	for _, e := range doc {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// docSet replaces the value for key in place, appending it if absent so that
// existing field order is preserved.
func docSet(doc bson.D, key string, value any) bson.D {
	for i := range doc {
		if doc[i].Key == key {
			doc[i].Value = value
			return doc
		}
	}
	return append(doc, bson.E{Key: key, Value: value})
}

// docDelete removes key if present.
func docDelete(doc bson.D, key string) bson.D {
	for i := range doc {
		if doc[i].Key == key {
			return append(doc[:i:i], doc[i+1:]...)
		}
	}
	return doc
}

// safeObjectID mirrors RangeManager._safeObjectId: parse as an ObjectId, and
// on failure hand back the original value untouched.
func safeObjectID(value any) any {
	s, ok := value.(string)
	if !ok {
		return value
	}
	id, ok := oid.Parse(s)
	if !ok {
		return value
	}
	return id
}

// jsDateLayouts covers the forms new Date(string) is given in practice: ISO
// 8601 from history and the editor, and Date.prototype.toString() from the
// acceptance suite.
var jsDateLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02",
	"Mon Jan 02 2006 15:04:05 GMT-0700 (MST)",
	"Mon Jan 02 2006 15:04:05 GMT-0700",
	"Mon, 02 Jan 2006 15:04:05 GMT",
}

// toDate converts a value the way new Date(value) would.
//
// An unparseable string yields an Invalid Date in JavaScript, which the driver
// then writes as a corrupt timestamp. Leaving the original value in place
// instead keeps the document readable; the second result says whether a
// conversion happened.
func toDate(value any) (any, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case bson.DateTime:
		return v, true
	case float64:
		return time.UnixMilli(int64(v)).UTC(), true
	case int32:
		return time.UnixMilli(int64(v)).UTC(), true
	case int64:
		return time.UnixMilli(v).UTC(), true
	case string:
		for _, layout := range jsDateLayouts {
			if t, err := time.Parse(layout, v); err == nil {
				return t.UTC(), true
			}
		}
		return value, false
	default:
		return value, false
	}
}

// updateMetadata mirrors the inner updateMetadata of jsonRangesToMongo.
func updateMetadata(entry bson.D) bson.D {
	raw, ok := docGet(entry, "metadata")
	if !ok {
		return entry
	}
	metadata, ok := raw.(bson.D)
	if !ok {
		return entry
	}
	if ts, present := docGet(metadata, "ts"); present && ts != nil {
		if converted, ok := toDate(ts); ok {
			metadata = docSet(metadata, "ts", converted)
		}
	}
	if userID, present := docGet(metadata, "user_id"); present && userID != nil {
		metadata = docSet(metadata, "user_id", safeObjectID(userID))
	}
	return docSet(entry, "metadata", metadata)
}

// JSONRangesToMongo converts the ids and timestamps of a range document into
// their BSON forms, matching RangeManager.jsonRangesToMongo.
func JSONRangesToMongo(ranges any) any {
	doc, ok := ranges.(bson.D)
	if !ok {
		return ranges
	}

	if raw, present := docGet(doc, "changes"); present {
		if changes, ok := raw.(bson.A); ok {
			for i, item := range changes {
				change, ok := item.(bson.D)
				if !ok {
					continue
				}
				if id, present := docGet(change, "id"); present {
					change = docSet(change, "id", safeObjectID(id))
				}
				changes[i] = updateMetadata(change)
			}
		}
	}

	if raw, present := docGet(doc, "comments"); present {
		if comments, ok := raw.(bson.A); ok {
			for i, item := range comments {
				comment, ok := item.(bson.D)
				if !ok {
					continue
				}
				// Two bugs produced mismatched ids, so the thread id on the op
				// wins over the comment's own id.
				idSource, _ := docGet(comment, "id")
				if op, present := docGet(comment, "op"); present {
					if opDoc, ok := op.(bson.D); ok {
						if t, present := docGet(opDoc, "t"); present && t != nil {
							idSource = t
						}
					}
				}
				id := safeObjectID(idSource)
				comment = docSet(comment, "id", id)

				if op, present := docGet(comment, "op"); present {
					if opDoc, ok := op.(bson.D); ok {
						opDoc = docSet(opDoc, "t", id)
						// `resolved` is added when comments come back from
						// history; it does not belong in the docs collection.
						opDoc = docDelete(opDoc, "resolved")
						comment = docSet(comment, "op", opDoc)
					}
				}
				comments[i] = updateMetadata(comment)
			}
		}
	}
	return doc
}

// FixCommentIds mirrors RangeManager.fixCommentIds: on the way out, a
// comment's id is taken from its op's thread id.
func FixCommentIds(ranges any) {
	doc, ok := ranges.(bson.D)
	if !ok {
		return
	}
	raw, present := docGet(doc, "comments")
	if !present {
		return
	}
	comments, ok := raw.(bson.A)
	if !ok {
		return
	}
	for i, item := range comments {
		comment, ok := item.(bson.D)
		if !ok {
			continue
		}
		op, present := docGet(comment, "op")
		if !present {
			continue
		}
		opDoc, ok := op.(bson.D)
		if !ok {
			continue
		}
		if t, present := docGet(opDoc, "t"); present && t != nil {
			comments[i] = docSet(comment, "id", t)
		}
	}
}

// ShouldUpdateRanges reports whether the stored ranges differ from the
// incoming ones, matching `!_.isEqual(docRanges, incomingRanges)`.
//
// Absent stored ranges compare as an empty document, because empty ranges are
// not written to the database and that is the shape an empty incoming set has.
func ShouldUpdateRanges(docRanges, incomingRanges any) bool {
	if docRanges == nil {
		docRanges = bson.D{}
	}
	return !looseEqual(docRanges, incomingRanges)
}

// looseEqual is a deep comparison with JavaScript's value semantics: object
// key order does not matter, and all numbers are one type.
func looseEqual(a, b any) bool {
	if an, ok := asNumber(a); ok {
		bn, ok := asNumber(b)
		return ok && (an == bn || (math.IsNaN(an) && math.IsNaN(bn)))
	}

	switch av := a.(type) {
	case bson.D:
		bv, ok := b.(bson.D)
		if !ok || len(av) != len(bv) {
			return false
		}
		for _, e := range av {
			other, present := docGet(bv, e.Key)
			if !present || !looseEqual(e.Value, other) {
				return false
			}
		}
		return true
	case bson.A:
		bv, ok := b.(bson.A)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !looseEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case bson.ObjectID:
		bv, ok := b.(bson.ObjectID)
		return ok && av == bv
	case time.Time:
		bv, ok := asTime(b)
		return ok && av.UTC().Equal(bv)
	case bson.DateTime:
		bv, ok := asTime(b)
		return ok && time.UnixMilli(int64(av)).UTC().Equal(bv)
	case nil:
		return b == nil
	default:
		return a == b
	}
}

func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t.UTC(), true
	case bson.DateTime:
		return time.UnixMilli(int64(t)).UTC(), true
	default:
		return time.Time{}, false
	}
}
