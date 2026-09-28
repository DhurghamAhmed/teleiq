package bot

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/models"
)

func TestRouteKey(t *testing.T) {
	user := models.User{ID: 7, FirstName: "Ann"}
	chat := models.Chat{ID: -100, Type: "supergroup"}
	msg := &models.Message{MessageID: 1, Date: 1, Chat: chat, From: &user}
	tests := []struct {
		name       string
		update     *models.Update
		wantKey    int64
		wantChat   bool
		wantSender bool
	}{
		{name: "message", update: &models.Update{Message: msg}, wantKey: -100, wantChat: true, wantSender: true},
		{name: "edited message", update: &models.Update{EditedMessage: msg}, wantKey: -100, wantChat: true, wantSender: true},
		{name: "channel post without a sender", update: &models.Update{ChannelPost: &models.Message{Chat: models.Chat{ID: -200}}}, wantKey: -200, wantChat: true},
		{name: "business message", update: &models.Update{BusinessMessage: msg}, wantKey: -100, wantChat: true, wantSender: true},
		{name: "guest message", update: &models.Update{GuestMessage: msg}, wantKey: -100, wantChat: true, wantSender: true},
		{name: "callback on a message", update: &models.Update{CallbackQuery: &models.CallbackQuery{From: user, Message: msg}}, wantKey: -100, wantChat: true, wantSender: true},
		{
			name:    "callback on an inaccessible message",
			update:  &models.Update{CallbackQuery: &models.CallbackQuery{From: user, Message: &models.InaccessibleMessage{Chat: models.Chat{ID: -300}}}},
			wantKey: -300, wantChat: true, wantSender: true,
		},
		{name: "inline callback", update: &models.Update{CallbackQuery: &models.CallbackQuery{From: user}}, wantKey: 7, wantSender: true},
		{name: "inline query", update: &models.Update{InlineQuery: &models.InlineQuery{From: user}}, wantKey: 7, wantSender: true},
		{name: "chosen inline result", update: &models.Update{ChosenInlineResult: &models.ChosenInlineResult{From: user}}, wantKey: 7, wantSender: true},
		{name: "shipping query", update: &models.Update{ShippingQuery: &models.ShippingQuery{From: user}}, wantKey: 7, wantSender: true},
		{name: "pre-checkout query", update: &models.Update{PreCheckoutQuery: &models.PreCheckoutQuery{From: user}}, wantKey: 7, wantSender: true},
		{name: "purchased paid media", update: &models.Update{PurchasedPaidMedia: &models.PaidMediaPurchased{From: user}}, wantKey: 7, wantSender: true},
		{name: "poll", update: &models.Update{Poll: &models.Poll{ID: "p"}}, wantKey: 0},
		{name: "poll answer of a user", update: &models.Update{PollAnswer: &models.PollAnswer{User: &user}}, wantKey: 7, wantSender: true},
		{name: "poll answer of a channel", update: &models.Update{PollAnswer: &models.PollAnswer{VoterChat: &chat}}, wantKey: -100, wantChat: true},
		{name: "reaction", update: &models.Update{MessageReaction: &models.MessageReactionUpdated{Chat: chat, User: &user}}, wantKey: -100, wantChat: true, wantSender: true},
		{name: "reaction count", update: &models.Update{MessageReactionCount: &models.MessageReactionCountUpdated{Chat: chat}}, wantKey: -100, wantChat: true},
		{name: "deleted business messages", update: &models.Update{DeletedBusinessMessages: &models.BusinessMessagesDeleted{Chat: chat}}, wantKey: -100, wantChat: true},
		{name: "business connection", update: &models.Update{BusinessConnection: &models.BusinessConnection{User: user}}, wantKey: 7, wantSender: true},
		{name: "my chat member", update: &models.Update{MyChatMember: &models.ChatMemberUpdated{Chat: chat, From: user}}, wantKey: -100, wantChat: true, wantSender: true},
		{name: "chat member", update: &models.Update{ChatMember: &models.ChatMemberUpdated{Chat: chat, From: user}}, wantKey: -100, wantChat: true, wantSender: true},
		{name: "join request", update: &models.Update{ChatJoinRequest: &models.ChatJoinRequest{Chat: chat, From: user}}, wantKey: -100, wantChat: true, wantSender: true},
		{name: "chat boost", update: &models.Update{ChatBoost: &models.ChatBoostUpdated{Chat: chat}}, wantKey: -100, wantChat: true},
		{name: "removed chat boost", update: &models.Update{RemovedChatBoost: &models.ChatBoostRemoved{Chat: chat}}, wantKey: -100, wantChat: true},
		{name: "managed bot", update: &models.Update{ManagedBot: &models.ManagedBotUpdated{User: user}}, wantKey: 7, wantSender: true},
		{name: "subscription", update: &models.Update{Subscription: &models.BotSubscriptionUpdated{User: user}}, wantKey: 7, wantSender: true},
		{name: "stopped message generation", update: &models.Update{StoppedMessageGeneration: &models.MessageGenerationStopped{Chat: chat}}, wantKey: -100, wantChat: true},
		{name: "unknown kind", update: &models.Update{UpdateID: 1}, wantKey: 0},
		{name: "nil", wantKey: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routeKey(tt.update); got != tt.wantKey {
				t.Errorf("routeKey() = %d, want %d", got, tt.wantKey)
			}
			if tt.update == nil {
				return
			}
			if got := updateChat(tt.update) != nil; got != tt.wantChat {
				t.Errorf("updateChat() found a chat = %v, want %v", got, tt.wantChat)
			}
			if got := updateSender(tt.update) != nil; got != tt.wantSender {
				t.Errorf("updateSender() found a sender = %v, want %v", got, tt.wantSender)
			}
		})
	}
}

