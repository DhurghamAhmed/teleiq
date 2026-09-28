package main

import (
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

func TestRenderApplyDefaults(t *testing.T) {
	text := schema.Field{Name: "text", Types: []string{"String"}, Required: true}
	api := fakeAPI(schema.Type{Name: "MessageEntity"})
	api = withMethod(api, schema.Method{Name: "sendText", Returns: []string{"True"}, Params: []schema.Field{
		text, {Name: "parse_mode", Types: []string{"String"}}, {Name: "entities", Types: []string{"Array of MessageEntity"}},
		{Name: "link_preview_options", Types: []string{"String"}}, {Name: "disable_notification", Types: []string{"Boolean"}},
	}})
	api = withMethod(api, schema.Method{Name: "sendCaption", Returns: []string{"True"}, Params: []schema.Field{
		{Name: "parse_mode", Types: []string{"String"}}, {Name: "caption_entities", Types: []string{"Array of MessageEntity"}},
	}})
	api = withMethod(api, schema.Method{Name: "forwardIt", Returns: []string{"True"}, Params: []schema.Field{
		text, {Name: "protect_content", Types: []string{"Boolean"}},
	}})
	api = withMethod(api, schema.Method{Name: "doOther", Returns: []string{"True"}, Params: []schema.Field{text}})
	m, err := newModel(api)
	if err != nil {
		t.Fatal(err)
	}
	files, err := generate(m)
	if err != nil {
		t.Fatal(err)
	}
	params, methods := string(files[paramsFile]), string(files[methodsFile])
	for _, want := range []string{
		"func (p *SendTextParams) applyDefaults(d *Defaults) {\n" +
			"\tif p.ParseMode == nil && d.ParseMode != \"\" && len(p.Entities) == 0 {\n\t\tp.ParseMode = Ptr(d.ParseMode)\n\t}\n" +
			"\tif p.LinkPreviewOptions == nil {\n\t\tp.LinkPreviewOptions = d.LinkPreviewOptions\n\t}\n" +
			"\tif p.DisableNotification == nil && d.DisableNotification {\n\t\tp.DisableNotification = Ptr(true)\n\t}\n}",
		"\tif p.ParseMode == nil && d.ParseMode != \"\" && len(p.CaptionEntities) == 0 {",
		"func (p *ForwardItParams) applyDefaults(d *Defaults) {\n\tif p.ProtectContent == nil && d.ProtectContent {",
	} {
		if !strings.Contains(params, want) {
			t.Errorf("%s does not have\n%s", paramsFile, want)
		}
	}
	if strings.Contains(params, "DoOtherParams) applyDefaults") || strings.Contains(methods, "func (c *Client) DoOther(ctx context.Context, params DoOtherParams) error {\n\tif c.defaults") {
		t.Errorf("a method without these parameters applies defaults")
	}
	if !strings.Contains(methods, "func (c *Client) SendText(ctx context.Context, params SendTextParams) error {\n\tif c.defaults != nil {\n\t\tparams.applyDefaults(c.defaults)\n\t}\n") {
		t.Errorf("SendText does not apply the defaults before its call:\n%s", methods)
	}
}
