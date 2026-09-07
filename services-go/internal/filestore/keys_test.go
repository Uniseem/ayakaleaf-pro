package filestore

import "testing"

// Checked against @overleaf/object-persistor's ProjectKey.format: pad to nine
// digits, reverse, then cut at 3 and 6. A blob written under the wrong path is
// simply lost, so these are exact.
func TestProjectKey(t *testing.T) {
	cases := map[string]string{
		"1":          "100/000/000",
		"123":        "321/000/000",
		"123456789":  "987/654/321",
		"1234567890": "098/765/4321",
		"0":          "000/000/000",
	}
	for id, want := range cases {
		got, err := ProjectKey(id)
		if err != nil {
			t.Errorf("ProjectKey(%q) errored: %v", id, err)
			continue
		}
		if got != want {
			t.Errorf("ProjectKey(%q) = %q, want %q", id, got, want)
		}
	}
	if _, err := ProjectKey("not-a-number"); err == nil {
		t.Error("a non-numeric id should be rejected")
	}
}

func TestBlobTargets(t *testing.T) {
	stores := Stores{GlobalBlobs: "gb", ProjectBlobs: "pb", TemplateFiles: "tf"}
	const hash = "0123456789abcdef0123456789abcdef01234567"

	g := GlobalBlobTarget(stores, hash)
	if g.Key != "01/23/456789abcdef0123456789abcdef01234567" {
		t.Errorf("global blob key = %q", g.Key)
	}
	if !g.UseSubdirectories {
		t.Error("history blobs are stored nested, not flattened")
	}

	p, err := ProjectBlobTarget(stores, "123", hash)
	if err != nil {
		t.Fatal(err)
	}
	if p.Key != "321/000/000/01/23456789abcdef0123456789abcdef01234567" {
		t.Errorf("project blob key = %q", p.Key)
	}
}

func TestTemplateTargetAndCacheKeys(t *testing.T) {
	stores := Stores{TemplateFiles: "tf"}
	if got := TemplateTarget(stores, "abc", "3", "pdf", "").Key; got != "abc/v/3/pdf" {
		t.Errorf("template key = %q", got)
	}
	if got := TemplateTarget(stores, "abc", "3", "pdf", "thumb").Key; got != "abc/v/3/pdf/thumb" {
		t.Errorf("template key with sub_type = %q", got)
	}
	// Template files are flattened on the filesystem backend.
	if TemplateTarget(stores, "abc", "3", "pdf", "").UseSubdirectories {
		t.Error("template files should not use subdirectories")
	}

	base := "abc/v/3/pdf"
	cases := map[string]string{
		CachedKey(base, "png", ""):          base + "-converted-cache/format-png",
		CachedKey(base, "", "thumbnail"):    base + "-converted-cache/style-thumbnail",
		CachedKey(base, "png", "thumbnail"): base + "-converted-cache/format-png-style-thumbnail",
		CachedKey(base, "", ""):             base + "-converted-cache/",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("cache key = %q, want %q", got, want)
		}
	}
}

func TestValidHash(t *testing.T) {
	if !ValidHash("0123456789abcdef0123456789abcdef01234567") {
		t.Error("a 40-char lowercase hex hash should be valid")
	}
	for _, bad := range []string{"", "xyz", "0123456789ABCDEF0123456789abcdef01234567",
		"0123456789abcdef0123456789abcdef0123456", "../../etc/passwd"} {
		if ValidHash(bad) {
			t.Errorf("ValidHash(%q) = true, want false", bad)
		}
	}
}
