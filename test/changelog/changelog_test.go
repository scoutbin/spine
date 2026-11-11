package changelog_test

import (
	"testing"

	"github.com/scoutbin/spine/internal/changelog"
)

func TestPrepend(t *testing.T) {
	testcases := []struct {
		name     string
		content  string
		tag      string
		subjects []string
		want     string
		wantErr  bool
	}{
		{
			name:     "empty content creates the titled file",
			tag:      "v1.0.0",
			subjects: []string{"Add parser", "Fix crash"},
			want:     "# Changelog\n\n## v1.0.0 - 2026-11-11\n\n- Add parser\n- Fix crash\n",
		},
		{
			name:     "section goes above the newest section",
			content:  "# Changelog\n\n## v1.0.0 - 2026-10-01\n\n- Old\n\n## v0.9.0 - 2026-09-01\n\n- Older\n",
			tag:      "v1.1.0",
			subjects: []string{"New"},
			want:     "# Changelog\n\n## v1.1.0 - 2026-11-11\n\n- New\n\n## v1.0.0 - 2026-10-01\n\n- Old\n\n## v0.9.0 - 2026-09-01\n\n- Older\n",
		},
		{
			name:     "file without a title keeps its start",
			content:  "## v1.0.0 - 2026-10-01\n\n- Old\n",
			tag:      "v1.1.0",
			subjects: []string{"New"},
			want:     "## v1.1.0 - 2026-11-11\n\n- New\n\n## v1.0.0 - 2026-10-01\n\n- Old\n",
		},
		{
			name:     "title without sections appends at the end",
			content:  "# Changelog\n",
			tag:      "v1.0.0",
			subjects: []string{"First"},
			want:     "# Changelog\n\n## v1.0.0 - 2026-11-11\n\n- First\n",
		},
		{
			name:     "text without a heading and without a final newline appends at the end",
			content:  "# Changelog\n\nNotable changes.",
			tag:      "v1.0.0",
			subjects: []string{"First"},
			want:     "# Changelog\n\nNotable changes.\n\n## v1.0.0 - 2026-11-11\n\n- First\n",
		},
		{
			name:     "existing section is refused",
			content:  "# Changelog\n\n## v1.0.0 - 2026-10-01\n\n- Old\n",
			tag:      "v1.0.0",
			subjects: []string{"New"},
			wantErr:  true,
		},
		{
			name:     "existing section without a date is refused",
			content:  "# Changelog\n\n## v1.0.0\n\n- Old\n",
			tag:      "v1.0.0",
			subjects: []string{"New"},
			wantErr:  true,
		},
		{
			name:     "prerelease section does not block its stable tag",
			content:  "# Changelog\n\n## v1.2.0-rc.1 - 2026-10-01\n\n- Old\n",
			tag:      "v1.2.0",
			subjects: []string{"New"},
			want:     "# Changelog\n\n## v1.2.0 - 2026-11-11\n\n- New\n\n## v1.2.0-rc.1 - 2026-10-01\n\n- Old\n",
		},
		{
			name:     "stable section does not block a prerelease tag",
			content:  "# Changelog\n\n## v1.2.0 - 2026-10-01\n\n- Old\n",
			tag:      "v1.2.0-rc.1",
			subjects: []string{"New"},
			want:     "# Changelog\n\n## v1.2.0-rc.1 - 2026-11-11\n\n- New\n\n## v1.2.0 - 2026-10-01\n\n- Old\n",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := changelog.Prepend([]byte(tc.content), tc.tag, "2026-11-11", tc.subjects)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Prepend() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepend() error = %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Prepend() =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestSection(t *testing.T) {
	const content = "# Changelog\n\n" +
		"## v1.2.0 - 2026-11-11\n\n- Stable\n- Stable two\n\n" +
		"## v1.2.0-rc.1 - 2026-11-01\n\n- Candidate\n\n\n" +
		"### Not a section\n\n" +
		"## v1.1.0\n- Bare\n"

	testcases := []struct {
		name    string
		content string
		tag     string
		want    string
		wantOK  bool
	}{
		{name: "section in the middle", content: content, tag: "v1.2.0-rc.1", want: "- Candidate\n\n\n### Not a section", wantOK: true},
		{name: "section at the top stops at the next heading", content: content, tag: "v1.2.0", want: "- Stable\n- Stable two", wantOK: true},
		{name: "section at the end of the file", content: content, tag: "v1.1.0", want: "- Bare", wantOK: true},
		{name: "missing section", content: content, tag: "v1.3.0"},
		{name: "prefix of another tag is not a match", content: content, tag: "v1.2", want: "", wantOK: false},
		{name: "empty content", content: "", tag: "v1.0.0"},
		{name: "heading without body", content: "## v1.0.0 - 2026-11-11\n", tag: "v1.0.0", want: "", wantOK: true},
		{name: "only the prerelease section exists", content: "## v1.2.0-rc.1 - 2026-11-01\n\n- Candidate\n", tag: "v1.2.0"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := changelog.Section([]byte(tc.content), tc.tag)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("Section(%q) = (%q, %v), want (%q, %v)", tc.tag, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
