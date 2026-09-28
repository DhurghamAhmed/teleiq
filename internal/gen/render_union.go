package main

import (
	"fmt"
	"strings"
)

func renderUnion(u *union) (*source, error) {
	s := newSource()
	marker := lowerFirst(u.name)
	var names []string
	for _, m := range u.members {
		if m.under != "" {
			names = append(names, m.name)
		} else {
			names = append(names, "*"+m.name)
		}
	}
	if u.decode {
		names = append(names, "*Unknown")
	}
	s.doc("", u.doc)
	s.line("//")
	s.doc("", "Implemented by "+joinList(names)+".")
	s.line("type %s interface{ %s() }", u.name, marker)
	for _, m := range u.members {
		if m.under == "" {
			continue
		}
		s.line("")
		s.doc("", m.doc)
		s.line("type %s %s", m.name, m.under)
	}
	s.line("")
	for _, m := range u.members {
		if m.under != "" {
			s.line("func (%s) %s() {}", m.name, marker)
		} else {
			s.line("func (*%s) %s() {}", m.name, marker)
		}
	}
	if !u.decode {
		return s, nil
	}
	s.line("func (*Unknown) %s() {}", marker)
	s.imports["encoding/json"] = true
	switch u.name {
	case "MaybeInaccessibleMessage":
		return s, renderDateDecode(s, u)
	case "RichText":
		renderRichTextDecode(s, u)
	default:
		renderTagDecode(s, u, "decode"+u.name)
	}
	return s, nil
}

// renderTagDecode writes a function that decodes a variant by its discriminator field.
func renderTagDecode(s *source, u *union, fn string) {
	s.line("")
	s.line("func %s(data []byte) (%s, error) {", fn, u.name)
	s.line("\tif isNull(data) {")
	s.line("\t\treturn nil, nil")
	s.line("\t}")
	s.line("\tvar head struct {")
	s.line("\t\tKind string `json:%q`", u.discriminator)
	s.line("\t}")
	s.line("\tif err := json.Unmarshal(data, &head); err != nil {")
	s.line("\t\treturn nil, err")
	s.line("\t}")
	s.line("\tvar v %s", u.name)
	s.line("\tswitch head.Kind {")
	for _, m := range u.members {
		if m.under != "" {
			continue
		}
		s.line("\tcase %q:", m.tag)
		s.line("\t\tv = new(%s)", m.name)
	}
	s.line("\tdefault:")
	s.line("\t\treturn newUnknown(head.Kind, data), nil")
	s.line("\t}")
	s.line("\tif err := json.Unmarshal(data, v); err != nil {")
	s.line("\t\treturn nil, err")
	s.line("\t}")
	s.line("\treturn v, nil")
	s.line("}")
}

// renderDateDecode writes the decoding of MaybeInaccessibleMessage by its date field.
func renderDateDecode(s *source, u *union) error {
	if len(u.members) != 2 || u.members[0].name != "Message" || u.members[1].name != "InaccessibleMessage" {
		return fmt.Errorf("union %s: members changed; update its decoding rule", u.name)
	}
	s.line("")
	s.line("func decode%s(data []byte) (%s, error) {", u.name, u.name)
	s.line("\tif isNull(data) {")
	s.line("\t\treturn nil, nil")
	s.line("\t}")
	s.line("\tvar head struct {")
	s.line("\t\tDate int64 `json:\"date\"`")
	s.line("\t}")
	s.line("\tif err := json.Unmarshal(data, &head); err != nil {")
	s.line("\t\treturn nil, err")
	s.line("\t}")
	s.line("\tvar v %s = new(Message)", u.name)
	s.line("\tif head.Date == 0 {")
	s.line("\t\tv = new(InaccessibleMessage)")
	s.line("\t}")
	s.line("\tif err := json.Unmarshal(data, v); err != nil {")
	s.line("\t\treturn nil, err")
	s.line("\t}")
	s.line("\treturn v, nil")
	s.line("}")
	return nil
}

// renderRichTextDecode writes the decoding of RichText, a string, array or object.
func renderRichTextDecode(s *source, u *union) {
	s.imports["bytes"] = true
	s.line("")
	s.line("func decodeRichText(data []byte) (RichText, error) {")
	s.line("\tswitch data = bytes.TrimSpace(data); {")
	s.line("\tcase isNull(data):")
	s.line("\t\treturn nil, nil")
	s.line("\tcase data[0] == '\"':")
	s.line("\t\tvar text RichTextString")
	s.line("\t\tif err := json.Unmarshal(data, &text); err != nil {")
	s.line("\t\t\treturn nil, err")
	s.line("\t\t}")
	s.line("\t\treturn text, nil")
	s.line("\tcase data[0] == '[':")
	s.line("\t\tvar raws []json.RawMessage")
	s.line("\t\tif err := json.Unmarshal(data, &raws); err != nil {")
	s.line("\t\t\treturn nil, err")
	s.line("\t\t}")
	s.line("\t\tparts, err := decodeEach(raws, decodeRichText)")
	s.line("\t\tif err != nil {")
	s.line("\t\t\treturn nil, err")
	s.line("\t\t}")
	s.line("\t\treturn RichTextArray(parts), nil")
	s.line("\t}")
	s.line("\treturn decodeRichTextObject(data)")
	s.line("}")
	renderTagDecode(s, u, "decodeRichTextObject")
}

func joinList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
