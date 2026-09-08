package realtime

import "errors"

// The errors below carry the exact messages Router._handleError matches on and
// forwards to the client. The editor branches on those strings -- "not
// authorized" closes the document, "joinLeaveEpoch mismatch" is retried
// silently -- so the wording is part of the protocol, not a log message.
var (
	ErrNotJoined              = errors.New("no project_id found on client")
	ErrJoinLeaveEpochMismatch = errors.New("joinLeaveEpoch mismatch")
	ErrUnexpectedArguments    = errors.New("unexpected arguments")
	ErrMissingOps             = errors.New("doc updater could not load requested ops")
	ErrUpdateTooLarge         = errors.New("update is too large")
	ErrNullBytesInOp          = errors.New("null bytes found in op")
)

// codedError is the Node CodedError: a message plus a code the client reads off
// the serialised error object.
type codedError struct {
	message string
	code    string
}

func (e *codedError) Error() string { return e.message }

// CodedError builds an error carrying a machine-readable code.
func CodedError(message, code string) error { return &codedError{message: message, code: code} }

// serializedError is the object the client receives in place of an error.
//
// Only whitelisted messages are passed through; anything else becomes the
// generic message, because the raw error would leak server internals to any
// user who can open a project.
type serializedError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// passThroughMessages are returned to the client verbatim, matching the list in
// Router._handleError.
var passThroughMessages = map[string]bool{
	"not authorized":                           true,
	"joinLeaveEpoch mismatch":                  true,
	"doc updater could not load requested ops": true,
	"no project_id found on client":            true,
	"cannot join multiple projects":            true,
	"unexpected arguments":                     true,
}

// serializeError renders an error for the client.
func serializeError(err error) serializedError {
	var coded *codedError
	if errors.As(err, &coded) {
		return serializedError{Message: coded.message, Code: coded.code}
	}
	if passThroughMessages[err.Error()] {
		return serializedError{Message: err.Error()}
	}
	return serializedError{Message: "Something went wrong in real-time service"}
}
