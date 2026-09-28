package schema

import (
	"reflect"
	"strings"
	"testing"
)

func render(toks []token) []string {
	var out []string
	for _, t := range toks {
		switch t.kind {
		case startTag:
			out = append(out, "<"+t.name+" "+t.href+">")
		case endTag:
			out = append(out, "</"+t.name+">")
		default:
			if n := len(out); n > 0 && strings.HasPrefix(out[n-1], "text:") {
				out[n-1] += t.text
			} else {
				out = append(out, "text:"+t.text)
			}
		}
	}
	return out
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"link with entity", `<a href="#terms">T&amp;C</a>`, []string{"<a #terms>", "text:T&C", "</a>"}},
		{"quoted greater-than", `<p title='a>b'>x</p>`, []string{"<p >", "text:x", "</p>"}},
		{"unquoted and uppercase", `<A HREF=#user CLASS=x>U</A>`, []string{"<a #user>", "text:U", "</a>"}},
		{"entity in href", `<a href="#a&amp;b">x</a>`, []string{"<a #a&b>", "text:x", "</a>"}},
		{"self-closing", `a<br/>b`, []string{"text:a", "<br >", "text:b"}},
		{"script is raw text", `<script>if (a<b) { x("<p>"); }</script><p>y</p>`, []string{"<script >", "</script>", "<p >", "text:y", "</p>"}},
		{"style is raw text", `<style>p > a { }</style>z`, []string{"<style >", "</style>", "text:z"}},
		{"comment skipped", `<!-- <p>no</p> -->z`, []string{"text:z"}},
		{"doctype skipped", `<!DOCTYPE html>z`, []string{"text:z"}},
		{"lone less-than", `a < b`, []string{"text:a < b"}},
		{"unterminated tag", `x<p`, []string{"text:x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(tokenize(tt.in)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tokenize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
