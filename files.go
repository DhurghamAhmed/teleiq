package teleiq

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/upload"
)

// Download streams the file at filePath, as returned by GetFile, to dst.
func (c *Client) Download(ctx context.Context, filePath string, dst io.Writer) error {
	switch {
	case c == nil || c.transport == nil:
		return errors.New("teleiq: Client must be created with NewClient")
	case ctx == nil:
		return errors.New("teleiq: nil Context")
	case dst == nil:
		return errors.New("teleiq: Download: nil writer")
	}
	d, ok := c.transport.(fileDownloader)
	if !ok {
		return errors.New("teleiq: Download: the transport given to WithTransport cannot download files")
	}
	start := time.Now()
	err := d.download(ctx, filePath, dst)
	c.logDownload(ctx, filePath, time.Since(start), err)
	return err
}

// fileCarrier is implemented by the generated parameters that can hold uploads.
type fileCarrier interface {
	collectFiles(*upload.Set)
}

// collectFrom adds to s the uploads of v, a method's parameters or a models value.
func collectFrom(s *upload.Set, v any) {
	if fc, ok := v.(fileCarrier); ok {
		fc.collectFiles(s)
		return
	}
	collectModel(s, v)
}

// encodeBody encodes parameters, as a multipart form when they hold uploads.
func encodeBody(params any) (*upload.Body, error) {
	return upload.Encode(params, func(s *upload.Set) { collectFrom(s, params) })
}
