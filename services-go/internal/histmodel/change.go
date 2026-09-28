package histmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// A change is one entry in the history: what was done, when, and by whom. What
// was done is a list of operations on the project -- a file added, moved, given
// new metadata, or edited.
//
// This is the port of Change, Operation and its subclasses, and Origin.

// Operation is one thing done to a project.
//
// Which kind it is comes from the fields it carries, as with files. The order
// of the checks is the reference implementation's, and it matters: an edit is
// recognised before a rename, so an operation carrying both is an edit.
type Operation interface {
	// Pathname is the file the operation is about.
	Pathname() string
	// Raw is the JSON form.
	Raw() (json.RawMessage, error)
}

// ErrBadOperation is a raw operation that is none of the known kinds.
var ErrBadOperation = errors.New("invalid raw operation")

// AddFileOperation puts a file into the project.
type AddFileOperation struct {
	Path string
	File *File
}

// Pathname is the file being added.
func (o *AddFileOperation) Pathname() string { return o.Path }

// Raw writes the operation.
func (o *AddFileOperation) Raw() (json.RawMessage, error) {
	file, err := json.Marshal(o.File)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"pathname": o.Path, "file": json.RawMessage(file),
	})
}

// MoveFileOperation renames a file, or deletes it when the new name is empty.
type MoveFileOperation struct {
	Path    string
	NewPath string
}

// Pathname is the file being moved.
func (o *MoveFileOperation) Pathname() string { return o.Path }

// Raw writes the operation.
func (o *MoveFileOperation) Raw() (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"pathname": o.Path, "newPathname": o.NewPath,
	})
}

// EditFileOperation changes the contents of a file, or a comment on it.
//
// The edit itself is kept as raw JSON and written back beside the pathname,
// which is how the reference implementation stores it: the fields of the edit
// sit at the top level of the operation rather than under a key of their own.
type EditFileOperation struct {
	Path string
	Edit json.RawMessage
	// TextOperation is the edit read into the model, when it is one. A comment
	// being resolved or deleted is an edit too, and carries no text operation.
	TextOperation *TextOperation
}

// Pathname is the file being edited.
func (o *EditFileOperation) Pathname() string { return o.Path }

// Raw writes the operation, with the edit spread alongside the pathname.
func (o *EditFileOperation) Raw() (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if len(o.Edit) > 0 {
		if err := json.Unmarshal(o.Edit, &fields); err != nil {
			return nil, err
		}
	}
	path, err := json.Marshal(o.Path)
	if err != nil {
		return nil, err
	}
	fields["pathname"] = path
	return json.Marshal(fields)
}

// SetFileMetadataOperation changes what the project records about a file.
type SetFileMetadataOperation struct {
	Path     string
	Metadata json.RawMessage
}

// Pathname is the file whose metadata is being set.
func (o *SetFileMetadataOperation) Pathname() string { return o.Path }

// Raw writes the operation.
func (o *SetFileMetadataOperation) Raw() (json.RawMessage, error) {
	metadata := o.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage("null")
	}
	return json.Marshal(map[string]any{
		"pathname": o.Path, "metadata": metadata,
	})
}

// NoOperation does nothing. An empty object in the history is one of these.
type NoOperation struct{}

// Pathname is empty.
func (o *NoOperation) Pathname() string { return "" }

// Raw writes an empty object.
func (o *NoOperation) Raw() (json.RawMessage, error) { return json.RawMessage("{}"), nil }

// operationFromRaw reads one operation.
func operationFromRaw(raw json.RawMessage) (Operation, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadOperation, err)
	}

	var pathname string
	if encoded, ok := fields["pathname"]; ok {
		_ = json.Unmarshal(encoded, &pathname)
	}

	if file, ok := fields["file"]; ok {
		var parsed File
		if err := json.Unmarshal(file, &parsed); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadOperation, err)
		}
		return &AddFileOperation{Path: pathname, File: &parsed}, nil
	}

	_, hasTextOperation := fields["textOperation"]
	_, hasCommentID := fields["commentId"]
	_, hasDeleteComment := fields["deleteComment"]
	if hasTextOperation || hasCommentID || hasDeleteComment {
		// The edit is everything but the pathname.
		edit := map[string]json.RawMessage{}
		for key, value := range fields {
			if key != "pathname" {
				edit[key] = value
			}
		}

		// A text operation is read into the model rather than passed through,
		// because reading it is what normalises it: two adjacent retains
		// written separately are one retain, and the history stores the one.
		var operation *TextOperation
		if hasTextOperation {
			operation = NewTextOperation()
			encoded, err := json.Marshal(map[string]json.RawMessage{
				"textOperation": fields["textOperation"],
			})
			if err != nil {
				return nil, err
			}
			if contentHash, ok := fields["contentHash"]; ok {
				var withHash map[string]json.RawMessage
				_ = json.Unmarshal(encoded, &withHash)
				withHash["contentHash"] = contentHash
				encoded, _ = json.Marshal(withHash)
			}
			if err := json.Unmarshal(encoded, operation); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrBadOperation, err)
			}

			normalised, err := json.Marshal(operation)
			if err != nil {
				return nil, err
			}
			var normalisedFields map[string]json.RawMessage
			if err := json.Unmarshal(normalised, &normalisedFields); err != nil {
				return nil, err
			}
			edit["textOperation"] = normalisedFields["textOperation"]
		}

		encoded, err := json.Marshal(edit)
		if err != nil {
			return nil, err
		}
		return &EditFileOperation{
			Path: pathname, Edit: encoded, TextOperation: operation,
		}, nil
	}

	if newPathname, ok := fields["newPathname"]; ok {
		var to string
		_ = json.Unmarshal(newPathname, &to)
		return &MoveFileOperation{Path: pathname, NewPath: to}, nil
	}

	if metadata, ok := fields["metadata"]; ok {
		return &SetFileMetadataOperation{Path: pathname, Metadata: metadata}, nil
	}

	if len(fields) == 0 {
		return &NoOperation{}, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrBadOperation, raw)
}

