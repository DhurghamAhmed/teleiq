package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"slices"
)

// Set lists the uploads of a request.
type Set struct {
	parts []filePart
}

type filePart struct {
	field string // the form field of the part
	file  *upload
	param bool // the part is a parameter itself, not referenced with attach://
}

// Field records an upload sent directly as the parameter name.
func (s *Set) Field(name string, f InputFile) {
	if f.upload != nil {
		s.parts = append(s.parts, filePart{field: name, file: f.upload, param: true})
	}
}

// Attach records an upload that a nested parameter references with attach://.
func (s *Set) Attach(f InputFile) {
	if f.upload != nil && !slices.ContainsFunc(s.parts, func(p filePart) bool { return !p.param && p.file == f.upload }) {
		s.parts = append(s.parts, filePart{field: f.upload.part, file: f.upload})
	}
}

// Body holds the encoded parameters of a request, reopenable for a retry.
type Body struct {
	contentType string
	json        []byte
	boundary    string
	fields      []formField // the form fields of a request with uploads
	files       []filePart
	writing     chan struct{} // closed when the multipart writer of the last attempt has stopped
}

type formField struct{ name, value string }

// Encode encodes params as JSON, or as a multipart form when they have uploads.
func Encode(params any, collect func(*Set)) (*Body, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var files Set
	collect(&files)
	if len(files.parts) == 0 {
		return &Body{contentType: "application/json", json: data}, nil
	}
	fields, err := formFields(data, files.parts)
	if err != nil {
		return nil, err
	}
	boundary := multipart.NewWriter(io.Discard).Boundary()
	return &Body{
		contentType: "multipart/form-data; boundary=" + boundary,
		boundary:    boundary,
		fields:      fields,
		files:       files.parts,
	}, nil
}

// formFields converts the JSON parameters, other than uploads, to form fields.
func formFields(data []byte, parts []filePart) ([]formField, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("teleiq: parameters with files must encode as a JSON object")
	}
	var fields []formField
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		if slices.ContainsFunc(parts, func(p filePart) bool { return p.param && p.field == name }) {
			continue
		}
		text := string(value)
		if len(value) > 0 && value[0] == '"' {
			if err := json.Unmarshal(value, &text); err != nil {
				return nil, err
			}
		}
		fields = append(fields, formField{name, text})
	}
	return fields, nil
}

// ContentType returns the Content-Type of the body.
func (b *Body) ContentType() string {
	return b.contentType
}

// HasUploads reports whether the body uploads files.
func (b *Body) HasUploads() bool {
	return len(b.files) > 0
}

// Replayable reports whether the body can be sent again.
func (b *Body) Replayable() bool {
	if !b.HasUploads() {
		return true
	}
	if b.writing != nil {
		<-b.writing
	}
	for _, p := range b.files {
		if !p.file.replayable() {
			return false
		}
	}
	return true
}

// Stop waits for the writer of the last attempt to stop, unless ctx ends first.
func (b *Body) Stop(ctx context.Context) {
	if b.writing == nil {
		return
	}
	select {
	case <-b.writing:
	case <-ctx.Done():
	}
}

// Open returns a new reader of the body for an attempt.
func (b *Body) Open() (io.Reader, error) {
	if !b.HasUploads() {
		return bytes.NewReader(b.json), nil
	}
	// The readers of the last attempt must not be in use when they are reused.
	if b.writing != nil {
		<-b.writing
	}
	readers := make([]io.ReadCloser, 0, len(b.files))
	for _, p := range b.files {
		r, err := p.file.content()
		if err != nil {
			closeAll(readers)
			return nil, err
		}
		readers = append(readers, r)
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	if err := mw.SetBoundary(b.boundary); err != nil {
		closeAll(readers)
		return nil, err
	}
	done := make(chan struct{})
	b.writing = done
	// The writer stops once the transport has read or closed the body.
	go func() {
		defer close(done)
		pw.CloseWithError(b.write(mw, readers))
	}()
	return pr, nil
}

func (b *Body) write(mw *multipart.Writer, readers []io.ReadCloser) error {
	defer closeAll(readers)
	for _, f := range b.fields {
		if err := mw.WriteField(f.name, f.value); err != nil {
			return err
		}
	}
	for i, p := range b.files {
		w, err := mw.CreateFormFile(p.field, p.file.name)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, readers[i]); err != nil {
			return err
		}
	}
	return mw.Close()
}

func closeAll(readers []io.ReadCloser) {
	for _, r := range readers {
		_ = r.Close()
	}
}
