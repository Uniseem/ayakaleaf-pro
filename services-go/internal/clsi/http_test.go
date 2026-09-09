package clsi

import "testing"

// synctex tells position from size by case alone, and a parser that folds
// case reads one position as several wrong ones. This is that output.
const viewOutput = `This is SyncTeX command line utility, version 1.5
SyncTeX result begin
Output:output.pdf
Page:1
x:157.318939
y:156.585541
h:133.768356
v:158.799469
W:343.711060
H:9.132425
before:
offset:-1
middle:
after:
SyncTeX result end
`

func TestParseSyncOutputKeepsPositionAndSizeApart(t *testing.T) {
	records := parseSyncOutput(viewOutput,
		syncField{"Page", "page"},
		syncField{"h", "h"},
		syncField{"v", "v"},
		syncField{"W", "width"},
		syncField{"H", "height"},
	)

	if len(records) != 1 {
		t.Fatalf("one position was printed, got %d: %v", len(records), records)
	}
	for field, want := range map[string]float64{
		"page":   1,
		"h":      133.768356,
		"v":      158.799469,
		"width":  343.711060,
		"height": 9.132425,
	} {
		if got := records[0][field]; got != want {
			t.Errorf("%s = %v, want %v", field, got, want)
		}
	}
}

// A line typeset more than once produces several positions, separated by
// nothing but a field appearing again.
func TestParseSyncOutputSplitsRepeatedPositions(t *testing.T) {
	output := `Page:1
h:10
v:20
W:100
H:8
Page:2
h:30
v:40
W:200
H:9
`
	records := parseSyncOutput(output,
		syncField{"Page", "page"},
		syncField{"h", "h"},
		syncField{"v", "v"},
		syncField{"W", "width"},
		syncField{"H", "height"},
	)
	if len(records) != 2 {
		t.Fatalf("two positions were printed, got %d: %v", len(records), records)
	}
	if records[0]["page"] != float64(1) || records[1]["page"] != float64(2) {
		t.Errorf("pages came out as %v and %v", records[0]["page"], records[1]["page"])
	}
	if records[1]["height"] != float64(9) {
		t.Errorf("second height = %v, want 9", records[1]["height"])
	}
}

func TestParseSyncOutputReadsTheReverseDirection(t *testing.T) {
	output := `SyncTeX result begin
Input:./chapters/one.tex
Line:42
Column:-1
SyncTeX result end
`
	records := parseSyncOutput(output,
		syncField{"Input", "file"},
		syncField{"Line", "line"},
		syncField{"Column", "column"},
	)
	if len(records) != 1 {
		t.Fatalf("one position was printed, got %d: %v", len(records), records)
	}
	if records[0]["file"] != "./chapters/one.tex" {
		t.Errorf("file = %v", records[0]["file"])
	}
	if records[0]["line"] != float64(42) {
		t.Errorf("line = %v, want 42", records[0]["line"])
	}
}

// Fields nothing asked for are ignored, and must not end a record: "x" and
// "y" sit between "Page" and "h" in every position synctex prints.
func TestParseSyncOutputIgnoresFieldsNobodyAskedFor(t *testing.T) {
	records := parseSyncOutput(viewOutput, syncField{"Page", "page"}, syncField{"h", "h"})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1: %v", len(records), records)
	}
	if _, present := records[0]["x"]; present {
		t.Error("x was kept")
	}
}
