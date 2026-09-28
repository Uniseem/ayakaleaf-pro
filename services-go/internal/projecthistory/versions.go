package projecthistory

import "strconv"

// A project version is a dotted pair -- "12.3" -- where the first number counts
// changes to the file tree and the second counts changes made while the tree
// stayed as it was. Comparing them is comparing the numbers in turn.
//
// The values arrive as strings and have to be compared as versions, so "12.3"
// is after "9.1"; comparing them as text would put it before.

// versionPart is one number of a version, which may not be a number at all.
type versionPart struct {
	value  int
	number bool
}

// compareVersions orders two versions.
func compareVersions(a, b string) int {
	left, right := versionParts(a), versionParts(b)
	for len(left) > 0 || len(right) > 0 {
		var x, y *versionPart
		if len(left) > 0 {
			x, left = &left[0], left[1:]
		}
		if len(right) > 0 {
			y, right = &right[0], right[1:]
		}
		// A part that is not a number compares as neither greater nor less,
		// which is what the arithmetic on the other side does with it, so only
		// the presence of a part decides.
		if x != nil && y != nil && x.number && y.number {
			if x.value > y.value {
				return 1
			}
			if x.value < y.value {
				return -1
			}
		}
		if x != nil && y == nil {
			return 1
		}
		if x == nil && y != nil {
			return -1
		}
	}
	return 0
}

// versionLess reports whether a comes before b.
func versionLess(a, b string) bool { return compareVersions(a, b) < 0 }

// versionParts splits a version into its numbers.
func versionParts(version string) []versionPart {
	var parts []versionPart
	start := 0
	for i := 0; i <= len(version); i++ {
		if i < len(version) && version[i] != '.' {
			continue
		}
		value, err := strconv.Atoi(version[start:i])
		parts = append(parts, versionPart{value: value, number: err == nil})
		start = i + 1
	}
	return parts
}
