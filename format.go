package teleiq

import "strings"

// Parse modes for the ParseMode fields: HTML and Telegram's MarkdownV2.
const (
	ParseModeHTML     = "HTML"
	ParseModeMarkdown = "MarkdownV2"
)

// markdownSpecial lists the characters Markdown requires escaped in plain text.
const markdownSpecial = "_*[]()~`>#+-=|{}.!\\"

// EscapeMarkdown escapes every character that Markdown reserves in plain text s.
func EscapeMarkdown(s string) string {
	n := 0
	for i := range len(s) {
		if strings.IndexByte(markdownSpecial, s[i]) >= 0 {
			n++
		}
	}
	if n == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + n)
	// Special characters are ASCII, so working on bytes keeps other text unchanged.
	for i := range len(s) {
		if strings.IndexByte(markdownSpecial, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
