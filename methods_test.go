package teleiq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
	"github.com/DhurghamAhmed/teleiq/models"
)

func TestGeneratedMethods(t *testing.T) {
	ann := models.User{ID: 1, FirstName: "Ann"}
	annJSON := `{"id":1,"is_bot":false,"first_name":"Ann"}`
	chat := `"chat":{"id":42,"type":"private"}`
	tests := []struct {
		name       string
		call       func(ctx context.Context, c *Client) (any, error)
		reply      string
		wantMethod string
		wantBody   string
		want       any
	}{
		{
			name:  "no parameters and no result",
			call:  func(ctx context.Context, c *Client) (any, error) { return nil, c.LogOut(ctx) },
			reply: `true`, wantMethod: "logOut",
		},
		{
			name:  "no parameters and an object",
			call:  func(ctx context.Context, c *Client) (any, error) { return c.GetWebhookInfo(ctx) },
			reply: `{"url":"","has_custom_certificate":false,"pending_update_count":3}`, wantMethod: "getWebhookInfo",
			want: &models.WebhookInfo{PendingUpdateCount: 3},
		},
		{
			name: "true",
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.DeleteMessage(ctx, DeleteMessageParams{ChatID: models.ID(42), MessageID: 7})
			},
			reply: `true`, wantMethod: "deleteMessage", wantBody: `{"chat_id":42,"message_id":7}`,
		},
		{
			name: "array of objects with unions inside",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetUpdates(ctx, GetUpdatesParams{Offset: Ptr(int64(10)), Limit: Ptr(100), AllowedUpdates: []string{"message"}})
			},
			reply:      `[{"update_id":10,"message":{"message_id":1,"date":1,` + chat + `,"forward_origin":{"type":"user","date":1,"sender_user":` + annJSON + `}}}]`,
			wantMethod: "getUpdates", wantBody: `{"offset":10,"limit":100,"allowed_updates":["message"]}`,
			want: []models.Update{{UpdateID: 10, Message: &models.Message{MessageID: 1, Date: 1, Chat: models.Chat{ID: 42, Type: "private"},
				ForwardOrigin: &models.MessageOriginUser{Date: 1, SenderUser: ann}}}},
		},
		{
			name: "edited message",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.EditMessageText(ctx, EditMessageTextParams{ChatID: models.ID(42), MessageID: Ptr(int64(1)), Text: Ptr("new")})
			},
			reply: `{"message_id":1,"date":1,` + chat + `,"text":"new"}`, wantMethod: "editMessageText",
			wantBody: `{"chat_id":42,"message_id":1,"text":"new"}`,
			want:     &models.Message{MessageID: 1, Date: 1, Chat: models.Chat{ID: 42, Type: "private"}, Text: Ptr("new")},
		},
		{
			name: "edited inline message",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.EditMessageText(ctx, EditMessageTextParams{InlineMessageID: Ptr("AbC"), Text: Ptr("new")})
			},
			reply: `true`, wantMethod: "editMessageText", wantBody: `{"inline_message_id":"AbC","text":"new"}`,
			want: (*models.Message)(nil),
		},
		{
			name: "union",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetChatMember(ctx, GetChatMemberParams{ChatID: models.Username("@group"), UserID: 1})
			},
			reply: `{"status":"member","user":` + annJSON + `}`, wantMethod: "getChatMember", wantBody: `{"chat_id":"@group","user_id":1}`,
			want: &models.ChatMemberMember{User: ann},
		},
		{
			name: "array of unions",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetChatAdministrators(ctx, GetChatAdministratorsParams{ChatID: models.ID(-100)})
			},
			reply:      `[{"status":"creator","user":` + annJSON + `,"is_anonymous":false},{"status":"owner_v2","user":` + annJSON + `}]`,
			wantMethod: "getChatAdministrators", wantBody: `{"chat_id":-100}`,
			want: []models.ChatMember{&models.ChatMemberOwner{User: ann}, &models.Unknown{Kind: "owner_v2", Raw: []byte(`{"status":"owner_v2","user":` + annJSON + `}`)}},
		},
		{
			name: "integer",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetChatMemberCount(ctx, GetChatMemberCountParams{ChatID: models.ID(-100)})
			},
			reply: `27`, wantMethod: "getChatMemberCount", wantBody: `{"chat_id":-100}`, want: 27,
		},
		{
			name: "string",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.ExportChatInviteLink(ctx, ExportChatInviteLinkParams{ChatID: models.ID(-100)})
			},
			reply: `"https://t.me/+abc"`, wantMethod: "exportChatInviteLink", wantBody: `{"chat_id":-100}`, want: "https://t.me/+abc",
		},
		{
			name: "file and reply markup",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.SendPhoto(ctx, SendPhotoParams{ChatID: models.ID(42), Photo: models.FileID("AgAD"), ReplyMarkup: &models.InlineKeyboardMarkup{
					InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Open", URL: Ptr("https://example.com")}}},
				}})
			},
			reply: `{"message_id":2,"date":1,` + chat + `}`, wantMethod: "sendPhoto",
			wantBody: `{"chat_id":42,"photo":"AgAD","reply_markup":{"inline_keyboard":[[{"text":"Open","url":"https://example.com"}]]}}`,
			want:     &models.Message{MessageID: 2, Date: 1, Chat: models.Chat{ID: 42, Type: "private"}},
		},
		{
			name: "array of synthetic unions",
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.SendMediaGroup(ctx, SendMediaGroupParams{ChatID: models.ID(42), Media: []models.InputMediaGroupItem{
					&models.InputMediaPhoto{Media: models.FileID("a")}, &models.InputMediaVideo{Media: models.FileURL("https://v")},
				}})
			},
			reply: `[]`, wantMethod: "sendMediaGroup",
			wantBody: `{"chat_id":42,"media":[{"type":"photo","media":"a"},{"type":"video","media":"https://v"}]}`,
			want:     []models.Message{},
		},
		{
			name: "array of unions as parameter",
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.SetMessageReaction(ctx, SetMessageReactionParams{ChatID: models.ID(42), MessageID: 7, Reaction: []models.ReactionType{&models.ReactionTypeEmoji{Emoji: "👍"}}})
			},
			reply: `true`, wantMethod: "setMessageReaction", wantBody: `{"chat_id":42,"message_id":7,"reaction":[{"type":"emoji","emoji":"👍"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(200, `{"ok":true,"result":`+tt.reply+`}`, rec))

			got, err := tt.call(context.Background(), c)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if rec.req.Method != tt.wantMethod || rec.body != tt.wantBody {
				t.Errorf("request = %s %s, want %s %s", rec.req.Method, rec.body, tt.wantMethod, tt.wantBody)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestGeneratedMethodErrors(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		reply     string
		call      func(ctx context.Context, c *Client) (any, error)
		is        error
		wantInErr string
		wantCalls int32
	}{
		{
			name: "api error", status: 403, reply: `{"ok":false,"error_code":403,"description":"Forbidden: bot was kicked"}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetChatMember(ctx, GetChatMemberParams{ChatID: models.ID(1), UserID: 1})
			},
			is: ErrForbidden, wantInErr: "getChatMember: Forbidden", wantCalls: 1,
		},
		{
			name: "union result of the wrong shape", status: 200, reply: `{"ok":true,"result":[1]}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetChatMember(ctx, GetChatMemberParams{ChatID: models.ID(1), UserID: 1})
			},
			wantInErr: "getChatMember: decoding result", wantCalls: 1,
		},
		{
			name: "array of unions of the wrong shape", status: 200, reply: `{"ok":true,"result":{"status":"member"}}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetChatAdministrators(ctx, GetChatAdministratorsParams{ChatID: models.ID(1)})
			},
			wantInErr: "getChatAdministrators: decoding result", wantCalls: 1,
		},
		{
			name: "edit result of the wrong shape", status: 200, reply: `{"ok":true,"result":"text"}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.EditMessageText(ctx, EditMessageTextParams{InlineMessageID: Ptr("A")})
			},
			wantInErr: "editMessageText: decoding result", wantCalls: 1,
		},
		{
			name: "value result on error", status: 400, reply: `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.GetChatMemberCount(ctx, GetChatMemberCountParams{ChatID: models.ID(1)})
			},
			wantInErr: "chat not found", wantCalls: 1,
		},
		{
			name: "missing required file", status: 200, reply: `{"ok":true,"result":true}`,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.SendPhoto(ctx, SendPhotoParams{ChatID: models.ID(1)})
			},
			wantInErr: "InputFile is not set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(tt.status, tt.reply, rec))

			got, err := tt.call(context.Background(), c)

			if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
				t.Fatalf("error = %v, want one mentioning %q", err, tt.wantInErr)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
			}
			if v := reflect.ValueOf(got); got != nil && !v.IsZero() {
				t.Errorf("result = %#v, want the zero value", got)
			}
			if n := rec.calls.Load(); n != tt.wantCalls {
				t.Errorf("transport called %d times, want %d", n, tt.wantCalls)
			}
		})
	}
}

