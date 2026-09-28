package projecthistory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// The history stores changes as scans over the whole document, with the
// tracked deletions still in the text. The editor shows a history as a list of
// what somebody inserted and what somebody deleted, at positions in the text
// as it is displayed, with the tracked deletions taken out.
//
// Turning the first into the second is what this does, and the hard part is
// that a tracked deletion is text the history has and the editor does not.
// Marking a stretch as deleted has to come out as a deletion even though the
// text is still there, and unmarking one has to come out as an insertion even
// though nothing was typed.

// SummarizedUpdate is one change as the editor lists it.
type SummarizedUpdate struct {
	Meta       UpdateMeta       `json:"meta"`
	V          int              `json:"v"`
	Pathnames  []string         `json:"pathnames"`
	ProjectOps []map[string]any `json:"project_ops"`
}

// UpdateMeta is who made a change and when.
type UpdateMeta struct {
	Users   []json.RawMessage `json:"users"`
	StartTS int64             `json:"start_ts"`
	EndTS   int64             `json:"end_ts"`
	Origin  json.RawMessage   `json:"origin,omitempty"`
}

// DiffUpdate is one change to one document, as the editor's diff wants it.
type DiffUpdate struct {
	Meta UpdateMeta `json:"meta"`
	V    int        `json:"v"`
	Op   []DiffOp   `json:"op"`
}

// DiffOp is an insertion or a deletion at a position.
type DiffOp struct {
	Insert *string `json:"i,omitempty"`
	Delete *string `json:"d,omitempty"`
	Pos    int     `json:"p"`
}

// DiffUpdates is a document's starting content and what happened to it.
type DiffUpdates struct {
	// Binary is set for a file that is not text, which has no diff.
	Binary         bool
	InitialContent string
	Updates        []DiffUpdate
}

// ConvertToSummarizedUpdates lists a chunk's changes for the editor's history
// view.
func ConvertToSummarizedUpdates(chunk *ChunkResponse) ([]SummarizedUpdate, error) {
	builder, err := newUpdateSetBuilder(chunk)
	if err != nil {
		return nil, err
	}
	for _, change := range chunk.Chunk.History.Changes {
		if err := builder.applyChange(change); err != nil {
			return nil, err
		}
	}
	return builder.updates, nil
}

// ConvertToDiffUpdates gathers what happened to one document between two
// versions.
func (t *ChunkTranslator) ConvertToDiffUpdates(ctx context.Context, projectID string,
	chunk *ChunkResponse, pathname string, fromVersion,
	toVersion int) (*DiffUpdates, error) {

	builder, err := newUpdateSetBuilder(chunk)
	if err != nil {
		return nil, err
	}

	version := chunk.Chunk.StartVersion
	var file *translatedFile
	for _, change := range chunk.Chunk.History.Changes {
		// The file is looked up by path, and a path can change, so the last
		// one in the range that had this path is the one wanted. It may not
		// exist yet at the version the diff starts from.
		if fromVersion <= version && version <= toVersion {
			if current, ok := builder.files[pathname]; ok && current != nil {
				file = current
			}
		}
		if err := builder.applyChange(change); err != nil {
			return nil, err
		}
		version++
	}
	// Versions are fence posts and changes are what is between them, so the
	// version after the last change has to be looked at as well.
	if fromVersion <= version && version <= toVersion {
		if current, ok := builder.files[pathname]; ok && current != nil {
			file = current
		}
	}

	if current, ok := builder.files[pathname]; ok && current == nil {
		// The path was marked as missing while the changes were walked, which
		// means the history refers to a file it does not have. The diff is
		// empty rather than an error: the rest of the history is still worth
		// showing.
		return &DiffUpdates{InitialContent: "", Updates: []DiffUpdate{}}, nil
	}
	if file == nil {
		return nil, fmt.Errorf("%w: pathname %q not found in range",
			ErrNotFound, pathname)
	}

	historyID, err := t.web.GetHistoryID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return file.diffUpdates(ctx, t.store, historyID, fromVersion, toVersion)
}

// ChunkTranslator turns chunks into what the editor shows.
type ChunkTranslator struct {
	store *HistoryStore
	web   *WebAPI
}

// NewChunkTranslator builds it.
func NewChunkTranslator(store *HistoryStore, web *WebAPI) *ChunkTranslator {
	return &ChunkTranslator{store: store, web: web}
}

