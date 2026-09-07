// Package rediskeys names the Redis keys the Overleaf services share.
//
// There are two schemas in use and they are not compatible. The services' own
// config/settings.defaults files wrap every id in braces -- Redis Cluster hash
// tags, which keep the keys of one project or document on the same slot --
// while server-ce replaces part of the schema in /etc/overleaf/settings.js with
// an unbraced one:
//
//	// server-ce/config/settings.js
//	docLines({ doc_id }) { return `doclines:${doc_id}` }
//
// The replacement is partial. server-ce overrides nine keys and leaves the rest
// to the defaults, so under it some keys carry braces and some do not. Deciding
// that with a single flag gets half of them wrong.
//
// Getting a name wrong is silent and total: one service writes under a name
// another is not reading, nothing fails, and the editor eventually declares the
// document out of sync.
package rediskeys

// Schema names every key the Go services use.
type Schema struct {
	// serverCE selects the server-ce spelling for the keys it overrides.
	serverCE bool
}

// Upstream is the schema in the services' settings.defaults files, and the one
// the acceptance suites assert.
var Upstream = Schema{}

// ServerCE is the schema a server-ce container actually runs with.
var ServerCE = Schema{serverCE: true}

// New selects a schema by name, defaulting to the upstream one. The runit
// scripts pass "server-ce" through REDIS_KEY_SCHEMA.
func New(name string) Schema {
	if name == "server-ce" {
		return ServerCE
	}
	return Upstream
}

// IsServerCE reports which schema this is, for logging at startup.
func (s Schema) IsServerCE() bool { return s.serverCE }

// braced wraps an id for a key server-ce leaves alone.
func braced(id string) string { return "{" + id + "}" }

// overridden wraps an id for a key server-ce redefines: braces upstream, bare
// under server-ce.
func (s Schema) overridden(id string) string {
	if s.serverCE {
		return id
	}
	return "{" + id + "}"
}

// The nine keys server-ce redefines.

// BlockingKey is the per-document lock.
func (s Schema) BlockingKey(docID string) string { return "Blocking:" + s.overridden(docID) }

// DocLines holds the document as a JSON array of lines.
func (s Schema) DocLines(docID string) string { return "doclines:" + s.overridden(docID) }

// DocOps is the list of recently applied operations, used to transform an
// update that arrives against an older version.
func (s Schema) DocOps(docID string) string { return "DocOps:" + s.overridden(docID) }

// DocVersion is the version the document is currently at.
func (s Schema) DocVersion(docID string) string { return "DocVersion:" + s.overridden(docID) }

// DocHash is a checksum of DocLines, used to detect corruption.
func (s Schema) DocHash(docID string) string { return "DocHash:" + s.overridden(docID) }

// ProjectKey records which project a document belongs to.
func (s Schema) ProjectKey(docID string) string { return "ProjectId:" + s.overridden(docID) }

// DocsInProject is the set of documents currently loaded for a project.
func (s Schema) DocsInProject(projectID string) string {
	return "DocsIn:" + s.overridden(projectID)
}

// Ranges holds the tracked changes and comments of a document.
func (s Schema) Ranges(docID string) string { return "Ranges:" + s.overridden(docID) }

// PendingUpdates is the queue real-time writes and document-updater reads. The
// two services must agree on this name or every edit is lost.
func (s Schema) PendingUpdates(docID string) string {
	return "PendingUpdates:" + s.overridden(docID)
}

// The keys server-ce does not redefine, which therefore keep their braces under
// both schemas.

// UnflushedTime is when the document first had unsaved changes.
func (s Schema) UnflushedTime(docID string) string { return "UnflushedTime:" + braced(docID) }

// Pathname is the document's path within the project.
func (s Schema) Pathname(docID string) string { return "Pathname:" + braced(docID) }

// ProjectHistoryID ties a document to its history.
func (s Schema) ProjectHistoryID(docID string) string {
	return "ProjectHistoryId:" + braced(docID)
}

// ProjectState marks whether a project's document list is up to date.
func (s Schema) ProjectState(projectID string) string {
	return "ProjectState:" + braced(projectID)
}

// ProjectBlock stops new documents being loaded while a project is migrated.
func (s Schema) ProjectBlock(projectID string) string {
	return "ProjectBlock:" + braced(projectID)
}

// LastUpdatedAt is when the document was last changed.
func (s Schema) LastUpdatedAt(docID string) string { return "lastUpdatedAt:" + braced(docID) }

// LastUpdatedBy is who changed it last.
func (s Schema) LastUpdatedBy(docID string) string { return "lastUpdatedBy:" + braced(docID) }

// ResolvedCommentIds is the set of comment threads marked resolved.
func (s Schema) ResolvedCommentIds(docID string) string {
	return "ResolvedCommentIds:" + braced(docID)
}

// ProjectNotificationTimestamp is when a project last changed, for email
// notifications.
func (s Schema) ProjectNotificationTimestamp(projectID string) string {
	return "ProjectNotificationTimestamp:" + braced(projectID)
}

// ClientsInProject is the set of real-time clients in a project.
func (s Schema) ClientsInProject(projectID string) string {
	return "clients_in_project:" + s.overridden(projectID)
}

// ConnectedUser holds one client's presence and cursor.
func (s Schema) ConnectedUser(projectID, clientID string) string {
	return "connected_user:" + s.overridden(projectID) + ":" + clientID
}

// ProjectNotEmptySince records when a project last had somebody in it.
//
// server-ce overrides the two keys above but not this one, so it keeps its
// braces. That is safe because nothing outside real-time reads it: the name
// only has to agree with itself.
func (s Schema) ProjectNotEmptySince(projectID string) string {
	return "projectNotEmptySince:" + braced(projectID)
}

// The project-history queue. These live under redis.project_history in the
// service defaults, which server-ce points at the same connection without
// redefining, so they keep their braces.

// ProjectHistoryOps is the queue project-history reads a project's edits from.
func (s Schema) ProjectHistoryOps(projectID string) string {
	return "ProjectHistory:Ops:" + braced(projectID)
}

// ProjectHistoryFirstOpTimestamp is when the oldest unprocessed edit was
// queued, which is how a history backlog is noticed.
func (s Schema) ProjectHistoryFirstOpTimestamp(projectID string) string {
	return "ProjectHistory:FirstOpTimestamp:" + braced(projectID)
}

// Keys with no id in them, identical under both schemas.

// HistoryRangesSupport is the set of documents whose ranges are tracked in
// history.
func (s Schema) HistoryRangesSupport() string { return "HistoryRangesSupport" }

// FlushAndDeleteQueue orders projects waiting to be written out and unloaded.
func (s Schema) FlushAndDeleteQueue() string { return "DocUpdaterFlushAndDeleteQueue" }
