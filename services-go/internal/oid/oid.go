// Package oid parses Mongo ObjectIds with exactly the semantics of the Node
// driver's ObjectId constructor (bson v6, as shipped with mongodb 6.12), which
// the services being replaced rely on for request validation.
package oid

import (
	"encoding/hex"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Parse accepts the two string forms the Node driver accepts:
//
//	a 24-character hex string, or
//	a 12-character string taken as raw bytes.
//
// The 12-character form looks odd but is load-bearing: ObjectId.isValid() —
// and therefore the 400-vs-200 behaviour of the Node services — returns true
// for it, so rejecting it here would be a behaviour change.
func Parse(s string) (bson.ObjectID, bool) {
	var id bson.ObjectID
	switch {
	case len(s) == 24:
		buf, err := hex.DecodeString(s)
		if err != nil {
			return id, false
		}
		copy(id[:], buf)
		return id, true
	case len(s) == 12 && isASCII(s):
		// JS measures length in UTF-16 units and requires the UTF-8 encoding
		// to be 12 bytes; both hold exactly when the string is 12 ASCII chars.
		copy(id[:], s)
		return id, true
	default:
		return id, false
	}
}

// IsValid reports whether Parse would succeed. It is the counterpart of
// ObjectId.isValid().
func IsValid(s string) bool {
	_, ok := Parse(s)
	return ok
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}
