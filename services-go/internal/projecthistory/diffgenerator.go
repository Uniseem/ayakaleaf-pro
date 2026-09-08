package projecthistory

import (
	"encoding/json"
	"fmt"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// The editor shows a diff as the document with each stretch marked as
// unchanged, inserted or deleted, and who did it. What it has to work from is
// the document as it was and the list of edits made since, so the edits are
// applied one at a time to a marked-up copy: an insertion adds a marked piece,
// a deletion turns an unchanged piece into a marked one, and a deletion of
// something that was itself inserted in this range removes it entirely.

// ErrDiffInconsistent is a delete whose text does not match what is there,
// which means the edits and the document have come apart.
var ErrDiffInconsistent = fmt.Errorf("%w: diff is inconsistent", ErrInconsistentChunk)

// DiffPart is one stretch of a diff.
type DiffPart struct {
	// Exactly one of these is set: unchanged, inserted, deleted.
	Unchanged *string `json:"u,omitempty"`
	Insert    *string `json:"i,omitempty"`
	Delete    *string `json:"d,omitempty"`
	// Meta is who made the change and when, absent on unchanged text.
	Meta *UpdateMeta `json:"meta,omitempty"`
}

// content is the text of a part.
func (p DiffPart) content() string {
	switch {
	case p.Unchanged != nil:
		return *p.Unchanged
	case p.Delete != nil:
		return *p.Delete
	case p.Insert != nil:
		return *p.Insert
	}
	return ""
}

// length is how much of the document a part covers.
func (p DiffPart) length() int { return utf16Len(p.content()) }

// slice takes part of a part, keeping what it is and who did it.
func (p DiffPart) slice(from, to int) DiffPart {
	sliced := DiffPart{Meta: p.Meta}
	switch {
	case p.Unchanged != nil:
		text := sliceUTF16(*p.Unchanged, from, to)
		sliced.Unchanged = &text
	case p.Insert != nil:
		text := sliceUTF16(*p.Insert, from, to)
		sliced.Insert = &text
	}
	return sliced
}

// BuildDiff marks up a document with the changes made to it.
func BuildDiff(initialContent string, updates []DiffUpdate) ([]DiffPart, error) {
	content := initialContent
	diff := []DiffPart{{Unchanged: &content}}

	for _, update := range updates {
		var err error
		diff, err = applyUpdateToDiff(diff, update)
		if err != nil {
			return nil, err
		}
	}
	return compressDiff(diff), nil
}

// applyUpdateToDiff marks up the diff with one update's operations.
func applyUpdateToDiff(diff []DiffPart, update DiffUpdate) ([]DiffPart, error) {
	for i := range update.Op {
		var err error
		diff, err = applyOpToDiff(diff, update.Op[i], update.Meta)
		if err != nil {
			return nil, err
		}
	}
	return diff, nil
}

// applyOpToDiff marks up the diff with one operation.
func applyOpToDiff(diff []DiffPart, op DiffOp, meta UpdateMeta) ([]DiffPart, error) {
	consumed, remaining := consumeToOffset(diff, op.Pos)
	result := consumed

	switch {
	case op.Insert != nil:
		recorded := meta
		result = append(result, DiffPart{Insert: op.Insert, Meta: &recorded})

	case op.Delete != nil:
		deleted, rest, err := consumeDiffAffectedByDelete(remaining, op, meta)
		if err != nil {
			return nil, err
		}
		result = append(result, deleted...)
		remaining = rest
	}

	return append(result, remaining...), nil
}

// consumeToOffset splits the diff at a position, keeping what is before it.
//
// Text that has already been deleted takes up no room in the document the
// positions are counted against, so it is passed over rather than counted.
func consumeToOffset(diff []DiffPart, offset int) ([]DiffPart, []DiffPart) {
	consumed := []DiffPart{}
	position := 0

	for index, part := range diff {
		if part.Delete != nil {
			consumed = append(consumed, part)
			continue
		}

		length := part.length()
		if position+length >= offset {
			partOffset := offset - position
			if partOffset > 0 {
				consumed = append(consumed, part.slice(0, partOffset))
			}
			remaining := append([]DiffPart{}, diff[index+1:]...)
			if partOffset < length {
				remaining = append([]DiffPart{part.slice(partOffset, length)}, remaining...)
			}
			return consumed, remaining
		}
		position += length
		consumed = append(consumed, part)
	}
	return consumed, []DiffPart{}
}

// consumeDiffAffectedByDelete marks up everything one delete covers.
func consumeDiffAffectedByDelete(diff []DiffPart, op DiffOp,
	meta UpdateMeta) ([]DiffPart, []DiffPart, error) {

	consumed := []DiffPart{}
	remainingOp := &op

	for remainingOp != nil && len(diff) > 0 {
		part, rest, nextOp, err := consumeDeletedPart(diff, *remainingOp, meta)
		if err != nil {
			return nil, nil, err
		}
		diff = rest
		remainingOp = nextOp
		if part != nil {
			consumed = append(consumed, *part)
		}
	}
	return consumed, diff, nil
}

// consumeDeletedPart marks up as much of one part as a delete covers.
//
// Deleting text that was inserted within the range being shown removes it from
// the diff entirely: it was never in the document the diff started from, and
// showing it as both inserted and deleted would say two things happened where
// nothing did.
func consumeDeletedPart(diff []DiffPart, op DiffOp,
	meta UpdateMeta) (*DiffPart, []DiffPart, *DiffOp, error) {

	part := diff[0]
	rest := append([]DiffPart{}, diff[1:]...)
	partLength := part.length()
	deleted := *op.Delete
	deletedLength := utf16Len(deleted)

	if part.Delete != nil {
		// Already deleted, so this delete passes over it.
		return &part, rest, &op, nil
	}

	switch {
	case partLength > deletedLength:
		// Only the front of the part goes.
		rest = append([]DiffPart{part.slice(deletedLength, partLength)}, rest...)
		if content := sliceUTF16(part.content(), 0, deletedLength); content != deleted {
			return nil, nil, nil, fmt.Errorf(
				"%w: deleted content %q does not match delete op %q",
				ErrDiffInconsistent, content, deleted)
		}
		if part.Unchanged != nil {
			recorded := meta
			return &DiffPart{Delete: op.Delete, Meta: &recorded}, rest, nil, nil
		}
		return nil, rest, nil, nil

	case partLength == deletedLength:
		if content := part.content(); content != deleted {
			return nil, nil, nil, fmt.Errorf(
				"%w: deleted content %q does not match delete op %q",
				ErrDiffInconsistent, content, deleted)
		}
		if part.Unchanged != nil {
			recorded := meta
			return &DiffPart{Delete: op.Delete, Meta: &recorded}, rest, nil, nil
		}
		return nil, rest, nil, nil

	default:
		// The whole part goes and there is more to delete after it.
		content := part.content()
		if want := sliceUTF16(deleted, 0, utf16Len(content)); content != want {
			return nil, nil, nil, fmt.Errorf(
				"%w: deleted content %q does not match delete op %q",
				ErrDiffInconsistent, content, want)
		}

		remainder := sliceUTF16(deleted, partLength, deletedLength)
		nextOp := &DiffOp{Pos: op.Pos, Delete: &remainder}
		if part.Unchanged != nil {
			recorded := meta
			return &DiffPart{Delete: part.Unchanged, Meta: &recorded}, rest, nextOp, nil
		}
		return nil, rest, nextOp, nil
	}
}

// compressDiff joins the neighbouring parts that say the same thing.
//
// The changes a resync made are not shown at all: they were not made by
// anybody, they were the history being corrected, and showing them would look
// like the project rewriting itself.
func compressDiff(diff []DiffPart) []DiffPart {
	compressed := []DiffPart{}

	for _, part := range diff {
		if originKind(part.Meta) == "history-resync" {
			switch {
			case part.Unchanged != nil:
				compressed = append(compressed, part)
			case part.Insert != nil:
				// Text the resync put in was already there as far as anybody
				// reading the history is concerned.
				text := *part.Insert
				compressed = append(compressed, DiffPart{Unchanged: &text})
			}
			// Text the resync removed is left out altogether.
			continue
		}

		if len(compressed) == 0 {
			compressed = append(compressed, part)
			continue
		}

		last := &compressed[len(compressed)-1]
		if !sameUsers(last.Meta, part.Meta) {
			compressed = append(compressed, part)
			continue
		}

		switch {
		case last.Insert != nil && part.Insert != nil:
			joined := *last.Insert + *part.Insert
			last.Insert = &joined
			mergeTimes(last.Meta, part.Meta)

		case last.Delete != nil && part.Delete != nil:
			joined := *last.Delete + *part.Delete
			last.Delete = &joined
			mergeTimes(last.Meta, part.Meta)

		default:
			compressed = append(compressed, part)
		}
	}
	return compressed
}

// originKind is what a change was made by, if it says.
func originKind(meta *UpdateMeta) string {
	if meta == nil || len(meta.Origin) == 0 {
		return ""
	}
	var origin struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(meta.Origin, &origin); err != nil {
		return ""
	}
	return origin.Kind
}

// sameUsers reports whether two parts were made by the same people.
func sameUsers(a, b *UpdateMeta) bool {
	left := map[string]bool{}
	right := map[string]bool{}
	if a != nil {
		for _, user := range a.Users {
			left[string(user)] = true
		}
	}
	if b != nil {
		for _, user := range b.Users {
			right[string(user)] = true
		}
	}
	if len(left) != len(right) {
		return false
	}
	for user := range left {
		if !right[user] {
			return false
		}
	}
	return true
}

// mergeTimes widens a part's times to cover another's.
func mergeTimes(into, from *UpdateMeta) {
	if into == nil || from == nil {
		return
	}
	if from.StartTS < into.StartTS {
		into.StartTS = from.StartTS
	}
	if from.EndTS > into.EndTS {
		into.EndTS = from.EndTS
	}
}

// FileTreeDiffEntry is what happened to one file between two versions.
type FileTreeDiffEntry struct {
	Pathname    string `json:"pathname"`
	NewPathname string `json:"newPathname,omitempty"`
	// Operation is "added", "edited", "renamed" or "removed", and is absent
	// for a file nothing happened to.
	Operation string `json:"operation,omitempty"`
	// Editable says whether the file is text, which the editor needs to know
	// to offer a diff of its contents.
	Editable *bool `json:"editable,omitempty"`
	// DeletedAtV is the version a removal happened at, which is what a restore
	// has to name.
	DeletedAtV *int `json:"deletedAtV,omitempty"`
}

// BuildFileTreeDiff says what happened to a project's files between two
// versions.
func BuildFileTreeDiff(chunk *ChunkResponse, fromVersion,
	toVersion int) ([]FileTreeDiffEntry, error) {

	start := chunk.Chunk.StartVersion
	snapshot, err := chunk.Chunk.GetSnapshotAt(fromVersion)
	if err != nil {
		return nil, err
	}

	// Starting from the file tree as it was, with nothing marked as changed.
	tree := &fileTreeDiff{entries: map[string]*FileTreeDiffEntry{}}
	for _, path := range snapshot.Files.Paths() {
		editable := snapshot.Files.GetFile(path).Data.IsEditable()
		tree.set(path, &FileTreeDiffEntry{Pathname: path, Editable: &editable})
	}

	changes := chunk.Chunk.History.Changes
	first := clampIndex(fromVersion-start, len(changes))
	last := clampIndex(toVersion-start, len(changes))

	for index, change := range changes[first:last] {
		for _, op := range change.Operations {
			if op.Pathname() == "" {
				// An operation on a file with no name says nothing that can be
				// shown, but the rest of the diff still can.
				continue
			}
			if err := tree.apply(op, fromVersion+index); err != nil {
				return nil, err
			}
		}
	}
	return tree.result(), nil
}

// fileTreeDiff is what has happened to each file so far, in the order the
// files were first seen.
type fileTreeDiff struct {
	entries map[string]*FileTreeDiffEntry
	order   []string
}

// set records an entry against a path.
func (t *fileTreeDiff) set(path string, entry *FileTreeDiffEntry) {
	if _, seen := t.entries[path]; !seen {
		t.order = append(t.order, path)
	}
	t.entries[path] = entry
}

// apply records what one operation did.
func (t *fileTreeDiff) apply(op histmodel.Operation, atVersion int) error {
	switch typed := op.(type) {
	case *histmodel.AddFileOperation:
		editable := typed.File.Data.IsEditable()
		t.set(typed.Path, &FileTreeDiffEntry{
			Pathname: typed.Path, Operation: "added", Editable: &editable,
		})

	case *histmodel.EditFileOperation:
		// A file that was added or renamed in this range keeps that as what
		// happened to it: editing it afterwards says nothing more.
		if entry, ok := t.entries[typed.Path]; ok && entry.Operation != "" {
			return nil
		}
		t.set(typed.Path, &FileTreeDiffEntry{
			Pathname: typed.Path, Operation: "edited",
		})

	case *histmodel.MoveFileOperation:
		if typed.NewPath == "" {
			return t.remove(typed.Path, atVersion)
		}
		return t.rename(typed.Path, typed.NewPath)
	}
	return nil
}

// rename moves an entry to a new path.
func (t *fileTreeDiff) rename(from, to string) error {
	if existing, ok := t.entries[to]; ok && existing.Operation != "removed" {
		return fmt.Errorf("%w: trying to move to a file that already exists: %s",
			ErrInconsistentChunk, to)
	}
	entry, ok := t.entries[from]
	if !ok {
		// The history renames a file it does not have; the rest of the diff is
		// still worth showing.
		return nil
	}

	entry.NewPathname = to
	if entry.Operation == "added" {
		// Added and then renamed inside the same range is just an add, at the
		// name it ended up with.
		entry.Pathname = to
		entry.NewPathname = ""
	} else {
		entry.Operation = "renamed"
	}

	delete(t.entries, from)
	t.entries[to] = entry
	for i, path := range t.order {
		if path == from {
			t.order[i] = to
			return nil
		}
	}
	t.order = append(t.order, to)
	return nil
}

// remove marks an entry as gone, and at which version, which is what a restore
// has to name.
func (t *fileTreeDiff) remove(path string, atVersion int) error {
	entry, ok := t.entries[path]
	if !ok {
		return nil
	}
	entry.Operation = "removed"
	version := atVersion
	entry.DeletedAtV = &version
	return nil
}

// result is the entries in the order their paths were first seen.
func (t *fileTreeDiff) result() []FileTreeDiffEntry {
	entries := make([]FileTreeDiffEntry, 0, len(t.entries))
	for _, path := range t.order {
		if entry, ok := t.entries[path]; ok {
			entries = append(entries, *entry)
		}
	}
	return entries
}

// clampIndex keeps a slice index inside a list.
func clampIndex(index, length int) int {
	if index < 0 {
		return 0
	}
	if index > length {
		return length
	}
	return index
}
