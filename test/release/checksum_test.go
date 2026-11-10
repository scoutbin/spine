package release_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/scoutbin/spine/internal/release"
)

func TestVerifyChecksum(t *testing.T) {
	name := tool.ArchiveName("v0.1.3", "linux", "amd64")
	other := tool.ArchiveName("v0.1.3", "darwin", "arm64")
	content := "archive bytes"
	sum := sha256.Sum256([]byte(content))
	good := hex.EncodeToString(sum[:])
	otherSum := sha256.Sum256([]byte("different"))
	bad := hex.EncodeToString(otherSum[:])

	testcases := []struct {
		name      string
		checksums string
		asset     string
		content   io.Reader
		wantErr   error
		errHas    string
	}{
		{name: "match text separator", checksums: good + "  " + name + "\n", asset: name, content: strings.NewReader(content)},
		{name: "match binary marker", checksums: good + " *" + name + "\n", asset: name, content: strings.NewReader(content)},
		{name: "match uppercase hex", checksums: strings.ToUpper(good) + "  " + name + "\n", asset: name, content: strings.NewReader(content)},
		{name: "match among several entries", checksums: bad + "  " + other + "\n" + good + "  " + name + "\n", asset: name, content: strings.NewReader(content)},
		{name: "match without trailing newline", checksums: good + "  " + name, asset: name, content: strings.NewReader(content)},
		{name: "match with CRLF and blank lines", checksums: good + "  " + name + "\r\n\r\n", asset: name, content: strings.NewReader(content)},
		{name: "mismatch", checksums: bad + "  " + name + "\n", asset: name, content: strings.NewReader(content), wantErr: release.ErrChecksumMismatch},
		{name: "missing entry", checksums: bad + "  " + other + "\n", asset: name, content: strings.NewReader(content), wantErr: release.ErrChecksumMissing},
		{name: "empty file", checksums: "", asset: name, content: strings.NewReader(content), wantErr: release.ErrChecksumMissing},
		{name: "name is not a prefix match", checksums: good + "  " + name + ".sig\n", asset: name, content: strings.NewReader(content), wantErr: release.ErrChecksumMissing},
		{name: "malformed: no name", checksums: good + "\n", asset: name, content: strings.NewReader(content), errHas: "line 1: malformed"},
		{name: "malformed: single space", checksums: good + " " + name + "\n", asset: name, content: strings.NewReader(content), errHas: "line 1: malformed"},
		{name: "malformed: short digest", checksums: good[:40] + "  " + name + "\n", asset: name, content: strings.NewReader(content), errHas: "line 1: malformed"},
		{name: "malformed: non-hex digest", checksums: strings.Repeat("z", 64) + "  " + name + "\n", asset: name, content: strings.NewReader(content), errHas: "line 1: malformed"},
		{name: "malformed: garbage", checksums: "not a checksum line\n", asset: name, content: strings.NewReader(content), errHas: "line 1: malformed"},
		{name: "malformed line after valid one", checksums: good + "  " + name + "\nbroken\n", asset: name, content: strings.NewReader(content), errHas: "line 2: malformed"},
		{name: "duplicate names", checksums: good + "  " + name + "\n" + bad + " *" + name + "\n", asset: name, content: strings.NewReader(content), errHas: "line 2: duplicate entry"},
		{name: "read error", checksums: good + "  " + name + "\n", asset: name, content: iotest.ErrReader(errors.New("boom")), errHas: "boom"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := release.VerifyChecksum([]byte(tc.checksums), tc.asset, tc.content)
			switch {
			case tc.wantErr == nil && tc.errHas == "":
				if err != nil {
					t.Fatalf("VerifyChecksum() = %v, want nil", err)
				}
			case err == nil:
				t.Fatal("VerifyChecksum() = nil, want an error")
			case tc.wantErr != nil && !errors.Is(err, tc.wantErr):
				t.Errorf("VerifyChecksum() = %v, want %v", err, tc.wantErr)
			case tc.errHas != "" && !strings.Contains(err.Error(), tc.errHas):
				t.Errorf("VerifyChecksum() = %q, want it to contain %q", err, tc.errHas)
			}
		})
	}
}
