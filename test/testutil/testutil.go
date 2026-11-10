package testutil

import "github.com/scoutbin/spine"

const Manifest = `{
  "name": "demo",
  "repo": "scoutbin/demo",
  "summary": "Demonstrate the spine",
  "license": "Apache-2.0",
  "channels": {
    "homebrew": {"tap": "scoutbin/homebrew-tap"},
    "npm": {"package": "@scoutbin/demo"},
    "chocolatey": {"id": "scoutbin-demo"},
    "action": true
  }
}`

func Tool() spine.Tool {
	return spine.MustLoad([]byte(Manifest))
}
