package bot

import (
	"github.com/DhurghamAhmed/teleiq/models"
)

// messageOf returns the message that u carries, if any.
func messageOf(u *models.Update) *models.Message {
	for _, m := range []*models.Message{u.Message, u.EditedMessage, u.ChannelPost, u.EditedChannelPost,
		u.BusinessMessage, u.EditedBusinessMessage, u.GuestMessage} {
		if m != nil {
			return m
		}
	}
	return nil
}

// updateChat returns the chat that u belongs to, or nil for updates without one.
func updateChat(u *models.Update) *models.Chat {
	if m := messageOf(u); m != nil {
		return &m.Chat
	}
	switch {
	case u.CallbackQuery != nil:
		switch m := u.CallbackQuery.Message.(type) {
		case *models.Message:
			return &m.Chat
		case *models.InaccessibleMessage:
			return &m.Chat
		}
	case u.DeletedBusinessMessages != nil:
		return &u.DeletedBusinessMessages.Chat
	case u.MessageReaction != nil:
		return &u.MessageReaction.Chat
	case u.MessageReactionCount != nil:
		return &u.MessageReactionCount.Chat
	case u.PollAnswer != nil:
		return u.PollAnswer.VoterChat
	case u.MyChatMember != nil:
		return &u.MyChatMember.Chat
	case u.ChatMember != nil:
		return &u.ChatMember.Chat
	case u.ChatJoinRequest != nil:
		return &u.ChatJoinRequest.Chat
	case u.ChatBoost != nil:
		return &u.ChatBoost.Chat
	case u.RemovedChatBoost != nil:
		return &u.RemovedChatBoost.Chat
	case u.StoppedMessageGeneration != nil:
		return &u.StoppedMessageGeneration.Chat
	}
	return nil
}

// updateSender returns the user who caused u, or nil when it has none.
func updateSender(u *models.Update) *models.User {
	if m := messageOf(u); m != nil {
		return m.From
	}
	switch {
	case u.BusinessConnection != nil:
		return &u.BusinessConnection.User
	case u.MessageReaction != nil:
		return u.MessageReaction.User
	case u.InlineQuery != nil:
		return &u.InlineQuery.From
	case u.ChosenInlineResult != nil:
		return &u.ChosenInlineResult.From
	case u.CallbackQuery != nil:
		return &u.CallbackQuery.From
	case u.ShippingQuery != nil:
		return &u.ShippingQuery.From
	case u.PreCheckoutQuery != nil:
		return &u.PreCheckoutQuery.From
	case u.PurchasedPaidMedia != nil:
		return &u.PurchasedPaidMedia.From
	case u.PollAnswer != nil:
		return u.PollAnswer.User
	case u.MyChatMember != nil:
		return &u.MyChatMember.From
	case u.ChatMember != nil:
		return &u.ChatMember.From
	case u.ChatJoinRequest != nil:
		return &u.ChatJoinRequest.From
	case u.ManagedBot != nil:
		return &u.ManagedBot.User
	case u.Subscription != nil:
		return &u.Subscription.User
	}
	return nil
}

// routeKey returns the key that orders u: its chat, else its sender, else 0.
func routeKey(u *models.Update) int64 {
	if u == nil {
		return 0
	}
	if c := updateChat(u); c != nil {
		return c.ID
	}
	if s := updateSender(u); s != nil {
		return s.ID
	}
	return 0
}

// updateKind returns the name of the field that u carries, or "" for none.
func updateKind(u *models.Update) string {
	switch {
	case u.Message != nil:
		return "message"
	case u.EditedMessage != nil:
		return "edited_message"
	case u.ChannelPost != nil:
		return "channel_post"
	case u.EditedChannelPost != nil:
		return "edited_channel_post"
	case u.BusinessConnection != nil:
		return "business_connection"
	case u.BusinessMessage != nil:
		return "business_message"
	case u.EditedBusinessMessage != nil:
		return "edited_business_message"
	case u.DeletedBusinessMessages != nil:
		return "deleted_business_messages"
	case u.GuestMessage != nil:
		return "guest_message"
	case u.MessageReaction != nil:
		return "message_reaction"
	case u.MessageReactionCount != nil:
		return "message_reaction_count"
	case u.InlineQuery != nil:
		return "inline_query"
	case u.ChosenInlineResult != nil:
		return "chosen_inline_result"
	case u.CallbackQuery != nil:
		return "callback_query"
	case u.ShippingQuery != nil:
		return "shipping_query"
	case u.PreCheckoutQuery != nil:
		return "pre_checkout_query"
	case u.PurchasedPaidMedia != nil:
		return "purchased_paid_media"
	case u.Poll != nil:
		return "poll"
	case u.PollAnswer != nil:
		return "poll_answer"
	case u.MyChatMember != nil:
		return "my_chat_member"
	case u.ChatMember != nil:
		return "chat_member"
	case u.ChatJoinRequest != nil:
		return "chat_join_request"
	case u.ChatBoost != nil:
		return "chat_boost"
	case u.RemovedChatBoost != nil:
		return "removed_chat_boost"
	case u.ManagedBot != nil:
		return "managed_bot"
	case u.Subscription != nil:
		return "subscription"
	case u.StoppedMessageGeneration != nil:
		return "stopped_message_generation"
	}
	return ""
}
