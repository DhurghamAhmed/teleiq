package teleiqtest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func newClient(t *testing.T, srv *teleiqtest.Server) *teleiq.Client {
	t.Helper()
	c, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL), teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// fill sets the required parameters that cannot stay zero: strings, chats and files.
func fill(v reflect.Value) {
	for i := range v.NumField() {
		f, sf := v.Field(i), v.Type().Field(i)
		if strings.Contains(sf.Tag.Get("json"), ",omit") {
			continue
		}
		switch f.Interface().(type) {
		case string:
			f.SetString("x")
		case models.ChatID:
			f.Set(reflect.ValueOf(models.ID(1)))
		case models.InputFile:
			f.Set(reflect.ValueOf(models.FileID("x")))
		default:
			if f.Kind() == reflect.Struct {
				fill(f)
			}
		}
	}
}

// TestEveryMethod calls each method of teleiq.Client and checks that the default answer decodes
// into a result that is not empty.
func TestEveryMethod(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	srv.AddFile("x", []byte("content"))
	c := reflect.ValueOf(newClient(t, srv))
	ctxType := reflect.TypeFor[context.Context]()
	n := 0
	for i := range c.NumMethod() {
		m, ft := c.Type().Method(i), c.Method(i).Type()
		if ft.NumIn() < 1 || ft.NumIn() > 2 || ft.In(0) != ctxType || m.Name == "Close" || m.Name == "LogOut" {
			continue
		}
		n++
		t.Run(m.Name, func(t *testing.T) {
			args := []reflect.Value{reflect.ValueOf(context.Background())}
			if ft.NumIn() == 2 {
				params := reflect.New(ft.In(1)).Elem()
				fill(params)
				args = append(args, params)
			}
			out := c.Method(i).Call(args)
			if err, _ := out[len(out)-1].Interface().(error); err != nil {
				t.Fatalf("%s: %v", m.Name, err)
			}
			if len(out) == 2 && out[0].IsZero() {
				t.Errorf("%s returned %v, want a result", m.Name, out[0])
			}
		})
	}
	if n < 180 {
		t.Errorf("called %d methods, want every method of the Bot API", n)
	}
}

func TestRespondAndFail(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	c := newClient(t, srv)
	ctx := context.Background()

	srv.Respond("getMe", models.User{ID: 7, IsBot: true, FirstName: "Echo", Username: teleiq.Ptr("echo_bot")})
	if me, err := c.GetMe(ctx); err != nil || me.ID != 7 || me.FirstName != "Echo" {
		t.Errorf("GetMe() = %+v, %v; want the stubbed user", me, err)
	}

	srv.Fail("sendMessage", &teleiq.Error{ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"})
	_, err := c.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.ID(7), Text: "hi"})
	if !errors.Is(err, teleiq.ErrForbidden) || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("SendMessage() = %v, want the stubbed 403", err)
	}

	srv.Fail("sendMessage", &teleiq.Error{ErrorCode: 429, Description: "Too Many Requests: retry after 5",
		Parameters: &models.ResponseParameters{RetryAfter: teleiq.Ptr(5)}})
	_, err = c.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.ID(8), Text: "hi"})
	var apiErr *teleiq.Error
	if !errors.As(err, &apiErr) || apiErr.Parameters == nil || apiErr.Parameters.RetryAfter == nil || *apiErr.Parameters.RetryAfter != 5 {
		t.Errorf("SendMessage() = %v, want flood control with retry_after 5", err)
	}

	srv.Respond("sendMessage", models.Message{MessageID: 99, Chat: models.Chat{ID: 7, Type: "private"}})
	if m, err := c.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.ID(7), Text: "hi"}); err != nil || m.MessageID != 99 {
		t.Errorf("SendMessage() = %+v, %v; want the stubbed message", m, err)
	}
	var stubbed models.Message
	if last := srv.Requests("sendMessage")[2]; json.Unmarshal(last.Result, &stubbed) != nil || stubbed.MessageID != 99 {
		t.Errorf("recorded result = %s, want the stubbed message", last.Result)
	}
	if failed := srv.Requests("sendMessage")[0]; failed.Result != nil {
		t.Errorf("recorded result of a failed call = %s, want nil", failed.Result)
	}
	if got := len(srv.Requests("sendMessage")); got != 3 {
		t.Errorf("%d sendMessage requests recorded, want 3, stubbed ones included", got)
	}
	if got := len(srv.Requests("")); got != 4 {
		t.Errorf("%d requests recorded, want 4", got)
	}
}

