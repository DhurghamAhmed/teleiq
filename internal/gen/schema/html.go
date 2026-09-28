package schema

import (
	"html"
	"strings"
)

type tokenKind int

const (
	textToken tokenKind = iota
	startTag
	endTag
)

type token struct {
	kind tokenKind
	name string
	href string
	text string
}

func tokenize(src string) []token {
	var toks []token
	for i := 0; i < len(src); {
		if src[i] != '<' {
			j := strings.IndexByte(src[i:], '<')
			if j < 0 {
				j = len(src) - i
			}
			toks = append(toks, token{kind: textToken, text: html.UnescapeString(src[i : i+j])})
			i += j
			continue
		}
		rest := src[i:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			end := strings.Index(rest, "-->")
			if end < 0 {
				return toks
			}
			i += end + len("-->")
		case strings.HasPrefix(rest, "<!"), strings.HasPrefix(rest, "<?"):
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				return toks
			}
			i += end + 1
		case strings.HasPrefix(rest, "</"):
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				return toks
			}
			toks = append(toks, token{kind: endTag, name: tagName(rest[2:end])})
			i += end + 1
		case len(rest) > 1 && isLetter(rest[1]):
			end := tagEnd(rest)
			if end < 0 {
				return toks
			}
			name, href := parseStartTag(rest[1:end])
			toks = append(toks, token{kind: startTag, name: name, href: href})
			i += end + 1
			if name == "script" || name == "style" {
				// Raw text: its "<" characters are not tags.
				closing := strings.Index(strings.ToLower(src[i:]), "</"+name)
				if closing < 0 {
					return toks
				}
				i += closing
			}
		default:
			toks = append(toks, token{kind: textToken, text: "<"})
			i++
		}
	}
	return toks
}

func tagEnd(s string) int {
	var quote byte
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i
		}
	}
	return -1
}

func tagName(s string) string {
	end := strings.IndexFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '/' })
	if end < 0 {
		end = len(s)
	}
	return strings.ToLower(s[:end])
}

func parseStartTag(s string) (name, href string) {
	name = tagName(s)
	rest := s[len(name):]
	for rest != "" {
		rest = strings.TrimLeft(rest, " \t\r\n/")
		if rest == "" {
			break
		}
		keyEnd := strings.IndexAny(rest, "= \t\r\n/")
		if keyEnd < 0 {
			break
		}
		key := strings.ToLower(rest[:keyEnd])
		rest = strings.TrimLeft(rest[keyEnd:], " \t\r\n")
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		rest = strings.TrimLeft(rest[1:], " \t\r\n")
		var value string
		if rest != "" && (rest[0] == '"' || rest[0] == '\'') {
			end := strings.IndexByte(rest[1:], rest[0])
			if end < 0 {
				break
			}
			value, rest = rest[1:1+end], rest[2+end:]
		} else {
			end := strings.IndexAny(rest, " \t\r\n")
			if end < 0 {
				end = len(rest)
			}
			value, rest = rest[:end], rest[end:]
		}
		if key == "href" {
			href = html.UnescapeString(value)
		}
	}
	return name, href
}

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