// TestEveryMethod calls each Bot API method of the snapshot once through a fake transport.
func TestEveryMethod(t *testing.T) {
	api := loadSchema(t)
	for _, meth := range api.Methods {
		t.Run(meth.Name, func(t *testing.T) {
			rec := &recordedCall{}
			reply := sampleResult(meth.Returns)
			c := newTestClient(t, fakeTransport(200, `{"ok":true,"result":`+reply+`}`, rec))
			fn := reflect.ValueOf(c).MethodByName(strings.ToUpper(meth.Name[:1]) + meth.Name[1:])
			if !fn.IsValid() {
				t.Fatalf("Client has no method for %s", meth.Name)
			}
			args := []reflect.Value{reflect.ValueOf(context.Background())}
			if fn.Type().NumIn() == 2 {
				params := reflect.New(fn.Type().In(1)).Elem()
				fillRequired(params)
				args = append(args, params)
			}
			if want := 1 + min(len(meth.Params), 1); fn.Type().NumIn() != want {
				t.Fatalf("%s takes %d arguments, want %d", meth.Name, fn.Type().NumIn(), want)
			}

			out := fn.Call(args)

			if err, _ := out[len(out)-1].Interface().(error); err != nil {
				t.Fatalf("%s error = %v", meth.Name, err)
			}
			if rec.req.Method != meth.Name {
				t.Errorf("request method = %q, want %q", rec.req.Method, meth.Name)
			}
			checkBody(t, rec.body, meth.Params)
			if slices.Equal(meth.Returns, []string{"True"}) {
				if len(out) != 1 {
					t.Errorf("%s returns %d values, want only an error", meth.Name, len(out))
				}
				return
			}
			if len(out) != 2 || out[0].IsZero() {
				t.Errorf("%s result = %v, want a decoded %s", meth.Name, out[0], strings.Join(meth.Returns, " or "))
			}
		})
	}
}

