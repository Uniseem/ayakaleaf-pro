package projecthistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// An update says what somebody did in the editor: insert this here, delete that
// there. A change says what the project became. Turning one into the other is
// what this does, and it is where the two ways of describing a document meet.
//
// The editor works in positions into the text it shows, with the tracked
// deletions taken out. The history works in a scan over the text it keeps, with
// them left in. So a delete that runs over a tracked deletion is not one delete
// in the history but a delete, a retain over the marked text, and another
// delete -- and a delete made with track changes on is not a delete at all
// there, but a retain that marks the text.
//
// This is the port of UpdateTranslator and OperationsCompressor.

// ErrUnknownUpdateFormat is an update that is none of the shapes this
// understands.
var ErrUnknownUpdateFormat = errors.New("update with unknown format")

// UpdateWithBlob is an update whose file contents have been stored, so the
// change can name them by hash.
type UpdateWithBlob struct {
	Update *Update
	// FileHash and RangesHash name the blobs the contents were stored in.
	FileHash   string
	RangesHash string
}

// ConvertToChanges turns updates into the entries the history stores.
func ConvertToChanges(updates []UpdateWithBlob) ([]*histmodel.Change, error) {
	changes := make([]*histmodel.Change, 0, len(updates))
	for _, update := range updates {
		change, err := convertToChange(update)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, nil
}

// convertToChange turns one update into one change.
func convertToChange(withBlob UpdateWithBlob) (*histmodel.Change, error) {
	update := withBlob.Update

	operations, projectVersion, docVersions, err := operationsForUpdate(withBlob)
	if err != nil {
		return nil, err
	}

	// history-v1 records an anonymous author as a null rather than by name.
	var authors []json.RawMessage
	switch {
	case update.Meta.UserID == "anonymous-user":
		authors = []json.RawMessage{json.RawMessage("null")}
	case update.Meta.UserID != "":
		encoded, err := json.Marshal(update.Meta.UserID)
		if err != nil {
			return nil, err
		}
		authors = []json.RawMessage{encoded}
	default:
		// The user id is missing on the resync updates that send a document's
		// contents, which nobody made.
		authors = []json.RawMessage{}
	}

	change := &histmodel.Change{
		Operations:     operations,
		Timestamp:      time.UnixMilli(update.Meta.Timestamp()).UTC(),
		Authors:        []json.RawMessage{},
		V2Authors:      authors,
		ProjectVersion: projectVersion,
	}
	if len(docVersions) > 0 {
		encoded, err := json.Marshal(docVersions)
		if err != nil {
			return nil, err
		}
		change.V2DocVersions = encoded
	}

	switch {
	case len(update.Meta.Origin) > 0:
		origin, err := originFromJSON(update.Meta.Origin)
		if err != nil {
			return nil, err
		}
		change.Origin = origin
	case update.Meta.Type == "external" && update.Meta.Source != "":
		change.Origin = &histmodel.Origin{Kind: update.Meta.Source}
	}

	change.Operations = compressOperations(change.Operations)
	return change, nil
}

// docVersion is where document-updater had got to for one document when the
// change was made.
type docVersion struct {
	Pathname string `json:"pathname"`
	V        int64  `json:"v"`
}

// operationsForUpdate builds the operations one update becomes.
func operationsForUpdate(withBlob UpdateWithBlob) ([]histmodel.Operation, string,
	map[string]docVersion, error) {

	update := withBlob.Update
	docVersions := map[string]docVersion{}

	switch {
	case isRename(update):
		return []histmodel.Operation{&histmodel.MoveFileOperation{
			Path:    ConvertPathname(update.Pathname),
			NewPath: ConvertPathname(rawString(update.Rest["new_pathname"])),
		}}, projectVersionOf(update), nil, nil

	case isAdd(update):
		file := map[string]any{"hash": withBlob.FileHash}
		if isAddDoc(update) && withBlob.RangesHash != "" {
			file["rangesHash"] = withBlob.RangesHash
		}
		if metadata, ok := update.Rest["metadata"]; ok && isAddFile(update) {
			file["metadata"] = metadata
		}
		encoded, err := json.Marshal(file)
		if err != nil {
			return nil, "", nil, err
		}
		var parsed histmodel.File
		if err := json.Unmarshal(encoded, &parsed); err != nil {
			return nil, "", nil, err
		}
		return []histmodel.Operation{&histmodel.AddFileOperation{
			Path: ConvertPathname(update.Pathname), File: &parsed,
		}}, projectVersionOf(update), nil, nil

	case isHistoryOTEditUpdate(update):
		// A project whose editor speaks the history's own operation type sends
		// operations that are already in the history's form; they need only
		// the path they apply to.
		pathname := ConvertPathname(update.Meta.Pathname)
		operations := make([]histmodel.Operation, 0, len(update.Op))
		for i := range update.Op {
			operation, err := historyOTOperation(pathname, &update.Op[i])
			if err != nil {
				return nil, "", nil, err
			}
			operations = append(operations, operation)
		}
		if update.V != nil {
			docVersions[update.Doc] = docVersion{Pathname: pathname, V: *update.V}
		}
		return operations, "", docVersions, nil

	case update.IsTextUpdate():
		pathname := ConvertPathname(update.Meta.Pathname)
		docLength := 0
		if update.Meta.DocLength != nil {
			docLength = *update.Meta.DocLength
		}
		if update.Meta.HistoryDocLength != nil {
			docLength = *update.Meta.HistoryDocLength
		}

		builder := newOperationsBuilder(docLength, pathname)
		for i := range update.Op {
			if err := builder.addOp(&update.Op[i], update); err != nil {
				return nil, "", nil, err
			}
		}
		if update.Meta.DocHash != "" {
			// Committing here is what attaches the hash to the operation the
			// builder is in the middle of.
			builder.commitTextOperation(update.Meta.DocHash)
		}
		operations, err := builder.finish()
		if err != nil {
			return nil, "", nil, err
		}
		if update.V != nil {
			docVersions[update.Doc] = docVersion{Pathname: pathname, V: *update.V}
		}
		return operations, "", docVersions, nil

	case hasField(update, "commentId") && hasField(update, "resolved"):
		var resolved bool
		_ = json.Unmarshal(update.Rest["resolved"], &resolved)
		edit, err := json.Marshal(map[string]any{
			"commentId": rawString(update.Rest["commentId"]), "resolved": resolved,
		})
		if err != nil {
			return nil, "", nil, err
		}
		return []histmodel.Operation{&histmodel.EditFileOperation{
			Path: ConvertPathname(update.Pathname), Edit: edit,
		}}, "", nil, nil

	case hasField(update, "metadata"):
		return []histmodel.Operation{&histmodel.SetFileMetadataOperation{
			Path: ConvertPathname(update.Pathname), Metadata: update.Rest["metadata"],
		}}, "", nil, nil

	case hasField(update, "deleteComment"):
		edit, err := json.Marshal(map[string]any{
			"deleteComment": rawString(update.Rest["deleteComment"]),
		})
		if err != nil {
			return nil, "", nil, err
		}
		return []histmodel.Operation{&histmodel.EditFileOperation{
			Path: ConvertPathname(update.Pathname), Edit: edit,
		}}, "", nil, nil
	}

	return nil, "", nil, fmt.Errorf("%w", ErrUnknownUpdateFormat)
}

func isRename(update *Update) bool { return hasField(update, "new_pathname") }

func isAdd(update *Update) bool { return isAddDoc(update) || isAddFile(update) }

func isAddDoc(update *Update) bool {
	return update.Doc != "" && hasField(update, "docLines")
}

func isAddFile(update *Update) bool {
	// A file that has already been stored says so, and then it carries no url:
	// there is nothing left to fetch.
	return hasField(update, "file") &&
		(isTrue(update.Rest["createdBlob"]) || hasField(update, "url"))
}

func hasField(update *Update, name string) bool {
	value, ok := update.Rest[name]
	return ok && string(value) != "null"
}

func rawString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func projectVersionOf(update *Update) string {
	return rawString(update.Rest["version"])
}

// originFromJSON reads an origin off an update.
func originFromJSON(raw json.RawMessage) (*histmodel.Origin, error) {
	var fields struct {
		Kind      string `json:"kind"`
		Version   *int   `json:"version"`
		Path      string `json:"path"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return &histmodel.Origin{
		Kind: fields.Kind, Version: fields.Version,
		Path: fields.Path, Timestamp: fields.Timestamp,
	}, nil
}

// ConvertPathname makes a path the history store will accept.
//
// Each of these is a workaround for something that used to be allowed in a
// project and is not any more. The files are still there, so the history still
// has to be able to name them.
func ConvertPathname(pathname string) string {
	// The history stores paths without a leading slash.
	pathname = strings.TrimPrefix(pathname, "/")
	// Backslashes are no longer allowed in a project, but old projects have
	// them.
	pathname = strings.ReplaceAll(pathname, `\`, "_")
	// An asterisk cannot be stored, so it is spelled out.
	pathname = strings.ReplaceAll(pathname, "*", "__ASTERISK__")
	// Nor can a leading space, at the top level or inside a folder.
	if strings.HasPrefix(pathname, " ") {
		pathname = "__SPACE__" + pathname[1:]
	}
	pathname = strings.ReplaceAll(pathname, "/ ", "/__SPACE__")
	return pathname
}

// operationsBuilder turns a run of editor operations into history operations.
//
// It keeps a text operation under construction and a cursor into it. An
// operation that cannot be expressed as part of the current scan -- a comment,
// or an edit before where the scan has reached -- commits what is there and
// starts again.
type operationsBuilder struct {
	operations    []histmodel.Operation
	textOperation []json.RawMessage
	cursor        int
	docLength     int
	pathname      string
}

func newOperationsBuilder(docLength int, pathname string) *operationsBuilder {
	return &operationsBuilder{docLength: docLength, pathname: pathname}
}

// addOp folds one editor operation into the scan.
func (b *operationsBuilder) addOp(op *Op, update *Update) error {
	// An operation can name a position past the end of the document; the editor
	// treats that as the end, and so does this.
	pos := op.Pos
	if op.HPos != nil {
		pos = *op.HPos
	}
	if pos > b.docLength {
		pos = b.docLength
	}

	if op.IsComment() {
		// A comment is not part of the scan, so whatever is under
		// construction is finished first.
		b.commitTextOperation("")

		// The comment text, not Op.Size: that reports the length of an insert
		// or a delete and nothing else, because the compressor relies on a
		// comment having no size to keep from merging one.
		length := utf16Len(*op.Comment)
		if op.HLen != nil {
			length = *op.HLen
		}
		ranges := []any{}
		if length > 0 {
			ranges = append(ranges, map[string]any{"pos": pos, "length": length})
		}
		comment := map[string]any{"commentId": op.Thread, "ranges": ranges}
		if op.Resolved != nil && *op.Resolved {
			// An unresolved comment says nothing: the model defaults to it and
			// leaves the field out again when it writes the operation back.
			comment["resolved"] = true
		}
		encoded, err := json.Marshal(comment)
		if err != nil {
			return err
		}
		b.operations = append(b.operations, &histmodel.EditFileOperation{
			Path: b.pathname, Edit: encoded,
		})
		return nil
	}

	if !op.IsInsert() && !op.IsDelete() && op.Retain == nil {
		return fmt.Errorf("%w: %+v", ErrUnexpectedOp, op)
	}

	if pos < b.cursor {
		// The scan has already gone past here, so it is finished and a new one
		// begins at the start of the document.
		b.commitTextOperation("")
	}
	if pos > b.cursor {
		b.retain(pos-b.cursor, nil)
	}

	switch {
	case op.IsInsert():
		if op.TrackedDeleteRejection {
			// Putting back what a tracked delete removed: in the history the
			// text never left, so this only takes the mark off it.
			b.retain(op.Size(), json.RawMessage(`{"type":"none"}`))
			return nil
		}
		var tracking json.RawMessage
		if update.IsTracked() {
			tracking = trackingProps("insert", update)
		}
		b.insert(*op.Insert, tracking, op.CommentIDs)

	case op.Retain != nil:
		b.retain(utf16Len(*op.Retain), op.Tracking)

	case op.IsDelete():
		b.addDelete(op, update)
	}
	return nil
}

// addDelete folds a delete into the scan, working around the tracked changes it
// runs over.
func (b *operationsBuilder) addDelete(op *Op, update *Update) {
	changes := append([]TrackedChangeInOp(nil), op.TrackedChanges...)
	// They should already be in order, with a deletion before an insertion at
	// the same place; making sure costs nothing and the arithmetic below
	// depends on it.
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Offset != changes[j].Offset {
			return changes[i].Offset < changes[j].Offset
		}
		return changes[i].Type == "delete" && changes[j].Type == "insert"
	})

	var offset int
	for _, change := range changes {
		if change.Offset > offset {
			b.deletePortion(change.Offset-offset, update)
			offset = change.Offset
		}

		switch change.Type {
		case "delete":
			// The history keeps text somebody deleted with track changes on,
			// so a delete over it steps past rather than through.
			b.retain(change.Length, nil)
		case "insert":
			// Deleting text somebody had just added takes it away outright,
			// whether or not track changes is on.
			b.delete(change.Length)
			offset += change.Length
		}
	}

	if offset < op.Size() {
		b.deletePortion(op.Size()-offset, update)
	}
}

// deletePortion removes a stretch, or marks it as deleted when track changes is
// on.
func (b *operationsBuilder) deletePortion(length int, update *Update) {
	if update.IsTracked() {
		b.retain(length, trackingProps("delete", update))
		return
	}
	b.delete(length)
}

// trackingProps builds the mark an edit made with track changes on leaves.
func trackingProps(kind string, update *Update) json.RawMessage {
	encoded, _ := json.Marshal(map[string]any{
		"type": kind, "userId": update.Meta.UserID,
		"ts": time.UnixMilli(update.Meta.Timestamp()).UTC().Format("2006-01-02T15:04:05.000Z"),
	})
	return encoded
}

func (b *operationsBuilder) retain(length int, tracking json.RawMessage) {
	if length == 0 {
		// A retain of nothing is dropped by the model, so it is not written.
		return
	}
	if tracking != nil {
		b.textOperation = append(b.textOperation, mustJSON(map[string]any{
			"r": length, "tracking": tracking,
		}))
	} else {
		b.textOperation = append(b.textOperation, mustJSON(length))
	}
	b.cursor += length
}

func (b *operationsBuilder) insert(text string, tracking json.RawMessage, commentIDs []string) {
	if tracking != nil || commentIDs != nil {
		fields := map[string]any{"i": text}
		if tracking != nil {
			fields["tracking"] = tracking
		}
		if commentIDs != nil {
			fields["commentIds"] = commentIDs
		}
		b.textOperation = append(b.textOperation, mustJSON(fields))
	} else {
		b.textOperation = append(b.textOperation, mustJSON(text))
	}
	b.cursor += utf16Len(text)
	b.docLength += utf16Len(text)
}

func (b *operationsBuilder) delete(length int) {
	b.textOperation = append(b.textOperation, mustJSON(-length))
	b.docLength -= length
}

// commitTextOperation finishes the scan under construction.
//
// A scan has to cover the whole document, so whatever is left after the last
// edit is retained.
func (b *operationsBuilder) commitTextOperation(contentHash string) {
	if len(b.textOperation) > 0 && b.cursor < b.docLength {
		b.retain(b.docLength-b.cursor, nil)
	}
	if len(b.textOperation) > 0 {
		edit := map[string]any{"textOperation": b.textOperation}
		if contentHash != "" {
			edit["contentHash"] = contentHash
		}
		b.operations = append(b.operations, &histmodel.EditFileOperation{
			Path: b.pathname, Edit: mustJSON(edit),
		})
		b.textOperation = nil
	}
	b.cursor = 0
}

func (b *operationsBuilder) finish() ([]histmodel.Operation, error) {
	b.commitTextOperation("")

	// The operations are read back through the model, which is what normalises
	// them: adjacent retains become one, and an insert next to a delete is put
	// in the order the model writes it.
	normalised := make([]histmodel.Operation, 0, len(b.operations))
	for _, op := range b.operations {
		encoded, err := op.Raw()
		if err != nil {
			return nil, err
		}
		var change histmodel.Change
		if err := json.Unmarshal(mustJSON(map[string]any{
			"operations": []json.RawMessage{encoded},
			"timestamp":  "2026-01-01T00:00:00.000Z",
		}), &change); err != nil {
			return nil, err
		}
		normalised = append(normalised, change.Operations...)
	}
	return normalised, nil
}

// compressOperations joins the consecutive edits to one file into one.
//
// A run of edits that were split only because the builder had to start a new
// scan is one edit as far as the history is concerned.
func compressOperations(operations []histmodel.Operation) []histmodel.Operation {
	if len(operations) == 0 {
		return []histmodel.Operation{}
	}

	compressed := []histmodel.Operation{}
	current := operations[0]
	for _, next := range operations[1:] {
		merged, ok := composeOperations(current, next)
		if !ok {
			compressed = append(compressed, current)
			current = next
			continue
		}
		current = merged
	}
	return append(compressed, current)
}

// composeOperations returns the two edits as one, when they can be.
func composeOperations(first, second histmodel.Operation) (histmodel.Operation, bool) {
	a, ok := first.(*histmodel.EditFileOperation)
	if !ok {
		return nil, false
	}
	b, ok := second.(*histmodel.EditFileOperation)
	if !ok || a.Path != b.Path {
		return nil, false
	}
	if a.TextOperation == nil || b.TextOperation == nil {
		// A text operation composes with nothing but another text operation,
		// but two edits about the same comment may still merge.
		edit, ok := composeCommentEdits(a.Edit, b.Edit)
		if !ok {
			return nil, false
		}
		return &histmodel.EditFileOperation{Path: a.Path, Edit: edit}, true
	}
	// The second has to start where the first finishes.
	if a.TextOperation.TargetLength != b.TextOperation.BaseLength {
		return nil, false
	}

	composed, err := a.TextOperation.Compose(b.TextOperation)
	if err != nil {
		return nil, false
	}
	encoded, err := json.Marshal(composed)
	if err != nil {
		return nil, false
	}
	return &histmodel.EditFileOperation{
		Path: a.Path, Edit: encoded, TextOperation: composed,
	}, true
}

func mustJSON(v any) json.RawMessage {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return encoded
}

// An edit is one of five things, and which one is decided by the fields it
// carries rather than by anything it says about itself. The order matters: an
// edit with a commentId, ranges and resolved is a comment being added with its
// state, not a state being set.
type editKind int

const (
	editUnknown editKind = iota
	editText
	editAddComment
	editDeleteComment
	editSetCommentState
	editNoOp
)

// classifyEdit says what an edit is, and hands back its fields.
func classifyEdit(raw json.RawMessage) (editKind, map[string]json.RawMessage) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return editUnknown, nil
	}
	_, named := fields["commentId"]
	switch {
	case hasKey(fields, "textOperation"):
		return editText, fields
	case named && isJSONArray(fields["ranges"]):
		return editAddComment, fields
	case hasKey(fields, "deleteComment"):
		return editDeleteComment, fields
	case named && isJSONBool(fields["resolved"]):
		return editSetCommentState, fields
	case hasKey(fields, "noOp"):
		return editNoOp, fields
	}
	return editUnknown, fields
}

// composeCommentEdits merges two edits about the same comment into one.
//
// Which pairs merge is not symmetric. Adding a comment and then saying
// anything else about it leaves only what was said last, because the add
// carries the ranges and the rest only qualifies them -- except a resolve,
// which keeps the ranges and takes the state. A comment that has been deleted
// composes with nothing after it.
func composeCommentEdits(first, second json.RawMessage) (json.RawMessage, bool) {
	a, aFields := classifyEdit(first)
	b, bFields := classifyEdit(second)

	id := commentIDOf(a, aFields)
	if id == "" || id != commentIDOf(b, bFields) {
		return nil, false
	}

	switch {
	case a == editAddComment && b == editAddComment,
		a == editAddComment && b == editDeleteComment,
		a == editSetCommentState && b == editSetCommentState,
		a == editSetCommentState && b == editDeleteComment:
		return second, true

	case a == editAddComment && b == editSetCommentState:
		merged := map[string]json.RawMessage{
			"commentId": aFields["commentId"],
			"ranges":    aFields["ranges"],
		}
		if string(bFields["resolved"]) == "true" {
			merged["resolved"] = json.RawMessage("true")
		}
		encoded, err := json.Marshal(merged)
		if err != nil {
			return nil, false
		}
		return encoded, true
	}
	return nil, false
}

// commentIDOf is the comment an edit is about, if it is about one.
func commentIDOf(kind editKind, fields map[string]json.RawMessage) string {
	switch kind {
	case editAddComment, editSetCommentState:
		return rawString(fields["commentId"])
	case editDeleteComment:
		return rawString(fields["deleteComment"])
	}
	return ""
}

func hasKey(fields map[string]json.RawMessage, name string) bool {
	_, ok := fields[name]
	return ok
}

func isJSONArray(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "[")
}

func isJSONBool(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "true" || trimmed == "false"
}

// isHistoryOTEditUpdate reports whether an update carries operations that are
// already in the history's form.
func isHistoryOTEditUpdate(update *Update) bool {
	if update.Doc == "" || update.Op == nil || update.Meta.Pathname == "" {
		return false
	}
	if len(update.Op) == 0 {
		return false
	}
	return isHistoryOTOp(&update.Op[0])
}

// isHistoryOTOp reports whether one operation is in the history's form.
func isHistoryOTOp(op *Op) bool {
	raw := op.Raw()
	if len(raw) == 0 {
		return false
	}
	kind, _ := classifyEdit(raw)
	return kind != editUnknown
}

// historyOTOperation turns an operation that is already in the history's form
// into one against a file.
func historyOTOperation(pathname string, op *Op) (histmodel.Operation, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(op.Raw(), &fields); err != nil {
		return nil, err
	}
	delete(fields, "pathname")

	edit, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}

	operation := &histmodel.EditFileOperation{Path: pathname, Edit: edit}
	if kind, _ := classifyEdit(edit); kind == editText {
		var text histmodel.TextOperation
		if err := json.Unmarshal(edit, &text); err != nil {
			return nil, err
		}
		operation.TextOperation = &text
	}
	return operation, nil
}

// composeHistoryOTOps merges two operations that are already in the history's
// form, when the history's rules say they can be.
func composeHistoryOTOps(first, second *Op) (json.RawMessage, bool) {
	a, b := first.Raw(), second.Raw()
	kindA, _ := classifyEdit(a)
	kindB, _ := classifyEdit(b)

	if kindA == editText && kindB == editText {
		var left, right histmodel.TextOperation
		if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
			return nil, false
		}
		// The second has to start where the first finishes.
		if left.TargetLength != right.BaseLength {
			return nil, false
		}
		composed, err := left.Compose(&right)
		if err != nil {
			return nil, false
		}
		encoded, err := json.Marshal(composed)
		if err != nil {
			return nil, false
		}
		return encoded, true
	}
	return composeCommentEdits(a, b)
}
