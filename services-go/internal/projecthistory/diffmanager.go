package projecthistory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/histmodel"
)

// A diff spans two versions, and the history is stored in chunks, so a diff
// can span more than one of them. They are fetched backwards from the later
// version until the earlier one is covered, and joined into one chunk for the
// rest of the work to read.
//
// There is a limit on how many are fetched. A diff over a thousand versions is
// not something anybody is reading; it is something asking for the whole
// history one request at a time.

// maxChunkRequests is how many chunks one diff may span.
const maxChunkRequests = 10

// DiffManager answers the editor's questions about what changed.
type DiffManager struct {
	store      *HistoryStore
	web        *WebAPI
	processor  *Processor
	translator *ChunkTranslator
}

// NewDiffManager builds it.
func NewDiffManager(store *HistoryStore, web *WebAPI, processor *Processor,
	translator *ChunkTranslator) *DiffManager {

	return &DiffManager{
		store: store, web: web, processor: processor, translator: translator,
	}
}

// GetDiff is what changed in one document between two versions.
//
// The queue is flushed first: a diff that did not include the edits still
// waiting would show the document as it was some minutes ago and say it was
// current.
func (m *DiffManager) GetDiff(ctx context.Context, projectID, pathname string,
	fromVersion, toVersion int) ([]DiffPart, bool, error) {

	if err := m.processor.ProcessUpdatesForProject(ctx, projectID); err != nil {
		return nil, false, err
	}

	chunk, err := m.chunksAsOne(ctx, projectID, fromVersion, toVersion)
	if err != nil {
		return nil, false, err
	}

	updates, err := m.translator.ConvertToDiffUpdates(ctx, projectID, chunk,
		pathname, fromVersion, toVersion)
	if err != nil {
		return nil, false, err
	}
	if updates.Binary {
		return nil, true, nil
	}

	diff, err := BuildDiff(updates.InitialContent, updates.Updates)
	if err != nil {
		return nil, false, fmt.Errorf("failed to build diff for %s: %w", pathname, err)
	}
	return diff, false, nil
}

// GetFileTreeDiff is what happened to a project's files between two versions.
func (m *DiffManager) GetFileTreeDiff(ctx context.Context, projectID string,
	fromVersion, toVersion int) ([]FileTreeDiffEntry, error) {

	if err := m.processor.ProcessUpdatesForProject(ctx, projectID); err != nil {
		return nil, err
	}
	chunk, err := m.chunksAsOne(ctx, projectID, fromVersion, toVersion)
	if err != nil {
		return nil, err
	}
	return BuildFileTreeDiff(chunk, fromVersion, toVersion)
}

// chunksAsOne fetches every chunk a version range falls in and joins them.
func (m *DiffManager) chunksAsOne(ctx context.Context, projectID string,
	fromVersion, toVersion int) (*ChunkResponse, error) {

	historyID, err := m.web.GetHistoryID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	var chunks []*ChunkResponse
	nextVersion := toVersion

	for {
		chunk, err := m.store.GetChunkAtVersion(ctx, historyID, nextVersion)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
		nextVersion = chunk.Chunk.StartVersion

		if len(chunks) >= maxChunkRequests {
			return nil, fmt.Errorf("%w: diff spans too many chunks", ErrBadRequest)
		}
		// Asking for the version a chunk starts at gives the chunk before it,
		// so this walks backwards until the range is covered.
		if fromVersion >= nextVersion || nextVersion <= 0 {
			break
		}
	}

	return concatChunks(chunks), nil
}

// concatChunks joins chunks fetched newest first into one, oldest first.
func concatChunks(chunks []*ChunkResponse) *ChunkResponse {
	if len(chunks) == 0 {
		return nil
	}

	// They were fetched backwards, so the last one is the earliest and is the
	// one the others are appended to.
	joined := chunks[len(chunks)-1]
	for i := len(chunks) - 2; i >= 0; i-- {
		joined.Chunk.History.Changes = append(joined.Chunk.History.Changes,
			chunks[i].Chunk.History.Changes...)
	}
	return joined
}

