package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

var initialisms = map[string]string{
	"html": "HTML",
	"id":   "ID",
	"ids":  "IDs",
	"ip":   "IP",
	"rtl":  "RTL",
	"url":  "URL",
}

// goName converts a snake_case Bot API name to an exported Go name.
func goName(snake string) string {
	var b strings.Builder
	for part := range strings.SplitSeq(snake, "_") {
		if up, ok := initialisms[part]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(upperFirst(part))
	}
	return b.String()
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

// firstSentence returns the first sentence of a description's first paragraph.
func firstSentence(desc string) string {
	desc, _, _ = strings.Cut(desc, "\n")
	for i := 0; i+2 < len(desc); i++ {
		// A period followed by a lowercase word, as in "e.g. the", does not end a sentence.
		if desc[i] != '.' || desc[i+1] != ' ' {
			continue
		}
		if next, _ := utf8.DecodeRuneInString(desc[i+2:]); !unicode.IsLower(next) {
			return desc[:i+1]
		}
	}
	return strings.TrimSpace(desc)
}

func typeDoc(name, desc string) string {
	s := firstSentence(desc)
	if rest, ok := strings.CutPrefix(s, "This object "); ok {
		return name + " " + rest
	}
	if s == "" {
		return name + " is a Bot API object."
	}
	return s
}
