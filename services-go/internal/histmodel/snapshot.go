package histmodel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// A snapshot is the project at one moment: every file it contains, by path. A
// chunk is a snapshot plus the changes that follow it, which is how the history
// is stored -- a starting point and a run of edits, rather than a copy of the
// project per version.
//
// This is the port of FileMap, Snapshot, History and Chunk.

// FileMap is the files of a project, by path.
//
// The order the paths came in is kept, because it is the order they are written
// back in, and two implementations that write the same project differently are
// two implementations somebody has to reconcile later.
type FileMap struct {
	order []string
	files map[string]*File
}

// NewFileMap builds an empty map.
func NewFileMap() *FileMap {
	return &FileMap{files: map[string]*File{}}
}

// Paths returns the paths in the order they were added.
func (m *FileMap) Paths() []string { return append([]string(nil), m.order...) }

// SortedPaths returns the paths in order, for a caller that wants them that way
// rather than as stored.
func (m *FileMap) SortedPaths() []string {
	paths := m.Paths()
	sort.Strings(paths)
	return paths
}

// GetFile returns one file, or nil.
func (m *FileMap) GetFile(path string) *File { return m.files[path] }

// AddFile puts a file in, replacing one at the same path in its existing place.
func (m *FileMap) AddFile(path string, file *File) {
	if _, existing := m.files[path]; !existing {
		m.order = append(m.order, path)
	}
	m.files[path] = file
}

// RemoveFile takes a file out.
func (m *FileMap) RemoveFile(path string) bool {
	if _, ok := m.files[path]; !ok {
		return false
	}
	delete(m.files, path)
	for i, existing := range m.order {
		if existing == path {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	return true
}

// MoveFile renames a file, or removes it when the new path is empty.
func (m *FileMap) MoveFile(from, to string) error {
	file, ok := m.files[from]
	if !ok {
		return fmt.Errorf("%w: %s", ErrFileNotFound, from)
	}
	m.RemoveFile(from)
	if to == "" {
		// An empty new path is how the history records a deletion.
		return nil
	}
	m.AddFile(to, file)
	return nil
}

// ErrFileNotFound is a path the project does not have.
var ErrFileNotFound = errors.New("file not found in the project")

// Length is how many files there are.
func (m *FileMap) Length() int { return len(m.files) }

// MarshalJSON writes the map.
func (m *FileMap) MarshalJSON() ([]byte, error) {
	if m == nil || len(m.order) == 0 {
		return []byte("{}"), nil
	}

	// Written in the order the files were added rather than by sorting them.
	// JSON says nothing about the order of an object's keys, but the file tree
	// this becomes is walked in the order it is read, and a project whose
	// files came out alphabetically would list them in a different order from
	// the one that wrote them.
	var out bytes.Buffer
	out.WriteByte('{')
	for index, path := range m.order {
		if index > 0 {
			out.WriteByte(',')
		}
		key, err := json.Marshal(path)
		if err != nil {
			return nil, err
		}
		out.Write(key)
		out.WriteByte(':')

		value, err := json.Marshal(m.files[path])
		if err != nil {
			return nil, err
		}
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

// UnmarshalJSON reads a map.
func (m *FileMap) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	*m = *NewFileMap()
	// In the order the object was written in, not sorted: that order is the
	// file tree's order, and the diffs the editor shows are built by walking
	// it.
	for _, path := range objectKeysInOrder(data) {
		if _, ok := fields[path]; !ok {
			continue
		}
		if string(fields[path]) == "null" {
			m.AddFile(path, nil)
			continue
		}
		var file File
		if err := json.Unmarshal(fields[path], &file); err != nil {
			return fmt.Errorf("file %s: %w", path, err)
		}
		m.AddFile(path, &file)
	}
	return nil
}

// objectKeysInOrder is the keys of a JSON object in the order they were
// written.
func objectKeysInOrder(data []byte) []string {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if _, err := decoder.Token(); err != nil {
		return nil
	}
	var keys []string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return keys
		}
		key, ok := token.(string)
		if !ok {
			return keys
		}
		keys = append(keys, key)
		var skipped json.RawMessage
		if err := decoder.Decode(&skipped); err != nil {
			return keys
		}
	}
	return keys
}

// Snapshot is the project at one moment.
type Snapshot struct {
	Files *FileMap
	// ProjectVersion and V2DocVersions place the snapshot against what the
	// editor and document-updater think the project is at.
	ProjectVersion string
	V2DocVersions  json.RawMessage
	Timestamp      string
}

// NewSnapshot builds an empty snapshot.
func NewSnapshot() *Snapshot { return &Snapshot{Files: NewFileMap()} }

// rawSnapshot is the wire form.
type rawSnapshot struct {
	Files          json.RawMessage `json:"files"`
	ProjectVersion string          `json:"projectVersion,omitempty"`
	V2DocVersions  json.RawMessage `json:"v2DocVersions,omitempty"`
	Timestamp      string          `json:"timestamp,omitempty"`
}

// MarshalJSON writes the snapshot.
func (s *Snapshot) MarshalJSON() ([]byte, error) {
	files, err := json.Marshal(s.Files)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rawSnapshot{
		Files: files, ProjectVersion: s.ProjectVersion,
		V2DocVersions: s.V2DocVersions, Timestamp: s.Timestamp,
	})
}

