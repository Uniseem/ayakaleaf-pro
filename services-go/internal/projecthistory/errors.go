package projecthistory

import "errors"

// The kinds of failure this service tells apart. They matter beyond their
// messages: the http layer turns some of them into particular status codes,
// and the sync decides whether to give up or to ask for the whole project
// again by what came back.

// ErrNotFound is a project, document or version that is not there.
var ErrNotFound = errors.New("not found")

// ErrBadRequest is a request this service will not act on.
var ErrBadRequest = errors.New("bad request")

// ErrSync is a project that could not be put back in step with its history.
var ErrSync = errors.New("sync error")

// ErrSyncOngoing is a resync that is already running. The message is the one
// the editor looks for.
var ErrSyncOngoing = errors.New("sync ongoing")

// ErrOpsOutOfOrder is a history whose versions do not increase, which means
// something has been written to it out of turn.
var ErrOpsOutOfOrder = errors.New("ops out of order")

// ErrInconsistentChunk is a chunk that does not say what it should.
var ErrInconsistentChunk = errors.New("inconsistent chunk")

// ErrTooManyRequests is a project asking for more than it may have.
var ErrTooManyRequests = errors.New("too many requests")

// ErrNeedFullProjectStructureResync is a project whose file tree has to be sent
// again before anything else can be done with it.
var ErrNeedFullProjectStructureResync = errors.New("need full project structure resync")

// ErrFileContentEmpty is a file that arrived with nothing in it.
var ErrFileContentEmpty = errors.New("file content empty")
