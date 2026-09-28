package main

import (
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

func TestSectionFile(t *testing.T) {
	tests := []struct{ section, want, wantErr string }{
		{section: "Available types", want: "models/available.gen.go"},
		{section: "Telegram Passport", want: "models/passport.gen.go"},
		{section: "Getting updates", want: "models/updates.gen.go"},
		{section: "A section Telegram added", wantErr: `"A section Telegram added"`},
		{section: "", wantErr: "add it to sectionFiles"},
	}
	for _, tt := range tests {
		t.Run(tt.section, func(t *testing.T) {
			got, err := sectionFile(tt.section)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("sectionFile() = %q, %v; want an error containing %s", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("sectionFile() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestRenderSections(t *testing.T) {
	file := schema.Field{Name: "photo", Types: []string{"InputFile"}, Required: true}
	api := fakeAPI(
		tagged("Zebra", "z"),
		schema.Type{Name: "Pick", Members: []string{"Zebra", "Apple"}, Discriminator: "type"},
		tagged("Apple", "a"),
		schema.Type{Name: "Media", Fields: []schema.Field{file}},
		schema.Type{Name: "Sticker", Section: "Stickers"},
	)
	api = withMethod(api, schema.Method{Name: "getSticker", Returns: []string{"Sticker"}, Section: "Stickers"})
	api = withMethod(api, schema.Method{Name: "getPick", Returns: []string{"Pick"}})
	api = withMethod(api, schema.Method{Name: "sendMedia", Returns: []string{"True"}, Params: []schema.Field{
		{Name: "media", Types: []string{"Media"}, Required: true},
		{Name: "document", Types: []string{"InputFile"}},
	}})
	m, err := newModel(api)
	if err != nil {
		t.Fatal(err)
	}
	files, err := generate(m)
	if err != nil {
		t.Fatal(err)
	}
	available := string(files["models/available.gen.go"])
	if !strings.Contains(available, "\npackage models\n") {
		t.Error("models/available.gen.go is not in package models")
	}
	var at []int
	for _, decl := range []string{"type Zebra struct", "type Pick interface", "type Apple struct", "type ReplyMarkup interface"} {
		at = append(at, strings.Index(available, "\n"+decl))
	}
	for i := range at {
		if at[i] < 0 || i > 0 && at[i] < at[i-1] {
			t.Fatalf("models/available.gen.go declares Zebra, Pick, Apple and then the synthetic ReplyMarkup at %v, want them all in that order", at)
		}
	}
	for name, decls := range map[string][]string{
		"models/available.gen.go": {"\nfunc DecodePick(data []byte) (Pick, error) { return decodePick(data) }"},
		"models/stickers.gen.go":  {"\ntype Sticker struct"},
		methodsFile: {
			"\npackage teleiq\n",
			"\nfunc (c *Client) GetSticker(ctx context.Context) (*models.Sticker, error) {",
			"\tvar result models.Sticker\n",
			"return decodeUnion(\"getPick\", raw, models.DecodePick)",
			"\nfunc (c *Client) SendMedia(ctx context.Context, params SendMediaParams) error {",
		},
		paramsFile: {
			"\tMedia models.Media `json:\"media\"`",
			"\tDocument models.InputFile `json:\"document,omitzero\"`",
			"func (v SendMediaParams) collectFiles(c *upload.Set) {\n\tcollectFrom(c, &v.Media)\n\tc.Field(\"document\", v.Document)\n}",
			"\tcase *models.Media:\n\t\tif v == nil {\n\t\t\treturn\n\t\t}\n\t\tc.Attach(v.Photo)\n",
			"\"github.com/DhurghamAhmed/teleiq/internal/upload\"",
		},
	} {
		// gofmt aligns fields, so spaces are compared as one.
		norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
		for _, decl := range decls {
			if !strings.Contains(norm(string(files[name])), norm(decl)) {
				t.Errorf("%s does not hold %q", name, decl)
			}
		}
	}
	if strings.Contains(available, "Sticker") || strings.Contains(available, "DecodeReplyMarkup") || strings.Contains(available, "collectFiles") {
		t.Error("models/available.gen.go holds a type of the Stickers section, a decoding that no method needs, or the uploads of a type")
	}
	if strings.Contains(string(files[methodsFile]), "type SendMediaParams") {
		t.Errorf("%s holds the parameters, which belong in %s", methodsFile, paramsFile)
	}

	api.Types = append(api.Types, schema.Type{Name: "Novelty", Section: "A section Telegram added"})
	if m, err = newModel(api); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(m); err == nil || !strings.Contains(err.Error(), "type Novelty") || !strings.Contains(err.Error(), "sectionFiles") {
		t.Errorf("generate() with a new section = %v, want an error naming the type and sectionFiles", err)
	}
}
