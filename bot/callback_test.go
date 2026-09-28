package bot_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// newBot returns a bot on a fresh fake server that makes each call once, so that failures come
// back at once.
func newBot(t *testing.T) (*teleiqtest.Server, *bot.Bot) {
	t.Helper()
	srv := teleiqtest.NewServer()
	t.Cleanup(srv.Close)
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL), teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}))
	if err != nil {
		t.Fatal(err)
	}
	return srv, bot.New(client)
}

// press returns a callback query update with data, from a button on msg or, when inline is not
// empty, on a message sent in inline mode.
func press(msg models.MaybeInaccessibleMessage, inline string) *models.Update {
	q := &models.CallbackQuery{ID: "q", From: models.User{ID: 7, FirstName: "Ann"}, ChatInstance: "c", Data: teleiq.Ptr("d"), Message: msg}
	if inline != "" {
		q.InlineMessageID = &inline
	}
	return &models.Update{UpdateID: 1, CallbackQuery: q}
}

var (
	accessible   = &models.Message{MessageID: 9, Date: 1, Chat: models.Chat{ID: 7, Type: "private"}}
	business     = &models.Message{MessageID: 9, Date: 1, Chat: models.Chat{ID: 7, Type: "private"}, BusinessConnectionID: teleiq.Ptr("biz")}
	inaccessible = &models.InaccessibleMessage{MessageID: 9, Chat: models.Chat{ID: 7, Type: "private"}}
	textMessage  = &models.Update{UpdateID: 1, Message: &models.Message{MessageID: 1, Date: 1, Chat: models.Chat{ID: 7, Type: "private"}, Text: teleiq.Ptr("hi")}}
)

const (
	notModified = "Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message"
	tooOld      = "Bad Request: query is too old and response timeout expired or query ID is invalid"
)

// checkCall checks what a shortcut returned and the request it made: none when want is empty, and
// an error, if any, that is the *teleiq.Error of fail when fail is set.
func checkCall(t *testing.T, srv *teleiqtest.Server, method string, err error, fail *teleiq.Error, wantErr bool, want string) {
	t.Helper()
	if (err != nil) != wantErr {
		t.Fatalf("error = %v, want error: %t", err, wantErr)
	}
	var apiErr *teleiq.Error
	if err != nil && fail != nil && (!errors.As(err, &apiErr) || apiErr.ErrorCode != fail.ErrorCode) {
		t.Errorf("error = %v, want the %d error of the server", err, fail.ErrorCode)
	}
	requests := srv.Requests(method)
	switch {
	case want == "" && len(requests) != 0:
		t.Errorf("%s = %v, want no request", method, requests[0].Params)
	case want != "" && len(requests) != 1:
		t.Errorf("%d %s requests, want 1", len(requests), method)
	case want != "":
		if got := fmt.Sprint(requests[0].Params); got != want {
			t.Errorf("%s = %s\nwant %s", method, got, want)
		}
	}
}

