package schema

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	methodName  = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	typeName    = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	versionText = regexp.MustCompile(`^Bot API (\d+\.\d+(?:\.\d+)?)$`)
	constText   = regexp.MustCompile(`(?:always “([^”]+)”|must be ([a-z0-9_]+))$`)
	returnToken = regexp.MustCompile(`((?i:array of)\s+)?\x00([^\x00]+)\x00|\b(True|Int|Integer|String)\b`)
	eitherText  = regexp.MustCompile(`(?s)either (.*) or any of the following`)
	linkText    = regexp.MustCompile(`\x00[^\x00]*\x00`)
	typeSplit   = regexp.MustCompile(`,\s*|\s+or\s+|\s+and\s+`)
)

// Parse builds an API description from the official Bot API documentation page.
func Parse(r io.Reader) (*API, error) {
	page, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	entries := scan(tokenize(string(page)))
	api := &API{}
	if err := parseRelease(api, entries); err != nil {
		return nil, err
	}
	anchors := map[string]string{}
	for _, e := range entries {
		if typeName.MatchString(e.name) {
			anchors[strings.ToLower(e.name)] = e.name
		}
	}
	for _, e := range entries {
		switch {
		case typeName.MatchString(e.name):
			t, err := buildType(e, anchors)
			if err != nil {
				return nil, fmt.Errorf("schema: type %s: %w", e.name, err)
			}
			api.Types = append(api.Types, t)
		case methodName.MatchString(e.name):
			m, err := buildMethod(e, anchors)
			if err != nil {
				return nil, fmt.Errorf("schema: method %s: %w", e.name, err)
			}
			api.Methods = append(api.Methods, m)
		}
	}
	if len(api.Methods) == 0 || len(api.Types) == 0 {
		return nil, fmt.Errorf("schema: found %d methods and %d types, want both", len(api.Methods), len(api.Types))
	}
	setDiscriminators(api)
	return api, nil
}

func parseRelease(api *API, entries []*entry) error {
	for _, e := range entries {
		if e.section != "Recent changes" || len(e.paras) == 0 {
			continue
		}
		m := versionText.FindStringSubmatch(collapse(linkText.ReplaceAllString(e.paras[0].text, "")))
		date, err := time.Parse("January 2, 2006", e.name)
		if m == nil || err != nil {
			return fmt.Errorf("schema: cannot read the latest release from %q", e.name)
		}
		api.Version, api.ReleaseDate = m[1], date.Format(time.DateOnly)
		return nil
	}
	return fmt.Errorf("schema: no release found under Recent changes")
}

func buildMethod(e *entry, anchors map[string]string) (Method, error) {
	m := Method{Name: e.name, Section: e.section, Description: description(e), Params: []Field{}}
	if len(e.rows) > 0 {
		if !slices.Equal(e.rows[0], []string{"Parameter", "Type", "Required", "Description"}) {
			return m, fmt.Errorf("unexpected table header %q", e.rows[0])
		}
		for _, r := range e.rows[1:] {
			if len(r) != 4 || (r[2] != "Yes" && r[2] != "Optional") {
				return m, fmt.Errorf("unexpected parameter row %q", r)
			}
			m.Params = append(m.Params, Field{Name: r[0], Types: typeExpr(r[1]), Required: r[2] == "Yes", Description: r[3]})
		}
	}
	m.Returns = returns(e, anchors)
	if len(m.Returns) == 0 {
		return m, fmt.Errorf("no return type found")
	}
	return m, nil
}

func buildType(e *entry, anchors map[string]string) (Type, error) {
	t := Type{Name: e.name, Section: e.section, Description: description(e), Fields: []Field{}}
	if len(e.rows) > 0 {
		if !slices.Equal(e.rows[0], []string{"Field", "Type", "Description"}) {
			return t, fmt.Errorf("unexpected table header %q", e.rows[0])
		}
		for _, r := range e.rows[1:] {
			if len(r) != 3 {
				return t, fmt.Errorf("unexpected field row %q", r)
			}
			desc, optional := strings.CutPrefix(r[2], "Optional. ")
			t.Fields = append(t.Fields, Field{Name: r[0], Types: typeExpr(r[1]), Required: !optional && !strings.HasPrefix(r[2], "Optional"), Description: desc})
		}
	}
	for _, p := range e.paras {
		if m := eitherText.FindStringSubmatch(p.text); m != nil {
			t.Members = append(t.Members, returnTypes(m[1], anchors)...)
		}
	}
	for _, list := range e.lists {
		if len(list) == 0 {
			continue
		}
		name, ok := anchors[list[0]]
		if !ok {
			return t, fmt.Errorf("union member %q is not a known type", list[0])
		}
		t.Members = append(t.Members, name)
	}
	return t, nil
}

func setDiscriminators(api *API) {
	byName := map[string]*Type{}
	for i := range api.Types {
		byName[api.Types[i].Name] = &api.Types[i]
	}
	for i := range api.Types {
		u := &api.Types[i]
		discriminator, complete := "", len(u.Members) > 0
		for _, name := range u.Members {
			member, ok := byName[name]
			if !ok {
				continue
			}
			if len(member.Fields) == 0 {
				complete = false
				continue
			}
			first := &member.Fields[0]
			m := constText.FindStringSubmatch(first.Description)
			if m == nil || (discriminator != "" && first.Name != discriminator) {
				complete = false
				continue
			}
			first.Const = m[1] + m[2]
			discriminator = first.Name
		}
		if complete {
			u.Discriminator = discriminator
		}
	}
}

func returns(e *entry, anchors map[string]string) []string {
	var found []string
	for _, p := range e.paras {
		for _, s := range sentences(p.text) {
			if strings.Contains(strings.ToLower(s), "return") {
				found = append(found, returnTypes(s, anchors)...)
			}
		}
	}
	return uniq(found)
}

func returnTypes(s string, anchors map[string]string) []string {
	var found []string
	for _, m := range returnToken.FindAllStringSubmatch(s, -1) {
		switch {
		case m[2] != "":
			if name, ok := anchors[m[2]]; ok {
				if m[1] != "" {
					name = "Array of " + name
				}
				found = append(found, name)
			}
		case m[3] == "Int":
			found = append(found, "Integer")
		case m[3] != "":
			found = append(found, m[3])
		}
	}
	return uniq(found)
}

func sentences(s string) []string {
	var out []string
	start := 0
	for i := 0; i+1 < len(s); i++ {
		if strings.ContainsRune(".!?", rune(s[i])) && strings.ContainsRune(" \t\r\n", rune(s[i+1])) {
			out = append(out, s[start:i+1])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func typeExpr(s string) []string {
	s, depth := strings.TrimSpace(s), 0
	for {
		rest, ok := strings.CutPrefix(s, "Array of ")
		if !ok {
			break
		}
		s, depth = rest, depth+1
	}
	var out []string
	for _, part := range typeSplit.Split(s, -1) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, strings.Repeat("Array of ", depth)+part)
		}
	}
	return out
}

func description(e *entry) string {
	lines := make([]string, 0, len(e.paras))
	for _, p := range e.paras {
		if text := collapse(linkText.ReplaceAllString(p.text, "")); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n")
}

func uniq(in []string) []string {
	var out []string
	for _, s := range in {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
