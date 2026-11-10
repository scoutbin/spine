// Command example is a minimal tool built on spine.Run. It is the fixture for
// spine's tests and CI.
package main

import (
	_ "embed"
	"os"

	"github.com/scoutbin/spine"
)

//go:embed spine.json
var manifest []byte

func main() {
	os.Exit(spine.Run(spine.MustLoad(manifest), spine.OSEnv(), os.Args[1:]))
}
