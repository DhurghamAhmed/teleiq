package main

import (
	"fmt"
	"strings"
)

// renderObject writes a type of package models or the parameters of a method.
func renderObject(o *object) *source {
	s := newSource()
	s.doc("", o.doc)
	if len(o.fields) == 0 {
		s.line("type %s struct{}", o.name)
	} else {
		s.line("type %s struct {", o.name)
		for _, f := range o.fields {
			if f.doc != "" {
				s.doc("\t", f.doc)
			}
			typ := f.goType()
			if o.params {
				typ = s.qualify(typ)
			}
			s.line("\t%s %s %s", f.name, typ, f.tag())
		}
		s.line("}")
	}
	if len(o.consts) > 0 {
		renderConstMarshal(s, o)
	}
	if o.decode {
		renderDecode(s, o)
	}
	// collectModel in package teleiq finds the uploads of models values.
	if o.carrier && o.params {
		renderCollectFiles(s, o)
	}
	if o.chat != "" {
		s.line("")
		s.line("func (v %s) targetChat() %s { return v.%s }", o.name, s.qualify("ChatID"), o.chat)
	}
	if o.params {
		renderApplyDefaults(s, o)
	}
	return s
}

// renderConstMarshal writes a MarshalJSON that adds the single-value fields.
func renderConstMarshal(s *source, o *object) {
	s.imports["encoding/json"] = true
	var set []string
	for _, c := range o.consts {
		set = append(set, fmt.Sprintf("%q set to %s", c.json, c.constant))
	}
	s.line("")
	s.doc("", fmt.Sprintf("MarshalJSON encodes v with %s.", strings.Join(set, " and ")))
	s.line("func (v %s) MarshalJSON() ([]byte, error) {", o.name)
	s.line("\ttype fields %s", o.name)
	s.line("\treturn json.Marshal(struct {")
	var values []string
	for _, c := range o.consts {
		typ := "string"
		if c.constant == "true" {
			typ = "bool"
		}
		s.line("\t\t%s %s `json:%q`", c.name, typ, c.json)
		values = append(values, c.constant)
	}
	s.line("\t\tfields")
	s.line("\t}{%s, fields(v)})", strings.Join(values, ", "))
	s.line("}")
}

// renderDecode writes an UnmarshalJSON that picks the type of each union field.
func renderDecode(s *source, o *object) {
	s.imports["encoding/json"] = true
	s.line("")
	s.doc("", "UnmarshalJSON decodes v, choosing the concrete type of each field that can hold several types.")
	s.line("func (v *%s) UnmarshalJSON(data []byte) error {", o.name)
	s.line("\ttype fields %s", o.name)
	s.line("\tvar raw struct {")
	s.line("\t\t*fields")
	var unions []field
	for _, f := range o.fields {
		if f.union == "" {
			continue
		}
		unions = append(unions, f)
		typ := "json.RawMessage"
		if f.kind == kindSlice {
			typ = "[]json.RawMessage"
		}
		s.line("\t\t%s %s `json:%q`", f.name, typ, f.json)
	}
	s.line("\t}")
	s.line("\traw.fields = (*fields)(v)")
	s.line("\tif err := json.Unmarshal(data, &raw); err != nil {")
	s.line("\t\treturn err")
	s.line("\t}")
	s.line("\tvar err error")
	for _, f := range unions {
		decode := "decode" + f.union + "(raw." + f.name + ")"
		if f.kind == kindSlice {
			decode = "decodeEach(raw." + f.name + ", decode" + f.union + ")"
		}
		s.line("\tif v.%s, err = %s; err != nil {", f.name, decode)
		s.line("\t\treturn err")
		s.line("\t}")
	}
	s.line("\treturn nil")
	s.line("}")
}
