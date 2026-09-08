package projecthistory

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// A project whose updates cannot be processed is recorded rather than retried
// forever in place: the record says how many times it has failed and with
// what, and that is what decides whether the next attempt should be another
// flush, a resync, or nothing at all.

// RecordSyncStart notes that a resync has begun.
//
// Only the start is recorded, not the end: the whole record is deleted when
// the project is processed successfully, so a record that is still there is a
// resync that did not finish.
func (s *Store) RecordSyncStart(ctx context.Context, projectID string) error {
	_, err := s.failures.UpdateOne(ctx,
		bson.M{"project_id": projectID},
		bson.M{
			"$currentDate": bson.M{"resyncStartedAt": true},
			"$inc":         bson.M{"resyncAttempts": 1},
			"$push": bson.M{
				"history": bson.M{
					"$each":     []any{bson.M{"resyncStartedAt": time.Now()}},
					"$position": 0, "$slice": 10,
				},
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// SetForceDebug marks a project as one whose stored history may be walked past
// even when its versions are out of order, so that somebody can look at what
// is wrong with it.
func (s *Store) SetForceDebug(ctx context.Context, projectID string, state bool) error {
	_, err := s.failures.UpdateOne(ctx,
		bson.M{"project_id": projectID},
		bson.M{"$set": bson.M{"forceDebug": state}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// CloneFailure copies one project's failure record onto another, which is what
// a project copied from a broken one needs so that it is not silently treated
// as healthy.
func (s *Store) CloneFailure(ctx context.Context, from, to string) error {
	var failure bson.M
	err := s.failures.FindOne(ctx, bson.M{"project_id": from},
		options.FindOne().SetProjection(bson.M{"_id": 0, "project_id": 0}),
	).Decode(&failure)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}
	failure["project_id"] = to
	_, err = s.failures.InsertOne(ctx, failure)
	return err
}

// GetLastFailure returns what a project last failed with, and counts the ask.
//
// The count is what tells a project nobody is looking at from one that is
// being asked about repeatedly, which is worth knowing when deciding what to
// retry.
func (s *Store) GetLastFailure(ctx context.Context, projectID string) (*Failure, error) {
	var failure Failure
	err := s.failures.FindOneAndUpdate(ctx,
		bson.M{"project_id": projectID},
		bson.M{"$inc": bson.M{"requestCount": 1}},
		options.FindOneAndUpdate().SetProjection(bson.M{"error": 1, "ts": 1}),
	).Decode(&failure)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	failure.Error = normalizeFailureMessage(failure.Error)
	return &failure, nil
}

// ForceDebug reports whether a project is marked for walking past out of order
// versions.
func (s *Store) ForceDebug(ctx context.Context, projectID string) (bool, error) {
	var record struct {
		ForceDebug bool `bson:"forceDebug"`
	}
	err := s.failures.FindOne(ctx, bson.M{"project_id": projectID},
		options.FindOne().SetProjection(bson.M{"forceDebug": 1}),
	).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return record.ForceDebug, nil
}

// shortErrorNames maps the failures worth counting separately to the labels
// they are counted under. Everything else is counted as "other".
//
// The messages are matched exactly, because they are what was stored: a
// failure whose wording changes stops being counted under its old label, which
// is the point -- the label follows the failure, not the other way round.
var shortErrorNames = map[string]string{
	"Error: bad response from filestore: 404":                          "filestore-404",
	"Error: bad response from filestore: 500":                          "filestore-500",
	"NotFoundError: got a 404 from web api":                            "web-api-404",
	"Error: history store a non-success status code: 413":              "history-store-413",
	"Error: history store a non-success status code: 422":              "history-store-422",
	"Error: history store a non-success status code: 500":              "history-store-500",
	"Error: history store a non-success status code: 503":              "history-store-503",
	"Error: web returned a non-success status code: 500 (attempts: 2)": "web-500",
	"Error: ESOCKETTIMEDOUT":                                           "socket-timeout",
	"Error: no project found":                                          "no-project-found",

	"OpsOutOfOrderError: project structure version out of order on incoming updates": "incoming-project-version-out-of-order",
	"OpsOutOfOrderError: doc version out of order on incoming updates":               "incoming-doc-version-out-of-order",
	"OpsOutOfOrderError: project structure version out of order":                     "chunk-project-version-out-of-order",
	"OpsOutOfOrderError: doc version out of order":                                   "chunk-doc-version-out-of-order",

	"Error: failed to extend lock":                             "lock-overrun",
	"Error: tried to release timed out lock":                   "lock-overrun",
	"Error: Timeout":                                           "lock-overrun",
	"Error: sync ongoing":                                      "sync-ongoing",
	"SyncError: unexpected resyncProjectStructure update":      "sync-error",
	"[object Error]":                                           "unknown-error-object",
	"UpdateWithUnknownFormatError: update with unknown format": "unknown-format",
	"Error: update with unknown format":                        "unknown-format",

	"TextOperationError: The base length of the second operation has to be the target length of the first operation": "text-op-error",

	"Error: ENOSPC: no space left on device, write": "ENOSPC",
	"*": "other",
}

// FailureSummary is how many projects are failing, by kind of failure.
type FailureSummary struct {
	Counts       map[string]int `json:"counts"`
	Attempts     map[string]int `json:"attempts"`
	Requests     map[string]int `json:"requests"`
	MaxQueueSize map[string]int `json:"maxQueueSize"`
}

// SummariseFailures counts the failure records by kind.
//
// Every known kind is reported even when nothing is failing that way, because
// these become gauges: one that stopped being reported would keep its last
// value rather than falling to zero.
func (s *Store) SummariseFailures(ctx context.Context) (*FailureSummary, error) {
	failures, err := s.GetFailures(ctx)
	if err != nil {
		return nil, err
	}

	summary := &FailureSummary{
		Counts: map[string]int{}, Attempts: map[string]int{},
		Requests: map[string]int{}, MaxQueueSize: map[string]int{},
	}
	for _, label := range shortErrorNames {
		summary.Counts[label] = 0
		summary.Attempts[label] = 0
		summary.Requests[label] = 0
		summary.MaxQueueSize[label] = 0
	}

	for _, failure := range failures {
		kind := failure.Error
		if kind == "" {
			// A record with no error is a resync that has not finished.
			kind = "resync"
		}
		label, known := shortErrorNames[kind]
		if !known {
			label = shortErrorNames["*"]
		}

		attempts := failure.Attempts
		if attempts == 0 {
			attempts = 1
		}
		summary.Counts[label]++
		summary.Attempts[label] += attempts
		summary.Requests[label] += failure.RequestCount
		if failure.QueueSize > summary.MaxQueueSize[label] {
			summary.MaxQueueSize[label] = failure.QueueSize
		}
	}
	return summary, nil
}

// FailureCategory is the label a failure is counted under, for the record
// listing.
func FailureCategory(failure *Failure) string {
	if failure.Error == "" {
		return ""
	}
	return shortErrorNames[failure.Error]
}