func TestReset(t *testing.T) {
	tests := []struct {
		name string
		stub func(*teleiqtest.Server)
	}{
		{"after Respond", func(s *teleiqtest.Server) {
			s.Respond("getMe", models.User{ID: 7, IsBot: true, FirstName: "Echo", Username: teleiq.Ptr("echo_bot")})
		}},
		{"after Fail", func(s *teleiqtest.Server) {
			s.Fail("getMe", &teleiq.Error{ErrorCode: 401, Description: "Unauthorized"})
		}},
		{"without a stub", func(*teleiqtest.Server) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := teleiqtest.NewServer()
			defer srv.Close()
			c := newClient(t, srv)
			ctx := context.Background()
			tt.stub(srv)
			srv.Fail("sendMessage", &teleiq.Error{ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"})

			srv.Reset("getMe")
			me, err := c.GetMe(ctx)
			if err != nil || me.FirstName != "Test Bot" || me.Username == nil || *me.Username != "test_bot" ||
				!strings.HasPrefix(teleiqtest.Token, strconv.FormatInt(me.ID, 10)+":") {
				t.Errorf("GetMe() = %+v, %v; want the default bot, whose ID starts Token", me, err)
			}
			if _, err := c.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.ID(7), Text: "hi"}); !errors.Is(err, teleiq.ErrForbidden) {
				t.Errorf("SendMessage() = %v, want the Fail of sendMessage kept", err)
			}
		})
	}
}

func TestSentMessages(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	c := newClient(t, srv)
	ctx := context.Background()

	m, err := c.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.ID(-1001234567890), Text: "hello",
		MessageThreadID: teleiq.Ptr(int64(3))})
	if err != nil || m.Text == nil || *m.Text != "hello" || m.Chat.Type != "supergroup" || m.MessageThreadID == nil || m.From == nil || !m.From.IsBot {
		t.Fatalf("SendMessage() = %+v, %v; want the message it sent", m, err)
	}
	var recorded models.Message
	if err := json.Unmarshal(srv.Requests("sendMessage")[0].Result, &recorded); err != nil || recorded.MessageID != m.MessageID {
		t.Errorf("recorded result = %+v, %v; want the message that SendMessage returned", recorded, err)
	}
	edited, err := c.EditMessageText(ctx, teleiq.EditMessageTextParams{ChatID: models.ID(-1001234567890), MessageID: &m.MessageID, Text: teleiq.Ptr("edited")})
	if err != nil || edited.MessageID != m.MessageID || edited.EditDate == nil || *edited.Text != "edited" {
		t.Errorf("EditMessageText() = %+v, %v; want the edited message", edited, err)
	}
	inline, err := c.EditMessageText(ctx, teleiq.EditMessageTextParams{InlineMessageID: teleiq.Ptr("i"), Text: teleiq.Ptr("edited")})
	if err != nil || inline != nil {
		t.Errorf("EditMessageText() of an inline message = %+v, %v; want nil, as the Bot API returns True", inline, err)
	}
	channel, err := c.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.Username("@news"), Text: "hi"})
	if err != nil || channel.Chat.Type != "channel" || channel.Chat.Username == nil || *channel.Chat.Username != "news" {
		t.Errorf("SendMessage() to @news = %+v, %v", channel, err)
	}
	sent, err := c.SendPhoto(ctx, teleiq.SendPhotoParams{ChatID: models.ID(7), Photo: models.FileID("AgACphoto")})
	if err != nil || len(sent.Photo) != 1 || sent.Photo[0].FileID != "AgACphoto" {
		t.Errorf("SendPhoto() by file_id = %+v, %v; want the same file_id back", sent, err)
	}
	if err := c.Call(ctx, "noSuchMethod", nil, nil); !strings.Contains(errString(err), "Not Found (404)") {
		t.Errorf("Call(noSuchMethod) = %v, want 404", err)
	}
}

