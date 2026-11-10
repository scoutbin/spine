package spine

import "github.com/scoutbin/spine/internal/manifest"

// Tool is the parsed spine.json manifest of a tool. Methods assume a Tool obtained via Load or MustLoad.
type Tool = manifest.Tool

// Load parses and validates a spine.json manifest. Unknown fields and trailing data are errors.
func Load(data []byte) (Tool, error) {
	return manifest.Load(data)
}

// MustLoad is like Load but panics on error.
func MustLoad(data []byte) Tool {
	return manifest.MustLoad(data)
}
