package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

// handwritten lists the Bot API types that package models defines by hand.
var handwritten = map[string]bool{"InputFile": true}

// synthetic names the unions that the Bot API uses without naming them.
var synthetic = []struct {
	name, doc string
	members   []string
}{
	{
		name:    "ReplyMarkup",
		doc:     "ReplyMarkup is an inline keyboard, a custom reply keyboard, an instruction to remove the reply keyboard or a request for a reply, sent with a message.",
		members: []string{"InlineKeyboardMarkup", "ReplyKeyboardMarkup", "ReplyKeyboardRemove", "ForceReply"},
	},
	{
		name:    "InputMediaGroupItem",
		doc:     "InputMediaGroupItem is one message of an album sent with sendMediaGroup.",
		members: []string{"InputMediaAudio", "InputMediaDocument", "InputMediaLivePhoto", "InputMediaPhoto", "InputMediaVideo"},
	},
	{
		name:    "InputRichMedia",
		doc:     "InputRichMedia is the media of an element embedded in an outgoing rich message.",
		members: []string{"InputMediaAnimation", "InputMediaAudio", "InputMediaDocument", "InputMediaPhoto", "InputMediaVideo", "InputMediaVoiceNote"},
	},
}

// valueMembers names the union members that are not objects, with their Go types.
var valueMembers = map[string]map[string]member{
	"RichText": {
		"String":            {name: "RichTextString", under: "string", doc: "RichTextString is plain text in a RichText."},
		"Array of RichText": {name: "RichTextArray", under: "[]RichText", doc: "RichTextArray is a sequence of RichText parts."},
	},
}

type object struct {
	name   string
	doc    string
	fields []field // without the constants
	consts []field
	decode bool // it is received from the Bot API and has union fields

	params  bool    // it holds the parameters of a method
	carrier bool    // it can hold uploads
	files   []field // the fields that can hold uploads
	chat    string  // the field of the target chat of a method, if any
}

type union struct {
	name, doc     string
	members       []member
	discriminator string
	decode        bool // it is received from the Bot API
	synthetic     bool
}

type member struct {
	name  string
	tag   string // the discriminator value
	under string // the underlying type of a member that is not an object
	doc   string
}

type model struct {
	version string
	types   map[string]*schema.Type
	unions  map[string]*union
	objects []*object
	order   []*union
	methods []*method
	page    []string // the names of the Bot API types, in the order of the page
}

func newModel(api *schema.API) (*model, error) {
	m := &model{version: api.Version, types: map[string]*schema.Type{}, unions: map[string]*union{}}
	for i := range api.Types {
		t := &api.Types[i]
		m.types[t.Name] = t
		m.page = append(m.page, t.Name)
		if len(t.Members) > 0 {
			u := &union{name: t.Name, doc: typeDoc(t.Name, t.Description), discriminator: t.Discriminator}
			m.unions[t.Name], m.order = u, append(m.order, u)
		}
	}
	for _, s := range synthetic {
		if m.types[s.name] != nil || handwritten[s.name] {
			return nil, fmt.Errorf("synthetic union %s collides with a Bot API type", s.name)
		}
		u := &union{name: s.name, doc: s.doc, synthetic: true}
		m.unions[s.name], m.order = u, append(m.order, u)
	}
	received := m.receivedTypes(api)
	if err := m.buildUnions(api, received); err != nil {
		return nil, err
	}
	for _, t := range api.Types {
		if len(t.Members) > 0 || handwritten[t.Name] {
			continue
		}
		o, err := m.buildObject(t, received[t.Name])
		if err != nil {
			return nil, err
		}
		m.objects = append(m.objects, o)
	}
	if err := m.checkRequiredCycles(); err != nil {
		return nil, err
	}
	if err := m.buildMethods(api); err != nil {
		return nil, err
	}
	return m, m.markFileCarriers()
}

func (m *model) syntheticFor(members []string) *union {
	for _, s := range synthetic {
		if slices.Equal(s.members, members) {
			return m.unions[s.name]
		}
	}
	return nil
}

