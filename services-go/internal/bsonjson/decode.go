package bsonjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// DecodeDocument parses a JSON object into an ordered bson.D.
//
// encoding/json's usual target for an unknown object is map[string]any, which
// loses field order and would then serialise back in Go's randomised map
// order. Documents that pass through this service -- comment and change ranges
// above all -- are free-form: only a handful of keys are understood, and every
// other key has to survive the round trip untouched and in place.
func DecodeDocument(data []byte) (bson.D, error) {
	dec := newDecoder(data)
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("bsonjson: expected a JSON object, got %v", tok)
	}
	doc, err := decodeObject(dec)
	if err != nil {
		return nil, err
	}
	if err := expectEOF(dec); err != nil {
		return nil, err
	}
	return doc, nil
}

// DecodeValue parses an arbitrary JSON value, using bson.D for objects and
// bson.A for arrays so that nested field order is preserved too.
func DecodeValue(data []byte) (any, error) {
	dec := newDecoder(data)
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	value, err := decodeFrom(dec, tok)
	if err != nil {
		return nil, err
	}
	if err := expectEOF(dec); err != nil {
		return nil, err
	}
	return value, nil
}

func newDecoder(data []byte) *json.Decoder {
	dec := json.NewDecoder(bytes.NewReader(data))
	// Large integers must not silently become float64s; UseNumber keeps the
	// literal so the caller can decide.
	dec.UseNumber()
	return dec
}

func expectEOF(dec *json.Decoder) error {
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("bsonjson: trailing data after the JSON value")
	}
	return nil
}

func decodeObject(dec *json.Decoder) (bson.D, error) {
	doc := bson.D{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := tok.(json.Delim); ok && delim == '}' {
			return doc, nil
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("bsonjson: expected an object key, got %v", tok)
		}
		valueTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		value, err := decodeFrom(dec, valueTok)
		if err != nil {
			return nil, err
		}
		doc = append(doc, bson.E{Key: key, Value: value})
	}
}

func decodeArray(dec *json.Decoder) (bson.A, error) {
	arr := bson.A{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := tok.(json.Delim); ok && delim == ']' {
			return arr, nil
		}
		value, err := decodeFrom(dec, tok)
		if err != nil {
			return nil, err
		}
		arr = append(arr, value)
	}
}

func decodeFrom(dec *json.Decoder, tok json.Token) (any, error) {
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			return decodeObject(dec)
		case '[':
			return decodeArray(dec)
		default:
			return nil, fmt.Errorf("bsonjson: unexpected delimiter %v", v)
		}
	case json.Number:
		return decodeNumber(v)
	default:
		// string, bool, or nil
		return v, nil
	}
}

// decodeNumber mirrors how the Node driver stores a JavaScript number: an
// integer that fits in int32 becomes an int32, anything else a double.
func decodeNumber(n json.Number) (any, error) {
	if i, err := n.Int64(); err == nil {
		if i >= -2147483648 && i <= 2147483647 {
			return int32(i), nil
		}
		// Larger integers are still Numbers in JavaScript, and the driver
		// writes those as doubles.
		return float64(i), nil
	}
	f, err := n.Float64()
	if err != nil {
		return nil, fmt.Errorf("bsonjson: cannot decode number %q: %w", n.String(), err)
	}
	return f, nil
}
