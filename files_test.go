package teleiq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/upload"
	"github.com/DhurghamAhmed/teleiq/models"
)

func TestInputFileMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		want    string
		wantErr bool
	}{
		{name: "file id", in: models.FileID("AgACAgIAAxkBAAIB"), want: `"AgACAgIAAxkBAAIB"`},
		{name: "url", in: models.FileURL("https://example.com/a.jpg?x=1&y=2"), want: `"https://example.com/a.jpg?x=1\u0026y=2"`},
		{name: "zero value", in: models.InputFile{}, wantErr: true},
		{name: "required in media", in: models.InputMediaPhoto{Media: models.FileID("abc")}, want: `{"type":"photo","media":"abc"}`},
		{name: "missing in media", in: models.InputMediaPhoto{}, wantErr: true},
		{name: "optional omitted", in: models.InputMediaVideo{Media: models.FileID("v")}, want: `{"type":"video","media":"v"}`},
		{name: "optional set", in: models.InputMediaVideo{Media: models.FileID("v"), Thumbnail: models.FileURL("https://t")}, want: `{"type":"video","media":"v","thumbnail":"https://t"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "InputFile is not set") {
					t.Fatalf("Marshal() = %s, %v; want an InputFile is not set error", got, err)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Fatalf("Marshal() = %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func newFileServer(t *testing.T, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return newTestClient(t, newHTTPTransport(srv.Client(), srv.URL, testToken), WithDefaultTimeout(50*time.Millisecond)), &requests
}

func TestDownload(t *testing.T) {
	c, _ := newFileServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file/bot" + testToken + "/documents/file_1.txt":
			_, _ = io.WriteString(w, "file content")
		case "/file/bot" + testToken + "/photos/my photo#1.jpg":
			_, _ = io.WriteString(w, "escaped")
		case "/file/bot" + testToken + "/slow.bin":
			time.Sleep(200 * time.Millisecond)
			_, _ = io.WriteString(w, "slow")
		case "/file/bot" + testToken + "/proxy.bin":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, "<html>Bad Gateway</html>")
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"ok":false,"error_code":404,"description":"Not Found"}`)
		}
	})
	tests := []struct {
		name     string
		path     string
		want     string
		wantCode int
	}{
		{name: "file", path: "documents/file_1.txt", want: "file content"},
		{name: "escaped path", path: "photos/my photo#1.jpg", want: "escaped"},
		{name: "longer than the default timeout", path: "slow.bin", want: "slow"},
		{name: "missing file", path: "documents/none.txt", wantCode: 404},
		{name: "error page that is not json", path: "proxy.bin", wantCode: 502},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst bytes.Buffer
			err := c.Download(context.Background(), tt.path, &dst)
			if tt.wantCode != 0 {
				var apiErr *Error
				if !errors.As(err, &apiErr) || apiErr.ErrorCode != tt.wantCode || apiErr.Method != "download" {
					t.Fatalf("Download() error = %v, want an *Error with code %d", err, tt.wantCode)
				}
				if dst.Len() != 0 || strings.Contains(err.Error(), testToken) {
					t.Errorf("Download() wrote %q and returned %q, want nothing written and no token", dst.String(), err)
				}
				return
			}
			if err != nil || dst.String() != tt.want {
				t.Fatalf("Download() = %q, %v; want %q", dst.String(), err, tt.want)
			}
		})
	}
}

