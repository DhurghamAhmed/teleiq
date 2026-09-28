package bot_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
)

var inlineQuery = &models.Update{UpdateID: 1, InlineQuery: &models.InlineQuery{ID: "iq", From: models.User{ID: 7, FirstName: "Ann"}, Query: "hi"}}

func TestAnswerInline(t *testing.T) {
	article := bot.Article("a", "A", "hi")
	tests := []struct {
		name    string
		update  *models.Update
		results []models.InlineQueryResult
		opts    []bot.InlineOption
		fail    *teleiq.Error
		wantErr bool
		want    string
	}{
		{name: "not an inline query", update: textMessage, wantErr: true},
		{name: "callback query", update: press(accessible, ""), wantErr: true},
		{name: "empty update", update: &models.Update{}, wantErr: true},
		{name: "no results", update: inlineQuery, want: "map[inline_query_id:iq results:[]]"},
		{name: "empty results", update: inlineQuery, results: []models.InlineQueryResult{}, want: "map[inline_query_id:iq results:[]]"},
		{
			name: "results and options", update: inlineQuery,
			results: []models.InlineQueryResult{article},
			opts:    []bot.InlineOption{bot.CacheTime(0), nil, bot.Personal(), bot.NextOffset("20"), bot.StartButton("Help", "help")},
			want: "map[button:map[start_parameter:help text:Help] cache_time:0 inline_query_id:iq is_personal:true next_offset:20 " +
				"results:[map[id:a input_message_content:map[message_text:hi] title:A type:article]]]",
		},
		{
			name: "fields set by an option", update: inlineQuery,
			opts: []bot.InlineOption{bot.CacheTime(60), func(p *teleiq.AnswerInlineQueryParams) {
				p.InlineQueryID = "other"
				p.CacheTime = teleiq.Ptr(5)
				p.Results = []models.InlineQueryResult{bot.Article("b", "B", "hi")}
				p.Button = &models.InlineQueryResultsButton{Text: "Open", WebApp: &models.WebAppInfo{URL: "https://example.com"}}
			}},
			want: "map[button:map[text:Open web_app:map[url:https://example.com]] cache_time:5 inline_query_id:iq " +
				"results:[map[id:b input_message_content:map[message_text:hi] title:B type:article]]]",
		},
		{name: "no more results", update: inlineQuery, opts: []bot.InlineOption{bot.NextOffset("")}, want: "map[inline_query_id:iq next_offset: results:[]]"},
		{name: "query is too old", update: inlineQuery, fail: &teleiq.Error{ErrorCode: 400, Description: tooOld}, want: "map[inline_query_id:iq results:[]]"},
		{
			name: "another bad request", update: inlineQuery, fail: &teleiq.Error{ErrorCode: 400, Description: "Bad Request: RESULT_ID_DUPLICATE"},
			wantErr: true, want: "map[inline_query_id:iq results:[]]",
		},
		{
			name: "too old but not a bad request", update: inlineQuery, fail: &teleiq.Error{ErrorCode: 500, Description: "Internal Server Error: query is too old"},
			wantErr: true, want: "map[inline_query_id:iq results:[]]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, b := newBot(t)
			if tt.fail != nil {
				srv.Fail("answerInlineQuery", tt.fail)
			}
			err := b.NewContext(tt.update).AnswerInline(context.Background(), tt.results, tt.opts...)
			checkCall(t, srv, "answerInlineQuery", err, tt.fail, tt.wantErr, tt.want)
		})
	}
}

func TestArticle(t *testing.T) {
	keyboard := models.NewInlineKeyboard(models.NewInlineRow(models.NewCallbackButton("A", "a")))
	tests := []struct {
		name string
		opts []bot.SendOption
		want string
	}{
		{name: "plain", want: `{"type":"article","id":"id","title":"Title","input_message_content":{"message_text":"hi"}}`},
		{name: "HTML", opts: []bot.SendOption{bot.HTML()}, want: `{"type":"article","id":"id","title":"Title","input_message_content":{"message_text":"hi","parse_mode":"HTML"}}`},
		{name: "Markdown", opts: []bot.SendOption{bot.Markdown()}, want: `{"type":"article","id":"id","title":"Title","input_message_content":{"message_text":"hi","parse_mode":"MarkdownV2"}}`},
		{
			name: "inline keyboard", opts: []bot.SendOption{bot.Keyboard(keyboard)},
			want: `{"type":"article","id":"id","title":"Title","input_message_content":{"message_text":"hi"},"reply_markup":{"inline_keyboard":[[{"text":"A","callback_data":"a"}]]}}`,
		},
		{
			name: "entities and link preview",
			opts: []bot.SendOption{func(p *teleiq.SendMessageParams) {
				p.Entities = []models.MessageEntity{{Type: "bold", Offset: 0, Length: 2}}
				p.LinkPreviewOptions = &models.LinkPreviewOptions{IsDisabled: teleiq.Ptr(true)}
			}},
			want: `{"type":"article","id":"id","title":"Title","input_message_content":{"message_text":"hi","entities":[{"type":"bold","offset":0,"length":2}],"link_preview_options":{"is_disabled":true}}}`,
		},
		{name: "text set by an option", opts: []bot.SendOption{func(p *teleiq.SendMessageParams) { p.Text = "changed" }},
			want: `{"type":"article","id":"id","title":"Title","input_message_content":{"message_text":"changed"}}`},
		{
			name: "options that do not apply", opts: []bot.SendOption{nil, func(p *teleiq.SendMessageParams) {
				p.DisableNotification = teleiq.Ptr(true)
				p.ChatID = models.ID(8)
			}},
			want: `{"type":"article","id":"id","title":"Title","input_message_content":{"message_text":"hi"}}`,
		},
		{name: "reply keyboard", opts: []bot.SendOption{bot.Keyboard(&models.ReplyKeyboardMarkup{})}, want: `{"type":"article","id":"id","title":"Title","input_message_content":{"message_text":"hi"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(bot.Article("id", "Title", "hi", tt.opts...))
			if err != nil || string(got) != tt.want {
				t.Errorf("Article = %s, %v\nwant %s", got, err, tt.want)
			}
		})
	}
}
