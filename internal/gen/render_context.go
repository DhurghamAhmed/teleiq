package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
)

// contextFile is the file of package bot that holds the send methods of bot.Context.
const contextFile = "bot/context_send.gen.go"

const (
	modulePath = "github.com/DhurghamAhmed/teleiq"
	modelsPath = modulePath + "/models"
	uploadPath = modulePath + "/internal/upload"
)

// destinationSlot is a parameter, besides chat_id, that says where a message goes.
type destinationSlot struct {
	json, typ string
	required  bool
	key       string
	words     string // what the parameter is, for the documentation
}

var destinationSlots = []destinationSlot{
	{json: "message_thread_id", typ: "int64", key: "thread", words: "forum topic"},
	{json: "business_connection_id", typ: "string", key: "business", words: "business connection"},
	{json: "business_connection_id", typ: "string", required: true, key: "businessID", words: "business connection"},
	{json: "direct_messages_topic_id", typ: "int64", key: "topic", words: "direct messages topic"},
}

// contextSend is a method that bot.Context offers to send to the chat of the update.
type contextSend struct {
	meth  *method
	chat  string // the Go name of chat_id
	slots []filledSlot
}

// filledSlot is a destinationSlot of a method, with the Go name of its parameter.
type filledSlot struct {
	destinationSlot
	name string
}

// contextSends returns the send methods whose chat_id is a required ChatID.
func contextSends(methods []*method) ([]contextSend, error) {
	var sends []contextSend
	for _, meth := range methods {
		if !strings.HasPrefix(meth.api, "send") || meth.params == nil {
			continue
		}
		fields := meth.params.fields
		i := slices.IndexFunc(fields, func(f field) bool { return f.json == "chat_id" })
		if i < 0 || fields[i].typ != "ChatID" || !fields[i].required {
			continue
		}
		cs := contextSend{meth: meth, chat: fields[i].name}
		for _, slot := range destinationSlots {
			for _, f := range fields {
				if f.json == slot.json && f.typ == slot.typ && f.required == slot.required {
					cs.slots = append(cs.slots, filledSlot{slot, f.name})
				}
			}
		}
		for _, f := range fields {
			known := slices.ContainsFunc(cs.slots, func(s filledSlot) bool { return s.name == f.name })
			if !known && slices.ContainsFunc(destinationSlots, func(s destinationSlot) bool { return s.json == f.json }) {
				return nil, fmt.Errorf("method %s: no rule to fill %s of the type %s", meth.api, f.json, f.goType())
			}
		}
		sends = append(sends, cs)
	}
	return sends, nil
}

// renderContextSends writes the bot.Context methods that send to the update's chat.
func renderContextSends(sends []contextSend) *source {
	s := newSource()
	s.pkg = "bot"
	s.imports["context"] = true
	s.imports[modulePath] = true
	s.imports[modelsPath] = true
	for _, cs := range sends {
		meth := cs.meth
		fields := []string{"chat: &p." + cs.chat}
		for _, slot := range cs.slots {
			fields = append(fields, slot.key+": &p."+slot.name)
		}
		zero := ""
		if meth.result != resultTrue {
			zero = meth.zero() + ", "
		}
		s.line("")
		s.doc("", contextSendDoc(cs))
		s.line("func (c *Context) %s(ctx context.Context, p teleiq.%s) %s {", meth.name, meth.params.name, meth.results("models."))
		s.line("\tif err := c.fillDestination(%q, destinationFields{%s}); err != nil {", meth.name, strings.Join(fields, ", "))
		s.line("\t\treturn %serr", zero)
		s.line("\t}")
		s.line("\treturn c.bot.client.%s(ctx, p)", meth.name)
		s.line("}")
	}
	return s
}

func contextSendDoc(cs contextSend) string {
	doc := fmt.Sprintf("%s calls Client.%s with p.ChatID set to the chat of the update.", cs.meth.name, cs.meth.name)
	var words []string
	for _, slot := range cs.slots {
		words = append(words, slot.words)
	}
	switch len(words) {
	case 0:
	case 1:
		doc += fmt.Sprintf(" Like Reply, it answers in the same %s as the message of the update, unless p sets it.", words[0])
	default:
		list := strings.Join(words[:len(words)-1], ", ") + " or " + words[len(words)-1]
		doc += fmt.Sprintf(" Like Reply, it answers in the same %s as the message of the update, unless p sets them.", list)
	}
	return doc + " It fails when the update has no chat."
}

// checkContextNames fails when dir hand-writes a generated bot.Context method.
func checkContextNames(dir string, generated []byte) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, contextFile, generated, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	ours := map[string]bool{}
	for _, name := range contextMethods(f) {
		ours[name] = true
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == filepath.Base(contextFile) {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, name := range contextMethods(f) {
			if ours[name] {
				return fmt.Errorf("%s: Context.%s is written by hand but also generated in %s; rename the handwritten method", path, name, contextFile)
			}
		}
	}
	return nil
}

// contextMethods returns the names of the methods on Context that f declares.
func contextMethods(f *ast.File) []string {
	var names []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		t := fn.Recv.List[0].Type
		if star, ok := t.(*ast.StarExpr); ok {
			t = star.X
		}
		if id, ok := t.(*ast.Ident); ok && id.Name == "Context" {
			names = append(names, fn.Name.Name)
		}
	}
	return names
}