// SummarizeOptions bound a request for a project's history.
type SummarizeOptions struct {
	// Before is the version to list backwards from, or zero for the latest.
	Before int
	// MinCount is how many entries to gather before stopping.
	MinCount int
}

// SummarizedProjectUpdate is one entry of the editor's history list: a run of
// changes made close together by the same people.
type SummarizedProjectUpdate struct {
	FromV      int              `json:"fromV"`
	ToV        int              `json:"toV"`
	Meta       UpdateMeta       `json:"meta"`
	Labels     []Label          `json:"labels"`
	Pathnames  []string         `json:"pathnames"`
	ProjectOps []map[string]any `json:"project_ops"`

	// pathnameSet is how membership is tested while entries are being merged,
	// and pathnameOrder keeps the order they were added in, which is the order
	// they are written out in.
	pathnameSet   map[string]bool
	pathnameOrder []string
}

// addPathname records a document as changed by this entry.
func (u *SummarizedProjectUpdate) addPathname(pathname string) {
	if u.pathnameSet == nil {
		u.pathnameSet = map[string]bool{}
	}
	if !u.pathnameSet[pathname] {
		u.pathnameOrder = append(u.pathnameOrder, pathname)
	}
	u.pathnameSet[pathname] = true
}

// removePathname takes one out again, which is what happens when the same
// entry also created the file.
func (u *SummarizedProjectUpdate) removePathname(pathname string) {
	delete(u.pathnameSet, pathname)
}

// pathnames is the documents this entry changed, in the order they were added.
func (u *SummarizedProjectUpdate) pathnames() []string {
	ordered := make([]string, 0, len(u.pathnameSet))
	for _, pathname := range u.pathnameOrder {
		if u.pathnameSet[pathname] {
			ordered = append(ordered, pathname)
		}
	}
	return ordered
}

// maxSummaryChunkRequests is how many chunks are read to fill one page of
// history.
const maxSummaryChunkRequests = 5

// timeBetweenDistinctUpdates is how long a gap has to be before two runs of
// editing are shown separately.
const timeBetweenDistinctUpdates = 5 * 60 * 1000

// Summarizer builds the editor's history list.
type Summarizer struct {
	store      *HistoryStore
	web        *WebAPI
	processor  *Processor
	translator *ChunkTranslator
	labels     *Store
}

// NewSummarizer builds it.
func NewSummarizer(store *HistoryStore, web *WebAPI, processor *Processor,
	translator *ChunkTranslator, labels *Store) *Summarizer {

	return &Summarizer{
		store: store, web: web, processor: processor,
		translator: translator, labels: labels,
	}
}