// Origin records where a change came from: which part of the editor, or which
// version it was restored from.
type Origin struct {
	Kind string
	// Version, Path and Timestamp are carried by the restore origins and by
	// nothing else.
	Version   *int
	Path      string
	Timestamp string
}

// The kinds that carry more than a name.
const (
	originRestore        = "restore"
	originFileRestore    = "file-restore"
	originProjectRestore = "project-restore"
)

// Raw writes the origin.
//
// A kind that is only a name is written as only a name, even if it arrived with
// other fields: that is what the reference implementation does, and a round
// trip through it is what the history stores.
func (o *Origin) Raw() (json.RawMessage, error) {
	fields := map[string]any{"kind": o.Kind}
	switch o.Kind {
	case originRestore, originProjectRestore:
		fields["version"] = o.Version
		fields["timestamp"] = o.Timestamp
	case originFileRestore:
		fields["version"] = o.Version
		fields["path"] = o.Path
		fields["timestamp"] = o.Timestamp
	}
	return json.Marshal(fields)
}

// OriginFromRaw reads an origin, for a caller outside this package that has
// one as JSON.
func OriginFromRaw(raw json.RawMessage) (*Origin, error) { return originFromRaw(raw) }

// originFromRaw reads an origin.
func originFromRaw(raw json.RawMessage) (*Origin, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var fields struct {
		Kind      string `json:"kind"`
		Version   *int   `json:"version"`
		Path      string `json:"path"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return &Origin{
		Kind: fields.Kind, Version: fields.Version,
		Path: fields.Path, Timestamp: fields.Timestamp,
	}, nil
}

// Change is one entry in the history.
type Change struct {
	Operations []Operation
	Timestamp  time.Time
	// Authors is the old author list, which may contain nulls for anonymous
	// authors.
	Authors []json.RawMessage
	// V2Authors is the current one.
	V2Authors []json.RawMessage
	Origin    *Origin
	// ProjectVersion and V2DocVersions place the change against what the
	// editor and document-updater think the project is at.
	ProjectVersion string
	V2DocVersions  json.RawMessage
}

// rawChange is the wire form.
type rawChange struct {
	Operations []json.RawMessage `json:"operations"`
	Timestamp  string            `json:"timestamp"`
	Authors    []json.RawMessage `json:"authors"`
	// Not omitempty: a change with no v2 authors still carries the empty list,
	// because the reference implementation defaults it to one and then writes
	// whatever it has.
	V2Authors      []json.RawMessage `json:"v2Authors"`
	Origin         json.RawMessage   `json:"origin,omitempty"`
	ProjectVersion string            `json:"projectVersion,omitempty"`
	V2DocVersions  json.RawMessage   `json:"v2DocVersions,omitempty"`
}

// MarshalJSON writes the change.
func (c *Change) MarshalJSON() ([]byte, error) {
	operations := make([]json.RawMessage, 0, len(c.Operations))
	for _, op := range c.Operations {
		encoded, err := op.Raw()
		if err != nil {
			return nil, err
		}
		operations = append(operations, encoded)
	}

	raw := rawChange{
		Operations:     operations,
		Timestamp:      formatTimestamp(c.Timestamp),
		Authors:        c.Authors,
		V2Authors:      c.V2Authors,
		ProjectVersion: c.ProjectVersion,
		V2DocVersions:  c.V2DocVersions,
	}
	if c.Origin != nil {
		encoded, err := c.Origin.Raw()
		if err != nil {
			return nil, err
		}
		raw.Origin = encoded
	}
	return json.Marshal(raw)
}

// UnmarshalJSON reads a change.
func (c *Change) UnmarshalJSON(data []byte) error {
	var raw rawChange
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Timestamp == "" {
		return errors.New("change: bad timestamp")
	}

	operations := make([]Operation, 0, len(raw.Operations))
	for _, encoded := range raw.Operations {
		op, err := operationFromRaw(encoded)
		if err != nil {
			return err
		}
		operations = append(operations, op)
	}

	timestamp, err := parseTimestamp(raw.Timestamp)
	if err != nil {
		return fmt.Errorf("change: %w", err)
	}
	origin, err := originFromRaw(raw.Origin)
	if err != nil {
		return err
	}

	// Both author lists default to empty rather than absent, which is what the
	// reference implementation does when it builds a change.
	v2Authors := raw.V2Authors
	if v2Authors == nil {
		v2Authors = []json.RawMessage{}
	}

	// An author recorded as 0 is old bad data meaning anonymous, which is null
	// now. The rows are still out there, so they are still read.
	authors := make([]json.RawMessage, 0, len(raw.Authors))
	for _, author := range raw.Authors {
		if string(author) == "0" {
			authors = append(authors, json.RawMessage("null"))
			continue
		}
		authors = append(authors, author)
	}

	c.Operations = operations
	c.Timestamp = timestamp
	c.Authors = authors
	c.V2Authors = v2Authors
	c.Origin = origin
	c.ProjectVersion = raw.ProjectVersion
	c.V2DocVersions = raw.V2DocVersions
	return nil
}