// sampleResult returns a JSON result of the given Bot API type that is not a zero value.
func sampleResult(returns []string) string {
	switch r := returns[0]; {
	case r == "String":
		return `"text"`
	case r == "Integer":
		return `3`
	case r == "True":
		return `true`
	case strings.HasPrefix(r, "Array of "):
		return `[{}]`
	}
	return `{}`
}

// fillRequired sets the required ChatID and InputFile fields, whose zero values cannot be encoded.
func fillRequired(v reflect.Value) {
	for i := range v.NumField() {
		f, sf := v.Field(i), v.Type().Field(i)
		if strings.Contains(sf.Tag.Get("json"), ",omit") {
			continue
		}
		switch f.Interface().(type) {
		case models.ChatID:
			f.Set(reflect.ValueOf(models.ID(1)))
		case models.InputFile:
			f.Set(reflect.ValueOf(models.FileID("file")))
		default:
			if f.Kind() == reflect.Struct {
				fillRequired(f)
			}
		}
	}
}

// checkBody verifies that a request with only required fields set sends exactly the required parameters.
func checkBody(t *testing.T, body string, params []schema.Field) {
	t.Helper()
	var want []string
	for _, p := range params {
		if p.Required {
			want = append(want, p.Name)
		}
	}
	if len(params) == 0 {
		if body != "" {
			t.Errorf("body = %s, want none", body)
		}
		return
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", body, err)
	}
	got := slices.Sorted(maps.Keys(sent))
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("sent parameters = %v, want the required ones %v", got, want)
	}
}

