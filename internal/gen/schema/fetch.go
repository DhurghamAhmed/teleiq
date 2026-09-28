package schema

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// PageURL is the official documentation page of the Bot API.
const PageURL = "https://core.telegram.org/bots/api"

// ReadPage returns the documentation page from the file in, or else from url.
func ReadPage(url, in string) ([]byte, error) {
	if in != "" {
		return os.ReadFile(in)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}