func TestUploads(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	c := newClient(t, srv)
	ctx := context.Background()
	data := []byte("a,b\n1,2\n")
	// The server notices at once that the bot has read an answer only if it has read the whole
	// form, which multipart parsing can leave unfinished.
	wait := func(method string) teleiqtest.Request {
		t.Helper()
		start := time.Now()
		req := srv.Wait(t, method, 1)[0]
		if waited := time.Since(start); waited > 500*time.Millisecond {
			t.Errorf("Wait after an upload to %s took %v, want it to see at once that the bot read the answer", method, waited)
		}
		return req
	}

	m, err := c.SendDocument(ctx, teleiq.SendDocumentParams{ChatID: models.ID(42), Document: models.FileFromBytes("report.csv", data), Caption: teleiq.Ptr("123")})
	if err != nil || m.Document == nil || m.Document.FileName == nil || *m.Document.FileName != "report.csv" {
		t.Fatalf("SendDocument() = %+v, %v; want the document", m, err)
	}
	req := wait("sendDocument")
	if up := req.Uploads["document"]; up.Name != "report.csv" || !bytes.Equal(up.Data, data) {
		t.Errorf("upload = %q %q", up.Name, up.Data)
	}
	if req.Params["chat_id"] != 42.0 || req.Params["caption"] != "123" {
		t.Errorf("params = %v, want chat_id 42 as a number and the caption as a string", req.Params)
	}
	info, err := c.GetFile(ctx, teleiq.GetFileParams{FileID: m.Document.FileID})
	if err != nil || info.FilePath == nil || !strings.HasSuffix(*info.FilePath, ".csv") {
		t.Fatalf("GetFile() = %+v, %v", info, err)
	}
	var got bytes.Buffer
	if err := c.Download(ctx, *info.FilePath, &got); err != nil || !bytes.Equal(got.Bytes(), data) {
		t.Errorf("Download() = %q, %v; want the uploaded content", got.Bytes(), err)
	}

	group, err := c.SendMediaGroup(ctx, teleiq.SendMediaGroupParams{ChatID: models.ID(42), Media: []models.InputMediaGroupItem{
		&models.InputMediaPhoto{Media: models.FileFromBytes("a.jpg", []byte("A")), Caption: teleiq.Ptr("first")},
		&models.InputMediaPhoto{Media: models.FileFromBytes("b.jpg", []byte("B"))},
	}})
	if err != nil || len(group) != 2 || len(group[0].Photo) != 1 || group[0].Caption == nil || *group[0].Caption != "first" {
		t.Fatalf("SendMediaGroup() = %+v, %v; want two photos", group, err)
	}
	if ups := wait("sendMediaGroup").Uploads; len(ups) != 2 {
		t.Errorf("uploads = %v, want the two photos", ups)
	}

	srv.AddFile("voice-1", []byte("OGG"))
	if info, err := c.GetFile(ctx, teleiq.GetFileParams{FileID: "voice-1"}); err != nil || info.FilePath == nil {
		t.Errorf("GetFile() of an added file = %+v, %v", info, err)
	}
	if _, err := c.GetFile(ctx, teleiq.GetFileParams{FileID: "unknown"}); !strings.Contains(errString(err), "invalid file_id") {
		t.Errorf("GetFile() of an unknown file = %v, want an error", err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
