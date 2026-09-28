package main

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

const snapshotPath = "schema/botapi.json"

func loadAPI(t *testing.T) *schema.API {
	t.Helper()
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	api, err := schema.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func TestModelFromSnapshot(t *testing.T) {
	api := loadAPI(t)
	m, err := newModel(api)
	if err != nil {
		t.Fatalf("newModel() error = %v", err)
	}
	unions := 0
	for _, ty := range api.Types {
		if len(ty.Members) > 0 {
			unions++
		}
	}
	if want := len(api.Types) - unions - len(handwritten); len(m.objects) != want {
		t.Errorf("%d objects, want %d", len(m.objects), want)
	}
	if want := unions + len(synthetic); len(m.order) != want {
		t.Errorf("%d unions, want %d", len(m.order), want)
	}
	var decoded []string
	for _, u := range m.order {
		if u.decode {
			decoded = append(decoded, u.name)
		}
	}
	slices.Sort(decoded)
	want := []string{"BackgroundFill", "BackgroundType", "ChatBoostSource", "ChatMember", "MaybeInaccessibleMessage", "MenuButton",
		"MessageOrigin", "OwnedGift", "PaidMedia", "ReactionType", "RevenueWithdrawalState", "RichBlock", "RichText", "TransactionPartner"}
	if !slices.Equal(decoded, want) {
		t.Errorf("decoded unions = %v, want %v", decoded, want)
	}
	objects := map[string]*object{}
	for _, o := range m.objects {
		objects[o.name] = o
	}
	for name, wantDecode := range map[string]bool{"Message": true, "ChatMemberUpdated": true, "Update": false, "User": false, "InlineQueryResultArticle": false} {
		if o := objects[name]; o == nil || o.decode != wantDecode {
			t.Errorf("object %s decode = %v, want %v", name, o != nil && o.decode, wantDecode)
		}
	}
	chats := 0
	for _, meth := range m.methods {
		if meth.params != nil && meth.params.chat != "" {
			chats++
		}
	}
	if chats != 92 {
		t.Errorf("%d methods have a target chat, want 92", chats)
	}
	for name := range handwritten {
		if objects[name] != nil {
			t.Errorf("handwritten type %s is generated", name)
		}
	}
	if want := len(api.Methods); len(m.methods) != want {
		t.Errorf("%d methods, want %d", len(m.methods), want)
	}
	methods := map[string]*method{}
	for _, meth := range m.methods {
		methods[meth.api] = meth
	}
	for name, want := range map[string]struct {
		result resultKind
		typ    string
		params bool
	}{
		"logOut":                {resultTrue, "", false},
		"getWebhookInfo":        {resultObject, "WebhookInfo", false},
		"getUpdates":            {resultValue, "[]Update", true},
		"editMessageText":       {resultOrTrue, "Message", true},
		"getChatMember":         {resultUnion, "ChatMember", true},
		"getChatAdministrators": {resultUnions, "[]ChatMember", true},
		"getChatMemberCount":    {resultValue, "int", true},
		"exportChatInviteLink":  {resultValue, "string", true},
	} {
		meth := methods[name]
		if meth == nil || meth.result != want.result || meth.typ != want.typ || (meth.params != nil) != want.params {
			t.Errorf("method %s = %+v, want %+v", name, meth, want)
		}
	}
}

// fakeAPI returns an API with the given types, the members of every synthetic union and a method returning the first type.
// fakeAPI returns a schema of types, with the members of the synthetic unions and one method; the
// types without a section are in Available types, as most are.
func fakeAPI(types ...schema.Type) *schema.API {
	api := &schema.API{Version: "1.0", Types: types}
	for _, s := range synthetic {
		for _, name := range s.members {
			if !slices.ContainsFunc(api.Types, func(t schema.Type) bool { return t.Name == name }) {
				api.Types = append(api.Types, schema.Type{Name: name})
			}
		}
	}
	for i := range api.Types {
		if api.Types[i].Section == "" {
			api.Types[i].Section = "Available types"
		}
	}
	api.Methods = []schema.Method{{Name: "getFirst", Returns: []string{types[0].Name}, Section: "Available methods"}}
	return api
}

// withMethod adds meth to api, in Available methods unless it has a section.
func withMethod(api *schema.API, meth schema.Method) *schema.API {
	if meth.Section == "" {
		meth.Section = "Available methods"
	}
	api.Methods = append(api.Methods, meth)
	return api
}

func tagged(name, tag string) schema.Type {
	return schema.Type{Name: name, Fields: []schema.Field{{Name: "type", Types: []string{"String"}, Required: true, Const: tag}}}
}

func TestModelErrors(t *testing.T) {
	str := []string{"String"}
	tests := []struct {
		name    string
		api     *schema.API
		wantErr string
	}{
		{
			name:    "received union without discriminator",
			api:     fakeAPI(schema.Type{Name: "U", Members: []string{"A", "B"}}, schema.Type{Name: "A"}, schema.Type{Name: "B"}),
			wantErr: "no discriminator",
		},
		{
			name:    "shared discriminator value",
			api:     fakeAPI(schema.Type{Name: "U", Members: []string{"A", "B"}, Discriminator: "type"}, tagged("A", "x"), tagged("B", "x")),
			wantErr: `share the "type" value "x"`,
		},
		{
			name:    "member that is not an object",
			api:     fakeAPI(schema.Type{Name: "U", Members: []string{"String"}}),
			wantErr: "member String is not a generated object",
		},
		{
			name: "required cycle",
			api: fakeAPI(
				schema.Type{Name: "A", Fields: []schema.Field{{Name: "b", Types: []string{"B"}, Required: true}}},
				schema.Type{Name: "B", Fields: []schema.Field{{Name: "a", Types: []string{"A"}, Required: true}}},
			),
			wantErr: "cycle",
		},
		{
			name:    "unknown field type",
			api:     fakeAPI(schema.Type{Name: "A", Fields: []schema.Field{{Name: "x", Types: []string{"Mystery"}}}}),
			wantErr: "A.x: unknown type",
		},
		{
			name: "received synthetic union",
			api: fakeAPI(schema.Type{Name: "A", Fields: []schema.Field{
				{Name: "reply_markup", Types: []string{"InlineKeyboardMarkup", "ReplyKeyboardMarkup", "ReplyKeyboardRemove", "ForceReply"}},
			}}),
			wantErr: "union ReplyMarkup cannot be decoded",
		},
		{
			name: "duplicate Go field names",
			api: fakeAPI(schema.Type{Name: "A", Fields: []schema.Field{
				{Name: "url", Types: str}, {Name: "u_r_l", Types: str},
			}}),
			wantErr: "two fields are named URL",
		},
		{
			name:    "unknown method result",
			api:     withMethod(fakeAPI(schema.Type{Name: "A"}), schema.Method{Name: "doIt", Returns: []string{"Boolean"}}),
			wantErr: `method doIt: no rule for the result "Boolean"`,
		},
		{
			name:    "unknown parameter type",
			api:     withMethod(fakeAPI(schema.Type{Name: "A"}), schema.Method{Name: "doIt", Returns: []string{"True"}, Params: []schema.Field{{Name: "x", Types: []string{"Mystery"}}}}),
			wantErr: "DoItParams.x: unknown type",
		},
		{
			name:    "parameters named like a Bot API type",
			api:     withMethod(fakeAPI(schema.Type{Name: "A"}, schema.Type{Name: "DoItParams"}), schema.Method{Name: "doIt", Returns: []string{"True"}}),
			wantErr: "DoItParams collides",
		},
		{
			name:    "synthetic name taken by the Bot API",
			api:     fakeAPI(schema.Type{Name: "ReplyMarkup"}),
			wantErr: "collides",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newModel(tt.api)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("newModel() error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestFileCarriers(t *testing.T) {
	m, err := newModel(loadAPI(t))
	if err != nil {
		t.Fatal(err)
	}
	var params []string
	for _, meth := range m.methods {
		if meth.params != nil && meth.params.carrier {
			params = append(params, meth.api)
		}
	}
	if len(params) != 33 {
		t.Errorf("%d methods can upload files, want 33: %v", len(params), params)
	}
	for _, name := range []string{"sendPhoto", "sendMediaGroup", "createNewStickerSet", "answerInlineQuery", "setMyProfilePhoto"} {
		if !slices.Contains(params, name) {
			t.Errorf("method %s cannot upload files", name)
		}
	}
	carriers := map[string]bool{}
	for _, o := range m.objects {
		carriers[o.name] = o.carrier
	}
	for name, want := range map[string]bool{"InputMediaPhoto": true, "InputSticker": true, "InputProfilePhotoStatic": true, "User": false, "Message": false} {
		if carriers[name] != want {
			t.Errorf("type %s can hold files = %v, want %v", name, carriers[name], want)
		}
	}

	file := []string{"InputFile"}
	tests := []struct {
		name    string
		api     *schema.API
		want    []string
		wantErr string
	}{
		{
			name: "through types that refer to each other",
			api: withMethod(fakeAPI(
				schema.Type{Name: "A", Fields: []schema.Field{{Name: "b", Types: []string{"B"}}}},
				schema.Type{Name: "B", Fields: []schema.Field{{Name: "a", Types: []string{"A"}}, {Name: "c", Types: []string{"Array of C"}}}},
				schema.Type{Name: "C", Fields: []schema.Field{{Name: "f", Types: file}}},
				schema.Type{Name: "InputFile"},
			), schema.Method{Name: "send", Returns: []string{"True"}, Params: []schema.Field{{Name: "a", Types: []string{"A"}}}}),
			want: []string{"A", "B", "C", "SendParams"},
		},
		{
			name: "nested arrays",
			api: withMethod(fakeAPI(schema.Type{Name: "C", Fields: []schema.Field{{Name: "f", Types: file}}}, schema.Type{Name: "InputFile"}),
				schema.Method{Name: "send", Returns: []string{"True"}, Params: []schema.Field{{Name: "grid", Types: []string{"Array of Array of C"}}}}),
			wantErr: "nested arrays that hold files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := newModel(tt.api)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("newModel() error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, o := range m.objects {
				if o.carrier {
					got = append(got, o.name)
				}
			}
			for _, meth := range m.methods {
				if meth.params != nil && meth.params.carrier {
					got = append(got, meth.params.name)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("carriers = %v, want %v", got, tt.want)
			}
		})
	}
}
