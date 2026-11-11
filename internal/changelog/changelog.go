// Package changelog reads and writes CHANGELOG.md and lists the git history a
// release section is drafted from.
package changelog

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const (
	title         = "# Changelog"
	headingPrefix = "## "
)

// Prepend adds a section for tag above the newest existing section of content,
// or at the end when there is none. Empty content yields a new titled file.
func Prepend(content []byte, tag, date string, subjects []string) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	if _, found := findSection(lines, tag); found {
		return nil, fmt.Errorf("CHANGELOG.md already has a section for %s", tag)
	}

	var section strings.Builder
	fmt.Fprintf(&section, "%s%s - %s\n\n", headingPrefix, tag, date)
	for _, subject := range subjects {
		fmt.Fprintf(&section, "- %s\n", subject)
	}

	if len(bytes.TrimSpace(content)) == 0 {
		return []byte(title + "\n\n" + section.String()), nil
	}
	offset := 0
	for _, line := range lines {
		if isHeading(line) {
			return []byte(string(content[:offset]) + section.String() + "\n" + string(content[offset:])), nil
		}
		offset += len(line) + 1
	}
	return []byte(strings.TrimRight(string(content), "\n") + "\n\n" + section.String()), nil
}

// Section returns the body of the section for tag, without surrounding blank
// lines. The boolean is false when content has no section for tag.
func Section(content []byte, tag string) (string, bool) {
	lines := strings.Split(string(content), "\n")
	start, found := findSection(lines, tag)
	if !found {
		return "", false
	}
	body := lines[start+1:]
	for i, line := range body {
		if isHeading(line) {
			body = body[:i]
			break
		}
	}
	return strings.TrimSpace(strings.Join(body, "\n")), true
}

// MergedTags returns the tags reachable from HEAD in the current directory.
func MergedTags() ([]string, error) {
	return gitLines("tag", "--merged", "HEAD", "--sort=refname", "--format=%(refname:strip=2)")
}

// Subjects returns the subjects of the non-merge commits in base..HEAD, newest
// first. An empty base lists every commit reachable from HEAD.
func Subjects(base string) ([]string, error) {
	revisions := "HEAD"
	if base != "" {
		revisions = base + "..HEAD"
	}
	return gitLines("log", "--no-merges", "--no-show-signature", "--encoding=UTF-8", "--format=%s", revisions, "--")
}

func isHeading(line string) bool {
	return strings.HasPrefix(line, headingPrefix)
}

// findSection returns the index of the heading line that belongs to tag: either
// exactly "## <tag>" or "## <tag> ...", so v1.2.0 never matches v1.2.0-rc.1.
func findSection(lines []string, tag string) (int, bool) {
	for i, line := range lines {
		if !isHeading(line) {
			continue
		}
		heading := strings.TrimPrefix(line, headingPrefix)
		if heading == tag || strings.HasPrefix(heading, tag+" ") {
			return i, true
		}
	}
	return 0, false
}

func gitLines(args ...string) ([]string, error) {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w\n%s", args[0], err, bytes.TrimSpace(stderr.Bytes()))
	}
	var lines []string
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
