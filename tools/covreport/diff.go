// diff.go turns a unified diff into the line numbers each file gained. Patch
// coverage is defined over those lines: what the pull request added, not what
// it deleted or moved.
//
// The parser tracks how much of a hunk body is still unread instead of
// matching line prefixes alone, because a body line and a file header are not
// distinguishable by prefix: an added line whose text starts with "++ b/x.go"
// reaches the diff as "+++ b/x.go".

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// parseDiff extracts the added-line numbers per new-side file path from a
// unified diff. Only non-test .go files outside test/ are kept.
func parseDiff(diffPath string) (map[string][]int, error) {
	data, err := os.ReadFile(diffPath)
	if err != nil {
		return nil, err
	}
	return parseDiffBytes(data, diffPath)
}

// hunkBody counts down the lines a hunk header promised, and tracks which
// new-side line the next body line lands on.
type hunkBody struct {
	remainingOld int
	remainingNew int
	nextNewLine  int
}

func (h *hunkBody) hasMore() bool {
	return h.remainingOld > 0 || h.remainingNew > 0
}

// consume reads one body line. It reports the new-side line number and true
// when the line was added, and false for a deleted, context, or no-newline
// marker line.
func (h *hunkBody) consume(line string) (int, bool) {
	switch {
	case strings.HasPrefix(line, `\`):
		// "\ No newline at end of file" belongs to neither side.
		return 0, false
	case strings.HasPrefix(line, "+"):
		addedLine := h.nextNewLine
		h.nextNewLine++
		h.remainingNew--
		return addedLine, true
	case strings.HasPrefix(line, "-"):
		h.remainingOld--
		return 0, false
	default:
		h.nextNewLine++
		h.remainingOld--
		h.remainingNew--
		return 0, false
	}
}

// parseDiffBytes extracts added-line numbers per new-side path. source names
// where the diff came from, for error messages.
func parseDiffBytes(data []byte, source string) (map[string][]int, error) {
	added := map[string][]int{}
	current := ""
	var body hunkBody
	for _, line := range strings.Split(string(data), "\n") {
		if body.hasMore() {
			// Hunks of files outside the coverage set are still counted
			// through, so that their body never reaches the header cases.
			if addedLine, isAdded := body.consume(line); isAdded && current != "" {
				added[current] = append(added[current], addedLine)
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "+++ "):
			current = coverablePathFromHeader(line)
		case strings.HasPrefix(line, "@@ "):
			newStart, oldCount, newCount, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", source, err)
			}
			body = hunkBody{
				remainingOld: oldCount,
				remainingNew: newCount,
				nextNewLine:  newStart,
			}
		}
	}
	return added, nil
}

// coverablePathFromHeader returns the repository path a "+++ " header names,
// or "" when that file is deleted or falls outside the coverage set.
func coverablePathFromHeader(header string) string {
	name := strings.TrimSpace(strings.TrimPrefix(header, "+++ "))
	if i := strings.IndexByte(name, '\t'); i >= 0 {
		name = name[:i]
	}
	if name == "/dev/null" {
		return ""
	}
	name = strings.TrimPrefix(name, "b/")
	if !isCoverableFile(name) {
		return ""
	}
	return name
}

// parseHunkHeader reads a "@@ -a,b +c,d @@" header and returns the new-side
// start line with both side's line counts. A side written as "-a" without a
// count covers exactly one line, and a count of 0 means the hunk touches
// nothing on that side.
func parseHunkHeader(line string) (newStart, oldCount, newCount int, err error) {
	oldSpec, newSpec := "", ""
	for _, field := range strings.Fields(line) {
		switch {
		case oldSpec == "" && strings.HasPrefix(field, "-"):
			oldSpec = strings.TrimPrefix(field, "-")
		case newSpec == "" && strings.HasPrefix(field, "+"):
			newSpec = strings.TrimPrefix(field, "+")
		}
	}
	if oldSpec == "" || newSpec == "" {
		return 0, 0, 0, fmt.Errorf("malformed hunk header %q", line)
	}
	if _, oldCount, err = parseHunkSpec(oldSpec); err != nil {
		return 0, 0, 0, fmt.Errorf("malformed hunk header %q", line)
	}
	if newStart, newCount, err = parseHunkSpec(newSpec); err != nil {
		return 0, 0, 0, fmt.Errorf("malformed hunk header %q", line)
	}
	return newStart, oldCount, newCount, nil
}

// parseHunkSpec reads one "start,count" side of a hunk header.
func parseHunkSpec(spec string) (start, count int, err error) {
	count = 1
	if i := strings.IndexByte(spec, ','); i >= 0 {
		if count, err = strconv.Atoi(spec[i+1:]); err != nil {
			return 0, 0, err
		}
		spec = spec[:i]
	}
	if start, err = strconv.Atoi(spec); err != nil {
		return 0, 0, err
	}
	return start, count, nil
}