// updateSetBuilder walks a chunk's changes, keeping track of what the project
// looks like as it goes.
type updateSetBuilder struct {
	version int
	updates []SummarizedUpdate
	// files maps a path to the file at it. A path present with a nil file is
	// one the history refers to but does not have; it is kept rather than
	// removed so that later operations on it can be recognised as the same
	// problem rather than as a new one.
	files map[string]*translatedFile
	// order keeps the paths in the order they were first seen.
	order []string

	current *SummarizedUpdate
	// touched are the paths the change being built has changed.
	touched map[string]bool
}

// newUpdateSetBuilder starts from a chunk's snapshot.
func newUpdateSetBuilder(chunk *ChunkResponse) (*updateSetBuilder, error) {
	builder := &updateSetBuilder{
		version: chunk.Chunk.StartVersion,
		updates: []SummarizedUpdate{},
		files:   map[string]*translatedFile{},
	}
	if chunk.Chunk.History.Snapshot == nil {
		return builder, nil
	}
	for _, path := range chunk.Chunk.History.Snapshot.Files.Paths() {
		file := chunk.Chunk.History.Snapshot.Files.GetFile(path)
		builder.setFile(path, &translatedFile{
			pathname: path, data: file, initialVersion: builder.version,
		})
	}
	return builder, nil
}

// setFile records a file at a path, keeping the order paths were first seen
// in.
func (b *updateSetBuilder) setFile(path string, file *translatedFile) {
	if _, seen := b.files[path]; !seen {
		b.order = append(b.order, path)
	}
	b.files[path] = file
}

// removeFile forgets a path entirely.
func (b *updateSetBuilder) removeFile(path string) {
	if _, seen := b.files[path]; !seen {
		return
	}
	delete(b.files, path)
	kept := b.order[:0]
	for _, existing := range b.order {
		if existing != path {
			kept = append(kept, existing)
		}
	}
	b.order = kept
}

// applyChange walks one change.
func (b *updateSetBuilder) applyChange(change *histmodel.Change) error {
	timestamp := change.Timestamp
	authors := append([]json.RawMessage{}, change.Authors...)
	authors = append(authors, change.V2Authors...)

	b.current = &SummarizedUpdate{
		Meta: UpdateMeta{
			Users:   authors,
			StartTS: timestamp.UnixMilli(),
			EndTS:   timestamp.UnixMilli(),
		},
		V:          b.version,
		Pathnames:  []string{},
		ProjectOps: []map[string]any{},
	}
	b.touched = map[string]bool{}
	if change.Origin != nil {
		origin, err := change.Origin.Raw()
		if err != nil {
			return err
		}
		b.current.Meta.Origin = origin
	}

	for _, op := range change.Operations {
		if err := b.applyOperation(op, timestamp, authors, b.current.Meta.Origin); err != nil {
			return err
		}
	}

	// In the order the paths were first seen, so that the list does not change
	// from run to run for the same chunk.
	for _, path := range b.order {
		if b.touched[path] {
			b.current.Pathnames = append(b.current.Pathnames, path)
		}
	}
	b.updates = append(b.updates, *b.current)
	b.version++
	return nil
}

// applyOperation walks one operation of a change.
func (b *updateSetBuilder) applyOperation(op histmodel.Operation,
	timestamp time.Time, authors []json.RawMessage, origin json.RawMessage) error {

	switch typed := op.(type) {
	case *histmodel.EditFileOperation:
		return b.applyEdit(typed, timestamp, authors, origin)

	case *histmodel.MoveFileOperation:
		if typed.NewPath == "" {
			return b.applyRemove(typed)
		}
		return b.applyRename(typed)

	case *histmodel.AddFileOperation:
		b.setFile(typed.Path, &translatedFile{
			pathname: typed.Path, data: typed.File, initialVersion: b.version,
		})
		b.current.ProjectOps = append(b.current.ProjectOps,
			map[string]any{"add": map[string]any{"pathname": typed.Path}})
	}
	return nil
}

// applyEdit records an edit against the file it changes.
func (b *updateSetBuilder) applyEdit(op *histmodel.EditFileOperation,
	timestamp time.Time, authors []json.RawMessage, origin json.RawMessage) error {

	if op.Path == "" {
		// This should not happen, but the rest of the history is still worth
		// showing.
		return nil
	}
	file, ok := b.files[op.Path]
	if !ok || file == nil {
		// The history refers to a file it does not have. It is marked as
		// missing rather than refused, for the same reason.
		b.setFile(op.Path, nil)
		return nil
	}

	file.operations = append(file.operations, translatedOperation{
		authors: authors, timestamp: timestamp, version: b.version,
		operation: op, origin: origin,
	})
	b.touched[op.Path] = true
	return nil
}

