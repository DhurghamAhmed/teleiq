package main

import (
	"fmt"
	"strings"
)

// markFileCarriers finds the parameters and types that can hold uploads.
func (m *model) markFileCarriers() error {
	objects := append([]*object(nil), m.objects...)
	byName := map[string]*object{}
	for _, o := range m.objects {
		byName[o.name] = o
	}
	for _, meth := range m.methods {
		if meth.params != nil {
			objects = append(objects, meth.params)
		}
	}
	carries := func(f field) bool {
		base := strings.TrimLeft(f.typ, "[]")
		if base == "InputFile" {
			return true
		}
		if u := m.unions[base]; u != nil {
			for _, mem := range u.members {
				if o := byName[mem.name]; o != nil && o.carrier {
					return true
				}
			}
			return false
		}
		return byName[base] != nil && byName[base].carrier
	}
	// Types can refer to each other, so repeat until nothing changes.
	for changed := true; changed; {
		changed = false
		for _, o := range objects {
			if o.carrier {
				continue
			}
			for _, f := range o.fields {
				if carries(f) {
					o.carrier, changed = true, true
					break
				}
			}
		}
	}
	for _, o := range objects {
		if !o.carrier {
			continue
		}
		for _, f := range o.fields {
			if !carries(f) {
				continue
			}
			if strings.HasPrefix(f.typ, "[][]") {
				return fmt.Errorf("%s.%s: nested arrays that hold files are not supported", o.name, f.json)
			}
			o.files = append(o.files, f)
		}
	}
	return nil
}

// renderCollectFiles writes the method that lists the uploads of the parameters o.
func renderCollectFiles(s *source, o *object) {
	s.imports[uploadPath] = true
	s.line("")
	s.line("func (v %s) collectFiles(c *upload.Set) {", o.name)
	renderFileFields(s, o, "collectFrom", "\t")
	s.line("}")
}

// renderCollectModel writes collectModel, which lists the uploads of models values.
func renderCollectModel(m *model) *source {
	s := newSource()
	s.imports[uploadPath] = true
	s.doc("", "collectModel adds to c the uploads of v, a value of package models that can hold files.")
	var carriers []*object
	for _, o := range m.objects {
		if o.carrier {
			carriers = append(carriers, o)
		}
	}
	if len(carriers) == 0 {
		s.line("func collectModel(*upload.Set, any) {}")
		return s
	}
	s.line("func collectModel(c *upload.Set, v any) {")
	s.line("\tswitch v := v.(type) {")
	for _, o := range carriers {
		s.line("\tcase *%s:", s.qualify(o.name))
		s.line("\t\tif v == nil {")
		s.line("\t\t\treturn")
		s.line("\t\t}")
		renderFileFields(s, o, "collectModel", "\t\t")
	}
	s.line("\t}")
	s.line("}")
	return s
}

// renderFileFields writes the statements that list the uploads of the fields of o.
func renderFileFields(s *source, o *object, collect, indent string) {
	for _, f := range o.files {
		switch {
		case f.typ == "InputFile" && o.params:
			s.line("%sc.Field(%q, v.%s)", indent, f.json, f.name)
		case f.typ == "InputFile":
			s.line("%sc.Attach(v.%s)", indent, f.name)
		case f.kind == kindUnion:
			s.line("%s%s(c, v.%s)", indent, collect, f.name)
		case f.kind == kindSlice && f.union != "":
			s.line("%sfor _, x := range v.%s {", indent, f.name)
			s.line("%s\t%s(c, x)", indent, collect)
			s.line("%s}", indent)
		case f.kind == kindSlice:
			s.line("%sfor i := range v.%s {", indent, f.name)
			s.line("%s\t%s(c, &v.%s[i])", indent, collect, f.name)
			s.line("%s}", indent)
		case f.required:
			s.line("%s%s(c, &v.%s)", indent, collect, f.name)
		default:
			s.line("%s%s(c, v.%s)", indent, collect, f.name)
		}
	}
}