// GetSummarizedProjectUpdates lists a project's history, newest first.
//
// Versions are fence posts and updates are what lies between them, so asking
// for the updates before a version includes the one that produced it.
func (s *Summarizer) GetSummarizedProjectUpdates(ctx context.Context,
	projectID string, options SummarizeOptions) ([]SummarizedProjectUpdate, int, error) {

	if options.MinCount <= 0 {
		options.MinCount = 25
	}

	if err := s.processor.ProcessUpdatesForProject(ctx, projectID); err != nil {
		return nil, 0, err
	}

	labels, err := s.labels.GetLabels(ctx, projectID)
	if err != nil {
		return nil, 0, err
	}
	labelsByVersion := map[int][]Label{}
	for _, label := range labels {
		labelsByVersion[label.Version] = append(labelsByVersion[label.Version], label)
	}

	historyID, err := s.web.GetHistoryID(ctx, projectID)
	if err != nil {
		return nil, 0, err
	}
	if historyID == "" {
		// The project has no history, so there is nothing to list.
		return []SummarizedProjectUpdate{}, 0, nil
	}

	nextVersion := options.Before
	haveNextVersion := options.Before != 0
	var summarized []SummarizedProjectUpdate
	toV := -1
	chunksRequested := 0

	for chunksRequested < maxSummaryChunkRequests &&
		(!haveNextVersion || nextVersion > 0) &&
		len(summarized) < options.MinCount {

		var chunk *ChunkResponse
		if haveNextVersion {
			chunk, err = s.store.GetChunkAtVersion(ctx, historyID, nextVersion)
		} else {
			chunk, err = s.store.GetMostRecentChunk(ctx, historyID)
		}
		if err != nil {
			return nil, 0, err
		}

		updates, err := ConvertToSummarizedUpdates(chunk)
		if err != nil {
			return nil, 0, err
		}

		// They come back oldest first, and the history is read backwards.
		reversed := make([]SummarizedUpdate, 0, len(updates))
		for i := len(updates) - 1; i >= 0; i-- {
			if options.Before != 0 && updates[i].V >= options.Before {
				// The chunk holds versions past the one asked for.
				continue
			}
			reversed = append(reversed, updates[i])
		}

		summarized, toV = summarizeUpdates(reversed, labelsByVersion, summarized, toV)
		nextVersion = chunk.Chunk.StartVersion
		haveNextVersion = true
		chunksRequested++
	}

	if summarized == nil {
		summarized = []SummarizedProjectUpdate{}
	}
	for i := range summarized {
		summarized[i].Pathnames = summarized[i].pathnames()
	}

	if nextVersion > 0 {
		return summarized, nextVersion, nil
	}
	return summarized, 0, nil
}

// summarizeUpdates joins the changes that belong together into one entry each.
func summarizeUpdates(updates []SummarizedUpdate, labels map[int][]Label,
	existing []SummarizedProjectUpdate, toV int) ([]SummarizedProjectUpdate, int) {

	summarized := append([]SummarizedProjectUpdate{}, existing...)

	for _, update := range updates {
		if toV < 0 {
			toV = update.V + 1
		}

		if len(update.ProjectOps) == 0 && len(update.Pathnames) == 0 {
			// Nothing to show: a change that only moved comments about. Its
			// version is still stepped over.
			continue
		}

		// The editor has to know exactly which version a file was removed at
		// in order to restore it, so that stays on each operation even when
		// the entries around it are merged.
		ops := make([]map[string]any, 0, len(update.ProjectOps))
		for _, op := range update.ProjectOps {
			copied := map[string]any{}
			for key, value := range op {
				copied[key] = value
			}
			copied["atV"] = update.V
			ops = append(ops, copied)
		}

		forVersion := labels[update.V+1]
		if len(summarized) > 0 &&
			shouldMergeUpdate(&update, &summarized[len(summarized)-1], forVersion) {

			mergeUpdate(&update, &summarized[len(summarized)-1], ops)
		} else {
			entry := SummarizedProjectUpdate{
				FromV: update.V, ToV: toV,
				Meta: UpdateMeta{
					Users:   update.Meta.Users,
					StartTS: update.Meta.StartTS,
					EndTS:   update.Meta.EndTS,
					Origin:  update.Meta.Origin,
				},
				Labels:      forVersion,
				ProjectOps:  ops,
				pathnameSet: map[string]bool{},
			}
			if entry.Labels == nil {
				entry.Labels = []Label{}
			}
			for _, path := range update.Pathnames {
				entry.addPathname(path)
			}
			summarized = append(summarized, entry)
		}
		toV = update.V
	}
	return summarized, toV
}