// receivedTypes returns the types reachable from method results.
func (m *model) receivedTypes(api *schema.API) map[string]bool {
	seen := map[string]bool{}
	var visit func(expr string)
	visit = func(expr string) {
		_, base := unwrapArray(expr)
		t := m.types[base]
		if t == nil || seen[base] {
			return
		}
		seen[base] = true
		for _, f := range t.Fields {
			for _, e := range f.Types {
				visit(e)
			}
		}
		for _, e := range t.Members {
			visit(e)
		}
	}
	for _, meth := range api.Methods {
		for _, r := range meth.Returns {
			visit(r)
		}
	}
	return seen
}

func (m *model) buildUnions(api *schema.API, received map[string]bool) error {
	for _, t := range api.Types {
		if len(t.Members) == 0 {
			continue
		}
		u := m.unions[t.Name]
		u.decode = received[t.Name]
		for _, name := range t.Members {
			if v, ok := valueMembers[t.Name][name]; ok {
				u.members = append(u.members, v)
				continue
			}
			mt := m.types[name]
			if mt == nil || handwritten[name] || len(mt.Members) > 0 {
				return fmt.Errorf("union %s: member %s is not a generated object", t.Name, name)
			}
			mem := member{name: name}
			if len(mt.Fields) > 0 && mt.Fields[0].Name == u.discriminator {
				mem.tag = mt.Fields[0].Const
			}
			u.members = append(u.members, mem)
		}
		if u.decode {
			if err := checkDecodable(u); err != nil {
				return err
			}
		}
	}
	for _, s := range synthetic {
		u := m.unions[s.name]
		for _, name := range s.members {
			if m.types[name] == nil {
				return fmt.Errorf("synthetic union %s: unknown member %s", s.name, name)
			}
			u.members = append(u.members, member{name: name})
		}
	}
	return nil
}

// decodeRules lists the received unions that have no discriminator field.
var decodeRules = map[string]bool{"MaybeInaccessibleMessage": true}

func checkDecodable(u *union) error {
	if decodeRules[u.name] {
		return nil
	}
	if u.discriminator == "" {
		return fmt.Errorf("union %s is received but has no discriminator and no decoding rule", u.name)
	}
	tags := map[string]string{}
	for _, mem := range u.members {
		if mem.under != "" {
			continue
		}
		if mem.tag == "" {
			return fmt.Errorf("union %s: member %s has no %q value", u.name, mem.name, u.discriminator)
		}
		if other, dup := tags[mem.tag]; dup {
			return fmt.Errorf("union %s: members %s and %s share the %q value %q", u.name, other, mem.name, u.discriminator, mem.tag)
		}
		tags[mem.tag] = mem.name
	}
	return nil
}

func (m *model) buildObject(t schema.Type, received bool) (*object, error) {
	o := &object{name: t.Name, doc: typeDoc(t.Name, t.Description)}
	names := map[string]bool{}
	for _, sf := range t.Fields {
		f, err := m.resolveField(t.Name, sf)
		if err != nil {
			return nil, err
		}
		if names[f.name] {
			return nil, fmt.Errorf("%s: two fields are named %s", t.Name, f.name)
		}
		names[f.name] = true
		if f.constant != "" {
			o.consts = append(o.consts, f)
			continue
		}
		if f.union != "" && received {
			if strings.HasPrefix(f.typ, "[][]") {
				return nil, fmt.Errorf("%s.%s: nested arrays of unions cannot be decoded", t.Name, f.json)
			}
			if !m.unions[f.union].decode {
				return nil, fmt.Errorf("%s.%s: union %s cannot be decoded", t.Name, f.json, f.union)
			}
			o.decode = true
		}
		o.fields = append(o.fields, f)
	}
	return o, nil
}

// checkRequiredCycles rejects required object fields that contain themselves.
func (m *model) checkRequiredCycles() error {
	byName := map[string]*object{}
	for _, o := range m.objects {
		byName[o.name] = o
	}
	state := map[string]int{} // 1: visiting, 2: done
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("required fields form a cycle: %v", append(path, name))
		case 2:
			return nil
		}
		if byName[name] == nil {
			return nil
		}
		state[name] = 1
		for _, f := range byName[name].fields {
			if f.required && f.kind == kindObject {
				if err := visit(f.typ, append(path, name)); err != nil {
					return err
				}
			}
		}
		state[name] = 2
		return nil
	}
	for _, o := range m.objects {
		if err := visit(o.name, nil); err != nil {
			return err
		}
	}
	return nil
}
