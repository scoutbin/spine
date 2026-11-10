package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrChecksumMissing  = errors.New("no checksum listed")
	ErrChecksumMismatch = errors.New("checksum mismatch")
)

const sha256HexLen = sha256.Size * 2

// VerifyChecksum hashes content and compares it with the entry for name in
// checksums, the contents of a checksums.txt in sha256sum format. A malformed
// or duplicated line makes the whole file untrusted.
func VerifyChecksum(checksums []byte, name string, content io.Reader) error {
	want, err := LookupChecksum(checksums, name)
	if err != nil {
		return err
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, content); err != nil {
		return fmt.Errorf("hash %s: %w", name, err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		return fmt.Errorf("%w for %s: got %s, want %s", ErrChecksumMismatch, name, got, want)
	}
	return nil
}

// LookupChecksum returns the lowercase SHA-256 hex digest listed for name in
// checksums. A malformed or duplicated line makes the whole file untrusted.
func LookupChecksum(checksums []byte, name string) (string, error) {
	entries, err := parseChecksums(checksums)
	if err != nil {
		return "", err
	}
	digest, ok := entries[name]
	if !ok {
		return "", fmt.Errorf("%w for %s", ErrChecksumMissing, name)
	}
	return digest, nil
}

// parseChecksums maps file names to lowercase SHA-256 hex digests. Each line is
// "<hex>  <name>" (text mode) or "<hex> *<name>" (binary mode).
func parseChecksums(data []byte) (map[string]string, error) {
	entries := make(map[string]string)
	for i, line := range bytes.Split(data, []byte("\n")) {
		text := strings.TrimSuffix(string(line), "\r")
		if text == "" {
			continue
		}
		digest, name, ok := parseChecksumLine(text)
		if !ok {
			return nil, fmt.Errorf("checksums line %d: malformed", i+1)
		}
		if _, dup := entries[name]; dup {
			return nil, fmt.Errorf("checksums line %d: duplicate entry for %s", i+1, name)
		}
		entries[name] = digest
	}
	return entries, nil
}

func parseChecksumLine(line string) (digest, name string, ok bool) {
	if len(line) < sha256HexLen+3 || line[sha256HexLen] != ' ' || (line[sha256HexLen+1] != ' ' && line[sha256HexLen+1] != '*') {
		return "", "", false
	}
	digest = strings.ToLower(line[:sha256HexLen])
	if _, err := hex.DecodeString(digest); err != nil {
		return "", "", false
	}
	return digest, line[sha256HexLen+2:], true
}