func TestCall(t *testing.T) {
	tests := []struct {
		name      string
		params    any
		result    any
		status    int
		reply     string
		wantBody  string
		wantType  string
		wantUser  *testUser
		wantAPI   *Error
		wantInErr string
	}{
		{
			name: "result decoded", params: map[string]int64{"chat_id": 1}, result: &testUser{},
			status: 200, reply: `{"ok":true,"result":{"id":7,"is_bot":true,"first_name":"Bot","extra":1}}`,
			wantBody: `{"chat_id":1}`, wantType: "application/json", wantUser: &testUser{ID: 7, IsBot: true, FirstName: "Bot"},
		},
		{
			name: "no parameters", result: &testUser{},
			status: 200, reply: `{"ok":true,"result":{"id":1}}`, wantUser: &testUser{ID: 1},
		},
		{name: "result ignored", status: 200, reply: `{"ok":true,"result":true}`},
		{
			name: "api error", status: 400, reply: `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
			wantAPI: &Error{ErrorCode: 400, Description: "Bad Request: chat not found", Method: "getChat"},
		},
		{
			name: "flood control", status: 429, reply: `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 5","parameters":{"retry_after":5}}`,
			wantAPI: &Error{ErrorCode: 429, Description: "Too Many Requests: retry after 5", Parameters: &models.ResponseParameters{RetryAfter: Ptr(5)}, Method: "getChat"},
		},
		{
			name: "token in description", status: 404, reply: `{"ok":false,"error_code":404,"description":"Not Found: /bot` + testToken + `/getChat"}`,
			wantAPI: &Error{ErrorCode: 404, Description: "Not Found: /bot<redacted>/getChat", Method: "getChat"},
		},
		{
			name: "error page that is not json", status: 502, reply: "<html>Bad Gateway</html>",
			wantAPI: &Error{ErrorCode: 502, Description: "Bad Gateway", Method: "getChat"},
		},
		{
			name: "error without code", status: 500, reply: `{"ok":false,"description":"Internal Server Error"}`,
			wantAPI: &Error{ErrorCode: 500, Description: "Internal Server Error", Method: "getChat"},
		},
		{name: "invalid json", status: 200, reply: "not json", wantInErr: "decoding response"},
		{name: "result of the wrong shape", result: &testUser{}, status: 200, reply: `{"ok":true,"result":"text"}`, wantInErr: "decoding result"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(tt.status, tt.reply, rec))

			err := c.Call(context.Background(), "getChat", tt.params, tt.result)

			if rec.req.Method != "getChat" || rec.req.ContentType != tt.wantType || rec.body != tt.wantBody {
				t.Errorf("request = %q %q %q, want getChat %q %q", rec.req.Method, rec.req.ContentType, rec.body, tt.wantType, tt.wantBody)
			}
			switch {
			case tt.wantAPI != nil:
				var apiErr *Error
				if !errors.As(err, &apiErr) {
					t.Fatalf("Call() error = %v, want *Error", err)
				}
				if apiErr.ErrorCode != tt.wantAPI.ErrorCode || apiErr.Description != tt.wantAPI.Description || apiErr.Method != tt.wantAPI.Method ||
					!equalPtr(paramRetry(apiErr), paramRetry(tt.wantAPI)) {
					t.Errorf("Call() error = %+v, want %+v", apiErr, tt.wantAPI)
				}
			case tt.wantInErr != "":
				var apiErr *Error
				if err == nil || errors.As(err, &apiErr) || !strings.Contains(err.Error(), tt.wantInErr) {
					t.Fatalf("Call() error = %v, want a non-API error mentioning %q", err, tt.wantInErr)
				}
			default:
				if err != nil {
					t.Fatalf("Call() error = %v", err)
				}
				if tt.wantUser != nil && *tt.result.(*testUser) != *tt.wantUser {
					t.Errorf("result = %+v, want %+v", tt.result, tt.wantUser)
				}
			}
		})
	}
}

func paramRetry(e *Error) *int {
	if e.Parameters == nil {
		return nil
	}
	return e.Parameters.RetryAfter
}

