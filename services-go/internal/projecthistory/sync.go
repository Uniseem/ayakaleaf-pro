package projecthistory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// A resync is what happens when the history and the editor stop agreeing about
// what a project contains. The editor's copy is the one that is right, so web
// is asked to send the project again -- its file tree, then each document's
// contents -- and this service works out what would have to change in the
// history for it to say the same thing, and queues those changes as if
// somebody had made them.
//
// Two things make it delicate. The first is that ordinary edits keep arriving
// while it runs, and applying them to a project that is being rebuilt would
// put it wrong again; so updates for anything still being resynced are
// dropped, and which ones those are is the sync state, kept in Mongo because
// it has to survive a restart. The second is that "what would have to change"
// includes the marks: a tracked change that the history has and the editor
// does not has to be taken off, one the editor has and the history does not
// has to be put on, and that is a diff over two sets of ranges rather than
// over text.

// maxResyncHistoryRecords is how many past sync states are kept.
const maxResyncHistoryRecords = 100

// expireResyncHistoryInterval is how long a finished sync state is kept before
// it is allowed to expire.
const expireResyncHistoryInterval = 90 * 24 * time.Hour

// syncStuckTimeout is how long a sync may be pending before it is treated as
// stuck.
const syncStuckTimeout = 4 * time.Hour

// maxStuckClearAttempts is how many times a stuck sync is cleared and started
// again before it is left alone.
const maxStuckClearAttempts = 5

// ErrSyncPermanentlyStuck is a sync that has been cleared and restarted too
// many times.
var ErrSyncPermanentlyStuck = errors.New("sync permanently stuck")

// ErrDataCorruption is a file in the history that cannot be read back: the
// operations stored against it do not fit the blob, or the marks stored beside
// it are not readable.
var ErrDataCorruption = errors.New("data corruption in stored file")

// ErrTooLong is a document too large for the history to hold.
var ErrTooLong = errors.New("string too long")

// SyncState is what part of a project is still being resynced.
type SyncState struct {
	ProjectID string
	// ResyncProjectStructure is set while the file tree is being resynced.
	ResyncProjectStructure bool
	// ResyncDocContents are the paths whose contents are still being resynced.
	// It is ordered so that what is written to Mongo does not change from run
	// to run for the same state.
	ResyncDocContents []string
	// Origin is what the changes a resync makes are recorded as.
	Origin json.RawMessage

	ResyncCount        int
	ResyncPendingSince *time.Time
	LastUpdated        *time.Time
	History            []syncStateHistoryEntry

	StuckClearCount   int
	LastStuckClearAt  *time.Time
	LastStuckDocPaths []string

	// HardResync says the project is being rebuilt rather than corrected.
	HardResync bool
	// RecoverCorruptedFiles allows a file the history cannot read to be
	// removed and added again from the editor's copy.
	RecoverCorruptedFiles bool
}

// syncStateHistoryEntry is one past sync state and when it was written.
type syncStateHistoryEntry struct {
	SyncState rawSyncState `bson:"syncState" json:"syncState"`
	Timestamp time.Time    `bson:"timestamp" json:"timestamp"`
}

// rawSyncState is the part of the state that is written back.
type rawSyncState struct {
	ResyncProjectStructure bool            `bson:"resyncProjectStructure" json:"resyncProjectStructure"`
	ResyncDocContents      []string        `bson:"resyncDocContents" json:"resyncDocContents"`
	Origin                 json.RawMessage `bson:"-" json:"origin,omitempty"`
	HardResync             bool            `bson:"hardResync" json:"hardResync"`
	RecoverCorruptedFiles  bool            `bson:"recoverCorruptedFiles" json:"recoverCorruptedFiles"`
}

// IsProjectStructureSyncing reports whether the file tree is being resynced.
func (s *SyncState) IsProjectStructureSyncing() bool { return s.ResyncProjectStructure }

// IsDocContentSyncing reports whether one document's contents are.
func (s *SyncState) IsDocContentSyncing(pathname string) bool {
	for _, path := range s.ResyncDocContents {
		if path == pathname {
			return true
		}
	}
	return false
}

// IsAnyDocContentSyncing reports whether any document's contents are.
func (s *SyncState) IsAnyDocContentSyncing() bool { return len(s.ResyncDocContents) > 0 }

