package main

import (
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

func testModel() *model {
	m := &model{types: map[string]*schema.Type{}, unions: map[string]*union{}}
	for _, name := range []string{"User", "MessageEntity", "InlineKeyboardMarkup", "ReplyKeyboardMarkup", "ReplyKeyboardRemove", "ForceReply",
		"InputMediaAudio", "InputMediaDocument", "InputMediaLivePhoto", "InputMediaPhoto", "InputMediaVideo"} {
		m.types[name] = &schema.Type{Name: name}
	}
	m.types["ReactionType"] = &schema.Type{Name: "ReactionType", Members: []string{"ReactionTypeEmoji"}}
	m.unions["ReactionType"] = &union{name: "ReactionType"}
	for _, s := range synthetic {
		m.unions[s.name] = &union{name: s.name, synthetic: true}
	}
	return m
}

func TestIntType(t *testing.T) {
	tests := []struct {
		name, desc, want string
	}{
		{"id", "Unique identifier for this user or bot", "int64"},
		{"chat_id", "Unique identifier for the target chat", "int64"},
		{"message_ids", "A list of message identifiers", "int64"},
		{"date", "Date the message was sent in Unix time", "int64"},
		{"send_date", "Proposed send date of the post", "int64"},
		{"file_size", "File size in bytes", "int64"},
		{"offset", "Identifier of the first update to be returned", "int64"},
		{"unix_time", "For “date_time” only, the Unix time associated with the entity", "int64"},
		{"limit", "Limits the number of updates to be retrieved", "int"},
		{"offset", "Offset in UTF-16 code units to the start of the entity", "int"},
		{"colspan", "The number of columns the cell spans if it is bigger than 1", "int"},
	}
	for _, tt := range tests {
		t.Run(tt.name+" "+tt.want, func(t *testing.T) {
			if got := intType(schema.Field{Name: tt.name, Description: tt.desc}); got != tt.want {
				t.Errorf("intType(%s) = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

func TestResolveField(t *testing.T) {
	tests := []struct {
		name      string
		in        schema.Field
		wantType  string
		wantTag   string
		wantConst string
		wantUnion string
		wantErr   string
	}{
		{name: "required string", in: schema.Field{Name: "text", Types: []string{"String"}, Required: true}, wantType: "string", wantTag: `json:"text"`},
		{name: "optional string", in: schema.Field{Name: "title", Types: []string{"String"}}, wantType: "*string", wantTag: `json:"title,omitempty"`},
		{name: "optional id", in: schema.Field{Name: "message_thread_id", Types: []string{"Integer"}}, wantType: "*int64", wantTag: `json:"message_thread_id,omitempty"`},
		{name: "optional count", in: schema.Field{Name: "limit", Types: []string{"Integer"}}, wantType: "*int", wantTag: `json:"limit,omitempty"`},
		{name: "float", in: schema.Field{Name: "latitude", Types: []string{"Float"}, Required: true}, wantType: "float64", wantTag: `json:"latitude"`},
		{name: "optional boolean", in: schema.Field{Name: "can_join_groups", Types: []string{"Boolean"}}, wantType: "*bool", wantTag: `json:"can_join_groups,omitempty"`},
		{name: "optional true", in: schema.Field{Name: "is_premium", Types: []string{"True"}}, wantType: "bool", wantTag: `json:"is_premium,omitempty"`},
		{name: "required true", in: schema.Field{Name: "force_reply", Types: []string{"True"}, Required: true}, wantConst: "true"},
		{name: "discriminator", in: schema.Field{Name: "type", Types: []string{"String"}, Required: true, Const: "photo"}, wantConst: `"photo"`},
		{name: "required object", in: schema.Field{Name: "from", Types: []string{"User"}, Required: true}, wantType: "User", wantTag: `json:"from"`},
		{name: "optional object", in: schema.Field{Name: "from", Types: []string{"User"}}, wantType: "*User", wantTag: `json:"from,omitempty"`},
		{name: "array", in: schema.Field{Name: "entities", Types: []string{"Array of MessageEntity"}}, wantType: "[]MessageEntity", wantTag: `json:"entities,omitzero"`},
		{name: "array of arrays", in: schema.Field{Name: "keyboard", Types: []string{"Array of Array of User"}, Required: true}, wantType: "[][]User", wantTag: `json:"keyboard"`},
		{name: "array of ids", in: schema.Field{Name: "message_ids", Types: []string{"Array of Integer"}, Required: true}, wantType: "[]int64", wantTag: `json:"message_ids"`},
		{name: "union", in: schema.Field{Name: "type", Types: []string{"ReactionType"}, Required: true}, wantType: "ReactionType", wantTag: `json:"type"`, wantUnion: "ReactionType"},
		{name: "array of unions", in: schema.Field{Name: "reaction", Types: []string{"Array of ReactionType"}}, wantType: "[]ReactionType", wantTag: `json:"reaction,omitzero"`, wantUnion: "ReactionType"},
		{name: "chat id", in: schema.Field{Name: "chat_id", Types: []string{"Integer", "String"}, Required: true}, wantType: "ChatID", wantTag: `json:"chat_id"`},
		{name: "optional chat id", in: schema.Field{Name: "chat_id", Types: []string{"Integer", "String"}}, wantType: "ChatID", wantTag: `json:"chat_id,omitzero"`},
		{name: "input file", in: schema.Field{Name: "photo", Types: []string{"InputFile", "String"}, Required: true}, wantType: "InputFile", wantTag: `json:"photo"`},
		{name: "upload only", in: schema.Field{Name: "certificate", Types: []string{"InputFile"}}, wantType: "InputFile", wantTag: `json:"certificate,omitzero"`},
		{name: "attach string", in: schema.Field{Name: "media", Types: []string{"String"}, Required: true, Description: "pass “attach://<file_attach_name>” to upload"}, wantType: "InputFile", wantTag: `json:"media"`},
		{
			name: "reply markup", in: schema.Field{Name: "reply_markup", Types: []string{"InlineKeyboardMarkup", "ReplyKeyboardMarkup", "ReplyKeyboardRemove", "ForceReply"}},
			wantType: "ReplyMarkup", wantTag: `json:"reply_markup,omitempty"`, wantUnion: "ReplyMarkup",
		},
		{
			name: "media group", in: schema.Field{Name: "media", Required: true, Types: []string{"Array of InputMediaAudio", "Array of InputMediaDocument", "Array of InputMediaLivePhoto", "Array of InputMediaPhoto", "Array of InputMediaVideo"}},
			wantType: "[]InputMediaGroupItem", wantTag: `json:"media"`, wantUnion: "InputMediaGroupItem",
		},
		{name: "unknown type", in: schema.Field{Name: "x", Types: []string{"Mystery"}}, wantErr: `unknown type "Mystery"`},
		{name: "unknown combination", in: schema.Field{Name: "x", Types: []string{"User", "MessageEntity"}}, wantErr: "no rule for the type"},
		{name: "mixed depths", in: schema.Field{Name: "x", Types: []string{"User", "Array of User"}}, wantErr: "mixed array depths"},
	}
	m := testModel()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := m.resolveField("Owner", tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "Owner.x") {
					t.Fatalf("resolveField() error = %v, want one naming Owner.x and %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveField() error = %v", err)
			}
			if f.constant != tt.wantConst || f.union != tt.wantUnion {
				t.Errorf("constant, union = %q, %q; want %q, %q", f.constant, f.union, tt.wantConst, tt.wantUnion)
			}
			if tt.wantConst != "" {
				return
			}
			if got := f.goType(); got != tt.wantType {
				t.Errorf("goType() = %s, want %s", got, tt.wantType)
			}
			if got := f.tag(); got != "`"+tt.wantTag+"`" {
				t.Errorf("tag() = %s, want `%s`", got, tt.wantTag)
			}
		})
	}
}

func TestResolveResult(t *testing.T) {
	m := testModel()
	m.unions["ReactionType"].decode = true
	m.types["InputMedia"] = &schema.Type{Name: "InputMedia", Members: []string{"InputMediaPhoto"}}
	m.unions["InputMedia"] = &union{name: "InputMedia"}
	m.types["InputFile"] = &schema.Type{Name: "InputFile"}
	tests := []struct {
		name       string
		returns    []string
		wantResult resultKind
		wantType   string
		wantErr    string
	}{
		{name: "true", returns: []string{"True"}, wantResult: resultTrue},
		{name: "object", returns: []string{"User"}, wantResult: resultObject, wantType: "User"},
		{name: "array", returns: []string{"Array of User"}, wantResult: resultValue, wantType: "[]User"},
		{name: "string", returns: []string{"String"}, wantResult: resultValue, wantType: "string"},
		{name: "integer", returns: []string{"Integer"}, wantResult: resultValue, wantType: "int"},
		{name: "object or true", returns: []string{"User", "True"}, wantResult: resultOrTrue, wantType: "User"},
		{name: "union", returns: []string{"ReactionType"}, wantResult: resultUnion, wantType: "ReactionType"},
		{name: "array of unions", returns: []string{"Array of ReactionType"}, wantResult: resultUnions, wantType: "[]ReactionType"},
		{name: "union that cannot be decoded", returns: []string{"InputMedia"}, wantErr: "cannot be decoded"},
		{name: "handwritten type", returns: []string{"InputFile"}, wantErr: "no rule"},
		{name: "boolean", returns: []string{"Boolean"}, wantErr: "no rule"},
		{name: "nested array", returns: []string{"Array of Array of User"}, wantErr: "no rule"},
		{name: "two objects", returns: []string{"User", "MessageEntity"}, wantErr: "no rule"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meth := &method{}
			err := m.resolveResult(meth, tt.returns)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveResult() error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || meth.result != tt.wantResult || meth.typ != tt.wantType {
				t.Fatalf("resolveResult() = %v %q, %v; want %v %q", meth.result, meth.typ, err, tt.wantResult, tt.wantType)
			}
		})
	}
}
