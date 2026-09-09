package historystore

import (
	"context"
	"fmt"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/persistor"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// DestroyProject removes a project's history and everything it refers to.
//
// A history is the largest thing a project owns: every version of every file
// it has ever had, and a copy of every image ever put in it. Deleting the
// project record and leaving this behind means the project is gone from the
// site while all of its content is still on the disk, for good, under an id
// nothing points at any more.
//
// The order is chosen so that a failure part-way through leaves something
// that can be tried again rather than something unreachable. The objects go
// first and the records that name them last: a record without its object
// reads as a broken history, which is what this is producing on purpose,
// while an object without its record is a file nothing can ever find.
func (s *Store) DestroyProject(ctx context.Context, historyID string) error {
	if historyID == "" {
		return fmt.Errorf("%w: no project", ErrBadRequest)
	}
	prefix := objectKey(historyID) + "/"

	// Global blobs are shared with every other project and are not touched:
	// they are the empty file and a handful of common ones, and deleting them
	// here would empty a file in somebody else's project.
	if err := s.objects.DeleteDirectory(
		ctx, s.buckets.ProjectBlobs, prefix, persistor.GetOptions{},
	); err != nil {
		return fmt.Errorf("removing the project's files: %w", err)
	}
	if err := s.objects.DeleteDirectory(
		ctx, s.buckets.Chunks, prefix, persistor.GetOptions{},
	); err != nil {
		return fmt.Errorf("removing the project's history: %w", err)
	}

	id, err := bson.ObjectIDFromHex(historyID)
	if err != nil {
		return fmt.Errorf("%w: %q is not a project", ErrBadRequest, historyID)
	}
	if _, err := s.chunks.DeleteMany(ctx, bson.M{"projectId": id}); err != nil {
		return fmt.Errorf("removing the history records: %w", err)
	}

	// The blob records are one document per project, and the sharded ones are
	// that document's overflow: their ids are the project's with a shard on
	// the end, so they are found by range rather than by equality.
	if _, err := s.blobs.DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return fmt.Errorf("removing the file records: %w", err)
	}
	first, err := shardedID(historyID, "00")
	if err != nil {
		return err
	}
	last, err := shardedID(historyID, "ff")
	if err != nil {
		return err
	}
	if _, err := s.blobs.DeleteMany(ctx, bson.M{
		"_id": bson.M{"$gte": first, "$lte": last},
	}); err != nil {
		return fmt.Errorf("removing the overflow file records: %w", err)
	}
	return nil
}
