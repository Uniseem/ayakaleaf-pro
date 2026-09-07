// Package bsonjson renders BSON values as JSON the way the Node MongoDB
// driver does when a raw document is handed to res.json().
//
// Two details matter for compatibility: field order is preserved (JSON objects
// built from Go maps would be sorted), and ObjectIds and dates use the same
// representations that the Node driver's toJSON() produces.
package bsonjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Document wraps an ordered BSON document so it marshals to JSON with its
// fields in their stored order.
type Document bson.D

// MarshalJSON implements json.Marshaler.
func (d Document) MarshalJSON() ([]byte, error) {
	return marshalD(bson.D(d))
}

// Documents wraps a slice of ordered BSON documents.
type Documents []bson.D

// MarshalJSON implements json.Marshaler.
func (docs Documents) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, d := range docs {
		if i > 0 {
			buf.WriteByte(',')
		}
		encoded, err := marshalD(d)
		if err != nil {
			return nil, err
		}
		buf.Write(encoded)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

func marshalD(d bson.D) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, elem := range d {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(elem.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		value, err := marshalValue(elem.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalValue(v any) ([]byte, error) {
	switch value := v.(type) {
	case nil:
		return []byte("null"), nil
	case bson.D:
		return marshalD(value)
	case bson.A:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				buf.WriteByte(',')
			}
			encoded, err := marshalValue(item)
			if err != nil {
				return nil, err
			}
			buf.Write(encoded)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	case bson.ObjectID:
		// ObjectId.toJSON() yields the hex string.
		return json.Marshal(value.Hex())
	case bson.DateTime:
		// Date.prototype.toJSON() yields an ISO 8601 string in UTC with
		// millisecond precision.
		t := time.UnixMilli(int64(value)).UTC()
		return json.Marshal(t.Format("2006-01-02T15:04:05.000Z"))
	case time.Time:
		return json.Marshal(value.UTC().Format("2006-01-02T15:04:05.000Z"))
	case bson.Binary:
		return json.Marshal(value.Data)
	case bson.Undefined, bson.MinKey, bson.MaxKey:
		return []byte("null"), nil
	case string, bool, int32, int64, float64:
		return json.Marshal(value)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("bsonjson: cannot render %T as JSON: %w", v, err)
		}
		return encoded, nil
	}
}
