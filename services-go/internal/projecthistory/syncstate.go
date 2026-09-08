package projecthistory

import (
	"context"
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// The sync state is the one piece of this service's working memory that has to
// survive a restart, because a resync spans many batches of updates and a
// service that forgot which documents it was still waiting for would start
// applying edits to a half-rebuilt project.
//
// It is written with the past few states beside it, so that a project that
// keeps getting stuck can be looked at afterwards, and it is given an expiry
// once the resync finishes so that the record does not outlive its usefulness.

// syncStateFromRaw reads a stored sync state.
func syncStateFromRaw(projectID string, raw bson.M) *SyncState {
	state := &SyncState{ProjectID: projectID}
	if raw == nil {
		return state
	}

	state.ResyncProjectStructure = bsonBool(raw["resyncProjectStructure"])
	state.ResyncDocContents = bsonStrings(raw["resyncDocContents"])
	state.Origin = bsonRawJSON(raw["origin"])
	state.ResyncCount = bsonInt(raw["resyncCount"])
	state.ResyncPendingSince = bsonTime(raw["resyncPendingSince"])
	state.LastUpdated = bsonTime(raw["lastUpdated"])
	state.StuckClearCount = bsonInt(raw["stuckClearCount"])
	state.LastStuckClearAt = bsonTime(raw["lastStuckClearAt"])
	state.LastStuckDocPaths = bsonStrings(raw["lastStuckDocPaths"])
	state.HardResync = bsonBool(raw["hardResync"])
	state.RecoverCorruptedFiles = bsonBool(raw["recoverCorruptedFiles"])
	state.History = syncStateHistory(raw["history"])

	if state.IsSyncOngoing() && state.ResyncPendingSince == nil &&
		len(state.History) > 0 {
		// The field was added after the collection was in use, so for a state
		// written before that it is worked out from the history: the time of
		// the first state, walking back, that was already syncing after one
		// that was not. The history is newest first, so it is walked in
		// reverse.
		for i := len(state.History) - 1; i >= 0; i-- {
			entry := state.History[i]
			ongoing := entry.SyncState.ResyncProjectStructure ||
				len(entry.SyncState.ResyncDocContents) > 0
			if ongoing {
				if state.ResyncPendingSince == nil {
					timestamp := entry.Timestamp
					state.ResyncPendingSince = &timestamp
				}
			} else {
				state.ResyncPendingSince = nil
			}
		}
	}
	return state
}

// syncStateHistory reads the past states beside a stored one.
func syncStateHistory(raw any) []syncStateHistoryEntry {
	list, ok := raw.(bson.A)
	if !ok {
		return nil
	}
	entries := make([]syncStateHistoryEntry, 0, len(list))
	for _, item := range list {
		fields, ok := item.(bson.M)
		if !ok {
			continue
		}
		entry := syncStateHistoryEntry{}
		if timestamp := bsonTime(fields["timestamp"]); timestamp != nil {
			entry.Timestamp = *timestamp
		}
		if state, ok := fields["syncState"].(bson.M); ok {
			entry.SyncState = rawSyncState{
				ResyncProjectStructure: bsonBool(state["resyncProjectStructure"]),
				ResyncDocContents:      bsonStrings(state["resyncDocContents"]),
				HardResync:             bsonBool(state["hardResync"]),
				RecoverCorruptedFiles:  bsonBool(state["recoverCorruptedFiles"]),
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// GetSyncStateFor reads a project's sync state, which is a state that says
// nothing is being resynced when the project has never been.
func (s *Store) GetSyncStateFor(ctx context.Context, projectID string) (*SyncState, error) {
	raw, err := s.GetSyncState(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return syncStateFromRaw(projectID, bson.M(raw)), nil
}

// WriteSyncState stores a sync state.
//
// A state that says a resync is running is kept from expiring and given a time
// to be measured as stuck from; one that says a resync has finished is given
// an expiry and has the stuck bookkeeping cleared.
func (s *Store) WriteSyncState(ctx context.Context, state *SyncState) error {
	id, err := bson.ObjectIDFromHex(state.ProjectID)
	if err != nil {
		return err
	}

	raw := state.raw()
	stored := bson.M{
		"resyncProjectStructure": raw.ResyncProjectStructure,
		"resyncDocContents":      raw.ResyncDocContents,
		"hardResync":             raw.HardResync,
		"recoverCorruptedFiles":  raw.RecoverCorruptedFiles,
	}
	if len(raw.Origin) > 0 && string(raw.Origin) != "null" {
		var origin any
		if err := json.Unmarshal(raw.Origin, &origin); err == nil {
			stored["origin"] = origin
		}
	}

	set := bson.M{}
	for key, value := range stored {
		set[key] = value
	}
	update := bson.M{
		"$push": bson.M{
			"history": bson.M{
				"$each": []any{bson.M{
					"syncState": stored, "timestamp": time.Now(),
				}},
				"$position": 0, "$slice": maxResyncHistoryRecords,
			},
		},
		"$currentDate": bson.M{"lastUpdated": true},
	}

	if state.IsSyncOngoing() {
		update["$inc"] = bson.M{"resyncCount": 1}
		update["$unset"] = bson.M{"expiresAt": true}
		// The earliest time it has been pending since, so that restarting a
		// resync does not reset the clock that decides it is stuck.
		update["$min"] = bson.M{"resyncPendingSince": time.Now()}
	} else {
		set["expiresAt"] = time.Now().Add(expireResyncHistoryInterval)
		update["$unset"] = bson.M{
			"resyncPendingSince": 1, "stuckClearCount": 1,
			"lastStuckClearAt": 1, "lastStuckDocPaths": 1,
		}
	}
	update["$set"] = set

	_, err = s.syncState.UpdateOne(ctx, bson.M{"project_id": id}, update,
		options.UpdateOne().SetUpsert(true))
	if err != nil {
		return err
	}

	if !state.IsSyncOngoing() {
		// The project records when it was last put back in step, which is what
		// the editor shows and what decides whether to offer a resync again.
		_, err = s.projects.UpdateOne(ctx, bson.M{"_id": id},
			bson.M{"$max": bson.M{"overleaf.history.lastResyncedAt": time.Now()}})
		if err != nil {
			return err
		}
	}
	return nil
}

// RecordStuckClear notes that a stuck resync was cleared and started again.
func (s *Store) RecordStuckClear(ctx context.Context, projectID string,
	docPaths []string) error {

	id, err := bson.ObjectIDFromHex(projectID)
	if err != nil {
		return err
	}
	if docPaths == nil {
		docPaths = []string{}
	}
	_, err = s.syncState.UpdateOne(ctx, bson.M{"project_id": id}, bson.M{
		"$inc": bson.M{"stuckClearCount": 1},
		"$set": bson.M{
			"lastStuckClearAt": time.Now(), "lastStuckDocPaths": docPaths,
		},
		"$unset": bson.M{"resyncPendingSince": 1},
	})
	return err
}

// ClearSyncStateIfAllAfter forgets a project's sync state, but only when
// everything recorded in it happened after the given time.
//
// It is how a resync that has since been superseded is tidied away without
// throwing out one that is still running or one whose record is still wanted.
func (s *Store) ClearSyncStateIfAllAfter(ctx context.Context, projectID string,
	after time.Time) error {

	id, err := bson.ObjectIDFromHex(projectID)
	if err != nil {
		return err
	}
	raw, err := s.GetSyncState(ctx, projectID)
	if err != nil || raw == nil {
		return err
	}

	state := syncStateFromRaw(projectID, bson.M(raw))
	if state.IsSyncOngoing() {
		// A new resync has started since, so the record is in use.
		return nil
	}
	for _, entry := range state.History {
		if entry.Timestamp.Before(after) {
			return nil
		}
	}

	// Matched on the expiry as it was read, so that a state written between
	// the read and the delete is left alone.
	filter := bson.M{"project_id": id}
	filter["expiresAt"] = bson.M(raw)["expiresAt"]
	_, err = s.syncState.DeleteOne(ctx, filter)
	return err
}

// bsonBool reads a stored boolean, defaulting to false.
func bsonBool(value any) bool {
	result, _ := value.(bool)
	return result
}

// bsonInt reads a stored number, defaulting to zero.
func bsonInt(value any) int {
	switch number := value.(type) {
	case int32:
		return int(number)
	case int64:
		return int(number)
	case int:
		return number
	case float64:
		return int(number)
	}
	return 0
}

// bsonTime reads a stored date, or nil.
func bsonTime(value any) *time.Time {
	switch stamp := value.(type) {
	case bson.DateTime:
		converted := stamp.Time().UTC()
		return &converted
	case time.Time:
		converted := stamp.UTC()
		return &converted
	}
	return nil
}

// bsonStrings reads a stored list of strings.
func bsonStrings(value any) []string {
	list, ok := value.(bson.A)
	if !ok {
		return nil
	}
	strings := make([]string, 0, len(list))
	for _, item := range list {
		if text, ok := item.(string); ok {
			strings = append(strings, text)
		}
	}
	return strings
}

// bsonRawJSON turns a stored value back into JSON, which is how the parts of
// an update this service passes through are held.
func bsonRawJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(bsonToJSON(value))
	if err != nil {
		return nil
	}
	return encoded
}

// bsonToJSON turns stored documents into the plain values JSON is made of.
func bsonToJSON(value any) any {
	switch typed := value.(type) {
	case bson.M:
		converted := map[string]any{}
		for key, item := range typed {
			converted[key] = bsonToJSON(item)
		}
		return converted
	case bson.A:
		converted := make([]any, 0, len(typed))
		for _, item := range typed {
			converted = append(converted, bsonToJSON(item))
		}
		return converted
	case bson.DateTime:
		return typed.Time().UTC().Format("2006-01-02T15:04:05.000Z")
	case bson.ObjectID:
		return typed.Hex()
	}
	return value
}
