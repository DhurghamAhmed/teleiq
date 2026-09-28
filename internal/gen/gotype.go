package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

type kind int

const (
	kindScalar kind = iota // string, bool, int, int64 or float64
	kindObject             // a struct type
	kindSlice              // any array
	kindUnion              // an interface type
	kindOpaque             // ChatID or InputFile: the zero value means unset
	kindFlag               // an optional True
)

type field struct {
	json     string
	name     string
	typ      string // the Go type, without the pointer of an optional field
	kind     kind
	required bool
	doc      string
	union    string // the union held by a union field or a slice of unions
	constant string // the JSON value of a field that has a single valid value
}

// goType returns the type of the struct field.
func (f field) goType() string {
	if !f.required && (f.kind == kindScalar || f.kind == kindObject) {
		return "*" + f.typ
	}
	return f.typ
}

// tag returns the struct tag of the field.
func (f field) tag() string {
	opt := ""
	switch {
	case f.required:
	// omitzero keeps an empty slice as [], which Telegram tells apart from none.
	case f.kind == kindOpaque, f.kind == kindSlice:
		opt = ",omitzero"
	default:
		opt = ",omitempty"
	}
	return fmt.Sprintf("`json:\"%s%s\"`", f.json, opt)
}

var wideInteger = regexp.MustCompile(`(?i)\bidentifier\b|Unix time|significant bits`)

// intType returns int64 for identifiers, dates and file sizes, and int otherwise.
func intType(f schema.Field) string {
	n := f.Name
	switch {
	case n == "id", strings.HasSuffix(n, "_id"), strings.HasSuffix(n, "_ids"),
		n == "date", strings.HasSuffix(n, "_date"), n == "file_size",
		wideInteger.MatchString(f.Description):
		return "int64"
	}
	return "int"
}

func (m *model) resolveField(owner string, f schema.Field) (field, error) {
	fd := field{json: f.Name, name: goName(f.Name), required: f.Required, doc: firstSentence(f.Description)}
	switch {
	case f.Const != "":
		v, err := json.Marshal(f.Const)
		fd.constant = string(v)
		return fd, err
	case f.Required && slices.Equal(f.Types, []string{"True"}):
		fd.constant = "true"
		return fd, nil
	}
	var err error
	fd.typ, fd.kind, fd.union, err = m.resolveTypes(f)
	if err != nil {
		return fd, fmt.Errorf("%s.%s: %w", owner, f.Name, err)
	}
	return fd, nil
}

func (m *model) resolveTypes(f schema.Field) (string, kind, string, error) {
	switch {
	case len(f.Types) == 1:
		return m.resolveOne(f.Types[0], f)
	case slices.Equal(f.Types, []string{"Integer", "String"}):
		return "ChatID", kindOpaque, "", nil
	case slices.Equal(f.Types, []string{"InputFile", "String"}):
		return "InputFile", kindOpaque, "", nil
	}
	depth, members := -1, make([]string, 0, len(f.Types))
	for _, t := range f.Types {
		d, base := unwrapArray(t)
		if depth != -1 && d != depth {
			return "", 0, "", fmt.Errorf("mixed array depths in %q", f.Types)
		}
		depth, members = d, append(members, base)
	}
	u := m.syntheticFor(members)
	if u == nil {
		return "", 0, "", fmt.Errorf("no rule for the type %q", strings.Join(f.Types, " or "))
	}
	if depth > 0 {
		return strings.Repeat("[]", depth) + u.name, kindSlice, u.name, nil
	}
	return u.name, kindUnion, u.name, nil
}

func (m *model) resolveOne(expr string, f schema.Field) (string, kind, string, error) {
	depth, base := unwrapArray(expr)
	var typ, union string
	k := kindScalar
	switch base {
	case "Integer":
		typ = intType(f)
	case "String":
		typ = "string"
		if strings.Contains(f.Description, "attach://") {
			typ, k = "InputFile", kindOpaque
		}
	case "Boolean":
		typ = "bool"
	case "True":
		typ, k = "bool", kindFlag
	case "Float":
		typ = "float64"
	case "InputFile":
		typ, k = "InputFile", kindOpaque
	default:
		switch {
		case m.unions[base] != nil:
			typ, k, union = base, kindUnion, base
		case m.types[base] != nil:
			typ, k = base, kindObject
		default:
			return "", 0, "", fmt.Errorf("unknown type %q", expr)
		}
	}
	if depth > 0 {
		return strings.Repeat("[]", depth) + typ, kindSlice, union, nil
	}
	return typ, k, union, nil
}

func unwrapArray(expr string) (int, string) {
	depth := 0
	for {
		rest, ok := strings.CutPrefix(expr, "Array of ")
		if !ok {
			return depth, expr
		}
		expr, depth = rest, depth+1
	}
}
