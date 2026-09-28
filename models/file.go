package models

import (
	"io"

	"github.com/DhurghamAhmed/teleiq/internal/upload"
)

// InputFile is a file to send: a file ID, an HTTP URL, or an upload.
type InputFile = upload.InputFile

// FileID returns the InputFile for a file already stored on the Telegram servers.
func FileID(id string) InputFile { return upload.FileID(id) }

// FileURL returns the InputFile for a file that Telegram downloads from an HTTP URL.
func FileURL(url string) InputFile { return upload.FileURL(url) }

// FileFromPath returns the InputFile that uploads the file at path.
func FileFromPath(path string) InputFile { return upload.FileFromPath(path) }

// FileFromBytes returns the InputFile that uploads data under name.
func FileFromBytes(name string, data []byte) InputFile { return upload.FileFromBytes(name, data) }

// FileFromReopener returns the InputFile that uploads what open returns under name.
func FileFromReopener(name string, open func() (io.ReadCloser, error)) InputFile {
	return upload.FileFromReopener(name, open)
}

// FileFromReader returns the InputFile that uploads what r reads under name.
func FileFromReader(name string, r io.Reader) InputFile { return upload.FileFromReader(name, r) }
