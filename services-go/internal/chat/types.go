package chat

import (
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Millis is a JavaScript Date.now() value: milliseconds since the epoch.
//
// The Node service stores it with `timestamp: Date.now()`, and the Node driver
// serialises a JS Number larger than int32 as a BSON double. Writing a double
// here keeps documents byte-identical with those the Node service produces, so
// the two implementations can run side by side against one collection.
type Millis float64

var (
	_ bson.ValueMarshaler   = Millis(0)
	_ bson.ValueUnmarshaler = (*Millis)(nil)
)

func (m Millis) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue(float64(m))
	return byte(typ), data, err
}

// UnmarshalBSONValue accepts every numeric BSON type, not just double, so that
// documents written by other tooling still decode.
func (m *Millis) UnmarshalBSONValue(typ byte, data []byte) error {
	rv := bson.RawValue{Type: bson.Type(typ), Value: data}
	switch bson.Type(typ) {
	case bson.TypeDouble:
		*m = Millis(rv.Double())
	case bson.TypeInt64:
		*m = Millis(rv.Int64())
	case bson.TypeInt32:
		*m = Millis(rv.Int32())
	case bson.TypeNull, bson.TypeUndefined:
		*m = 0
	default:
		return fmt.Errorf("chat: cannot decode BSON type %v as a timestamp", bson.Type(typ))
	}
	return nil
}

// MarshalJSON renders the value as a bare JSON number, the way JSON.stringify
// renders the Number the Node service holds.
func (m Millis) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(m), 'f', -1, 64)), nil
}

// NowMillis returns the current time as Date.now() would.
func NowMillis() Millis {
	return Millis(time.Now().UnixMilli())
}

// JSDate serialises like a JavaScript Date passed through JSON.stringify:
// UTC, always three decimal places, "Z" suffix.
type JSDate struct{ time.Time }

func (d JSDate) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.UTC().Format("2006-01-02T15:04:05.000Z") + `"`), nil
}

// Resolved is the `resolved` sub-document on a room.
//
// user_id is stored as the raw string from the request body: the Node service
// does not cast it to an ObjectId, and callers read it back as a string.
type Resolved struct {
	UserID string    `bson:"user_id" json:"user_id"`
	TS     time.Time `bson:"ts" json:"ts"`
}

// Room is a document in the `rooms` collection. A room with no thread_id is
// the project's global chat.
type Room struct {
	ID        bson.ObjectID  `bson:"_id,omitempty"`
	ProjectID bson.ObjectID  `bson:"project_id"`
	ThreadID  *bson.ObjectID `bson:"thread_id,omitempty"`
	Resolved  *Resolved      `bson:"resolved,omitempty"`
}

// Message is a document in the `messages` collection.
type Message struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	Content   string        `bson:"content"`
	RoomID    bson.ObjectID `bson:"room_id"`
	UserID    bson.ObjectID `bson:"user_id"`
	Timestamp Millis        `bson:"timestamp"`
	EditedAt  *Millis       `bson:"edited_at,omitempty"`
}