// applyRename moves a file.
func (b *updateSetBuilder) applyRename(op *histmodel.MoveFileOperation) error {
	file, ok := b.files[op.Path]
	if !ok || file == nil {
		b.setFile(op.Path, nil)
		return nil
	}
	file.pathname = op.NewPath
	b.removeFile(op.Path)
	b.setFile(op.NewPath, file)
	b.current.ProjectOps = append(b.current.ProjectOps, map[string]any{
		"rename": map[string]any{
			"pathname": op.Path, "newPathname": op.NewPath,
		},
	})
	return nil
}

// applyRemove takes a file out.
func (b *updateSetBuilder) applyRemove(op *histmodel.MoveFileOperation) error {
	file, ok := b.files[op.Path]
	if !ok || file == nil {
		b.setFile(op.Path, nil)
		return nil
	}
	b.removeFile(op.Path)
	b.current.ProjectOps = append(b.current.ProjectOps,
		map[string]any{"remove": map[string]any{"pathname": op.Path}})
	return nil
}

// translatedOperation is one edit to a file, with who made it.
type translatedOperation struct {
	authors   []json.RawMessage
	timestamp time.Time
	version   int
	operation *histmodel.EditFileOperation
	origin    json.RawMessage
}

// translatedFile is one file and the edits made to it in a chunk.
type translatedFile struct {
	pathname       string
	data           *histmodel.File
	initialVersion int
	operations     []translatedOperation
}

// diffUpdates replays the edits and reports what the editor should show.
func (f *translatedFile) diffUpdates(ctx context.Context, store *HistoryStore,
	historyID string, fromVersion, toVersion int) (*DiffUpdates, error) {

	if f.data == nil || f.data.Data.StringLength() < 0 {
		// Not text: there is nothing to diff.
		return &DiffUpdates{Binary: true}, nil
	}

	content, trackedChanges, err := f.loadContentAndRanges(ctx, store, historyID)
	if err != nil {
		return nil, err
	}

	initialSet := false
	initialContent := ""
	updates := []DiffUpdate{}

	for _, info := range f.operations {
		if info.operation.TextOperation == nil {
			// Only text operations show up in a diff; a comment being
			// resolved is not a change to the text.
			continue
		}

		// The content before this change is what the diff starts from, so it
		// is taken the first time a change inside the range is reached.
		if info.version >= fromVersion && !initialSet {
			initialContent = removeTrackedDeletes(content, trackedChanges)
			initialSet = true
		}

		builder := newTextUpdateBuilder(content, trackedChanges)
		if err := builder.apply(info.operation.TextOperation); err != nil {
			return nil, err
		}
		builder.finish()
		content = builder.result.String()

		if fromVersion <= info.version && info.version < toVersion {
			update := DiffUpdate{
				Meta: UpdateMeta{
					Users:   info.authors,
					StartTS: info.timestamp.UnixMilli(),
					EndTS:   info.timestamp.UnixMilli(),
					Origin:  info.origin,
				},
				V:  info.version,
				Op: builder.changes,
			}
			updates = append(updates, update)
		}
	}

	if !initialSet {
		initialContent = removeTrackedDeletes(content, trackedChanges)
	}
	return &DiffUpdates{InitialContent: initialContent, Updates: updates}, nil
}

// loadContentAndRanges fetches the file's text and its marks.
func (f *translatedFile) loadContentAndRanges(ctx context.Context,
	store *HistoryStore, historyID string) (string, *histmodel.TrackedChangeList, error) {

	hash, rangesHash := "", ""
	switch data := f.data.Data.(type) {
	case *histmodel.LazyStringFileData:
		hash, rangesHash = data.Hash, data.RangesHash
	case *histmodel.HashFileData:
		hash, rangesHash = data.Hash, data.RangesHash
	case *histmodel.StringFileData:
		return data.Content, data.TrackedChanges, nil
	}

	content, err := store.GetProjectBlob(ctx, historyID, hash)
	if err != nil {
		return "", nil, err
	}
	trackedChanges := histmodel.NewTrackedChangeList(nil)
	if rangesHash != "" {
		encoded, err := store.GetProjectBlob(ctx, historyID, rangesHash)
		if err != nil {
			return "", nil, err
		}
		var ranges histmodel.RangesBlob
		if err := json.Unmarshal(encoded, &ranges); err != nil {
			return "", nil, err
		}
		trackedChanges = histmodel.NewTrackedChangeList(ranges.TrackedChanges)
	}
	return string(content), trackedChanges, nil
}

