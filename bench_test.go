package teleiq

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/models"
)

// benchUpdates are typical updates: a command in a group that replies to another message, and the
// press of a button under a message of the bot.
var benchUpdates = []struct{ name, json string }{
	{"message", `{"update_id":1,"message":{"message_id":12,"from":{"id":7,"is_bot":false,"first_name":"Ann","username":"ann",` +
		`"language_code":"en"},"chat":{"id":-1001234567890,"title":"Team","type":"supergroup"},"date":1790000000,` +
		`"text":"/start@test_bot hello","entities":[{"type":"bot_command","offset":0,"length":15}],"reply_to_message":` +
		`{"message_id":11,"from":{"id":8,"is_bot":false,"first_name":"Bob"},"chat":{"id":-1001234567890,"title":"Team",` +
		`"type":"supergroup"},"date":1789999990,"text":"who is there?"}}}`},
	{"callback_query", `{"update_id":2,"callback_query":{"id":"4382bfdwdsb323b2d9","from":{"id":7,"is_bot":false,` +
		`"first_name":"Ann","username":"ann"},"message":{"message_id":13,"from":{"id":1,"is_bot":true,"first_name":"Bot",` +
		`"username":"test_bot"},"chat":{"id":7,"first_name":"Ann","type":"private"},"date":1790000001,"text":"Pick a color:",` +
		`"reply_markup":{"inline_keyboard":[[{"text":"Red","callback_data":"color:red"},{"text":"Blue",` +
		`"callback_data":"color:blue"}]]}},"chat_instance":"-3000000000000000000","data":"color:red"}}`},
}

func BenchmarkDecodeUpdate(b *testing.B) {
	for _, u := range benchUpdates {
		b.Run(u.name, func(b *testing.B) {
			data := []byte(u.json)
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				var v models.Update
				if err := json.Unmarshal(data, &v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkGetUpdates receives a full batch of 100 updates, the most that getUpdates returns.
func BenchmarkGetUpdates(b *testing.B) {
	updates := make([]string, 100)
	for i := range updates {
		updates[i] = benchUpdates[i%len(benchUpdates)].json
	}
	reply := []byte(`{"ok":true,"result":[` + strings.Join(updates, ",") + `]}`)
	c := newTestClient(b, TransportFunc(func(context.Context, *Request) (*Response, error) {
		return &Response{StatusCode: 200, Body: reply}, nil
	}))
	ctx := context.Background()
	b.SetBytes(int64(len(reply)))
	b.ReportAllocs()
	for b.Loop() {
		if got, err := c.GetUpdates(ctx, GetUpdatesParams{}); err != nil || len(got) != 100 {
			b.Fatalf("GetUpdates() = %d updates, %v", len(got), err)
		}
	}
}

// BenchmarkSendMessage measures what a call costs besides the network: encoding the parameters,
// the client's own work and decoding the result.
func BenchmarkSendMessage(b *testing.B) {
	reply := []byte(sentMessageReply)
	c := newTestClient(b, TransportFunc(func(context.Context, *Request) (*Response, error) {
		return &Response{StatusCode: 200, Body: reply}, nil
	}))
	ctx := context.Background()
	params := SendMessageParams{ChatID: models.ID(42), Text: "Hello, <b>Ann</b>!", ParseMode: Ptr("HTML")}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.SendMessage(ctx, params); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpload streams a 1 MiB document as multipart/form-data to a transport that reads it all.
func BenchmarkUpload(b *testing.B) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 1<<16)
	reply := []byte(sentMessageReply)
	c := newTestClient(b, TransportFunc(func(_ context.Context, req *Request) (*Response, error) {
		if _, err := io.Copy(io.Discard, req.Body); err != nil {
			return nil, err
		}
		return &Response{StatusCode: 200, Body: reply}, nil
	}))
	ctx := context.Background()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.SendDocument(ctx, SendDocumentParams{ChatID: models.ID(42), Document: models.FileFromBytes("report.bin", data)}); err != nil {
			b.Fatal(err)
		}
	}
}
