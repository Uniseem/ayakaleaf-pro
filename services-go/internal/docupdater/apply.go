package docupdater

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/textot"
)

// maxAgeOfOp is how many versions behind an update may be and still be
// transformed here. Anything older is handed back to the client to transform
// itself, because the server no longer keeps the operations in between.
const maxAgeOfOp = 80

// Errors from applying an update. The Node service distinguishes these by
// message, and the client acts on them: a duplicate is acknowledged silently, a
// delete mismatch means the client and server disagree about the text.
var (
	// ErrOpAlreadySubmitted means the client resent an operation the server
	// already applied. It is acknowledged rather than applied again.
	ErrOpAlreadySubmitted = errors.New("op already submitted")
	// ErrOpAtFutureVersion means the client claims a version the document has
	// not reached, which cannot be transformed against anything.
	ErrOpAtFutureVersion = errors.New("op at future version")
	// ErrOpTooOld means the operations needed to catch the update up are gone.
	ErrOpTooOld = errors.New("op too old")
	// ErrVersionMissing means the update carried no version at all.
	ErrVersionMissing = errors.New("version missing")
	// ErrDocTooLargeAfterUpdate means applying the update would push the
	// document past the size limit.
	ErrDocTooLargeAfterUpdate = errors.New("update takes doc over max doc size")
	// ErrInvalidHash means the client's checksum of the resulting document does
	// not match the server's, so the two have diverged.
	ErrInvalidHash = errors.New("invalid hash")
)

// AppliedUpdate is the result of applying one update.
type AppliedUpdate struct {
	// Lines is the document after the update.
	Lines []string
	// Version is what the document is at afterwards.
	Version int64
	// Applied is the update as it was actually applied: the same update with
	// its operation transformed against anything that landed in between, and
	// its version moved up accordingly. This is what is stored and republished.
	Applied *Update
	// Duplicate is true when the client resent an operation already applied.
	Duplicate bool
}

// applyUpdate applies one update to a document, transforming it against the
// operations applied since the client last saw it.
//
// It is the Go port of ShareJsUpdateManager.applyUpdate together with the
// applyOp path of the vendored ShareJS model. The model's own caching and
// persistence are not ported: a fresh model is built for every update in the
// Node service too, seeded with the lines it was handed, so nothing of the
// model survives between calls.
func (m *UpdateManager) applyUpdate(
	ctx context.Context, projectID, docID string, update *Update,
	lines []string, version int64,
) (*AppliedUpdate, error) {
	if update.V < 0 {
		return nil, ErrVersionMissing
	}
	if update.V > version {
		return nil, fmt.Errorf("%w: at %d, doc is at %d", ErrOpAtFutureVersion, update.V, version)
	}
	if update.V+maxAgeOfOp < version {
		return nil, fmt.Errorf("%w: at %d, doc is at %d", ErrOpTooOld, update.V, version)
	}

	var op textot.Op
	if err := json.Unmarshal(update.Op, &op); err != nil {
		return nil, fmt.Errorf("malformed operation: %w", err)
	}

	incomingVersion := update.V
	transformed := op
	appliedVersion := update.V

	if update.V < version {
		previous, err := m.redis.GetPreviousDocOps(ctx, docID, update.V, version)
		if err != nil {
			return nil, err
		}
		if int64(len(previous)) != version-update.V {
			// Asking for a range and getting fewer than that means the update
			// would be transformed against an incomplete history, which is
			// worse than refusing it.
			return nil, fmt.Errorf("could not get old ops for doc %s: expected %d, got %d",
				docID, version-update.V, len(previous))
		}

		for _, raw := range previous {
			var old Update
			if err := json.Unmarshal(raw, &old); err != nil {
				return nil, fmt.Errorf("malformed stored op: %w", err)
			}
			// The client tells us which sources it has already sent this
			// operation under. Seeing one of them back means this is a resend,
			// not a new edit.
			if source := metaString(old.Meta, "source"); source != "" {
				for _, dupSource := range update.DupIfSource {
					if dupSource == source {
						return &AppliedUpdate{Duplicate: true}, nil
					}
				}
			}

			var oldOp textot.Op
			if err := json.Unmarshal(old.Op, &oldOp); err != nil {
				return nil, fmt.Errorf("malformed stored operation: %w", err)
			}
			transformed, err = textot.Transform(transformed, oldOp, textot.Left)
			if err != nil {
				return nil, err
			}
			appliedVersion++
		}
	}

	snapshot, err := textot.Apply(textot.JoinLines(lines), transformed)
	if err != nil {
		return nil, err
	}
	if snapshot.Len() > m.maxDocLength {
		return nil, ErrDocTooLargeAfterUpdate
	}

	// The client sends a checksum of what it expects the document to become,
	// but only when nothing was applied in between: with a transform, the two
	// sides are computing over different text and would never agree.
	if update.Hash != "" && incomingVersion == version {
		if computed := blobHash(snapshot.String()); computed != update.Hash {
			return nil, fmt.Errorf("%w: computed %s, client sent %s",
				ErrInvalidHash, computed, update.Hash)
		}
	}

	applied := *update
	applied.V = appliedVersion
	encoded, err := json.Marshal(transformed)
	if err != nil {
		return nil, err
	}
	applied.Op = encoded
	applied.Meta = withMetaTimestamp(applied.Meta)

	return &AppliedUpdate{
		Lines: textot.SplitLines(snapshot),
		// The document ends up one version beyond where the operation applied.
		Version: appliedVersion + 1,
		Applied: &applied,
	}, nil
}

