package historystore

import (
	"bytes"
	"io"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// The small things the two halves share.

func newReader(content []byte) io.Reader { return bytes.NewReader(content) }

// projected is the read form the index lookups use.
func projected() *options.FindOneOptionsBuilder { return options.FindOne() }

// upsert writes the document if it is not there, which is what makes
// initialising a project safe to do twice.
func upsert() *options.UpdateOneOptionsBuilder {
	return options.UpdateOne().SetUpsert(true)
}

// validUTF8 says whether these bytes are text at all.
func validUTF8(content []byte) bool { return utf8.Valid(content) }