// UnmarshalJSON reads a snapshot.
func (s *Snapshot) UnmarshalJSON(data []byte) error {
	var raw rawSnapshot
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	files := NewFileMap()
	if len(raw.Files) > 0 && string(raw.Files) != "null" {
		if err := json.Unmarshal(raw.Files, files); err != nil {
			return err
		}
	}
	s.Files = files
	s.ProjectVersion = raw.ProjectVersion
	s.V2DocVersions = raw.V2DocVersions
	s.Timestamp = raw.Timestamp
	return nil
}

// ApplyChange moves the snapshot on by one entry of the history.
//
// An operation naming a file the project does not have is skipped rather than
// treated as a failure. The history holds years of data, some of it written by
// versions of the code that allowed things this one does not, and refusing to
// replay a project because of one such entry would make its whole history
// unreadable. Everything else is a real failure and is reported.
func (s *Snapshot) ApplyChange(change *Change) error {
	for _, op := range change.Operations {
		if err := s.applyOperation(op); err != nil {
			if errors.Is(err, ErrFileNotFound) {
				continue
			}
			return err
		}
	}
	if change.ProjectVersion != "" {
		s.ProjectVersion = change.ProjectVersion
	}
	if len(change.V2DocVersions) > 0 {
		s.V2DocVersions = mergeDocVersions(s.V2DocVersions, change.V2DocVersions)
	}
	// The snapshot carries the time of the change that produced it, which is
	// what a reader of the history shows against the version.
	s.Timestamp = formatTimestamp(change.Timestamp)
	return nil
}

// applyOperation carries out one operation against the snapshot.
func (s *Snapshot) applyOperation(op Operation) error {
	switch typed := op.(type) {
	case *AddFileOperation:
		s.Files.AddFile(typed.Path, typed.File)

	case *MoveFileOperation:
		if err := s.Files.MoveFile(typed.Path, typed.NewPath); err != nil {
			return err
		}

	case *SetFileMetadataOperation:
		file := s.Files.GetFile(typed.Path)
		if file == nil {
			return fmt.Errorf("%w: %s", ErrFileNotFound, typed.Path)
		}
		file.Metadata = typed.Metadata

	case *EditFileOperation:
		file := s.Files.GetFile(typed.Path)
		if file == nil {
			return fmt.Errorf("%w: %s", ErrFileNotFound, typed.Path)
		}
		switch content := file.Data.(type) {
		case *StringFileData:
			if typed.TextOperation == nil {
				// An edit that is not a text operation -- a comment being
				// resolved or deleted -- is applied to the marks.
				return content.ApplyEdit(typed.Edit)
			}
			if err := content.Edit(typed.TextOperation); err != nil {
				return err
			}

		case *LazyStringFileData:
			// The file has not been fetched, and for most of what a snapshot
			// is asked it does not need to be: the edit is recorded against it
			// and the length kept up to date, so the content can be worked out
			// later by replaying them onto the blob.
			if typed.TextOperation != nil {
				length, err := typed.TextOperation.ApplyToLength(content.StringLength_)
				if err != nil {
					return err
				}
				content.StringLength_ = length
			}
			content.Operations = append(content.Operations, typed.Edit)

		default:
			return fmt.Errorf("%w: %s is not editable", ErrApply, typed.Path)
		}

	case *NoOperation:
	}
	return nil
}

// mergeDocVersions puts the versions a change carries over the ones the
// snapshot had.
func mergeDocVersions(existing, update json.RawMessage) json.RawMessage {
	current := map[string]json.RawMessage{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &current)
	}
	incoming := map[string]json.RawMessage{}
	if err := json.Unmarshal(update, &incoming); err != nil {
		return existing
	}
	for doc, version := range incoming {
		current[doc] = version
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return existing
	}
	return encoded
}

// History is a snapshot and the changes that follow it.
type History struct {
	Snapshot *Snapshot `json:"snapshot"`
	Changes  []*Change `json:"changes"`
}

// Chunk is a run of the history, as it is stored and fetched.
type Chunk struct {
	History *History `json:"history"`
	// StartVersion is the version the snapshot is at, so a version number can
	// be turned into a position in the changes.
	StartVersion int `json:"startVersion"`
}

// EndVersion is the version the chunk finishes at.
func (c *Chunk) EndVersion() int {
	if c.History == nil {
		return c.StartVersion
	}
	return c.StartVersion + len(c.History.Changes)
}

// GetSnapshotAt returns the project as it was at a version.
//
// The snapshot the chunk starts from is moved on one change at a time, which is
// how a version is recovered: the history stores a starting point and the edits
// after it, not a copy per version.
func (c *Chunk) GetSnapshotAt(version int) (*Snapshot, error) {
	if c.History == nil || c.History.Snapshot == nil {
		return nil, errors.New("chunk has no history")
	}
	// A version outside the chunk is taken as the nearest end of it rather than
	// refused: the caller is asking what the project looked like, and this
	// chunk can only say what it holds.
	applied := version - c.StartVersion
	if applied < 0 {
		applied = 0
	}
	if applied > len(c.History.Changes) {
		applied = len(c.History.Changes)
	}

	// A copy, so that moving it on does not change the chunk.
	encoded, err := json.Marshal(c.History.Snapshot)
	if err != nil {
		return nil, err
	}
	snapshot := NewSnapshot()
	if err := json.Unmarshal(encoded, snapshot); err != nil {
		return nil, err
	}

	for _, change := range c.History.Changes[:applied] {
		if err := snapshot.ApplyChange(change); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}
