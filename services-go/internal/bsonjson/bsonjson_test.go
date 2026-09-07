package bsonjson

import (
	"encoding/json"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDocumentPreservesFieldOrder(t *testing.T) {
	id, _ := bson.ObjectIDFromHex("507f1f77bcf86cd799439011")
	user, _ := bson.ObjectIDFromHex("507f191e810c19729de860ea")
	doc := Document{
		{Key: "_id", Value: id},
		{Key: "user_id", Value: user},
		{Key: "key", Value: "some-key"},
		{Key: "messageOpts", Value: ""},
		{Key: "templateKey", Value: "f4g5"},
	}
	got, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"_id":"507f1f77bcf86cd799439011","user_id":"507f191e810c19729de860ea",` +
		`"key":"some-key","messageOpts":"","templateKey":"f4g5"}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestDateRendersLikeJavaScript(t *testing.T) {
	ts := time.Date(2026, 9, 7, 12, 0, 0, 123_000_000, time.UTC)
	doc := Document{{Key: "expires", Value: bson.DateTime(ts.UnixMilli())}}
	got, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"expires":"2026-09-07T12:00:00.123Z"}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestNestedAndEmpty(t *testing.T) {
	doc := Document{
		{Key: "messageOpts", Value: bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: nil}}},
		{Key: "tags", Value: bson.A{"x", int64(2)}},
	}
	got, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"messageOpts":{"a":1,"b":null},"tags":["x",2]}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
	empty, err := json.Marshal(Documents{})
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != "[]" {
		t.Errorf("empty Documents = %s, want []", empty)
	}
}