// blobHash is the checksum the client sends with an update.
//
// It is the git blob hash of the document text -- not the plain SHA-1 of the
// serialised lines that RedisStore keeps. The two cover different bytes.
func blobHash(content string) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(content)) + "\x00"))
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// metaString reads one string field out of an update's metadata, which is
// carried as raw JSON so that the fields this service does not read survive.
func metaString(meta json.RawMessage, field string) string {
	if len(meta) == 0 {
		return ""
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(meta, &fields); err != nil {
		return ""
	}
	raw, ok := fields[field]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// metaHas reports whether a metadata field is present.
func metaHas(meta json.RawMessage, field string) bool {
	if len(meta) == 0 {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(meta, &fields); err != nil {
		return false
	}
	_, ok := fields[field]
	return ok
}

// withMetaTimestamp stamps the update with the time it was applied, which the
// model does before transforming.
func withMetaTimestamp(meta json.RawMessage) json.RawMessage {
	fields := map[string]json.RawMessage{}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &fields)
	}
	fields["ts"], _ = json.Marshal(nowMillis())
	encoded, err := json.Marshal(fields)
	if err != nil {
		return meta
	}
	return encoded
}

// sanitizeUpdate replaces lone surrogate characters in inserted text.
//
// A JavaScript string is a sequence of 16-bit units, and something client side
// occasionally splits a surrogate pair so that half of one arrives on its own.
// U+D835 -- the first half of the mathematical bold characters -- is the usual
// offender. A lone surrogate cannot be stored, so it is replaced here rather
// than allowed to corrupt the document.
func sanitizeUpdate(update *Update) error {
	if len(update.Op) == 0 {
		return nil
	}
	var op textot.Op
	if err := json.Unmarshal(update.Op, &op); err != nil {
		// A malformed operation is left for applyUpdate to reject, which
		// reports it with the version and document that produced it.
		return nil
	}

	changed := false
	for i, c := range op {
		if c.Kind != textot.Insert {
			continue
		}
		cleaned := make(textot.Text, len(c.Text))
		copy(cleaned, c.Text)
		for j, unit := range cleaned {
			if unit >= 0xD800 && unit <= 0xDFFF {
				cleaned[j] = 0xFFFD
				changed = true
			}
		}
		op[i].Text = cleaned
	}
	if !changed {
		return nil
	}
	encoded, err := json.Marshal(op)
	if err != nil {
		return err
	}
	update.Op = encoded
	return nil
}