// IsSyncOngoing reports whether a resync is running.
func (s *SyncState) IsSyncOngoing() bool {
	return s.IsProjectStructureSyncing() || s.IsAnyDocContentSyncing()
}

// IsSyncStuck reports whether a resync has been pending long enough that
// nothing is going to finish it.
func (s *SyncState) IsSyncStuck() bool {
	if !s.IsSyncOngoing() {
		return false
	}
	if s.ResyncPendingSince == nil {
		// No time was recorded, which is a state written before the field
		// existed: treat it as stuck rather than as pending forever.
		return true
	}
	return time.Since(*s.ResyncPendingSince) > syncStuckTimeout
}

// StartProjectStructureSync begins a resync of the file tree.
func (s *SyncState) StartProjectStructureSync() {
	s.ResyncProjectStructure = true
	s.ResyncDocContents = nil
}

// StopProjectStructureSync marks the file tree as done.
func (s *SyncState) StopProjectStructureSync() { s.ResyncProjectStructure = false }

// StartDocContentSync begins a resync of one document's contents.
func (s *SyncState) StartDocContentSync(pathname string) {
	if !s.IsDocContentSyncing(pathname) {
		s.ResyncDocContents = append(s.ResyncDocContents, pathname)
	}
}

// StopDocContentSync marks one document as done.
func (s *SyncState) StopDocContentSync(pathname string) {
	kept := s.ResyncDocContents[:0]
	for _, path := range s.ResyncDocContents {
		if path != pathname {
			kept = append(kept, path)
		}
	}
	s.ResyncDocContents = kept
}

// UpdateState moves the state on for a sync update that has arrived.
//
// An update that does not fit the state is refused rather than applied: it
// means the queue and the state disagree about what is being resynced, and
// going on would leave the project half rebuilt.
func (s *SyncState) UpdateState(update *Update) error {
	switch {
	case hasField(update, "resyncProjectStructure"):
		if !s.IsProjectStructureSyncing() {
			return fmt.Errorf("%w: unexpected resyncProjectStructure update", ErrSync)
		}
		if s.IsAnyDocContentSyncing() {
			return fmt.Errorf("%w: unexpected resyncDocContents update", ErrSync)
		}

		if !isTrue(update.Rest["resyncProjectStructureOnly"]) {
			structure, err := update.ResyncProjectStructure()
			if err != nil {
				return err
			}
			for _, doc := range structure.Docs {
				s.StartDocContentSync(doc.Path)
			}
		}
		s.StopProjectStructureSync()

	case hasField(update, "resyncDocContent"):
		if s.IsProjectStructureSyncing() {
			return fmt.Errorf("%w: unexpected resyncDocContent update", ErrSync)
		}
		path := rawString(update.Rest["path"])
		if !s.IsDocContentSyncing(path) {
			return fmt.Errorf("%w: unexpected resyncDocContent update", ErrSync)
		}
		s.StopDocContentSync(path)
	}
	return nil
}

// ShouldSkipUpdate reports whether an update has to be dropped because the
// part of the project it touches is being rebuilt.
func (s *SyncState) ShouldSkipUpdate(update *Update) bool {
	if hasField(update, "resyncProjectStructure") || hasField(update, "resyncDocContent") {
		return false
	}
	if s.IsProjectStructureSyncing() {
		return true
	}
	if update.IsTextUpdate() && s.IsDocContentSyncing(update.Meta.Pathname) {
		return true
	}
	return false
}

// raw is the state as it is stored.
func (s *SyncState) raw() rawSyncState {
	contents := s.ResyncDocContents
	if contents == nil {
		contents = []string{}
	}
	return rawSyncState{
		ResyncProjectStructure: s.ResyncProjectStructure,
		ResyncDocContents:      contents,
		Origin:                 s.Origin,
		HardResync:             s.HardResync,
		RecoverCorruptedFiles:  s.RecoverCorruptedFiles,
	}
}

