package persistor

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSKeyMapping(t *testing.T) {
	ctx := context.Background()
	bucket := t.TempDir()

	// Flattened by default: FSPersistor._getFsPath turns "/" into "_" unless
	// subdirectories are in use.
	flat := NewFS(false)
	if err := flat.SendStream(ctx, bucket, "aa/bb/cc", strings.NewReader("flat"), GetOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bucket, "aa_bb_cc")); err != nil {
		t.Errorf("expected a flattened path aa_bb_cc: %v", err)
	}

	// History blobs ask for real directories.
	if err := flat.SendStream(ctx, bucket, "dd/ee/ff", strings.NewReader("nested"),
		GetOptions{UseSubdirectories: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bucket, "dd", "ee", "ff")); err != nil {
		t.Errorf("expected a nested path dd/ee/ff: %v", err)
	}
}

func TestFSRoundTripAndRange(t *testing.T) {
	ctx := context.Background()
	bucket := t.TempDir()
	p := NewFS(false)

	const body = "0123456789"
	if err := p.SendStream(ctx, bucket, "k", strings.NewReader(body), GetOptions{}); err != nil {
		t.Fatal(err)
	}

	r, err := p.GetObject(ctx, bucket, "k", GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if string(got) != body {
		t.Errorf("full read = %q, want %q", got, body)
	}

	// Range is inclusive of End, as in the Range header.
	r, err = p.GetObject(ctx, bucket, "k", GetOptions{Start: 2, End: 5, HasRange: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(r)
	_ = r.Close()
	if string(got) != "2345" {
		t.Errorf("range 2-5 = %q, want %q", got, "2345")
	}

	if size, err := p.GetObjectSize(ctx, bucket, "k", GetOptions{}); err != nil || size != 10 {
		t.Errorf("size = %d, %v; want 10, nil", size, err)
	}
	if ok, err := p.ObjectExists(ctx, bucket, "k", GetOptions{}); err != nil || !ok {
		t.Errorf("ObjectExists = %v, %v; want true, nil", ok, err)
	}
}

func TestFSMissingObject(t *testing.T) {
	ctx := context.Background()
	p := NewFS(false)
	if _, err := p.GetObject(ctx, t.TempDir(), "nope", GetOptions{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetObject on a missing key = %v, want ErrNotFound", err)
	}
	if ok, err := p.ObjectExists(ctx, t.TempDir(), "nope", GetOptions{}); err != nil || ok {
		t.Errorf("ObjectExists = %v, %v; want false, nil", ok, err)
	}
	// Deleting something absent is not an error.
	if err := p.DeleteObject(ctx, t.TempDir(), "nope", GetOptions{}); err != nil {
		t.Errorf("DeleteObject on a missing key = %v, want nil", err)
	}
}

// A key must not be able to walk out of its bucket.
func TestFSRejectsTraversal(t *testing.T) {
	ctx := context.Background()
	bucket := t.TempDir()
	p := NewFS(true) // subdirectories on, so "/" is not flattened away
	if _, err := p.GetObject(ctx, bucket, "../../etc/passwd", GetOptions{}); err == nil ||
		errors.Is(err, ErrNotFound) {
		t.Errorf("traversal key = %v, want a rejection", err)
	}
}

func TestFSDeleteDirectory(t *testing.T) {
	ctx := context.Background()
	bucket := t.TempDir()

	// Flattened: members are siblings named "<prefix>_...".
	flat := NewFS(false)
	for _, k := range []string{"pre/one", "pre/two", "other/three"} {
		if err := flat.SendStream(ctx, bucket, k, strings.NewReader("x"), GetOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := flat.DeleteDirectory(ctx, bucket, "pre", GetOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"pre/one", "pre/two"} {
		if ok, _ := flat.ObjectExists(ctx, bucket, k, GetOptions{}); ok {
			t.Errorf("%s should have been deleted", k)
		}
	}
	if ok, _ := flat.ObjectExists(ctx, bucket, "other/three", GetOptions{}); !ok {
		t.Error("other/three should have survived")
	}

	// Nested: the prefix is a real directory.
	nested := NewFS(true)
	for _, k := range []string{"dir/a", "dir/b"} {
		if err := nested.SendStream(ctx, bucket, k, strings.NewReader("x"), GetOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := nested.DeleteDirectory(ctx, bucket, "dir", GetOptions{}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := nested.ObjectExists(ctx, bucket, "dir/a", GetOptions{}); ok {
		t.Error("dir/a should have been deleted")
	}
}
