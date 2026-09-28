package filter

import (
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
)

// NewChatMembers matches the service messages about new members of a group.
func NewChatMembers() bot.Filter {
	return Message(func(m *models.Message) bool { return len(m.NewChatMembers) > 0 })
}

// LeftChatMember matches service messages about a member who left or was removed.
func LeftChatMember() bot.Filter {
	return Message(func(m *models.Message) bool { return m.LeftChatMember != nil })
}

// NewChatTitle matches the service messages about a new title of a group.
func NewChatTitle() bot.Filter {
	return Message(func(m *models.Message) bool { return m.NewChatTitle != nil })
}

// NewChatPhoto matches the service messages about a new photo of a group.
func NewChatPhoto() bot.Filter {
	return Message(func(m *models.Message) bool { return len(m.NewChatPhoto) > 0 })
}

// DeleteChatPhoto matches the service messages about a deleted group photo.
func DeleteChatPhoto() bot.Filter {
	return Message(func(m *models.Message) bool { return m.DeleteChatPhoto })
}

// GroupChatCreated matches the service messages about the creation of a group.
func GroupChatCreated() bot.Filter {
	return Message(func(m *models.Message) bool { return m.GroupChatCreated })
}

// MigrateToChat matches the service messages in a group that became a supergroup.
func MigrateToChat() bot.Filter {
	return Message(func(m *models.Message) bool { return m.MigrateToChatID != nil })
}

// MigrateFromChat matches the service messages in a supergroup made from a group.
func MigrateFromChat() bot.Filter {
	return Message(func(m *models.Message) bool { return m.MigrateFromChatID != nil })
}

// PinnedMessage matches the service messages about a pinned message.
func PinnedMessage() bot.Filter {
	return Message(func(m *models.Message) bool { return m.PinnedMessage != nil })
}

// VideoChatStarted matches the service messages about a video chat that started.
func VideoChatStarted() bot.Filter {
	return Message(func(m *models.Message) bool { return m.VideoChatStarted != nil })
}

// VideoChatEnded matches the service messages about a video chat that ended.
func VideoChatEnded() bot.Filter {
	return Message(func(m *models.Message) bool { return m.VideoChatEnded != nil })
}

// VideoChatParticipantsInvited matches service messages about video chat invites.
func VideoChatParticipantsInvited() bot.Filter {
	return Message(func(m *models.Message) bool { return m.VideoChatParticipantsInvited != nil })
}

// Service matches service messages, which tell of an event in the chat.
func Service() bot.Filter {
	return Message(isService)
}

func isService(m *models.Message) bool {
	return len(m.NewChatMembers) > 0 || m.LeftChatMember != nil || m.ChatOwnerLeft != nil || m.ChatOwnerChanged != nil ||
		m.NewChatTitle != nil || len(m.NewChatPhoto) > 0 || m.DeleteChatPhoto || m.GroupChatCreated ||
		m.SupergroupChatCreated || m.ChannelChatCreated || m.MessageAutoDeleteTimerChanged != nil ||
		m.MigrateToChatID != nil || m.MigrateFromChatID != nil || m.PinnedMessage != nil ||
		m.SuccessfulPayment != nil || m.RefundedPayment != nil || m.UsersShared != nil || m.ChatShared != nil ||
		m.Gift != nil || m.UniqueGift != nil || m.GiftUpgradeSent != nil || m.ConnectedWebsite != nil ||
		m.WriteAccessAllowed != nil || m.PassportData != nil || m.ProximityAlertTriggered != nil ||
		m.BoostAdded != nil || m.ChatBackgroundSet != nil || m.ChecklistTasksDone != nil ||
		m.ChecklistTasksAdded != nil || m.CommunityChatAdded != nil || m.CommunityChatJoined != nil ||
		m.CommunityChatRemoved != nil || m.DirectMessagePriceChanged != nil || m.ForumTopicCreated != nil ||
		m.ForumTopicEdited != nil || m.ForumTopicClosed != nil || m.ForumTopicReopened != nil ||
		m.GeneralForumTopicHidden != nil || m.GeneralForumTopicUnhidden != nil || m.GiveawayCreated != nil ||
		m.GiveawayCompleted != nil || m.ManagedBotCreated != nil || m.PaidMessagePriceChanged != nil ||
		m.PollOptionAdded != nil || m.PollOptionDeleted != nil || m.SuggestedPostApproved != nil ||
		m.SuggestedPostApprovalFailed != nil || m.SuggestedPostDeclined != nil || m.SuggestedPostPaid != nil ||
		m.SuggestedPostRefunded != nil || m.VideoChatScheduled != nil || m.VideoChatStarted != nil ||
		m.VideoChatEnded != nil || m.VideoChatParticipantsInvited != nil || m.WebAppData != nil
}