// tokenMarshaler fails with an error that holds the token, as code of the caller might.
type tokenMarshaler struct{}

func (tokenMarshaler) MarshalJSON() ([]byte, error) {
	return nil, errors.New("cannot reach https://api.telegram.org/bot" + testToken + "/getMe")
}

func TestCallRequestErrors(t *testing.T) {
	failing := TransportFunc(func(context.Context, *Request) (*Response, error) { return nil, context.Canceled })
	empty := TransportFunc(func(context.Context, *Request) (*Response, error) { return nil, nil })

	tests := []struct {
		name      string
		client    func(rec *recordedCall) *Client
		ctx       context.Context
		method    string
		params    any
		is        error
		wantInErr string
		wantCalls int32
	}{
		{name: "invalid method", method: "get/Me", wantInErr: "invalid method name"},
		{name: "unencodable parameters", method: "getMe", params: map[string]any{"c": make(chan int)}, wantInErr: "encoding parameters"},
		{name: "parameters failing with a token", method: "getMe", params: map[string]any{"p": tokenMarshaler{}}, wantInErr: "encoding parameters"},
		{name: "nil context", method: "getMe", ctx: nil, wantInErr: "nil Context"},
		{name: "zero client", method: "getMe", client: func(*recordedCall) *Client { return &Client{} }, wantInErr: "NewClient"},
		{name: "nil client", method: "getMe", client: func(*recordedCall) *Client { return nil }, wantInErr: "NewClient"},
		{name: "transport error", method: "getMe", client: func(*recordedCall) *Client { return newTestClient(t, failing) }, is: context.Canceled, wantInErr: "getMe"},
		{name: "transport without response", method: "getMe", client: func(*recordedCall) *Client { return newTestClient(t, empty) }, wantInErr: "no response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(200, `{"ok":true,"result":true}`, rec))
			if tt.client != nil {
				c = tt.client(rec)
			}
			ctx := tt.ctx
			if ctx == nil && tt.name != "nil context" {
				ctx = context.Background()
			}

			err := c.Call(ctx, tt.method, tt.params, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
				t.Fatalf("Call() error = %v, want one mentioning %q", err, tt.wantInErr)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
			}
			if strings.Contains(err.Error(), testSecret) {
				t.Errorf("error %q shows the token", err)
			}
			if got := rec.calls.Load(); got != tt.wantCalls {
				t.Errorf("transport called %d times, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestCallThroughHTTPTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path == "/bot"+testToken+"/getChat" && string(body) == `{"chat_id":42}`:
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":42,"first_name":"Ann"}}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
		}
	}))
	t.Cleanup(srv.Close)
	c := newTestClient(t, newHTTPTransport(srv.Client(), srv.URL, testToken))

	tests := []struct {
		name    string
		chatID  int64
		want    testUser
		wantErr int
	}{
		{name: "success", chatID: 42, want: testUser{ID: 42, FirstName: "Ann"}},
		{name: "api error", chatID: 7, wantErr: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got testUser
			err := c.Call(context.Background(), "getChat", map[string]int64{"chat_id": tt.chatID}, &got)
			if tt.wantErr != 0 {
				var apiErr *Error
				if !errors.As(err, &apiErr) || apiErr.ErrorCode != tt.wantErr {
					t.Fatalf("Call() error = %v, want an *Error with code %d", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("Call() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestCallConcurrentUse(t *testing.T) {
	c := newTestClient(t, fakeTransport(200, `{"ok":true,"result":{"id":1}}`, nil))
	var wg sync.WaitGroup
	errs := make(chan error, 16*20)
	for range 16 {
		wg.Go(func() {
			for range 20 {
				var u testUser
				if err := c.Call(context.Background(), "getMe", nil, &u); err != nil || u.ID != 1 {
					errs <- errors.Join(err, errors.New("wrong result"))
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

const sentMessageReply = `{"ok":true,"result":{"message_id":7,"from":{"id":1,"is_bot":true,"first_name":"Bot"},` +
	`"date":1790000000,"chat":{"id":42,"type":"private","first_name":"Ann"},"text":"hi"}}`

var sentMessage = &models.Message{
	MessageID: 7, From: &models.User{ID: 1, IsBot: true, FirstName: "Bot"}, Date: 1790000000,
	Chat: models.Chat{ID: 42, Type: "private", FirstName: Ptr("Ann")}, Text: Ptr("hi"),
}

func TestSendMessageParams(t *testing.T) {
	tests := []struct {
		name     string
		params   SendMessageParams
		wantBody string
	}{
		{
			name:     "required only",
			params:   SendMessageParams{ChatID: models.ID(42), Text: "hi"},
			wantBody: `{"chat_id":42,"text":"hi"}`,
		},
		{
			name:     "username",
			params:   SendMessageParams{ChatID: models.Username("@channel"), Text: "hi"},
			wantBody: `{"chat_id":"@channel","text":"hi"}`,
		},
		{
			name:     "optional zero values sent",
			params:   SendMessageParams{ChatID: models.ID(-1001), Text: "", MessageThreadID: Ptr(int64(0)), DisableNotification: Ptr(false)},
			wantBody: `{"chat_id":-1001,"message_thread_id":0,"text":"","disable_notification":false}`,
		},
		{
			name: "ephemeral with a reply keyboard",
			params: SendMessageParams{
				ChatID: models.ID(42), Text: "hi",
				EphemeralMessageParameters: &models.EphemeralMessageParameters{ReceiverUserID: 7},
				ReplyMarkup:                &models.ForceReply{InputFieldPlaceholder: Ptr("Answer")},
			},
			wantBody: `{"chat_id":42,"ephemeral_message_parameters":{"receiver_user_id":7},"text":"hi",` +
				`"reply_markup":{"force_reply":true,"input_field_placeholder":"Answer"}}`,
		},
		{
			name: "all parameters",
			params: SendMessageParams{
				BusinessConnectionID:  Ptr("bc"),
				ChatID:                models.ID(42),
				MessageThreadID:       Ptr(int64(3)),
				DirectMessagesTopicID: Ptr(int64(4)),
				Text:                  "<b>hi</b> @ann",
				ParseMode:             Ptr("HTML"),
				Entities:              []models.MessageEntity{{Type: "text_mention", Offset: 3, Length: 4, User: &models.User{ID: 9, FirstName: "Ann"}}},
				LinkPreviewOptions:    &models.LinkPreviewOptions{IsDisabled: Ptr(true)},
				DisableNotification:   Ptr(true),
				ProtectContent:        Ptr(true),
				AllowPaidBroadcast:    Ptr(false),
				MessageEffectID:       Ptr("e1"),
				ReplyParameters: &models.ReplyParameters{
					MessageID: Ptr(int64(5)), ChatID: models.Username("@other"), AllowSendingWithoutReply: Ptr(true),
					Quote: Ptr("q"), QuoteEntities: []models.MessageEntity{{Type: "bold", Offset: 0, Length: 1}}, QuotePosition: Ptr(0),
				},
			},
			wantBody: `{"business_connection_id":"bc","chat_id":42,"message_thread_id":3,"direct_messages_topic_id":4,` +
				`"text":"\u003cb\u003ehi\u003c/b\u003e @ann","parse_mode":"HTML",` +
				`"entities":[{"type":"text_mention","offset":3,"length":4,"user":{"id":9,"is_bot":false,"first_name":"Ann"}}],` +
				`"link_preview_options":{"is_disabled":true},"disable_notification":true,"protect_content":true,` +
				`"allow_paid_broadcast":false,"message_effect_id":"e1",` +
				`"reply_parameters":{"message_id":5,"chat_id":"@other","allow_sending_without_reply":true,"quote":"q",` +
				`"quote_entities":[{"type":"bold","offset":0,"length":1}],"quote_position":0}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(200, sentMessageReply, rec))

			if _, err := c.SendMessage(context.Background(), tt.params); err != nil {
				t.Fatalf("SendMessage() error = %v", err)
			}
			if rec.req.Method != "sendMessage" || rec.req.ContentType != "application/json" {
				t.Errorf("request = %q %q, want sendMessage application/json", rec.req.Method, rec.req.ContentType)
			}
			if rec.body != tt.wantBody {
				t.Errorf("body =\n%s\nwant\n%s", rec.body, tt.wantBody)
			}
		})
	}
}

func TestSendMessage(t *testing.T) {
	tests := []struct {
		name      string
		params    SendMessageParams
		status    int
		reply     string
		want      *models.Message
		wantCalls int32
		is        error
		wantInErr string
	}{
		{name: "sent", params: SendMessageParams{ChatID: models.ID(42), Text: "hi"}, status: 200, reply: sentMessageReply, want: sentMessage, wantCalls: 1},
		{
			name: "reply in a topic", params: SendMessageParams{ChatID: models.ID(-100), Text: "ok"}, status: 200, wantCalls: 1,
			reply: `{"ok":true,"result":{"message_id":8,"message_thread_id":2,"date":1,"chat":{"id":-100,"type":"supergroup","title":"G","is_forum":true},` +
				`"is_topic_message":true,"reply_to_message":{"message_id":2,"date":1,"chat":{"id":-100,"type":"supergroup"}},"text":"ok",` +
				`"entities":[{"type":"url","offset":0,"length":2}],"link_preview_options":{"is_disabled":true},"future_field":{"a":1}}}`,
			want: &models.Message{
				MessageID: 8, MessageThreadID: Ptr(int64(2)), Date: 1, Chat: models.Chat{ID: -100, Type: "supergroup", Title: Ptr("G"), IsForum: true},
				IsTopicMessage: true, ReplyToMessage: &models.Message{MessageID: 2, Date: 1, Chat: models.Chat{ID: -100, Type: "supergroup"}}, Text: Ptr("ok"),
				Entities: []models.MessageEntity{{Type: "url", Offset: 0, Length: 2}}, LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: Ptr(true)},
			},
		},
		{
			name: "chat not found", params: SendMessageParams{ChatID: models.ID(1), Text: "hi"}, status: 400, wantCalls: 1,
			reply:     `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
			wantInErr: "sendMessage: Bad Request: chat not found (400)",
		},
		{
			name: "blocked by the user", params: SendMessageParams{ChatID: models.ID(1), Text: "hi"}, status: 403, wantCalls: 1,
			reply: `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`, is: ErrForbidden, wantInErr: "sendMessage",
		},
		{name: "chat not set", params: SendMessageParams{Text: "hi"}, wantInErr: "ChatID is not set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(tt.status, tt.reply, rec))

			got, err := c.SendMessage(context.Background(), tt.params)

			if n := rec.calls.Load(); n != tt.wantCalls {
				t.Errorf("transport called %d times, want %d", n, tt.wantCalls)
			}
			if tt.want == nil {
				if got != nil || err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
					t.Fatalf("SendMessage() = %+v, %v; want nil and an error mentioning %q", got, err, tt.wantInErr)
				}
				if tt.is != nil && !errors.Is(err, tt.is) {
					t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SendMessage() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestSendMessageThroughHTTPTransport(t *testing.T) {
	srv, seen := newRecordingServer(t, http.StatusOK, sentMessageReply)
	c := newTestClient(t, newHTTPTransport(srv.Client(), srv.URL, testToken))

	got, err := c.SendMessage(context.Background(), SendMessageParams{ChatID: models.ID(42), Text: "hi", ParseMode: Ptr("HTML")})
	if err != nil || !reflect.DeepEqual(got, sentMessage) {
		t.Fatalf("SendMessage() = %+v, %v; want %+v", got, err, sentMessage)
	}
	r := <-seen
	want := received{http.MethodPost, "/bot" + testToken + "/sendMessage", "application/json", `{"chat_id":42,"text":"hi","parse_mode":"HTML"}`}
	if r != want {
		t.Errorf("server saw %s %s %q %s, want %s /bot<token>/sendMessage %q %s",
			r.method, strings.ReplaceAll(r.path, testToken, "<token>"), r.contentType, r.body, want.method, want.contentType, want.body)
	}
}

func TestKeyboardBuildersInReplyMarkup(t *testing.T) {
	var m models.ReplyMarkup = models.NewInlineKeyboard(models.NewInlineRow(models.NewCallbackButton("OK", "ok")))
	got, err := json.Marshal(SendMessageParams{ChatID: models.ID(1), Text: "x", ReplyMarkup: m})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"chat_id":1,"text":"x","reply_markup":{"inline_keyboard":[[{"text":"OK","callback_data":"ok"}]]}}`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

type sourceField struct {
	json     string
	optional bool
}

type sourcePackage struct {
	structs map[string][]sourceField
	markers map[string][]string // receivers of each no-argument unexported method
	client  map[string]bool     // methods of *Client
}

// parsePackage reads the declarations of this package, of package models, which holds the types,
// and of internal/upload, which defines the InputFile that models names, from their source.
func parsePackage(t *testing.T) sourcePackage {
	t.Helper()
	var paths []string
	for _, pattern := range []string{"*.go", filepath.Join("models", "*.go"), filepath.Join("internal", "upload", "*.go")} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, matches...)
	}
	pkg := sourcePackage{structs: map[string][]sourceField{}, markers: map[string][]string{}, client: map[string]bool{}}
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if st, ok := ts.Type.(*ast.StructType); ok {
						pkg.structs[ts.Name.Name] = tagFields(st)
					}
				}
			case *ast.FuncDecl:
				switch {
				case d.Recv == nil:
				case d.Name.IsExported() && receiverName(d.Recv.List[0].Type) == "Client":
					pkg.client[d.Name.Name] = true
				case !d.Name.IsExported() && d.Type.Params.NumFields() == 0:
					pkg.markers[d.Name.Name] = append(pkg.markers[d.Name.Name], receiverName(d.Recv.List[0].Type))
				}
			}
		}
	}
	return pkg
}

func tagFields(st *ast.StructType) []sourceField {
	fields := []sourceField{}
	for _, f := range st.Fields.List {
		if f.Tag == nil {
			continue
		}
		tag, _ := strconv.Unquote(f.Tag.Value)
		name, opts, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
		fields = append(fields, sourceField{json: name, optional: opts == "omitempty" || opts == "omitzero"})
	}
	return fields
}

func receiverName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	return expr.(*ast.Ident).Name
}

func loadSchema(t *testing.T) *schema.API {
	t.Helper()
	data, err := os.ReadFile("internal/gen/schema/botapi.json")
	if err != nil {
		t.Fatal(err)
	}
	api, err := schema.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func TestSourceMatchesSchema(t *testing.T) {
	api := loadSchema(t)
	pkg := parsePackage(t)
	valueMembers := map[string]string{"String": "RichTextString", "Array of RichText": "RichTextArray"}

	for _, ty := range api.Types {
		if len(ty.Members) > 0 {
			implemented := pkg.markers[strings.ToLower(ty.Name[:1])+ty.Name[1:]]
			for _, m := range ty.Members {
				if v, ok := valueMembers[m]; ok {
					m = v
				}
				if !slices.Contains(implemented, m) {
					t.Errorf("union %s: member %s does not implement it", ty.Name, m)
				}
			}
			continue
		}
		got, ok := pkg.structs[ty.Name]
		if !ok {
			t.Errorf("type %s is not defined", ty.Name)
			continue
		}
		want := []sourceField{}
		for _, sf := range ty.Fields {
			if sf.Const != "" || (sf.Required && slices.Equal(sf.Types, []string{"True"})) {
				continue
			}
			want = append(want, sourceField{json: sf.Name, optional: !sf.Required})
		}
		if !slices.Equal(got, want) {
			t.Errorf("type %s fields = %v, want %v", ty.Name, got, want)
		}
	}

	for _, meth := range api.Methods {
		name := strings.ToUpper(meth.Name[:1]) + meth.Name[1:]
		if !pkg.client[name] {
			t.Errorf("method %s: Client.%s is not defined", meth.Name, name)
		}
		params, ok := pkg.structs[name+"Params"]
		if len(meth.Params) == 0 {
			if ok {
				t.Errorf("method %s takes no parameters but %sParams is defined", meth.Name, name)
			}
			continue
		}
		want := []sourceField{}
		for _, p := range meth.Params {
			want = append(want, sourceField{json: p.Name, optional: !p.Required})
		}
		if !slices.Equal(params, want) {
			t.Errorf("method %s parameters = %v, want %v", meth.Name, params, want)
		}
	}
}
