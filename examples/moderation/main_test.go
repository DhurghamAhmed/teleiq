package main

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestModeration(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error { return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL)) })
	send := func(u models.Update) { srv.WaitHandled(t, srv.Push(u)) }
	const group = -1001234567890
	reply := func(text string, to int64) models.Update {
		u := teleiqtest.GroupMessageUpdate(group, 7, text)
		u.Message.ReplyToMessage = &models.Message{From: &models.User{ID: to}}
		return u
	}

	joined := teleiqtest.GroupMessageUpdate(group, 8, "")
	joined.Message.Text, joined.Message.NewChatMembers = nil, []models.User{{ID: 8}}
	send(joined)
	send(teleiqtest.MessageUpdate(8, "/ban")) // a private chat is not a group
	send(teleiqtest.GroupMessageUpdate(group, 7, "/ban"))
	send(reply("/ban", 9))
	send(reply("/mute", 10))
	srv.Fail("banChatMember", &teleiq.Error{ErrorCode: 400, Description: "Bad Request: user is an administrator of the chat"})
	send(reply("/ban", 11))
	send(teleiqtest.JoinRequestUpdate(group, 12))
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	if n := len(srv.Requests("deleteMessage")); n != 1 {
		t.Errorf("%d messages deleted, want the one about the new member", n)
	}
	if bans := srv.Requests("banChatMember"); len(bans) != 2 || bans[0].Params["user_id"] != float64(9) {
		t.Errorf("bans = %v, want user 9 and the administrator 11", bans)
	}
	if mutes := srv.Requests("restrictChatMember"); len(mutes) != 1 || mutes[0].Params["until_date"] == nil {
		t.Errorf("mutes = %v, want user 10 for an hour", mutes)
	}
	var replies []any
	for _, r := range srv.Requests("sendMessage") {
		replies = append(replies, r.Params["text"])
	}
	want := []any{"Answer the message of the member with /ban.", "Done.", "Done.", "I cannot do that to an administrator."}
	if len(replies) != len(want) {
		t.Fatalf("replies = %q, want %q", replies, want)
	}
	for i := range want {
		if replies[i] != want[i] {
			t.Errorf("reply %d = %q, want %q", i, replies[i], want[i])
		}
	}
	if len(srv.Requests("approveChatJoinRequest")) != 1 {
		t.Errorf("the request to join was not approved")
	}
}