// ResyncEntity is one file or document as the editor has it.
type ResyncEntity struct {
	Path string `json:"path"`
	// Doc and File are the ids of a document and of a binary file; exactly one
	// of them is set.
	Doc  string `json:"doc,omitempty"`
	File string `json:"file,omitempty"`
	// URL is where a binary file can be fetched from, and Hash names the blob
	// it is already stored in.
	URL         string          `json:"url,omitempty"`
	Hash        string          `json:"_hash,omitempty"`
	CreatedBlob bool            `json:"createdBlob,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

// ResyncProjectStructureData is the file tree as the editor has it.
type ResyncProjectStructureData struct {
	Docs  []ResyncEntity `json:"docs"`
	Files []ResyncEntity `json:"files"`
}

// ResyncProjectStructure reads the file tree an update carries.
func (u *Update) ResyncProjectStructure() (*ResyncProjectStructureData, error) {
	var structure ResyncProjectStructureData
	if err := json.Unmarshal(u.Rest["resyncProjectStructure"], &structure); err != nil {
		return nil, err
	}
	return &structure, nil
}

// HistoryOTRanges are the marks of a document kept in the history's own form,
// which is what a project using the history operation type sends.
type HistoryOTRanges struct {
	Comments       []*histmodel.Comment      `json:"comments"`
	TrackedChanges []histmodel.TrackedChange `json:"trackedChanges"`
}

// ResyncDocContentData is one document as the editor has it.
type ResyncDocContentData struct {
	Content            string           `json:"content"`
	Ranges             *updateRanges    `json:"ranges,omitempty"`
	ResolvedCommentIDs []string         `json:"resolvedCommentIds,omitempty"`
	HistoryOTRanges    *HistoryOTRanges `json:"historyOTRanges,omitempty"`
	Version            *int64           `json:"version,omitempty"`
}

// ResyncDocContent reads the document an update carries.
func (u *Update) ResyncDocContent() (*ResyncDocContentData, error) {
	var content ResyncDocContentData
	if err := json.Unmarshal(u.Rest["resyncDocContent"], &content); err != nil {
		return nil, err
	}
	return &content, nil
}

// SyncUpdateExpander turns the project the editor sent into the changes the
// history would need to say the same thing.
type SyncUpdateExpander struct {
	projectID string
	// files is the project as the history has it, changed as updates are
	// expanded so that later ones see what earlier ones did. It keeps the
	// order the files were written in, which is the order this walks them in.
	files *histmodel.FileMap
	// expanded is what has been queued so far.
	expanded []*Update

	origin                json.RawMessage
	hardResync            bool
	recoverCorruptedFiles bool

	store *HistoryStore
	web   *WebAPI
	redis *RedisStore
	mongo *Store
}

// NewSyncUpdateExpander builds an expander over a project's files.
func NewSyncUpdateExpander(projectID string, files *histmodel.FileMap,
	origin json.RawMessage, hardResync, recoverCorruptedFiles bool,
	store *HistoryStore, web *WebAPI, redis *RedisStore, mongo *Store) *SyncUpdateExpander {

	return &SyncUpdateExpander{
		projectID: projectID, files: files, origin: origin,
		hardResync: hardResync, recoverCorruptedFiles: recoverCorruptedFiles,
		store: store, web: web, redis: redis, mongo: mongo,
	}
}

// ExpandedUpdates is what the expander has queued.
func (e *SyncUpdateExpander) ExpandedUpdates() []*Update { return e.expanded }

// isEditable reports whether a file in the history should be treated as text.
//
// The history and the editor can disagree: a file the history holds as text
// may be a binary file in the project, in which case the project is right and
// the history's copy has to be replaced rather than diffed.
func (e *SyncUpdateExpander) isEditable(path string, file *histmodel.File,
	expectedFiles []ResyncEntity) bool {

	if !file.Data.IsEditable() {
		return false
	}
	hash := file.Hash()
	for _, item := range expectedFiles {
		if item.Hash != "" && hash != item.Hash {
			continue
		}
		if ConvertPathname(item.Path) == path {
			return false
		}
	}
	return true
}

// ExpandUpdate queues whatever one update means for the history.
func (e *SyncUpdateExpander) ExpandUpdate(ctx context.Context, update *Update) error {
	switch {
	case hasField(update, "resyncProjectStructure"):
		return e.expandProjectStructure(ctx, update)
	case hasField(update, "resyncDocContent"):
		return e.expandDocContent(ctx, update)
	}
	e.expanded = append(e.expanded, update)
	return nil
}

// expandProjectStructure works out what the file tree would have to become.
func (e *SyncUpdateExpander) expandProjectStructure(ctx context.Context,
	update *Update) error {

	structure, err := update.ResyncProjectStructure()
	if err != nil {
		return err
	}

	// The history's files, split by whether they should be treated as text.
	// The two are compared separately because a file can be in both lists
	// under the same path -- text on one side and binary on the other -- and
	// that has to come out as a removal and an addition.
	var persistedText, persistedBinary []persistedFile
	for _, path := range e.files.Paths() {
		file := e.files.GetFile(path)
		if e.isEditable(path, file, structure.Files) {
			persistedText = append(persistedText, persistedFile{Path: path, File: file})
		} else {
			persistedBinary = append(persistedBinary, persistedFile{Path: path, File: file})
		}
	}

	expectedText := convertEntityPaths(structure.Docs)
	expectedBinary := convertEntityPaths(structure.Files)

	e.queueRemoveOpsForUnexpectedFiles(update, expectedBinary, persistedBinary)
	e.queueRemoveOpsForUnexpectedFiles(update, expectedText, persistedText)
	e.queueAddOpsForMissingFiles(update, expectedBinary, persistedBinary)
	e.queueAddOpsForMissingFiles(update, expectedText, persistedText)
	e.queueUpdateForOutOfSyncBinaryFiles(update, expectedBinary, persistedBinary)
	e.queueSetMetadataOpsForLinkedFiles(update, structure)

	if isTrue(update.Rest["resyncProjectStructureOnly"]) {
		// A resync of the tree alone cannot touch a document: if it would,
		// the tree is not the only thing out of step and the whole project
		// has to be sent.
		docPaths := map[string]bool{}
		for _, doc := range structure.Docs {
			docPaths[ConvertPathname(doc.Path)] = true
		}
		for _, expanded := range e.expanded {
			if !docPaths[expanded.Pathname] {
				continue
			}
			e.expanded = nil
			if err := e.mongo.ClearSyncState(ctx, e.projectID); err != nil {
				return err
			}
			if err := e.redis.DeleteAppliedUpdate(ctx, e.projectID, update.Raw); err != nil {
				return err
			}
			return fmt.Errorf("%w: aborting partial resync: touched doc",
				ErrNeedFullProjectStructureResync)
		}
	}
	return nil
}

// persistedFile is one file the history has, with its path.
type persistedFile struct {
	Path string
	File *histmodel.File
}

// convertEntityPaths rewrites the editor's paths into the history's.
func convertEntityPaths(entities []ResyncEntity) []ResyncEntity {
	converted := make([]ResyncEntity, 0, len(entities))
	for _, entity := range entities {
		copied := entity
		copied.Path = ConvertPathname(entity.Path)
		converted = append(converted, copied)
	}
	return converted
}

// resyncMeta is the metadata every update a resync queues carries.
func (e *SyncUpdateExpander) resyncMeta(update *Update) Meta {
	return Meta{Resync: true, Origin: e.origin, TS: update.Meta.TS}
}

// queueRemoveOpsForUnexpectedFiles removes what the history has and the editor
// does not.
func (e *SyncUpdateExpander) queueRemoveOpsForUnexpectedFiles(update *Update,
	expected []ResyncEntity, persisted []persistedFile) {

	expectedPaths := map[string]bool{}
	for _, entity := range expected {
		expectedPaths[entity.Path] = true
	}
	for _, file := range persisted {
		if expectedPaths[file.Path] {
			continue
		}
		removal := &Update{Pathname: file.Path, Meta: e.resyncMeta(update)}
		removal.setRest("new_pathname", "")
		e.expanded = append(e.expanded, removal)
	}
}

// queueAddOpsForMissingFiles adds what the editor has and the history does
// not.
func (e *SyncUpdateExpander) queueAddOpsForMissingFiles(update *Update,
	expected []ResyncEntity, persisted []persistedFile) {

	persistedPaths := map[string]bool{}
	for _, file := range persisted {
		persistedPaths[file.Path] = true
	}
	for _, entity := range expected {
		if persistedPaths[entity.Path] {
			continue
		}

		addition := &Update{Pathname: entity.Path, Meta: e.resyncMeta(update)}
		if entity.Doc != "" {
			addition.Doc = entity.Doc
			addition.setRest("docLines", "")
			// The document is added empty here and its contents arrive in a
			// later update, which is diffed against this.
			e.files.AddFile(entity.Path, &histmodel.File{
				Data: histmodel.NewStringFileData("", nil, nil),
			})
		} else {
			addition.setRest("file", entity.File)
			if entity.URL != "" {
				addition.setRest("url", entity.URL)
			}
			if entity.Hash != "" {
				addition.setRest("hash", entity.Hash)
			}
			if entity.CreatedBlob {
				addition.setRest("createdBlob", true)
			}
			if len(entity.Metadata) > 0 {
				addition.Rest["metadata"] = entity.Metadata
			}
		}
		e.expanded = append(e.expanded, addition)
	}
}

// queueUpdateForOutOfSyncBinaryFiles replaces a binary file whose contents
// differ from the editor's.
func (e *SyncUpdateExpander) queueUpdateForOutOfSyncBinaryFiles(update *Update,
	expected []ResyncEntity, persisted []persistedFile) {

	byPath := map[string]*histmodel.File{}
	for _, file := range persisted {
		byPath[file.Path] = file.File
	}

	for _, entity := range expected {
		file, ok := byPath[entity.Path]
		if !ok {
			continue
		}
		persistedHash := file.Hash()
		if entity.Hash == "" || persistedHash == "" || persistedHash == entity.Hash {
			continue
		}

		removal := &Update{Pathname: entity.Path, Meta: e.resyncMeta(update)}
		removal.setRest("new_pathname", "")
		e.expanded = append(e.expanded, removal)

		addition := &Update{Pathname: entity.Path, Meta: e.resyncMeta(update)}
		addition.setRest("file", entity.File)
		if entity.URL != "" {
			addition.setRest("url", entity.URL)
		}
		if entity.Hash != "" {
			addition.setRest("hash", entity.Hash)
		}
		if entity.CreatedBlob {
			addition.setRest("createdBlob", true)
		}
		if len(entity.Metadata) > 0 {
			addition.Rest["metadata"] = entity.Metadata
		}
		e.expanded = append(e.expanded, addition)
	}
}

// queueSetMetadataOpsForLinkedFiles brings the metadata of a linked file back
// into step. A linked file records where it came from, and that can change
// without its contents changing.
func (e *SyncUpdateExpander) queueSetMetadataOpsForLinkedFiles(update *Update,
	structure *ResyncProjectStructureData) {

	entities := append(append([]ResyncEntity{}, structure.Docs...), structure.Files...)
	for _, entity := range entities {
		pathname := ConvertPathname(entity.Path)

		// A file that has just been queued to be added already carries this
		// metadata, so there is nothing to set.
		matched := false
		for _, queued := range e.expanded {
			if queued.Pathname == pathname &&
				sameRawJSON(queued.Rest["metadata"], entity.Metadata) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		file := e.files.GetFile(pathname)
		if file == nil {
			continue
		}
		existing := metadataFields(file.Metadata)

		shouldUpdate := false
		switch {
		case len(entity.Metadata) > 0 && string(entity.Metadata) != "null":
			for key, value := range metadataFields(entity.Metadata) {
				if !sameRawJSON(existing[key], value) {
					shouldUpdate = true
					break
				}
			}
		case len(existing["provider"]) > 0:
			// The file was a linked file and is not any more, either because
			// something else was put in its place or because it became a
			// document.
			shouldUpdate = true
		}
		if !shouldUpdate {
			continue
		}

		metadata := entity.Metadata
		if len(metadata) == 0 || string(metadata) == "null" {
			metadata = json.RawMessage("{}")
		}
		queued := &Update{Pathname: pathname, Meta: e.resyncMeta(update)}
		queued.ensureRest()
		queued.Rest["metadata"] = metadata
		e.expanded = append(e.expanded, queued)
	}
}

// metadataFields reads a metadata object into its fields.
func metadataFields(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]json.RawMessage{}
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return map[string]json.RawMessage{}
	}
	return fields
}

// sameRawJSON compares two JSON values, treating absent and null alike.
func sameRawJSON(a, b json.RawMessage) bool {
	empty := func(raw json.RawMessage) bool {
		return len(raw) == 0 || string(raw) == "null"
	}
	if empty(a) || empty(b) {
		return empty(a) && empty(b)
	}
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return string(a) == string(b)
	}
	encodedLeft, _ := json.Marshal(left)
	encodedRight, _ := json.Marshal(right)
	return string(encodedLeft) == string(encodedRight)
}

// setRest puts a value on an update's passthrough fields.
func (u *Update) setRest(name string, value any) {
	u.ensureRest()
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	u.Rest[name] = encoded
}

// ensureRest makes sure the passthrough map exists.
func (u *Update) ensureRest() {
	if u.Rest == nil {
		u.Rest = map[string]json.RawMessage{}
	}
}
