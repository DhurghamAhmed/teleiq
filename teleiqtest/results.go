package teleiqtest

import (
	"context"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// methodInfo is what the server knows about a Bot API method.
type methodInfo struct {
	result reflect.Type    // nil when the method returns only an error, that is True
	text   map[string]bool // the parameters whose type is a string
}

// methods holds every Bot API method of package teleiq, by its Bot API name.
var methods = func() map[string]methodInfo {
	ctxType := reflect.TypeFor[context.Context]()
	out := map[string]methodInfo{}
	client := reflect.TypeFor[*teleiq.Client]()
	for i := range client.NumMethod() {
		m := client.Method(i)
		ft := m.Type // the receiver is the first argument
		if ft.NumIn() < 2 || ft.NumIn() > 3 || ft.In(1) != ctxType {
			continue
		}
		info := methodInfo{text: map[string]bool{}}
		if ft.NumIn() == 3 {
			params := ft.In(2)
			if params.Kind() != reflect.Struct {
				continue
			}
			for j := range params.NumField() {
				f := params.Field(j)
				name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
				if t := f.Type; t.Kind() == reflect.String || t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.String {
					info.text[name] = true
				}
			}
		}
		if ft.NumOut() == 2 {
			info.result = ft.Out(0)
		}
		r, size := utf8.DecodeRuneInString(m.Name)
		out[string(unicode.ToLower(r))+m.Name[size:]] = info
	}
	return out
}()

var (
	messageType    = reflect.TypeFor[*models.Message]()
	messagesType   = reflect.TypeFor[[]models.Message]()
	messageIDType  = reflect.TypeFor[*models.MessageId]()
	messageIDsType = reflect.TypeFor[[]models.MessageId]()
)

// botUser is the bot that getMe describes.
var botUser = models.User{ID: 123456, IsBot: true, FirstName: "Test Bot", Username: ptr("test_bot")}

// defaultResult returns the answer to a request with no stub; s.mu must be held.
func (s *Server) defaultResult(r Request) (any, *teleiq.Error) {
	info, ok := methods[r.Method]
	if !ok {
		return nil, &teleiq.Error{ErrorCode: http.StatusNotFound, Description: "Not Found"}
	}
	p := r.Params
	switch r.Method {
	case "getMe":
		return botUser, nil
	case "getFile":
		return s.fileInfo(stringOf(p["file_id"]))
	case "uploadStickerFile":
		return s.describe(s.fileFor(r, "sticker")), nil
	case "getChat":
		c := chatFor(p["chat_id"])
		return models.ChatFullInfo{ID: c.ID, Type: c.Type, Title: c.Title, Username: c.Username, FirstName: c.FirstName}, nil
	case "getChatMember":
		return models.ChatMemberMember{User: *userOf(int64Of(p["user_id"]))}, nil
	case "getChatMemberCount":
		return 1, nil
	case "getChatMenuButton":
		return models.MenuButtonDefault{}, nil
	}
	switch _, inline := p["inline_message_id"]; {
	case info.result == nil:
		return true, nil
	case info.result == messageType && inline:
		return true, nil // the Bot API returns True for a message sent by inline mode
	case info.result == messageType:
		return s.message(r), nil
	case info.result == messagesType:
		return s.mediaGroup(r), nil
	case info.result == messageIDType:
		s.lastMessage++
		return models.MessageId{MessageID: s.lastMessage}, nil
	case info.result == messageIDsType:
		ids, _ := p["message_ids"].([]any)
		out := make([]models.MessageId, len(ids))
		for i := range out {
			s.lastMessage++
			out[i].MessageID = s.lastMessage
		}
		return out, nil
	case info.result.Kind() == reflect.String:
		return "test", nil
	case info.result.Kind() == reflect.Pointer:
		return reflect.New(info.result.Elem()).Interface(), nil
	case info.result.Kind() == reflect.Slice:
		return reflect.MakeSlice(info.result, 0, 0).Interface(), nil
	}
	return reflect.Zero(info.result).Interface(), nil
}

// mediaParams gives the parameter that holds the file of each method that sends one.
var mediaParams = map[string]string{
	"sendPhoto": "photo", "sendDocument": "document", "sendVideo": "video", "sendAudio": "audio",
	"sendVoice": "voice", "sendAnimation": "animation", "sendSticker": "sticker", "sendVideoNote": "video_note",
}

// message returns the message that a send or edit method makes; s.mu must be held.
func (s *Server) message(r Request) *models.Message {
	p := r.Params
	m := &models.Message{Date: time.Now().Unix(), Chat: chatFor(p["chat_id"]), From: &botUser}
	if strings.HasPrefix(r.Method, "edit") || r.Method == "stopMessageLiveLocation" || r.Method == "setGameScore" {
		m.MessageID = int64Of(p["message_id"])
		m.EditDate = ptr(m.Date)
	} else {
		s.lastMessage++
		m.MessageID = s.lastMessage
	}
	if text, ok := p["text"].(string); ok {
		m.Text = &text
	}
	if caption, ok := p["caption"].(string); ok {
		m.Caption = &caption
	}
	if id := int64Of(p["message_thread_id"]); id != 0 {
		m.MessageThreadID = &id
	}
	if id, ok := p["business_connection_id"].(string); ok {
		m.BusinessConnectionID = &id
	}
	if param, ok := mediaParams[r.Method]; ok {
		attach(m, param, s.fileFor(r, param))
	}
	return m
}

// mediaGroup returns the messages that sendMediaGroup sends; s.mu must be held.
func (s *Server) mediaGroup(r Request) []models.Message {
	items, _ := r.Params["media"].([]any)
	out := make([]models.Message, 0, len(items))
	for _, item := range items {
		media, _ := item.(map[string]any)
		one := Request{Method: "sendMediaGroup", Params: map[string]any{"chat_id": r.Params["chat_id"],
			"media": media["media"], "caption": media["caption"]}, Uploads: r.Uploads}
		m := s.message(one)
		attach(m, stringOf(media["type"]), s.fileFor(one, "media"))
		out = append(out, *m)
	}
	return out
}

// attach sets the media of a message from the kind of file it carries.
func attach(m *models.Message, kind string, f *storedFile) {
	switch kind {
	case "photo":
		m.Photo = []models.PhotoSize{{FileID: f.id, FileUniqueID: f.unique, Width: 100, Height: 100, FileSize: ptr(int64(len(f.data)))}}
	case "document":
		m.Document = &models.Document{FileID: f.id, FileUniqueID: f.unique, FileName: f.fileName(), FileSize: ptr(int64(len(f.data)))}
	case "video":
		m.Video = &models.Video{FileID: f.id, FileUniqueID: f.unique, Width: 100, Height: 100, Duration: 1}
	case "audio":
		m.Audio = &models.Audio{FileID: f.id, FileUniqueID: f.unique, Duration: 1, FileName: f.fileName()}
	case "voice":
		m.Voice = &models.Voice{FileID: f.id, FileUniqueID: f.unique, Duration: 1}
	case "animation":
		m.Animation = &models.Animation{FileID: f.id, FileUniqueID: f.unique, Width: 100, Height: 100, Duration: 1}
	case "sticker":
		m.Sticker = &models.Sticker{FileID: f.id, FileUniqueID: f.unique, Type: "regular", Width: 100, Height: 100}
	case "video_note":
		m.VideoNote = &models.VideoNote{FileID: f.id, FileUniqueID: f.unique, Length: 100, Duration: 1}
	}
}

// chatFor returns the chat that a chat_id parameter names.
func chatFor(v any) models.Chat {
	if name, ok := v.(string); ok && strings.HasPrefix(name, "@") {
		return models.Chat{ID: -1001000000000, Type: "channel", Title: ptr("Test channel"), Username: ptr(name[1:])}
	}
	if id := int64Of(v); id != 0 {
		return chatOf(id)
	}
	return models.Chat{}
}

func int64Of(v any) int64 {
	switch v := v.(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}
