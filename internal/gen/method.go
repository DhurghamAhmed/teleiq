package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

type resultKind int

const (
	resultTrue   resultKind = iota // only an error
	resultObject                   // a pointer to a struct
	resultValue                    // a slice or a scalar
	resultOrTrue                   // a pointer to a struct, or nil for true
	resultUnion                    // an interface
	resultUnions                   // a slice of an interface
)

type method struct {
	api     string // the Bot API name, such as sendMessage
	name    string
	doc     string
	section string  // the section of the Bot API page
	params  *object // nil when the method takes no parameters
	result  resultKind
	typ     string // the Go type of the result value
	union   string
}

func (m *model) buildMethods(api *schema.API) error {
	for _, sm := range api.Methods {
		meth := &method{api: sm.Name, name: upperFirst(sm.Name), doc: firstSentence(sm.Description), section: sm.Section}
		if m.types[meth.name+"Params"] != nil {
			return fmt.Errorf("%sParams collides with a Bot API type", meth.name)
		}
		if err := m.resolveResult(meth, sm.Returns); err != nil {
			return fmt.Errorf("method %s: %w", sm.Name, err)
		}
		if len(sm.Params) > 0 {
			params, err := m.buildObject(schema.Type{Name: meth.name + "Params", Fields: sm.Params}, false)
			if err != nil {
				return err
			}
			params.doc = fmt.Sprintf("%sParams holds the parameters of %s.", meth.name, meth.name)
			params.params = true
			for _, f := range params.fields {
				if f.json == "chat_id" && f.typ == "ChatID" {
					params.chat = f.name
				}
			}
			meth.params = params
		}
		m.methods = append(m.methods, meth)
	}
	return nil
}

func (m *model) resolveResult(meth *method, returns []string) error {
	if slices.Equal(returns, []string{"True"}) {
		meth.result = resultTrue
		return nil
	}
	if len(returns) == 2 && returns[1] == "True" && m.types[returns[0]] != nil && m.unions[returns[0]] == nil {
		meth.result, meth.typ = resultOrTrue, returns[0]
		return nil
	}
	if len(returns) != 1 {
		return fmt.Errorf("no rule for the result %q", strings.Join(returns, " or "))
	}
	depth, base := unwrapArray(returns[0])
	switch {
	case depth > 1:
		return fmt.Errorf("no rule for the result %q", returns[0])
	case base == "String" && depth == 0:
		meth.result, meth.typ = resultValue, "string"
	case base == "Integer" && depth == 0:
		meth.result, meth.typ = resultValue, "int"
	case m.unions[base] != nil:
		if !m.unions[base].decode {
			return fmt.Errorf("union %s cannot be decoded", base)
		}
		meth.result, meth.typ, meth.union = resultUnion, base, base
		if depth == 1 {
			meth.result, meth.typ = resultUnions, "[]"+base
		}
	case m.types[base] != nil && !handwritten[base]:
		meth.result, meth.typ = resultObject, base
		if depth == 1 {
			meth.result, meth.typ = resultValue, "[]"+base
		}
	default:
		return fmt.Errorf("no rule for the result %q", returns[0])
	}
	return nil
}