// removeTrackedDeletes is the text as the editor shows it: what the history
// holds, with the stretches marked as deleted taken out.
func removeTrackedDeletes(content string, trackedChanges *histmodel.TrackedChangeList) string {
	var result strings.Builder
	cursor := 0
	for _, change := range trackedChanges.Sorted() {
		if change.Tracking == nil || change.Tracking.Kind != "delete" {
			continue
		}
		if cursor < change.Range.Start() {
			result.WriteString(sliceUTF16(content, cursor, change.Range.Start()))
		}
		cursor = change.Range.End()
	}
	result.WriteString(sliceUTF16(content, cursor, utf16Len(content)))
	return result.String()
}

// textUpdateBuilder replays one text operation, building both the document it
// leaves behind and the insertions and deletions the editor should show.
type textUpdateBuilder struct {
	trackedChanges *histmodel.TrackedChangeList
	source         string
	sourceCursor   int
	result         strings.Builder
	resultLength   int
	changes        []DiffOp
}

// newTextUpdateBuilder starts from a document and its marks.
func newTextUpdateBuilder(source string,
	trackedChanges *histmodel.TrackedChangeList) *textUpdateBuilder {

	return &textUpdateBuilder{
		trackedChanges: trackedChanges, source: source, changes: []DiffOp{},
	}
}

// apply replays a whole operation.
func (b *textUpdateBuilder) apply(operation *histmodel.TextOperation) error {
	for _, op := range operation.Ops {
		if err := b.applyOp(op); err != nil {
			return err
		}
	}
	return nil
}

// applyOp replays one scan step, moving the marks along with it.
func (b *textUpdateBuilder) applyOp(op histmodel.ScanOp) error {
	switch typed := op.(type) {
	case *histmodel.RetainOp:
		at := b.resultLength
		b.applyRetain(typed)
		return b.trackedChanges.ApplyRetain(at, typed.Length, typed.Tracking)

	case *histmodel.InsertOp:
		at := b.resultLength
		b.applyInsert(typed)
		var tracking *histmodel.TrackingProps
		if props, ok := typed.Tracking.(*histmodel.TrackingProps); ok {
			tracking = props
		}
		return b.trackedChanges.ApplyInsert(at, utf16Len(typed.Text), tracking)

	case *histmodel.RemoveOp:
		at := b.resultLength
		b.applyDelete(typed)
		return b.trackedChanges.ApplyDelete(at, typed.Length)
	}
	return nil
}

// write appends to the document being built.
func (b *textUpdateBuilder) write(text string) {
	b.result.WriteString(text)
	b.resultLength += utf16Len(text)
}

// overlappingTrackedDeletes are the stretches marked as deleted that a range
// runs over, in order.
func (b *textUpdateBuilder) overlappingTrackedDeletes(
	over histmodel.Range) []histmodel.TrackedChange {

	var found []histmodel.TrackedChange
	for _, change := range b.trackedChanges.Sorted() {
		if change.Tracking == nil || change.Tracking.Kind != "delete" {
			continue
		}
		if change.Range.Overlaps(over) {
			found = append(found, change)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		return found[i].Range.Start() < found[j].Range.Start()
	})
	return found
}

// applyRetain keeps a stretch of text, and reports the marking of it as a
// change when the marking is what changed.
//
// Marking text as deleted is a deletion to the editor even though the text
// stays; unmarking it is an insertion even though nothing was typed.
func (b *textUpdateBuilder) applyRetain(retain *histmodel.RetainOp) {
	resultRange := histmodel.Range{Pos: b.resultLength, Length: retain.Length}
	sourceRange := histmodel.Range{Pos: b.sourceCursor, Length: retain.Length}

	scanCursor := b.resultLength
	if retain.Tracking != nil && retain.Tracking.Type() != "" {
		sourceOffset := b.sourceCursor - b.resultLength

		for _, trackedDelete := range b.overlappingTrackedDeletes(resultRange) {
			// Clamped to the start of what is being retained: an earlier
			// insert marked as a deletion can have merged with this one and
			// extended it backwards, and that part has no counterpart in the
			// source.
			clampedStart := maxInt(trackedDelete.Range.Start(), resultRange.Start())
			resultTrackedDelete := histmodel.Range{
				Pos: clampedStart, Length: trackedDelete.Range.End() - clampedStart,
			}
			sourceTrackedDelete := resultTrackedDelete.MoveBy(sourceOffset)

			if scanCursor < resultTrackedDelete.Start() {
				text := sliceUTF16(b.source, b.sourceCursor, sourceTrackedDelete.Start())
				if retain.Tracking.Type() == "delete" {
					b.pushDelete(text)
				}
				b.write(text)
				scanCursor = resultTrackedDelete.Start()
				b.sourceCursor = sourceTrackedDelete.Start()
			}

			endOfResult := minInt(resultTrackedDelete.End(), resultRange.End())
			endOfSource := minInt(sourceTrackedDelete.End(), sourceRange.End())
			text := sliceUTF16(b.source, b.sourceCursor, endOfSource)
			if kind := retain.Tracking.Type(); kind == "none" || kind == "insert" {
				b.pushInsert(text)
			}
			b.write(text)
			scanCursor = endOfResult
			b.sourceCursor = endOfSource

			if scanCursor >= resultRange.End() {
				break
			}
		}
	}

	if scanCursor < resultRange.End() {
		// What is left is not marked as deleted, but marking it now is still
		// a deletion.
		text := sliceUTF16(b.source, b.sourceCursor, sourceRange.End())
		if retain.Tracking != nil && retain.Tracking.Type() == "delete" {
			b.pushDelete(text)
		}
		b.write(text)
	}
	b.sourceCursor = sourceRange.End()
}

