package manifest_test

import (
	"reflect"
	"testing"

	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/test/testutil"
)

func TestTargets(t *testing.T) {
	want := []manifest.Target{
		{GOOS: "darwin", GOARCH: "amd64"}, {GOOS: "darwin", GOARCH: "arm64"},
		{GOOS: "linux", GOARCH: "amd64"}, {GOOS: "linux", GOARCH: "arm64"},
		{GOOS: "windows", GOARCH: "amd64"}, {GOOS: "windows", GOARCH: "arm64"},
	}
	tool := testutil.Tool()
	got := tool.Targets()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	got[0].GOOS = "plan9"
	if again := tool.Targets(); !reflect.DeepEqual(again, want) {
		t.Errorf("mutating the result changed Targets: %v", again)
	}
}
