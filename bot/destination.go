package bot

import (
	"fmt"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// destination is where a message sent in answer to an update goes.
type destination struct {
	chat     models.ChatID
	thread   *int64
	business *string
	topic    *int64
}

// destination returns where an answer to the update goes; method names the caller.
func (c *Context) destination(method string) (destination, error) {
	chat := c.Chat()
	if chat == nil {
		return destination{}, fmt.Errorf("bot: %s: the update has no chat", method)
	}
	d := destination{chat: models.ID(chat.ID)}
	// A callback query on a message too old to be accessible leaves only the chat.
	if m := c.Message(); m != nil {
		// Outside a forum topic, message_thread_id names the thread of a reply, not a topic.
		if m.IsTopicMessage && m.MessageThreadID != nil {
			d.thread = teleiq.Ptr(*m.MessageThreadID)
		}
		if m.BusinessConnectionID != nil {
			d.business = teleiq.Ptr(*m.BusinessConnectionID)
		}
		if m.DirectMessagesTopic != nil {
			d.topic = teleiq.Ptr(m.DirectMessagesTopic.TopicID)
		}
	}
	return d, nil
}

// destinationFields points at the fields that say where a sent message goes.
type destinationFields struct {
	chat       *models.ChatID
	thread     **int64
	business   **string
	businessID *string // a required business_connection_id, as sendChecklist has
	topic      **int64
}

// fillDestination sets the chat of f and fills its empty fields from the update.
func (c *Context) fillDestination(method string, f destinationFields) error {
	d, err := c.destination(method)
	if err != nil {
		return err
	}
	*f.chat = d.chat
	if f.thread != nil && *f.thread == nil {
		*f.thread = d.thread
	}
	if f.business != nil && *f.business == nil {
		*f.business = d.business
	}
	if f.businessID != nil && *f.businessID == "" && d.business != nil {
		*f.businessID = *d.business
	}
	if f.topic != nil && *f.topic == nil {
		*f.topic = d.topic
	}
	return nil
}