func TestEdit(t *testing.T) {
	keyboard := models.NewInlineKeyboard(models.NewInlineRow(models.NewCallbackButton("A", "a")))
	tests := []struct {
		name    string
		update  *models.Update
		opts    []bot.SendOption
		fail    *teleiq.Error
		wantErr bool
		want    string // the parameters of editMessageText, or "" for no request
	}{
		{name: "not a callback query", update: textMessage, wantErr: true},
		{name: "empty update", update: &models.Update{}, wantErr: true},
		{name: "callback query without a message", update: press(nil, ""), wantErr: true},
		{name: "inline message", update: press(nil, "inl"), want: "map[inline_message_id:inl text:hi]"},
		{name: "message", update: press(accessible, ""), want: "map[chat_id:7 message_id:9 text:hi]"},
		{name: "business message", update: press(business, ""), want: "map[business_connection_id:biz chat_id:7 message_id:9 text:hi]"},
		{name: "inaccessible message", update: press(inaccessible, ""), want: "map[chat_id:7 message_id:9 text:hi]"},
		{name: "HTML", update: press(accessible, ""), opts: []bot.SendOption{bot.HTML()}, want: "map[chat_id:7 message_id:9 parse_mode:HTML text:hi]"},
		{name: "Markdown", update: press(accessible, ""), opts: []bot.SendOption{bot.Markdown()}, want: "map[chat_id:7 message_id:9 parse_mode:MarkdownV2 text:hi]"},
		{name: "Markdown on an inline message", update: press(nil, "inl"), opts: []bot.SendOption{bot.Markdown()}, want: "map[inline_message_id:inl parse_mode:MarkdownV2 text:hi]"},
		{
			name: "inline keyboard", update: press(accessible, ""), opts: []bot.SendOption{bot.Keyboard(keyboard)},
			want: "map[chat_id:7 message_id:9 reply_markup:map[inline_keyboard:[[map[callback_data:a text:A]]]] text:hi]",
		},
		{
			name: "entities and link preview", update: press(nil, "inl"),
			opts: []bot.SendOption{func(p *teleiq.SendMessageParams) {
				p.Entities = []models.MessageEntity{{Type: "bold", Offset: 0, Length: 2}}
				p.LinkPreviewOptions = &models.LinkPreviewOptions{IsDisabled: teleiq.Ptr(true)}
			}},
			want: "map[entities:[map[length:2 offset:0 type:bold]] inline_message_id:inl link_preview_options:map[is_disabled:true] text:hi]",
		},
		{
			name: "text set by an option", update: press(accessible, ""),
			opts: []bot.SendOption{func(p *teleiq.SendMessageParams) { p.Text = "changed" }},
			want: "map[chat_id:7 message_id:9 text:changed]",
		},
		{
			name: "options that do not apply to an edit", update: press(accessible, ""),
			opts: []bot.SendOption{nil, func(p *teleiq.SendMessageParams) {
				p.DisableNotification = teleiq.Ptr(true)
				p.ReplyParameters = &models.ReplyParameters{MessageID: teleiq.Ptr[int64](1)}
				p.ChatID = models.ID(8)
				p.BusinessConnectionID = teleiq.Ptr("other")
			}},
			want: "map[chat_id:7 message_id:9 text:hi]",
		},
		{name: "reply keyboard", update: press(accessible, ""), opts: []bot.SendOption{bot.Keyboard(&models.ReplyKeyboardMarkup{})}, wantErr: true},
		{name: "keyboard removal", update: press(accessible, ""), opts: []bot.SendOption{bot.Keyboard(&models.ReplyKeyboardRemove{})}, wantErr: true},
		{name: "force reply", update: press(nil, "inl"), opts: []bot.SendOption{bot.Keyboard(&models.ForceReply{})}, wantErr: true},
		{
			name: "message is not modified", update: press(accessible, ""), fail: &teleiq.Error{ErrorCode: 400, Description: notModified},
			want: "map[chat_id:7 message_id:9 text:hi]",
		},
		{
			name: "another bad request", update: press(accessible, ""), fail: &teleiq.Error{ErrorCode: 400, Description: "Bad Request: message to edit not found"},
			wantErr: true, want: "map[chat_id:7 message_id:9 text:hi]",
		},
		{
			name: "query is too old", update: press(accessible, ""), fail: &teleiq.Error{ErrorCode: 400, Description: tooOld},
			wantErr: true, want: "map[chat_id:7 message_id:9 text:hi]",
		},
		{
			name: "not modified but not a bad request", update: press(accessible, ""), fail: &teleiq.Error{ErrorCode: 500, Description: "Internal Server Error: message is not modified"},
			wantErr: true, want: "map[chat_id:7 message_id:9 text:hi]",
		},
		{
			name: "forbidden", update: press(accessible, ""), fail: &teleiq.Error{ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"},
			wantErr: true, want: "map[chat_id:7 message_id:9 text:hi]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, b := newBot(t)
			if tt.fail != nil {
				srv.Fail("editMessageText", tt.fail)
			}
			err := b.NewContext(tt.update).Edit(context.Background(), "hi", tt.opts...)
			checkCall(t, srv, "editMessageText", err, tt.fail, tt.wantErr, tt.want)
		})
	}
}

