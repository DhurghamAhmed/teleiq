// Package upload holds the files to send and encodes requests that upload them.
package upload

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// InputFile is a file to send: a file ID, an HTTP URL, or an upload.
type InputFile struct {
	ref    string
	upload *upload
}

// FileID returns the InputFile for a file already stored on the Telegram servers.
func FileID(id string) InputFile {
	return InputFile{ref: id}
}

// FileURL returns the InputFile for a file that Telegram downloads from an HTTP URL.
func FileURL(url string) InputFile {
	return InputFile{ref: url}
}

// FileFromPath returns the InputFile that uploads the file at path.
func FileFromPath(path string) InputFile {
	return newUpload(filepath.Base(path), func() (io.ReadCloser, error) { return os.Open(path) }, nil)
}

// FileFromBytes returns the InputFile that uploads data under name.
func FileFromBytes(name string, data []byte) InputFile {
	return newUpload(name, func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }, nil)
}

// FileFromReopener returns the InputFile that uploads what open returns under name.
func FileFromReopener(name string, open func() (io.ReadCloser, error)) InputFile {
	if open == nil {
		open = func() (io.ReadCloser, error) { return nil, errors.New("teleiq: FileFromReopener: nil open function") }
	}
	return newUpload(name, open, nil)
}

// FileFromReader returns the InputFile that uploads what r reads under name.
func FileFromReader(name string, r io.Reader) InputFile {
	if r == nil {
		return FileFromReopener(name, func() (io.ReadCloser, error) { return nil, errors.New("teleiq: FileFromReader: nil reader") })
	}
	return newUpload(name, nil, r)
}

// MarshalJSON encodes the file ID or URL, or an attach:// reference to an upload.
func (f InputFile) MarshalJSON() ([]byte, error) {
	switch {
	case f.upload != nil:
		return json.Marshal("attach://" + f.upload.part)
	case f.ref != "":
		return json.Marshal(f.ref)
	}
	return nil, errors.New("teleiq: InputFile is not set")
}

// ErrNotReplayable reports an upload that cannot be read again for a retry.
var ErrNotReplayable = errors.New("teleiq: the file was read by an earlier attempt and cannot be sent again")

var uploadCount atomic.Uint64

// upload is the content of an uploaded file, shared by copies of its InputFile.
type upload struct {
	name string
	part string // the name of its multipart part when it is nested in a parameter
	open func() (io.ReadCloser, error)

	mu     sync.Mutex
	reader io.Reader
	start  int64 // the position of a seekable reader at the first attempt
	seen   bool  // the reader was used by an attempt
	read   atomic.Bool
}

func newUpload(name string, open func() (io.ReadCloser, error), r io.Reader) InputFile {
	if name == "" {
		name = "file"
	}
	part := "file" + strconv.FormatUint(uploadCount.Add(1), 10)
	return InputFile{upload: &upload{name: name, part: part, open: open, reader: r}}
}

// content returns a reader of the file for a new attempt of a request.
func (u *upload) content() (io.ReadCloser, error) {
	if strings.ContainsAny(u.name, "\r\n") {
		return nil, errors.New("teleiq: file names cannot contain line breaks")
	}
	if u.open != nil {
		rc, err := u.open()
		if err == nil && rc == nil {
			err = errors.New("teleiq: the open function of a file returned no reader")
		}
		return rc, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if s, ok := u.reader.(io.Seeker); ok {
		if !u.seen {
			pos, err := s.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, err
			}
			u.start, u.seen = pos, true
		} else if _, err := s.Seek(u.start, io.SeekStart); err != nil {
			return nil, err
		}
		return io.NopCloser(u.reader), nil
	}
	// A reader that an attempt never read from can still be sent.
	if u.read.Load() {
		return nil, ErrNotReplayable
	}
	return io.NopCloser(readTracker{u}), nil
}

// replayable reports whether content can return the file again.
func (u *upload) replayable() bool {
	if u.open != nil {
		return true
	}
	if _, ok := u.reader.(io.Seeker); ok {
		return true
	}
	return !u.read.Load()
}

type readTracker struct{ u *upload }

func (t readTracker) Read(p []byte) (int, error) {
	t.u.read.Store(true)
	return t.u.reader.Read(p)
}
