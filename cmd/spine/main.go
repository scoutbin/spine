// Command spine is the development CLI for tools built on spine. Tools run it
// as "go tool spine"; it is never shipped to end users.
package main

import (
	"os"

	"github.com/scoutbin/spine/internal/devcmd"
)

func main() {
	os.Exit(devcmd.Run(os.Args[1:], os.Stdout, os.Stderr))
}
