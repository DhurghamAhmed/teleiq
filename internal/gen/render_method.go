package main

import (
	"fmt"
	"strings"
)

// renderMethod writes the method of Client that calls meth.
func renderMethod(meth *method) *source {
	s := newSource()
	s.imports["context"] = true
	doc := meth.doc
	if meth.result == resultOrTrue {
		doc += fmt.Sprintf(" The %s is nil when the Bot API returns True, as it does for inline messages.", meth.typ)
	}
	s.doc("", doc)
	args, params := "ctx context.Context", "nil"
	if meth.params != nil {
		args, params = "ctx context.Context, params "+meth.params.name, "params"
	}
	results := meth.results("models.")
	if strings.Contains(results, "models.") {
		s.imports[modelsPath] = true
	}
	call := fmt.Sprintf("c.Call(ctx, %q, %s, ", meth.api, params)
	s.line("func (c *Client) %s(%s) %s {", meth.name, args, results)
	if hasDefaults(meth.params) {
		s.line("\tif c.defaults != nil {")
		s.line("\t\tparams.applyDefaults(c.defaults)")
		s.line("\t}")
	}
	switch meth.result {
	case resultTrue:
		s.line("\treturn %snil)", call)
	case resultObject:
		s.line("\tvar result %s", s.qualify(meth.typ))
		s.line("\tif err := %s&result); err != nil {", call)
		s.line("\t\treturn nil, err")
		s.line("\t}")
		s.line("\treturn &result, nil")
	case resultValue:
		s.line("\tvar result %s", s.qualify(meth.typ))
		s.line("\tif err := %s&result); err != nil {", call)
		s.line("\t\treturn %s, err", meth.zero())
		s.line("\t}")
		s.line("\treturn result, nil")
	default:
		s.imports["encoding/json"] = true
		var decode string
		switch meth.result {
		case resultOrTrue:
			decode = fmt.Sprintf("decodeOrTrue[%s](%q, raw)", s.qualify(meth.typ), meth.api)
		case resultUnion:
			decode = fmt.Sprintf("decodeUnion(%q, raw, %s)", meth.api, s.qualify("Decode"+meth.union))
		case resultUnions:
			decode = fmt.Sprintf("decodeUnions(%q, raw, %s)", meth.api, s.qualify("Decode"+meth.union))
		}
		s.line("\tvar raw json.RawMessage")
		s.line("\tif err := %s&raw); err != nil {", call)
		s.line("\t\treturn nil, err")
		s.line("\t}")
		s.line("\treturn %s", decode)
	}
	s.line("}")
	return s
}

// results returns the result list of the method's signature, qualified by qual.
func (meth *method) results(qual string) string {
	if meth.result == resultTrue {
		return "error"
	}
	typ := meth.typ
	if base := strings.TrimPrefix(typ, "[]"); base != "string" && base != "int" {
		typ = strings.TrimSuffix(typ, base) + qual + base
	}
	if meth.result == resultObject || meth.result == resultOrTrue {
		typ = "*" + typ
	}
	return "(" + typ + ", error)"
}

// zero returns the zero value of the result value of a method that has one.
func (meth *method) zero() string {
	switch meth.typ {
	case "string":
		return `""`
	case "int":
		return "0"
	}
	return "nil"
}
