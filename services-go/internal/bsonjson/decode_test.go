package bsonjson

import (
	"encoding/json"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDecodeDocumentPreservesOrderAndUnknownFields(t *testing.T) {
	const in = `{"z":1,"a":{"nested":true},"m":[1,"two",null],"meta":{"ts":"Sun Sep 07 2026","user_id":"abc"}}`
	doc, err := DecodeDocument([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, e := range doc {
		keys = append(keys, e.Key)
	}
	want := []string{"z", "a", "m", "meta"}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("key order = %v, want %v", keys, want)
		}
	}
	// A round trip must not disturb anything, including the "meta" key that
	// the range transformations deliberately do not touch.
	out, err := json.Marshal(Document(doc))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Errorf("round trip:\n got %s\nwant %s", out, in)
	}
}

func TestDecodeNumbers(t *testing.T) {
	doc, err := DecodeDocument([]byte(`{"small":42,"big":1757251200000,"frac":1.5,"neg":-7}`))
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]any{}
	for _, e := range doc {
		byKey[e.Key] = e.Value
	}
	if got, ok := byKey["small"].(int32); !ok || got != 42 {
		t.Errorf("small = %#v, want int32(42)", byKey["small"])
	}
	if got, ok := byKey["neg"].(int32); !ok || got != -7 {
		t.Errorf("neg = %#v, want int32(-7)", byKey["neg"])
	}
	// Beyond int32 a JavaScript number is stored as a double.
	if got, ok := byKey["big"].(float64); !ok || got != 1757251200000 {
		t.Errorf("big = %#v, want float64(1757251200000)", byKey["big"])
	}
	if got, ok := byKey["frac"].(float64); !ok || got != 1.5 {
		t.Errorf("frac = %#v, want float64(1.5)", byKey["frac"])
	}
}

func TestDecodeValueArrays(t *testing.T) {
	v, err := DecodeValue([]byte(`[{"a":1},{"b":2}]`))
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := v.(bson.A)
	if !ok || len(arr) != 2 {
		t.Fatalf("got %#v, want a 2-element bson.A", v)
	}
	if _, ok := arr[0].(bson.D); !ok {
		t.Errorf("array element = %T, want bson.D", arr[0])
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	for _, in := range []string{`[]`, `{"a":1} trailing`, `{`, ``} {
		if _, err := DecodeDocument([]byte(in)); err == nil {
			t.Errorf("DecodeDocument(%q) succeeded, want an error", in)
		}
	}
}