// shouldMergeUpdate reports whether a change belongs with the entry before it.
func shouldMergeUpdate(update *SummarizedUpdate, entry *SummarizedProjectUpdate,
	labels []Label) bool {

	// A labelled version is a point somebody marked, so the history is split
	// there whatever else is true.
	if len(labels) > 0 {
		return false
	}

	updateOrigin := parseOrigin(update.Meta.Origin)
	entryOrigin := parseOrigin(entry.Meta.Origin)
	switch {
	case updateOrigin != nil && entryOrigin == nil,
		updateOrigin == nil && entryOrigin != nil:
		return false
	case updateOrigin != nil && entryOrigin != nil:
		if updateOrigin.Kind != entryOrigin.Kind {
			return false
		}
		if updateOrigin.Path != entryOrigin.Path {
			return false
		}
		if updateOrigin.Kind == "file-restore" &&
			updateOrigin.Timestamp != entryOrigin.Timestamp {
			return false
		}
		if updateOrigin.Kind == "project-restore" &&
			updateOrigin.Timestamp != entryOrigin.Timestamp {
			return false
		}
	}

	// The history is read backwards, so the update comes before the entry.
	if entry.Meta.EndTS-update.Meta.StartTS >= timeBetweenDistinctUpdates {
		return false
	}

	// Editing a document and adding a file are different kinds of thing and
	// are shown separately -- except during a resync, which does both at once
	// and is one event as far as anybody reading is concerned.
	resync := updateOrigin != nil &&
		(updateOrigin.Kind == "history-resync" || updateOrigin.Kind == "history-migration")
	if !resync {
		updateHasText := len(update.Pathnames) > 0
		updateHasFiles := len(update.ProjectOps) > 0
		entryHasText := len(entry.pathnameSet) > 0
		entryHasFiles := len(entry.ProjectOps) > 0
		if (updateHasText && entryHasFiles) || (updateHasFiles && entryHasText) {
			return false
		}
	}
	return true
}

// mergeUpdate folds a change into the entry before it.
func mergeUpdate(update *SummarizedUpdate, entry *SummarizedProjectUpdate,
	ops []map[string]any) {

	entry.Meta.Users = mergeUsers(entry.Meta.Users, update.Meta.Users)

	if update.V < entry.FromV {
		entry.FromV = update.V
	}
	if update.V+1 > entry.ToV {
		entry.ToV = update.V + 1
	}
	if update.Meta.StartTS < entry.Meta.StartTS {
		entry.Meta.StartTS = update.Meta.StartTS
	}
	if update.Meta.EndTS > entry.Meta.EndTS {
		entry.Meta.EndTS = update.Meta.EndTS
	}

	for _, op := range ops {
		entry.ProjectOps = append(entry.ProjectOps, op)
		if added, ok := op["add"].(map[string]any); ok {
			if pathname, ok := added["pathname"].(string); ok {
				// A file that was created here does not also need to be listed
				// as edited: the creation says everything.
				entry.removePathname(pathname)
			}
		}
	}
	for _, path := range update.Pathnames {
		entry.addPathname(path)
	}
}

// mergeUsers joins two lists of people, keeping each one once.
//
// A person is recognised by their id when they arrive as an object and by
// their whole value otherwise, because both shapes reach here: the history
// stores whichever the editor sent.
func mergeUsers(into, from []json.RawMessage) []json.RawMessage {
	seen := map[string]bool{}
	merged := make([]json.RawMessage, 0, len(into)+len(from))

	add := func(users []json.RawMessage) {
		for _, user := range users {
			key := userKey(user)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, user)
		}
	}
	add(into)
	add(from)
	return merged
}

// userKey is what makes two entries the same person.
func userKey(user json.RawMessage) string {
	trimmed := strings.TrimSpace(string(user))
	if trimmed == "null" || trimmed == "" {
		return "null"
	}
	var fields struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(user, &fields); err == nil && len(fields.ID) > 0 {
		return string(fields.ID)
	}
	return trimmed
}

// parseOrigin reads what a change was made by.
func parseOrigin(raw []byte) *histmodel.Origin {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	origin, err := histmodel.OriginFromRaw(raw)
	if err != nil {
		return nil
	}
	return origin
}
