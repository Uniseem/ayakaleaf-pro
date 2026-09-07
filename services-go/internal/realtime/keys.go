package realtime

// KeySchema names the Redis keys real-time shares with document-updater.
//
// There are two schemas in use and they are not compatible. The services'
// own settings.defaults wrap the id in braces -- Redis Cluster hash tags,
// which keep every key of one project on the same slot -- while server-ce
// replaces the whole schema in /etc/overleaf/settings.js with an unbraced one:
//
//	// server-ce/config/settings.js
//	pendingUpdates({ doc_id }) { return `PendingUpdates:${doc_id}` }
//
// Getting this wrong is silent and total. real-time queues an update under one
// name, document-updater waits on another, and the edit is never applied: the
// editor shows the typing, then declares the document out of sync.
type KeySchema struct {
	// hashTags wraps ids in braces. server-ce does not.
	hashTags bool
}

// UpstreamKeySchema is the one in the services' settings.defaults files, and
// the one the acceptance suites assert.
var UpstreamKeySchema = KeySchema{hashTags: true}

// ServerCEKeySchema is the one server-ce installs over it.
var ServerCEKeySchema = KeySchema{hashTags: false}

// NewKeySchema selects a schema by name, defaulting to the upstream one.
func NewKeySchema(name string) KeySchema {
	if name == "server-ce" {
		return ServerCEKeySchema
	}
	return UpstreamKeySchema
}

func (k KeySchema) id(v string) string {
	if k.hashTags {
		return "{" + v + "}"
	}
	return v
}

// ClientsInProject is the set of client ids currently in a project.
func (k KeySchema) ClientsInProject(projectID string) string {
	return "clients_in_project:" + k.id(projectID)
}

// ConnectedUser holds one client's presence and cursor.
func (k KeySchema) ConnectedUser(projectID, clientID string) string {
	return "connected_user:" + k.id(projectID) + ":" + clientID
}

// PendingUpdates is the queue document-updater reads a document's edits from.
func (k KeySchema) PendingUpdates(docID string) string {
	return "PendingUpdates:" + k.id(docID)
}

// ProjectNotEmptySince records when a project last had somebody in it.
//
// It is braced under both schemas: server-ce overrides the other three keys
// and leaves this one alone. That is safe because nothing outside this service
// reads it -- the name only has to agree with itself.
func (k KeySchema) ProjectNotEmptySince(projectID string) string {
	return "projectNotEmptySince:{" + projectID + "}"
}
