package teleiqtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Request is a call that the bot made, other than getUpdates.
type Request struct {
	Method string

	// Params are the parameters as encoding/json decodes them into an any.
	Params map[string]any

	// Uploads are the uploaded files, by the name of their form field.
	Uploads map[string]Upload

	// Result is the result that the server answered with, or nil after an error.
	Result json.RawMessage
}

// Upload is an uploaded file.
type Upload struct {
	Name string
	Data []byte
}

// Requests returns the requests to method so far, or all of them if method is empty.
func (s *Server) Requests(method string) []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requestsTo(method)
}

// requestsTo filters the requests; s.mu must be held.
func (s *Server) requestsTo(method string) []Request {
	var out []Request
	for _, r := range s.requests {
		if method == "" || r.Method == method {
			out = append(out, r.Request)
		}
	}
	return out
}

// Wait waits for the bot to read the answers to n requests to method and returns them.
func (s *Server) Wait(t testing.TB, method string, n int) []Request {
	t.Helper()
	var got []Request
	s.waitFor(t, fmt.Sprintf("%d %s requests", n, method), func() bool {
		read := 0
		for _, r := range s.requests {
			if r.read && (method == "" || r.Method == method) {
				read++
			}
		}
		got = s.requestsTo(method)
		return read >= n
	})
	return got
}

// Start runs run in the background and returns a function that stops it.
func Start(t testing.TB, run func(ctx context.Context) error) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- run(ctx) }()
	var once sync.Once
	var err error
	stop = func() error {
		once.Do(func() {
			cancel()
			select {
			case err = <-result:
			case <-time.After(10 * time.Second):
				err = fmt.Errorf("teleiqtest: the bot did not stop within ten seconds")
				t.Error(err)
			}
		})
		return err
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

// readRequest decodes the parameters of a request, sent as JSON or as a multipart form.
func readRequest(method string, r *http.Request) (Request, error) {
	req := Request{Method: method, Params: map[string]any{}}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "multipart/form-data" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return req, err
		}
		if len(bytes.TrimSpace(body)) > 0 {
			err = json.Unmarshal(body, &req.Params)
		}
		return req, err
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return req, err
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	text := methods[method].text
	for name, values := range r.MultipartForm.Value {
		req.Params[name] = formValue(values[0], text[name])
	}
	for name, files := range r.MultipartForm.File {
		f, err := files[0].Open()
		if err != nil {
			return req, err
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return req, err
		}
		if req.Uploads == nil {
			req.Uploads = map[string]Upload{}
		}
		req.Uploads[name] = Upload{Name: files[0].Filename, Data: data}
	}
	return req, nil
}

// formValue decodes a form field as a JSON parameter would be.
func formValue(v string, text bool) any {
	if text {
		return v
	}
	var out any
	if json.Unmarshal([]byte(v), &out) == nil {
		return out
	}
	return v
}