// TestRouteKeyCoversEveryUpdateKind fails when the Bot API adds a kind of update, so that its
// routing is decided instead of silently sharing the key of updates without a chat or a sender.
func TestRouteKeyCoversEveryUpdateKind(t *testing.T) {
	routed := []string{"UpdateID", "Message", "EditedMessage", "ChannelPost", "EditedChannelPost", "BusinessConnection",
		"BusinessMessage", "EditedBusinessMessage", "DeletedBusinessMessages", "GuestMessage", "MessageReaction",
		"MessageReactionCount", "InlineQuery", "ChosenInlineResult", "CallbackQuery", "ShippingQuery", "PreCheckoutQuery",
		"PurchasedPaidMedia", "Poll", "PollAnswer", "MyChatMember", "ChatMember", "ChatJoinRequest", "ChatBoost",
		"RemovedChatBoost", "ManagedBot", "Subscription", "StoppedMessageGeneration"}
	var fields []string
	for f := range reflect.TypeFor[models.Update]().Fields() {
		fields = append(fields, f.Name)
	}
	if !slices.Equal(fields, routed) {
		t.Errorf("Update has fields %v;\nrouting covers %v", fields, routed)
	}
}

// TestUpdateKindNamesEveryField checks that updateKind returns the JSON name of each field.
func TestUpdateKindNamesEveryField(t *testing.T) {
	for f := range reflect.TypeFor[models.Update]().Fields() {
		if f.Type.Kind() != reflect.Pointer {
			continue
		}
		var u models.Update
		reflect.ValueOf(&u).Elem().FieldByIndex(f.Index).Set(reflect.New(f.Type.Elem()))
		want, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if got := updateKind(&u); got != want {
			t.Errorf("updateKind() with %s = %q, want %q", f.Name, got, want)
		}
	}
	if got := updateKind(&models.Update{UpdateID: 1}); got != "" {
		t.Errorf("updateKind() of an empty update = %q, want empty", got)
	}
}