func TestDownloadErrors(t *testing.T) {
	c, requests := newFileServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, "content")
	})
	expired, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	custom := newTestClient(t, fakeTransport(200, `{"ok":true,"result":true}`, nil))
	tests := []struct {
		name         string
		client       *Client
		ctx          context.Context
		path         string
		dst          io.Writer
		is           error
		wantInErr    string
		wantRequests int32
	}{
		{name: "empty path", client: c, path: "", dst: io.Discard, wantInErr: "invalid file path"},
		{name: "parent segment", client: c, path: "../bot1:x/getMe", dst: io.Discard, wantInErr: "invalid file path"},
		{name: "absolute path", client: c, path: "/etc/passwd", dst: io.Discard, wantInErr: "invalid file path"},
		{name: "empty segment", client: c, path: "a//b", dst: io.Discard, wantInErr: "invalid file path"},
		{name: "nil writer", client: c, path: "a.txt", wantInErr: "nil writer"},
		{name: "custom transport", client: custom, path: "a.txt", dst: io.Discard, wantInErr: "cannot download files"},
		{name: "caller deadline", client: c, ctx: expired, path: "a.txt", dst: io.Discard, is: context.DeadlineExceeded, wantInErr: "download", wantRequests: 1},
		{name: "writer error", client: c, path: "a.txt", dst: failingWriter{}, wantInErr: "disk full", wantRequests: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests.Store(0)
			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			err := tt.client.Download(ctx, tt.path, tt.dst)
			if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
				t.Fatalf("Download() error = %v, want one mentioning %q", err, tt.wantInErr)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Errorf("error %q contains the token", err)
			}
			if n := requests.Load(); n != tt.wantRequests {
				t.Errorf("server got %d requests, want %d", n, tt.wantRequests)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestDownloadStreamsLargeFiles(t *testing.T) {
	const size = 16 << 20
	c, _ := newFileServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(zeros{}, size))
	})
	var n countingWriter
	if err := c.Download(context.Background(), "videos/big.mp4", &n); err != nil || n != size {
		t.Fatalf("Download() wrote %d bytes, %v; want %d", n, err, size)
	}
}

type countingWriter int64

func (w *countingWriter) Write(p []byte) (int, error) {
	*w += countingWriter(len(p))
	return len(p), nil
}

type sentFile struct{ name, content string }

// readForm reads an attempt of body as a multipart form.
func readForm(t *testing.T, contentType string, r io.Reader) (map[string]string, map[string]sentFile) {
	t.Helper()
	mediaType, ps, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type %q is not multipart/form-data: %v", contentType, err)
	}
	fields, files := map[string]string{}, map[string]sentFile{}
	mr := multipart.NewReader(r, ps["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return fields, files
		}
		if err != nil {
			t.Fatalf("reading the form: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("reading part %s: %v", part.FormName(), err)
		}
		if part.FileName() != "" {
			files[part.FormName()] = sentFile{part.FileName(), string(data)}
		} else {
			fields[part.FormName()] = string(data)
		}
	}
}

func TestEncodeBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("NOTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	photo := models.FileFromBytes("cat.jpg", []byte("JPEG"))
	thumb := models.FileFromBytes("", []byte("THUMB"))
	ref := func(f models.InputFile) string { b, _ := json.Marshal(f); return strings.Trim(string(b), `"`) }
	sticker := models.FileFromBytes("s.webp", []byte("WEBP"))
	stickerJSON := `{"sticker":"` + ref(sticker) + `","format":"static","emoji_list":["🙂"]}`
	tests := []struct {
		name       string
		params     any
		wantJSON   string
		wantFields map[string]string
		wantFiles  map[string]sentFile
	}{
		{
			name:     "no files",
			params:   SendMessageParams{ChatID: models.ID(42), Text: "hi"},
			wantJSON: `{"chat_id":42,"text":"hi"}`,
		},
		{
			name:     "file id",
			params:   SendPhotoParams{ChatID: models.ID(42), Photo: models.FileID("AgAD")},
			wantJSON: `{"chat_id":42,"photo":"AgAD"}`,
		},
		{
			name:     "nil slice left out",
			params:   GetUpdatesParams{Timeout: Ptr(30)},
			wantJSON: `{"timeout":30}`,
		},
		{
			name:     "empty slice sent",
			params:   GetUpdatesParams{Timeout: Ptr(30), AllowedUpdates: []string{}},
			wantJSON: `{"timeout":30,"allowed_updates":[]}`,
		},
		{
			name:       "empty slice in a form",
			params:     SendPhotoParams{ChatID: models.ID(1), Photo: photo, CaptionEntities: []models.MessageEntity{}},
			wantFields: map[string]string{"chat_id": "1", "caption_entities": "[]"},
			wantFiles:  map[string]sentFile{"photo": {"cat.jpg", "JPEG"}},
		},
		{
			name: "uploaded parameter",
			params: SendPhotoParams{ChatID: models.Username("@chan"), Photo: photo, Caption: Ptr("a \"cat\""),
				ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Go", CallbackData: Ptr("go")}}}}},
			wantFields: map[string]string{"chat_id": "@chan", "caption": `a "cat"`,
				"reply_markup": `{"inline_keyboard":[[{"text":"Go","callback_data":"go"}]]}`},
			wantFiles: map[string]sentFile{"photo": {"cat.jpg", "JPEG"}},
		},
		{
			name:       "two uploaded parameters",
			params:     SendDocumentParams{ChatID: models.ID(1), Document: models.FileFromBytes("a.pdf", []byte("PDF")), Thumbnail: thumb},
			wantFields: map[string]string{"chat_id": "1"},
			wantFiles:  map[string]sentFile{"document": {"a.pdf", "PDF"}, "thumbnail": {"file", "THUMB"}},
		},
		{
			name:       "path named by its base name",
			params:     SendDocumentParams{ChatID: models.ID(1), Document: models.FileFromPath(path)},
			wantFields: map[string]string{"chat_id": "1"},
			wantFiles:  map[string]sentFile{"document": {"notes.txt", "NOTES"}},
		},
		{
			name: "nested uploads",
			params: SendMediaGroupParams{ChatID: models.ID(1), Media: []models.InputMediaGroupItem{
				&models.InputMediaPhoto{Media: photo},
				&models.InputMediaVideo{Media: models.FileID("BAAD"), Thumbnail: thumb},
				&models.InputMediaPhoto{Media: photo, Caption: Ptr("again")},
			}},
			wantFields: map[string]string{"chat_id": "1", "media": `[{"type":"photo","media":"` + ref(photo) + `"},` +
				`{"type":"video","media":"BAAD","thumbnail":"` + ref(thumb) + `"},{"type":"photo","media":"` + ref(photo) + `","caption":"again"}]`},
			wantFiles: map[string]sentFile{strings.TrimPrefix(ref(photo), "attach://"): {"cat.jpg", "JPEG"}, strings.TrimPrefix(ref(thumb), "attach://"): {"file", "THUMB"}},
		},
		{
			name: "upload in a nested value",
			params: AddStickerToSetParams{UserID: 7, Name: "pack_by_bot",
				Sticker: models.InputSticker{Sticker: sticker, Format: "static", EmojiList: []string{"🙂"}}},
			wantFields: map[string]string{"user_id": "7", "name": "pack_by_bot", "sticker": stickerJSON},
			wantFiles:  map[string]sentFile{strings.TrimPrefix(ref(sticker), "attach://"): {"s.webp", "WEBP"}},
		},
		{
			name: "uploads in a list of values",
			params: CreateNewStickerSetParams{UserID: 7, Name: "pack_by_bot", Title: "Pack", Stickers: []models.InputSticker{
				{Sticker: sticker, Format: "static", EmojiList: []string{"🙂"}},
				{Sticker: models.FileID("CAAD"), Format: "static", EmojiList: []string{"🙃"}},
			}},
			wantFields: map[string]string{"user_id": "7", "name": "pack_by_bot", "title": "Pack",
				"stickers": `[` + stickerJSON + `,{"sticker":"CAAD","format":"static","emoji_list":["🙃"]}]`},
			wantFiles: map[string]sentFile{strings.TrimPrefix(ref(sticker), "attach://"): {"s.webp", "WEBP"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := encodeBody(tt.params)
			if err != nil {
				t.Fatalf("encodeBody() error = %v", err)
			}
			r, err := body.Open()
			if err != nil {
				t.Fatalf("open() error = %v", err)
			}
			if tt.wantJSON != "" {
				got, _ := io.ReadAll(r)
				if body.HasUploads() || body.ContentType() != "application/json" || string(got) != tt.wantJSON {
					t.Fatalf("body = %s %s, want JSON %s", body.ContentType(), got, tt.wantJSON)
				}
				return
			}
			fields, files := readForm(t, body.ContentType(), r)
			if !reflect.DeepEqual(fields, tt.wantFields) {
				t.Errorf("fields = %v\nwant %v", fields, tt.wantFields)
			}
			if !reflect.DeepEqual(files, tt.wantFiles) {
				t.Errorf("files = %v, want %v", files, tt.wantFiles)
			}
		})
	}
}

// attempts opens body n times and returns the uploaded document of each attempt; when readFirst is
// false, the first attempt closes the body unread, like a request whose connection failed.
func attempts(t *testing.T, body *upload.Body, n int, readFirst bool) []string {
	t.Helper()
	var got []string
	for i := range n {
		r, err := body.Open()
		if err != nil {
			got = append(got, "error: "+err.Error())
			continue
		}
		if i == 0 && !readFirst {
			_ = r.(io.Closer).Close()
			got = append(got, "not read")
			continue
		}
		_, files := readForm(t, body.ContentType(), r)
		got = append(got, files["document"].content)
	}
	return got
}

func TestUploadReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("from disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	var opens atomic.Int32
	reopener := models.FileFromReopener("r.txt", func() (io.ReadCloser, error) {
		opens.Add(1)
		return io.NopCloser(strings.NewReader("reopened")), nil
	})
	seeker := strings.NewReader("skipped:seekable")
	_, _ = seeker.Seek(8, io.SeekStart)
	notRead := upload.ErrNotReplayable.Error()
	tests := []struct {
		name      string
		file      models.InputFile
		readFirst bool
		want      []string
	}{
		{"path", models.FileFromPath(path), true, []string{"from disk", "from disk"}},
		{"bytes", models.FileFromBytes("b.txt", []byte("in memory")), true, []string{"in memory", "in memory"}},
		{"reopener", reopener, true, []string{"reopened", "reopened"}},
		{"seekable reader from its position", models.FileFromReader("s.txt", seeker), true, []string{"seekable", "seekable"}},
		{"reader read once", models.FileFromReader("o.txt", io.MultiReader(strings.NewReader("once"))), true, []string{"once", "error: " + notRead}},
		{"reader not read by the failed attempt", models.FileFromReader("o.txt", io.MultiReader(strings.NewReader("once"))), false, []string{"not read", "once", "error: " + notRead}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := encodeBody(SendDocumentParams{ChatID: models.ID(1), Document: tt.file})
			if err != nil {
				t.Fatal(err)
			}
			if got := attempts(t, body, len(tt.want), tt.readFirst); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("attempts = %q, want %q", got, tt.want)
			}
		})
	}
	if n := opens.Load(); n != 2 {
		t.Errorf("reopener opened %d times, want 2", n)
	}
}

func TestUploadErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.jpg")
	tests := []struct {
		name      string
		file      models.InputFile
		is        error
		wantInErr string
	}{
		{"missing file", models.FileFromPath(missing), fs.ErrNotExist, "missing.jpg"},
		{"failing reopener", models.FileFromReopener("a", func() (io.ReadCloser, error) { return nil, io.ErrClosedPipe }), io.ErrClosedPipe, "opening files"},
		{"reopener failing with a token", models.FileFromReopener("a", func() (io.ReadCloser, error) {
			return nil, fmt.Errorf("GET https://api.telegram.org/file/bot%s/a: %w", testToken, io.ErrUnexpectedEOF)
		}), io.ErrUnexpectedEOF, "opening files"},
		{"reopener without reader", models.FileFromReopener("a", func() (io.ReadCloser, error) { return nil, nil }), nil, "returned no reader"},
		{"nil reopener", models.FileFromReopener("a", nil), nil, "nil open function"},
		{"nil reader", models.FileFromReader("a", nil), nil, "nil reader"},
		{"line break in the name", models.FileFromBytes("a\r\nX-Evil: 1", []byte("x")), nil, "line breaks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(200, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`, rec))

			_, err := c.SendPhoto(context.Background(), SendPhotoParams{ChatID: models.ID(1), Photo: tt.file})

			if err == nil || !strings.Contains(err.Error(), tt.wantInErr) || !strings.Contains(err.Error(), "sendPhoto") {
				t.Fatalf("SendPhoto() error = %v, want one naming sendPhoto and %q", err, tt.wantInErr)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
			}
			if strings.Contains(err.Error(), testSecret) {
				t.Errorf("error %q shows the token", err)
			}
			if n := rec.calls.Load(); n != 0 {
				t.Errorf("transport called %d times, want no request", n)
			}
		})
	}
}

