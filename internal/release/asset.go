package release

import "github.com/scoutbin/spine/internal/manifest"

// DownloadURL returns the download URL of a release asset for a v-prefixed tag.
func DownloadURL(t manifest.Tool, tag, asset string) string {
	return "https://github.com/" + t.Repo + "/releases/download/" + tag + "/" + asset
}
