package persistor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// These run against the MinIO started for the filestore work. Without
// S3_TEST_ENDPOINT they are skipped, so the suite stays runnable anywhere.
func testS3(t *testing.T) *S3 {
	t.Helper()
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("set S3_TEST_ENDPOINT to run the S3 persistor tests")
	}
	p, err := NewS3(context.Background(), S3Config{
		Key: os.Getenv("S3_TEST_KEY"), Secret: os.Getenv("S3_TEST_SECRET"),
		Endpoint: endpoint, PathStyle: true, SignedURLExpiry: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestS3RoundTrip(t *testing.T) {
	ctx := context.Background()
	p := testS3(t)
	bucket := os.Getenv("S3_TEST_BUCKET")
	const body = "0123456789abcdef"

	if err := p.SendStream(ctx, bucket, "dir/obj", strings.NewReader(body), GetOptions{}); err != nil {
		t.Fatal(err)
	}

	r, err := p.GetObject(ctx, bucket, "dir/obj", GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if string(got) != body {
		t.Errorf("full read = %q, want %q", got, body)
	}

	// A Range header is inclusive of the end byte.
	r, err = p.GetObject(ctx, bucket, "dir/obj", GetOptions{Start: 3, End: 7, HasRange: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(r)
	_ = r.Close()
	if string(got) != "34567" {
		t.Errorf("range 3-7 = %q, want %q", got, "34567")
	}

	if size, err := p.GetObjectSize(ctx, bucket, "dir/obj", GetOptions{}); err != nil || size != 16 {
		t.Errorf("size = %d, %v; want 16, nil", size, err)
	}
	if ok, err := p.ObjectExists(ctx, bucket, "dir/obj", GetOptions{}); err != nil || !ok {
		t.Errorf("ObjectExists = %v, %v; want true, nil", ok, err)
	}
}

// A presigned URL is what lets a client fetch straight from storage instead of
// through this service, so it has to actually work.
func TestS3RedirectURL(t *testing.T) {
	ctx := context.Background()
	p := testS3(t)
	bucket := os.Getenv("S3_TEST_BUCKET")
	if err := p.SendStream(ctx, bucket, "signed", strings.NewReader("signed body"), GetOptions{}); err != nil {
		t.Fatal(err)
	}
	url, err := p.RedirectURL(ctx, bucket, "signed")
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("presigned GET returned %d", res.StatusCode)
	}
	got, _ := io.ReadAll(res.Body)
	if string(got) != "signed body" {
		t.Errorf("presigned body = %q, want %q", got, "signed body")
	}
}

func TestS3MissingAndDelete(t *testing.T) {
	ctx := context.Background()
	p := testS3(t)
	bucket := os.Getenv("S3_TEST_BUCKET")

	if _, err := p.GetObject(ctx, bucket, "absent", GetOptions{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetObject on a missing key = %v, want ErrNotFound", err)
	}
	if ok, err := p.ObjectExists(ctx, bucket, "absent", GetOptions{}); err != nil || ok {
		t.Errorf("ObjectExists = %v, %v; want false, nil", ok, err)
	}
	if err := p.DeleteObject(ctx, bucket, "absent", GetOptions{}); err != nil {
		t.Errorf("DeleteObject on a missing key = %v, want nil", err)
	}

	// A prefix must not delete a sibling whose name merely starts the same.
	for _, k := range []string{"pre/a", "pre/b", "prefix-sibling"} {
		if err := p.SendStream(ctx, bucket, k, strings.NewReader("x"), GetOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.DeleteDirectory(ctx, bucket, "pre", GetOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"pre/a", "pre/b"} {
		if ok, _ := p.ObjectExists(ctx, bucket, k, GetOptions{}); ok {
			t.Errorf("%s should have been deleted", k)
		}
	}
	if ok, _ := p.ObjectExists(ctx, bucket, "prefix-sibling", GetOptions{}); !ok {
		t.Error("prefix-sibling should have survived")
	}
}