func TestUploadHasNoDefaultTimeout(t *testing.T) {
	tests := []struct {
		name         string
		photo        models.InputFile
		wantDeadline bool
	}{
		{"file id keeps the default timeout", models.FileID("AgAD"), true},
		{"upload has none", models.FileFromBytes("a.jpg", []byte("x")), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(200, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`, rec))
			if _, err := c.SendPhoto(context.Background(), SendPhotoParams{ChatID: models.ID(1), Photo: tt.photo}); err != nil {
				t.Fatal(err)
			}
			if rec.hasDeadline != tt.wantDeadline {
				t.Errorf("request has a deadline = %v, want %v", rec.hasDeadline, tt.wantDeadline)
			}
		})
	}
}

func TestUploadThroughHTTPTransport(t *testing.T) {
	const size = 8 << 20
	var received atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fields := map[string]string{}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if part.FormName() == "document" && part.FileName() == "big.bin" {
				n, _ := io.Copy(io.Discard, part)
				received.Store(n)
				continue
			}
			data, _ := io.ReadAll(part)
			fields[part.FormName()] = string(data)
		}
		if r.URL.Path != "/bot"+testToken+"/sendDocument" || fields["chat_id"] != "42" || fields["caption"] != "big" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":9,"date":1,"chat":{"id":42,"type":"private"}}}`)
	}))
	t.Cleanup(srv.Close)
	c := newTestClient(t, newHTTPTransport(srv.Client(), srv.URL, testToken))

	msg, err := c.SendDocument(context.Background(), SendDocumentParams{
		ChatID: models.ID(42), Caption: Ptr("big"),
		Document: models.FileFromReader("big.bin", io.LimitReader(zeros{}, size)),
	})
	if err != nil || msg.MessageID != 9 {
		t.Fatalf("SendDocument() = %+v, %v", msg, err)
	}
	if n := received.Load(); n != size {
		t.Errorf("server received %d bytes, want %d", n, size)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestUploadConcurrentUse(t *testing.T) {
	photo := models.FileFromBytes("shared.jpg", []byte("SHARED"))
	var bad atomic.Int32
	read := TransportFunc(func(_ context.Context, req *Request) (*Response, error) {
		mediaType, ps, err := mime.ParseMediaType(req.ContentType)
		if err != nil || mediaType != "multipart/form-data" {
			return nil, errors.New("not multipart")
		}
		photo := ""
		mr := multipart.NewReader(req.Body, ps["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if data, _ := io.ReadAll(part); part.FormName() == "photo" {
				photo = string(data)
			}
		}
		if photo != "SHARED" {
			bad.Add(1)
		}
		return &Response{StatusCode: 200, Body: []byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`)}, nil
	})
	c := newTestClient(t, read)
	errs := make(chan error, 16)
	for range 16 {
		go func() {
			_, err := c.SendPhoto(context.Background(), SendPhotoParams{ChatID: models.ID(1), Photo: photo})
			errs <- err
		}()
	}
	for range 16 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	if n := bad.Load(); n != 0 {
		t.Errorf("%d requests sent wrong content", n)
	}
}
