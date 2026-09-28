package bot_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestDownload(t *testing.T) {
	tests := []struct {
		name string
		set  func(m *models.Message)
		want string // the file_id downloaded, or "" for an error without a request
	}{
		{"largest size of a photo", func(m *models.Message) {
			m.Photo = []models.PhotoSize{{FileID: "big", Width: 1280, Height: 960}, {FileID: "small", Width: 90, Height: 68}, {FileID: "medium", Width: 320, Height: 240}}
		}, "big"},
		{"video of a live photo", func(m *models.Message) {
			m.LivePhoto, m.Photo = &models.LivePhoto{FileID: "live"}, []models.PhotoSize{{FileID: "still", Width: 90, Height: 68}}
		}, "live"},
		{"animation rather than its document", func(m *models.Message) {
			m.Animation, m.Document = &models.Animation{FileID: "gif"}, &models.Document{FileID: "doc"}
		}, "gif"},
		{"document", func(m *models.Message) { m.Document = &models.Document{FileID: "doc"} }, "doc"},
		{"audio", func(m *models.Message) { m.Audio = &models.Audio{FileID: "audio"} }, "audio"},
		{"video", func(m *models.Message) { m.Video = &models.Video{FileID: "video"} }, "video"},
		{"video note", func(m *models.Message) { m.VideoNote = &models.VideoNote{FileID: "note"} }, "note"},
		{"voice", func(m *models.Message) { m.Voice = &models.Voice{FileID: "voice"} }, "voice"},
		{"sticker", func(m *models.Message) { m.Sticker = &models.Sticker{FileID: "sticker"} }, "sticker"},
		{"text", func(m *models.Message) {}, ""},
		{"contact", func(m *models.Message) { m.Contact = &models.Contact{} }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, b := newBot(t)
			if tt.want != "" {
				srv.AddFile(tt.want, []byte("data of "+tt.want))
			}
			u := teleiqtest.MessageUpdate(7, "")
			u.Message.Text = nil
			tt.set(u.Message)
			var buf bytes.Buffer
			err := b.NewContext(&u).Download(t.Context(), &buf)
			if tt.want == "" {
				checkCall(t, srv, "getFile", err, nil, true, "")
				return
			}
			checkCall(t, srv, "getFile", err, nil, false, "map[file_id:"+tt.want+"]")
			if buf.String() != "data of "+tt.want {
				t.Errorf("downloaded %q, want %q", buf.String(), "data of "+tt.want)
			}
		})
	}
	t.Run("without a message", func(t *testing.T) {
		srv, b := newBot(t)
		err := b.NewContext(inlineQuery).Download(t.Context(), &bytes.Buffer{})
		checkCall(t, srv, "getFile", err, nil, true, "")
	})
}

func TestOnJoinRequest(t *testing.T) {
	srv, b := newBot(t)
	b.OnJoinRequest(func(ctx context.Context, c *bot.Context) error { return c.Approve(ctx) })
	b.OnMessage(bot.ReplyWith("a message"))
	stop := teleiqtest.Start(t, b.Run)
	srv.WaitHandled(t, srv.Push(teleiqtest.GroupMessageUpdate(-100, 7, "hi")))
	srv.WaitHandled(t, srv.Push(teleiqtest.JoinRequestUpdate(-100, 8)))
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	approved := srv.Requests("approveChatJoinRequest")
	if len(approved) != 1 || approved[0].Params["chat_id"] != float64(-100) || approved[0].Params["user_id"] != float64(8) {
		t.Errorf("approveChatJoinRequest = %v, want one for user 8 in chat -100", approved)
	}
	if sent := srv.Requests("sendMessage"); len(sent) != 1 {
		t.Errorf("%d replies, want one, to the message only", len(sent))
	}
}
