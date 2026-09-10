package documents

import (
	"strings"
	"testing"
)

// A name in a zip is whatever the program that wrote it put there, and some
// of those names are attempts to write outside the project.
func TestSafeArchivePathRefusesWhatIsNotAPathInTheProject(t *testing.T) {
	for _, name := range []string{
		"../escape.tex",
		"a/../../escape.tex",
		"/etc/passwd",
		"C:\\Windows\\system32\\evil.tex",
		"..",
		"",
		"chapters/\x00.tex",
	} {
		if got, ok := safeArchivePath(name); ok {
			t.Errorf("safeArchivePath(%q) allowed it as %q", name, got)
		}
	}
}

func TestSafeArchivePathKeepsOrdinaryFiles(t *testing.T) {
	for from, want := range map[string]string{
		"main.tex":                  "main.tex",
		"./main.tex":                "main.tex",
		"chapters/one.tex":          "chapters/one.tex",
		"chapters\\one.tex":         "chapters/one.tex",
		"figures/./plot.png":        "figures/plot.png",
		"paper/chapters/../fig.pdf": "paper/fig.pdf",
		"latexmkrc":                 "latexmkrc",
		".latexmkrc":                ".latexmkrc",
	} {
		got, ok := safeArchivePath(from)
		if !ok {
			t.Errorf("safeArchivePath(%q) refused it", from)
			continue
		}
		if got != want {
			t.Errorf("safeArchivePath(%q) = %q, want %q", from, got, want)
		}
	}
}

// Tooling and build output are somebody's working directory, not their
// project. A stale .aux or .bbl is worse than untidy: LaTeX reads it in
// preference to regenerating it.
func TestSafeArchivePathSkipsToolingAndBuildOutput(t *testing.T) {
	for _, name := range []string{
		"__MACOSX/._main.tex",
		"__macosx/._main.tex",
		".git/config",
		"paper/.git/HEAD",
		"main.aux",
		"main.bbl",
		"chapters/one.log",
		"main.synctex.gz",
		".DS_Store",
	} {
		if got, ok := safeArchivePath(name); ok {
			t.Errorf("safeArchivePath(%q) allowed it as %q", name, got)
		}
	}
}

// A zip made by right-clicking a folder holds one folder holding everything.
func TestWithoutCommonRootLiftsASingleFolder(t *testing.T) {
	lifted := withoutCommonRoot([]importEntry{
		{path: "paper/main.tex"},
		{path: "paper/chapters/one.tex"},
		{path: "paper/figures/plot.png"},
	})
	want := []string{"main.tex", "chapters/one.tex", "figures/plot.png"}
	for index, entry := range lifted {
		if entry.path != want[index] {
			t.Errorf("entry %d = %q, want %q", index, entry.path, want[index])
		}
	}
}

func TestWithoutCommonRootLeavesEverythingElseAlone(t *testing.T) {
	// Two folders at the top: neither is "the" folder.
	two := []importEntry{{path: "paper/main.tex"}, {path: "notes/one.tex"}}
	if lifted := withoutCommonRoot(two); lifted[0].path != "paper/main.tex" {
		t.Errorf("two top-level folders were collapsed: %q", lifted[0].path)
	}
	// A file at the top means the project is already at the top.
	mixed := []importEntry{{path: "main.tex"}, {path: "chapters/one.tex"}}
	if lifted := withoutCommonRoot(mixed); lifted[1].path != "chapters/one.tex" {
		t.Errorf("a top-level file was collapsed away: %q", lifted[1].path)
	}
}

// The name says what a file is meant to be; the content says whether it can
// be. The history stores a document as text, so anything else is a file.
func TestIsText(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"main.tex", "\\documentclass{article}", true},
		{"sample.bib", "@book{a,b=c}", true},
		{"Makefile", "all:\n\techo hi", true},
		{"latexmkrc", "$pdf_mode = 1;", true},
		{"frog.jpg", "\xff\xd8\xff\xe0 jpeg", false},
		{"notes.tex", "text with a \x00 in it", false},
		{"broken.tex", "\xff\xfe not utf-8", false},
		{"huge.tex", strings.Repeat("x", maxDocBytes+1), false},
	}
	for _, each := range cases {
		if got := isText(each.name, []byte(each.content)); got != each.want {
			t.Errorf("isText(%q) = %v, want %v", each.name, got, each.want)
		}
	}
}

// The file to compile is the one that declares a document class, shallowest
// first -- which is where a paper's main file almost always is.
func TestRootDocumentIn(t *testing.T) {
	entries := []importEntry{
		{path: "chapters/one.tex", text: true, content: []byte("\\section{One}")},
		{path: "preamble/setup.tex", text: true, content: []byte("\\usepackage{amsmath}")},
		{path: "paper.tex", text: true, content: []byte("\\documentclass{article}\n\\title{A Paper}\n")},
		{path: "figures/plot.png", text: false, content: []byte("\xff\xd8")},
	}
	root, title := rootDocumentIn(entries)
	if root != "paper.tex" {
		t.Errorf("root = %q, want paper.tex", root)
	}
	if title != "A Paper" {
		t.Errorf("title = %q, want A Paper", title)
	}
}

func TestRootDocumentInPrefersTheShallowest(t *testing.T) {
	entries := []importEntry{
		{path: "src/deep/main.tex", text: true, content: []byte("\\documentclass{article}")},
		{path: "main.tex", text: true, content: []byte("\\documentclass{report}")},
	}
	if root, _ := rootDocumentIn(entries); root != "main.tex" {
		t.Errorf("root = %q, want main.tex", root)
	}
}

// A commented-out document class is not a document class.
func TestRootDocumentInIgnoresACommentedClass(t *testing.T) {
	entries := []importEntry{
		{path: "notes.tex", text: true, content: []byte("% \\documentclass{article}\nnotes")},
	}
	if root, _ := rootDocumentIn(entries); root != "" {
		t.Errorf("root = %q, want none", root)
	}
}

// A title is a line of LaTeX, not a name: it can carry commands and breaks.
func TestTitleIn(t *testing.T) {
	for content, want := range map[string]string{
		"\\title{A Paper}":          "A Paper",
		"\\Title{Shouting}":         "Shouting",
		"\\title{Two\\\\Lines}":     "Two Lines",
		"\\title{  spaced   out  }": "spaced out",
		// A short form for running heads. It is the better name for a
		// project: the abbreviation its author already chose.
		"\\title[Short]{Long and unwieldy}": "Short",
		"no title here":                     "",
	} {
		if got := titleIn(content); got != want {
			t.Errorf("titleIn(%q) = %q, want %q", content, got, want)
		}
	}
}
