package schema

import "strings"

const linkMark = "\x00"

type para struct {
	text  string
	links []string
}

type entry struct {
	name    string
	section string
	paras   []para
	lists   [][]string
	rows    [][]string
	table   bool
}

func scan(toks []token) []*entry {
	var (
		entries []*entry
		section string
		cur     *entry
		heading *strings.Builder
		headTag string
		p       *para
		inList  bool
		inTable bool
		row     []string
		cell    *strings.Builder
	)
	write := func(s string) {
		if heading != nil {
			heading.WriteString(s)
		}
		if p != nil {
			p.text += s
		}
		if cell != nil {
			cell.WriteString(s)
		}
	}
	for _, t := range toks {
		switch t.kind {
		case textToken:
			write(t.text)
		case startTag:
			switch t.name {
			case "h3", "h4":
				heading, headTag = &strings.Builder{}, t.name
			case "p":
				if cur != nil && !cur.table {
					p = &para{}
				}
			case "li":
				if cur != nil && !cur.table {
					inList = true
					cur.lists = append(cur.lists, nil)
				}
			case "a":
				anchor, ok := strings.CutPrefix(t.href, "#")
				if !ok || heading != nil {
					break
				}
				if p != nil {
					p.text += linkMark + anchor + linkMark
					p.links = append(p.links, anchor)
				}
				if inList {
					cur.lists[len(cur.lists)-1] = append(cur.lists[len(cur.lists)-1], anchor)
				}
			case "table":
				if cur != nil && !cur.table {
					cur.table, inTable = true, true
				}
			case "tr":
				if inTable {
					row = []string{}
				}
			case "td", "th":
				if row != nil {
					cell = &strings.Builder{}
				}
			case "br":
				write(" ")
			}
		case endTag:
			switch t.name {
			case "h3", "h4":
				if heading == nil || t.name != headTag {
					break
				}
				text := collapse(heading.String())
				heading = nil
				if t.name == "h3" {
					section, cur = text, nil
				} else {
					cur = &entry{name: text, section: section}
					entries = append(entries, cur)
				}
			case "p":
				if p != nil {
					cur.paras = append(cur.paras, *p)
					p = nil
				}
			case "li":
				inList = false
			case "td", "th":
				if cell != nil {
					row = append(row, collapse(cell.String()))
					cell = nil
				}
			case "tr":
				if row != nil {
					cur.rows = append(cur.rows, row)
					row = nil
				}
			case "table":
				inTable = false
			}
		}
	}
	return entries
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
