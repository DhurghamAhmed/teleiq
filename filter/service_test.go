package filter_test

import (
	"reflect"
	"testing"

	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/models"
)

// content are the fields of Message that are not a service message: what a user sent, and the
// facts about it. A field that a new version of the Bot API adds goes here or in the list of Service.
var content = map[string]bool{
	"MessageID": true, "MessageThreadID": true, "DirectMessagesTopic": true, "From": true, "SenderChat": true,
	"SenderBoostCount": true, "SenderBusinessBot": true, "SenderTag": true, "ReceiverUser": true,
	"EphemeralMessageID": true, "Date": true, "GuestQueryID": true, "BusinessConnectionID": true, "Chat": true,
	"ForwardOrigin": true, "IsTopicMessage": true, "IsAutomaticForward": true, "ReplyToMessage": true,
	"ExternalReply": true, "Quote": true, "ReplyToStory": true, "ReplyToChecklistTaskID": true,
	"ReplyToPollOptionID": true, "ViaBot": true, "GuestBotCallerUser": true, "GuestBotCallerChat": true,
	"EditDate": true, "HasProtectedContent": true, "IsFromOffline": true, "IsPaidPost": true, "MediaGroupID": true,
	"AuthorSignature": true, "PaidStarCount": true, "Text": true, "Entities": true, "LinkPreviewOptions": true,
	"SuggestedPostInfo": true, "EffectID": true, "RichMessage": true, "Animation": true, "Audio": true,
	"Document": true, "LivePhoto": true, "PaidMedia": true, "Photo": true, "Sticker": true, "Story": true,
	"Video": true, "VideoNote": true, "Voice": true, "Caption": true, "CaptionEntities": true,
	"ShowCaptionAboveMedia": true, "HasMediaSpoiler": true, "Checklist": true, "Contact": true, "Dice": true,
	"Game": true, "Poll": true, "Venue": true, "Location": true, "Invoice": true, "Giveaway": true,
	"GiveawayWinners": true, "ReplyMarkup": true,
}

// TestServiceClassifiesEveryField sets each field of Message alone and checks that Service matches
// the message exactly when the field is not in content.
func TestServiceClassifiesEveryField(t *testing.T) {
	b := newTestBot(t)
	interfaces := map[reflect.Type]any{
		reflect.TypeFor[models.MaybeInaccessibleMessage](): &models.Message{},
		reflect.TypeFor[models.MessageOrigin]():            &models.MessageOriginUser{},
	}
	service := 0
	typ := reflect.TypeFor[models.Message]()
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Name == "Chat" {
			continue // every message has one
		}
		var m models.Message
		v := reflect.ValueOf(&m).Elem().Field(i)
		switch v.Kind() {
		case reflect.Pointer:
			v.Set(reflect.New(f.Type.Elem()))
		case reflect.Slice:
			v.Set(reflect.MakeSlice(f.Type, 1, 1))
		case reflect.Bool:
			v.SetBool(true)
		case reflect.Int, reflect.Int64:
			v.SetInt(1)
		case reflect.String:
			v.SetString("x")
		case reflect.Interface:
			value, ok := interfaces[f.Type]
			if !ok {
				t.Fatalf("%s: no value for the interface %s; add one to the test", f.Name, f.Type)
			}
			v.Set(reflect.ValueOf(value))
		default:
			t.Fatalf("%s: no value for a %s; add one to the test", f.Name, f.Type)
		}
		want := !content[f.Name]
		if want {
			service++
		}
		if got := filter.Service().Match(b.NewContext(&models.Update{Message: &m})); got != want {
			t.Errorf("%s: Service matched %t, want %t: classify the field in content or in Service", f.Name, got, want)
		}
	}
	if service != 55 {
		t.Errorf("%d fields are service messages, want 55", service)
	}
	for name := range content {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("content names %s, which Message no longer has", name)
		}
	}
}
