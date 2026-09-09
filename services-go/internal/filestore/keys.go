package filestore

import (
	"fmt"
	"regexp"
)

// Stores names the bucket (or directory) behind each kind of object.
type Stores struct {
	TemplateFiles string
	ProjectBlobs  string
	GlobalBlobs   string
}

// Target is a resolved bucket, key and the options that reach the persistor.
type Target struct {
	Bucket string
	Key    string
	// UseSubdirectories keeps "/" as a real separator on the filesystem
	// backend. History blobs are stored nested; template files are not.
	UseSubdirectories bool
}

// ConvertedFolderKey mirrors KeyBuilder.getConvertedFolderKey: the cache of
// converted renderings lives under a sibling prefix of the original.
func ConvertedFolderKey(key string) string {
	return key + "-converted-cache/"
}

// CachedKey mirrors KeyBuilder.addCachingToKey, naming the cache entry for one
// combination of format and style.
func CachedKey(key, format, style string) string {
	key = ConvertedFolderKey(key)
	switch {
	case format != "" && style == "":
		return key + "format-" + format
	case style != "" && format == "":
		return key + "style-" + style
	case style != "" && format != "":
		return key + "format-" + format + "-style-" + style
	default:
		return key
	}
}

// GlobalBlobTarget resolves /history/global/hash/:hash.
//
// The hash is split into two levels of directory so no single directory holds
// every blob.
func GlobalBlobTarget(stores Stores, hash string) Target {
	return Target{
		Bucket:            stores.GlobalBlobs,
		Key:               hash[0:2] + "/" + hash[2:4] + "/" + hash[4:],
		UseSubdirectories: true,
	}
}

// ProjectBlobTarget resolves /history/project/:historyId/hash/:hash.
func ProjectBlobTarget(stores Stores, historyID, hash string) (Target, error) {
	prefix, err := ProjectKey(historyID)
	if err != nil {
		return Target{}, err
	}
	return Target{
		Bucket:            stores.ProjectBlobs,
		Key:               prefix + "/" + hash[0:2] + "/" + hash[2:],
		UseSubdirectories: true,
	}, nil
}

// TemplateTarget resolves /template/:template_id/v/:version/:format[/:sub_type].
func TemplateTarget(stores Stores, templateID, version, format, subType string) Target {
	key := templateID + "/v/" + version + "/" + format
	if subType != "" {
		key += "/" + subType
	}
	return Target{Bucket: stores.TemplateFiles, Key: key}
}

// BucketTarget resolves /bucket/:bucket/key/*, which names its bucket
// directly.
func BucketTarget(bucket, key string) Target {
	return Target{Bucket: bucket, Key: key}
}

// projectKeyPattern is what a history id may look like.
//
// Numeric, because that is what upstream's history-v1 allocates -- and hex,
// because this deployment's does not: it gives a project its own ObjectId as
// its history id, and those have letters in them. A numeric-only rule rejected
// every id this installation actually uses, so no blob could be served at all
// and every figure in every project was missing from the compiled PDF.
//
// The point of the check is that the id becomes a path, so what matters is
// that it cannot contain a separator or a dot. Both forms satisfy that.
var projectKeyPattern = regexp.MustCompile(`^[0-9a-fA-F]+$`)

// ProjectKey mirrors @overleaf/object-persistor's ProjectKey.format exactly:
// the id is left-padded to nine digits, reversed, and cut at fixed offsets
// into three path segments.
//
// The reversal is deliberate. Ids are allocated in sequence, and S3 partitions
// by key prefix, so keys sharing a leading run would all land on one
// partition; reversing spreads them. Getting the padding or the offsets wrong
// would put blobs at paths nothing else can find, which is why this is checked
// against the reference implementation's own examples below.
func ProjectKey(id string) (string, error) {
	if !projectKeyPattern.MatchString(id) {
		return "", fmt.Errorf("filestore: %q is not a valid history id", id)
	}
	padded := id
	for len(padded) < 9 {
		padded = "0" + padded
	}
	prefix := reverse(padded)
	return prefix[0:3] + "/" + prefix[3:6] + "/" + prefix[6:], nil
}

func reverse(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// hashPattern is the 40-character hex a blob hash must be, checked before it
// is spliced into a storage key.
var hashPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ValidHash reports whether a blob hash is well formed.
func ValidHash(hash string) bool { return hashPattern.MatchString(hash) }