// applyInsert puts text in.
func (b *textUpdateBuilder) applyInsert(insert *histmodel.InsertOp) {
	if insert.Tracking == nil || insert.Tracking.Type() != "delete" {
		// Text inserted as a tracked deletion was never shown, so there is
		// nothing to report.
		b.pushInsert(insert.Text)
	}
	b.write(insert.Text)
	// The source cursor does not move: nothing was consumed.
}

// applyDelete takes text out, skipping what was already marked as deleted:
// that text was not being shown, so removing it changes nothing.
func (b *textUpdateBuilder) applyDelete(remove *histmodel.RemoveOp) {
	sourceRange := histmodel.Range{Pos: b.sourceCursor, Length: remove.Length}
	resultRange := histmodel.Range{Pos: b.resultLength, Length: remove.Length}

	scanCursor := b.resultLength
	sourceOffset := b.sourceCursor - b.resultLength

	for _, trackedDelete := range b.overlappingTrackedDeletes(resultRange) {
		clampedStart := maxInt(trackedDelete.Range.Start(), resultRange.Start())
		resultTrackedDelete := histmodel.Range{
			Pos: clampedStart, Length: trackedDelete.Range.End() - clampedStart,
		}
		sourceTrackedDelete := resultTrackedDelete.MoveBy(sourceOffset)

		if scanCursor < resultTrackedDelete.Start() {
			b.pushDelete(sliceUTF16(b.source, b.sourceCursor,
				sourceTrackedDelete.Start()))
		}
		scanCursor = minInt(resultTrackedDelete.End(), resultRange.End())
		b.sourceCursor = minInt(sourceTrackedDelete.End(), sourceRange.End())

		if scanCursor >= resultRange.End() {
			break
		}
	}

	if scanCursor < resultRange.End() {
		b.pushDelete(sliceUTF16(b.source, b.sourceCursor, sourceRange.End()))
	}
	b.sourceCursor = sourceRange.End()
}

// pushInsert records an insertion at the current position.
func (b *textUpdateBuilder) pushInsert(text string) {
	inserted := text
	b.changes = append(b.changes, DiffOp{Insert: &inserted, Pos: b.resultLength})
}

// pushDelete records a deletion at the current position.
func (b *textUpdateBuilder) pushDelete(text string) {
	deleted := text
	b.changes = append(b.changes, DiffOp{Delete: &deleted, Pos: b.resultLength})
}

// finish appends whatever the operation did not cover, and moves every
// reported position back past the tracked deletions in front of it: the editor
// counts in the text it shows, which does not have them.
func (b *textUpdateBuilder) finish() {
	if b.sourceCursor < utf16Len(b.source) {
		b.write(sliceUTF16(b.source, b.sourceCursor, utf16Len(b.source)))
	}

	for i := range b.changes {
		position := b.changes[i].Pos
		hidden := 0
		for _, change := range b.trackedChanges.Sorted() {
			if change.Tracking == nil || change.Tracking.Kind != "delete" {
				continue
			}
			if change.Range.Start() >= position {
				continue
			}
			if change.Range.End() < position {
				hidden += change.Range.Length
				continue
			}
			hidden += position - change.Range.Start()
		}
		b.changes[i].Pos = position - hidden
	}
}

// maxInt is the larger of two numbers.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
