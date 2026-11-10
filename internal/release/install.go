package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scoutbin/spine/internal/manifest"
)

const maxBinaryBytes = 200 << 20

// ExtractBinary returns the contents of the regular file name at the top level
// of a tar.gz archive. It never writes to disk. Entries with absolute or ".."
// paths are rejected, and so is a name that is not a regular file (a symlink,
// for example).
func ExtractBinary(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("archive has no %s at its top level", name)
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if path.IsAbs(hdr.Name) || slices.Contains(strings.Split(hdr.Name, "/"), "..") {
			return nil, fmt.Errorf("archive entry %q has an unsafe path", hdr.Name)
		}
		if strings.TrimPrefix(hdr.Name, "./") != name {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("archive entry %q is not a regular file", hdr.Name)
		}
		binary, err := io.ReadAll(io.LimitReader(tr, maxBinaryBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read %s from archive: %w", name, err)
		}
		if len(binary) > maxBinaryBytes {
			return nil, fmt.Errorf("%s in archive exceeds %d MB", name, maxBinaryBytes>>20)
		}
		return binary, nil
	}
}

// ReplaceBinary atomically replaces the executable at target with binary. The
// content goes to a temp file in the same directory, with the permissions of
// the current executable, and is renamed over target. On any failure target is
// untouched and the temp file is removed.
func ReplaceBinary(t manifest.Tool, target string, binary []byte) (err error) {
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("stat %s: %w", target, err)
	}

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, "."+t.Name+"-upgrade-*")
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("cannot write to %s: permission denied; re-run with elevated permissions (for example, sudo %s upgrade)", dir, t.Name)
	}
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	if _, err = tmp.Write(binary); err != nil {
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err = tmp.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmp.Name(), err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err = os.Rename(tmp.Name(), target); err != nil {
		return fmt.Errorf("replace %s: %w", target, err)
	}
	return nil
}
