// Package socketio implements the socket.io 0.9 wire protocol on the server
// side, as spoken by the client Overleaf ships.
//
// The editor loads its client from this service's own /socket.io/socket.io.js,
// and that client is github:overleaf/socket.io-client#0.9.17-overleaf-5 -- a
// fork of a protocol that predates the modern socket.io by a decade. Nothing
// current speaks it, so the framing has to be reproduced exactly. The fork's
// own changes are small: lib/manager.js delegates static file serving to the
// surrounding app, and some unused pieces are removed. The framing below is
// stock 0.9.
package socketio

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// PacketType is the numeric type prefix of a frame.
type PacketType int

// The packet types from lib/parser.js.
const (
	PacketDisconnect PacketType = 0
	PacketConnect    PacketType = 1
	PacketHeartbeat  PacketType = 2
	PacketMessage    PacketType = 3
	PacketJSON       PacketType = 4
	PacketEvent      PacketType = 5
	PacketAck        PacketType = 6
	PacketError      PacketType = 7
	PacketNoop       PacketType = 8
)

// Packet is one decoded frame.
type Packet struct {
	Type PacketType
	ID   string
	// Ack is set when the id carried a "+", meaning the client wants an
	// explicit acknowledgement rather than an automatic one.
	Ack      bool
	Endpoint string
	Data     string

	// Name and Args are filled for event packets.
	Name string
	Args []json.RawMessage
}

// frameRE is the decoder from lib/parser.js:
//
//	/^([^:]+):([0-9]+)?(\+)?:([^:]+)?:?([\s\S]*)?/
//
// which reads as type:id[+]:endpoint:data.
var frameRE = regexp.MustCompile(`^([^:]+):([0-9]+)?(\+)?:([^:]+)?:?([\s\S]*)?$`)

// Decode parses one frame.
func Decode(frame string) (Packet, error) {
	m := frameRE.FindStringSubmatch(frame)
	if m == nil {
		return Packet{}, fmt.Errorf("socketio: malformed frame %q", frame)
	}
	typ, err := strconv.Atoi(m[1])
	if err != nil || typ < 0 || typ > 8 {
		return Packet{}, fmt.Errorf("socketio: unknown packet type %q", m[1])
	}

	p := Packet{
		Type:     PacketType(typ),
		ID:       m[2],
		Ack:      m[3] == "+",
		Endpoint: m[4],
		Data:     m[5],
	}
	if p.Type == PacketEvent && p.Data != "" {
		var ev struct {
			Name string            `json:"name"`
			Args []json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal([]byte(p.Data), &ev); err != nil {
			return p, fmt.Errorf("socketio: malformed event payload: %w", err)
		}
		p.Name, p.Args = ev.Name, ev.Args
	}
	return p, nil
}

// Encode renders a frame.
func Encode(p Packet) string {
	id := p.ID
	if p.Ack && id != "" {
		id += "+"
	}
	data := p.Data
	if p.Type == PacketEvent && p.Name != "" {
		ev := map[string]any{"name": p.Name}
		if len(p.Args) > 0 {
			ev["args"] = p.Args
		}
		encoded, err := json.Marshal(ev)
		if err != nil {
			// A frame that cannot be built is dropped rather than sent
			// malformed; the caller controls these payloads.
			return ""
		}
		data = string(encoded)
	}

	// The trailing colon and data are omitted when there is no data, which is
	// what the client's own encoder produces for heartbeats and connects.
	frame := fmt.Sprintf("%d:%s:%s", p.Type, id, p.Endpoint)
	if data != "" {
		frame += ":" + data
	}
	return frame
}

// EncodeEvent builds an event frame with JSON-encodable arguments.
func EncodeEvent(name string, args ...any) (string, error) {
	raw := make([]json.RawMessage, 0, len(args))
	for _, a := range args {
		encoded, err := json.Marshal(a)
		if err != nil {
			return "", err
		}
		raw = append(raw, encoded)
	}
	return Encode(Packet{Type: PacketEvent, Name: name, Args: raw}), nil
}

// Handshake is the body of a successful handshake response:
//
//	<session id>:<heartbeat timeout>:<close timeout>:<transports>
//
// An empty heartbeat field means heartbeats are disabled.
func Handshake(sessionID string, heartbeatSeconds, closeSeconds int, transports []string) string {
	heartbeat := ""
	if heartbeatSeconds > 0 {
		heartbeat = strconv.Itoa(heartbeatSeconds)
	}
	return strings.Join([]string{
		sessionID, heartbeat, strconv.Itoa(closeSeconds), strings.Join(transports, ","),
	}, ":")
}

// Error reason and advice codes from lib/parser.js.
const (
	ReasonTransportNotSupported = 0
	ReasonClientNotHandshaken   = 1
	ReasonUnauthorized          = 2

	AdviceReconnect = 0
)

// payloadDelimiter is the U+FFFD separator socket.io 0.9 wraps around each
// frame when more than one is sent in a single polling response.
const payloadDelimiter = "�"

// EncodePayload joins frames for a polling transport.
//
// A single frame is sent bare; several are framed as
// �<length>�<frame>. The length counts UTF-16 code units, because
// the decoder on the other side is JavaScript slicing a string.
func EncodePayload(frames []string) string {
	if len(frames) == 1 {
		return frames[0]
	}
	var b strings.Builder
	for _, f := range frames {
		b.WriteString(payloadDelimiter)
		b.WriteString(strconv.Itoa(utf16Len(f)))
		b.WriteString(payloadDelimiter)
		b.WriteString(f)
	}
	return b.String()
}

// DecodePayload splits frames received from a polling transport.
func DecodePayload(data string) []string {
	if data == "" {
		return nil
	}
	if !strings.HasPrefix(data, payloadDelimiter) {
		return []string{data}
	}

	// Work in UTF-16 units so the declared lengths line up with what the
	// JavaScript encoder counted.
	units := utf16.Encode([]rune(data))
	delim := utf16.Encode([]rune(payloadDelimiter))[0]

	var frames []string
	for i := 0; i < len(units); {
		if units[i] != delim {
			break
		}
		i++
		start := i
		for i < len(units) && units[i] != delim {
			i++
		}
		if i >= len(units) {
			break
		}
		length, err := strconv.Atoi(string(utf16.Decode(units[start:i])))
		if err != nil || length < 0 {
			break
		}
		i++ // skip the closing delimiter
		if i+length > len(units) {
			break
		}
		frames = append(frames, string(utf16.Decode(units[i:i+length])))
		i += length
	}
	return frames
}

func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}
