// Package changelog reads and writes CHANGELOG.md, lists the git history a
// release section is drafted from, and runs the git operations that commit the
// section and tag the release.
package changelog

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

const (
	title         = "# Changelog"
	headingPrefix = "## "
)

// Prepend adds a section for tag above the newest existing section of content,
// or at the end when there is none. Empty content yields a new titled file. The
// section is the trimmed message followed by one line per subject, if any. The
// message must not be empty and must not contain a heading line, which would end
// the section early.
func Prepend(content []byte, tag, date, message string, subjects []string) ([]byte, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, errors.New("the message is empty")
	}
	if slices.ContainsFunc(strings.Split(message, "\n"), isHeading) {
		return nil, fmt.Errorf("the message must not have a line starting with %q", headingPrefix)
	}
	lines := strings.Split(string(content), "\n")
	if _, found := findSection(lines, tag); found {
		return nil, fmt.Errorf("CHANGELOG.md already has a section for %s", tag)
	}

	var section strings.Builder
	fmt.Fprintf(&section, "%s%s - %s\n\n%s\n", headingPrefix, tag, date, message)
	if len(subjects) > 0 {
		section.WriteString("\n")
	}
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
// first, leaving out commits that change nothing but file. An empty base lists
// every commit reachable from HEAD.
func Subjects(base, file string) ([]string, error) {
	revisions := "HEAD"
	if base != "" {
		revisions = base + "..HEAD"
	}
	// --full-history keeps commits that came in through a merge, which the
	// default history simplification can prune once a path limit is given.
	return gitLines("log", "--no-merges", "--full-history", "--no-show-signature", "--encoding=UTF-8", "--format=%s", revisions, "--", ".", ":(exclude)"+file)
}

// Branch returns the branch HEAD is on, or "" when HEAD is detached.
func Branch() (string, error) {
	lines, err := gitLines("branch", "--show-current")
	if err != nil || len(lines) == 0 {
		return "", err
	}
	return lines[0], nil
}

// TrackedClean reports whether no tracked file has staged or unstaged changes.
// Untracked files are ignored.
func TrackedClean() (bool, error) {
	lines, err := gitLines("status", "--porcelain", "--untracked-files=no")
	return len(lines) == 0, err
}

// TagExists reports whether a local tag named tag exists, reachable or not.
func TagExists(tag string) (bool, error) {
	lines, err := gitLines("tag", "--list", tag)
	return len(lines) > 0, err
}

// Commit commits file, and nothing else, with subject. The user's git config
// applies, including hooks and signing.
func Commit(file, subject string) error {
	if _, err := gitLines("add", "--", file); err != nil {
		return err
	}
	_, err := gitLines("commit", "--only", "--message", subject, "--", file)
	return err
}

// Tag creates a lightweight tag on HEAD.
func Tag(tag string) error {
	_, err := gitLines("tag", tag)
	return err
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