func TestAnswer(t *testing.T) {
	tests := []struct {
		name    string
		update  *models.Update
		text    string
		fail    *teleiq.Error
		wantErr bool
		want    string // the parameters of answerCallbackQuery, or "" for no request
	}{
		{name: "not a callback query", update: textMessage, text: "hi", wantErr: true},
		{name: "text", update: press(accessible, ""), text: "hi", want: "map[callback_query_id:q text:hi]"},
		{name: "no text", update: press(accessible, ""), want: "map[callback_query_id:q]"},
		{name: "inline message", update: press(nil, "inl"), text: "hi", want: "map[callback_query_id:q text:hi]"},
		{name: "inaccessible message", update: press(inaccessible, ""), text: "hi", want: "map[callback_query_id:q text:hi]"},
		{name: "query is too old", update: press(accessible, ""), text: "hi", fail: &teleiq.Error{ErrorCode: 400, Description: tooOld}, want: "map[callback_query_id:q text:hi]"},
		{
			name: "another bad request", update: press(accessible, ""), text: "hi", fail: &teleiq.Error{ErrorCode: 400, Description: "Bad Request: MESSAGE_TOO_LONG"},
			wantErr: true, want: "map[callback_query_id:q text:hi]",
		},
		{
			name: "message is not modified", update: press(accessible, ""), text: "hi", fail: &teleiq.Error{ErrorCode: 400, Description: notModified},
			wantErr: true, want: "map[callback_query_id:q text:hi]",
		},
		{
			name: "too old but not a bad request", update: press(accessible, ""), text: "hi", fail: &teleiq.Error{ErrorCode: 500, Description: "Internal Server Error: query is too old"},
			wantErr: true, want: "map[callback_query_id:q text:hi]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, b := newBot(t)
			if tt.fail != nil {
				srv.Fail("answerCallbackQuery", tt.fail)
			}
			err := b.NewContext(tt.update).Answer(context.Background(), tt.text)
			checkCall(t, srv, "answerCallbackQuery", err, tt.fail, tt.wantErr, tt.want)
		})
	}
}

func TestCallbackData(t *testing.T) {
	tests := []struct {
		name   string
		update *models.Update
		want   string
	}{
		{"button", press(accessible, ""), "d"},
		{"game", &models.Update{CallbackQuery: &models.CallbackQuery{ID: "q", GameShortName: teleiq.Ptr("g")}}, ""},
		{"message", textMessage, ""},
		{"empty update", nil, ""},
	}
	_, b := newBot(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.NewContext(tt.update).CallbackData(); got != tt.want {
				t.Errorf("CallbackData = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOnCallbackPrefix(t *testing.T) {
	srv, b := newBot(t)
	b.OnCallbackPrefix("color:", func(ctx context.Context, c *bot.Context) error {
		return c.Send(ctx, "color "+c.CallbackData())
	})
	b.OnCallback(func(ctx context.Context, c *bot.Context) error {
		return c.Send(ctx, "other "+c.CallbackData())
	})
	b.OnMessage(bot.ReplyWith("message"))
	stop := teleiqtest.Start(t, b.Run)

	game := teleiqtest.CallbackUpdate(7, 1, "")
	game.CallbackQuery.Data, game.CallbackQuery.GameShortName = nil, teleiq.Ptr("g")
	pushes := []struct {
		update models.Update
		want   string
	}{
		{teleiqtest.CallbackUpdate(7, 1, "color:red"), "color color:red"},
		{teleiqtest.CallbackUpdate(7, 1, "size:big"), "other size:big"},
		{teleiqtest.CallbackUpdate(7, 1, "color:"), "color color:"},
		{teleiqtest.CallbackUpdate(7, 1, "Color:red"), "other Color:red"},
		{teleiqtest.CallbackUpdate(7, 1, "color"), "other color"},
		{teleiqtest.CallbackUpdate(7, 1, "old color:red"), "other old color:red"},
		{game, "other "},
		{teleiqtest.MessageUpdate(7, "color:red"), "message"},
	}
	for _, p := range pushes {
		srv.Push(p.update)
	}
	sent := srv.Wait(t, "sendMessage", len(pushes))
	if len(sent) != len(pushes) {
		t.Fatalf("%d messages, want %d", len(sent), len(pushes))
	}
	for i, p := range pushes {
		if got := sent[i].Params["text"]; got != p.want {
			t.Errorf("update %d: sent %q, want %q", i, got, p.want)
		}
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}
