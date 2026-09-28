package bot_test

import (
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// TestShortcutsWithDefaults checks that the shortcuts send the defaults of their Client, and that
// an option of the call wins over them.
func TestShortcutsWithDefaults(t *testing.T) {
	srv := teleiqtest.NewServer()
	t.Cleanup(srv.Close)
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL), teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}),
		teleiq.WithDefaults(teleiq.Defaults{ParseMode: teleiq.ParseModeHTML, DisableNotification: true}))
	if err != nil {
		t.Fatal(err)
	}
	b := bot.New(client)
	ctx := t.Context()
	message := teleiqtest.MessageUpdate(7, "hi")
	if err := b.NewContext(&message).Send(ctx, "<b>hi</b>"); err != nil {
		t.Fatal(err)
	}
	if err := b.NewContext(&message).Send(ctx, "*hi*", bot.Markdown()); err != nil {
		t.Fatal(err)
	}
	press := teleiqtest.CallbackUpdate(7, 9, "x")
	if err := b.NewContext(&press).Edit(ctx, "<i>edited</i>"); err != nil {
		t.Fatal(err)
	}
	sent := srv.Requests("sendMessage")
	if len(sent) != 2 || sent[0].Params["parse_mode"] != "HTML" || sent[0].Params["disable_notification"] != true ||
		sent[1].Params["parse_mode"] != "MarkdownV2" {
		t.Errorf("sendMessage = %v, want HTML by default and MarkdownV2 by the option, both silent", sent)
	}
	if edit := srv.Requests("editMessageText"); len(edit) != 1 || edit[0].Params["parse_mode"] != "HTML" {
		t.Errorf("editMessageText = %v, want the default parse mode", edit)
	}
}
