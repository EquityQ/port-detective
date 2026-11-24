package fingerprint

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
)

const (
	// maxManifestFileSize is the largest manifest file the analyzer is willing to
	// read. A file at or above this limit is treated as "not a usable manifest"
	// and is rejected before it is read, so an oversized or hostile manifest can
	// never stall detection or exhaust memory.
	maxManifestFileSize int64 = 1 << 20 // 1 MiB
	// manifestReadChunkSize is the read granularity for manifest files. Reads are
	// chunked so a slow filesystem (network share, antivirus scan, cold disk) can
	// be abandoned at a chunk boundary as soon as the detection context is done.
	manifestReadChunkSize = 32 << 10 // 32 KiB
	// maxManifestReadStalls is the number of consecutive zero-byte, error-free
	// reads tolerated before a reader is treated as stalled. It prevents a broken
	// reader from spinning forever instead of reporting no fingerprint.
	maxManifestReadStalls = 100
)

var (
	// errManifestTooLarge reports a manifest at or above maxManifestFileSize.
	errManifestTooLarge = errors.New("fingerprint: manifest file exceeds the size limit")
	// errManifestNotRegular reports a manifest path that is not a plain file.
	errManifestNotRegular = errors.New("fingerprint: manifest path is not a regular file")
	// errManifestReadStalled reports a reader that repeatedly returns no data.
	errManifestReadStalled = errors.New("fingerprint: manifest read stalled")
)

// manifestFileReader loads at most limit bytes from a manifest file, aborting as
// soon as ctx is done. It is a package-level seam so tests can substitute a
// blocking reader and prove the detection walk honors context cancellation.
type manifestFileReader func(ctx context.Context, path string, limit int64) ([]byte, error)

// readManifestFile is the reader used for every manifest file in the walk.
var readManifestFile manifestFileReader = readManifestFileLimited

// readManifestFileLimited reads a manifest file without trusting its size: the
// path is stat'ed first so an oversized file is rejected unread, then the body
// is read in chunks with a cancellation check before every chunk.
func readManifestFileLimited(ctx context.Context, path string, limit int64) ([]byte, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}

	// Open fail-closed: permission errors, missing files, and directories are all
	// "not a usable manifest" for the caller.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errManifestNotRegular
	}
	// Reject an oversized manifest before any of its bytes are read.
	if info.Size() >= limit {
		return nil, errManifestTooLarge
	}

	data, err := readManifestLimited(ctx, f, limit)
	if err != nil {
		return nil, err
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	return data, nil
}

// readManifestLimited buffers at most limit bytes from r, checking ctx before
// every chunk so a slow or growing file cannot outrun the detection budget.
func readManifestLimited(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	var (
		buf    bytes.Buffer
		chunk  = make([]byte, manifestReadChunkSize)
		total  int64
		stalls int
	)

	for {
		if err := contextErr(ctx); err != nil {
			return nil, err
		}

		n, readErr := r.Read(chunk)
		if n > 0 {
			stalls = 0
			total += int64(n)
			// The file grew past the cap between the stat and the read.
			if total >= limit {
				return nil, errManifestTooLarge
			}
			buf.Write(chunk[:n])
		} else if readErr == nil {
			stalls++
			if stalls >= maxManifestReadStalls {
				return nil, errManifestReadStalled
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				if err := contextErr(ctx); err != nil {
					return nil, err
				}
				return buf.Bytes(), nil
			}
			return nil, readErr
		}
	}
}

// contextErr returns ctx's cancellation error, treating a nil context as
// "no deadline" so callers never panic on an unset context.
func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
