package bot

import (
	"strings"
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		text, name, mention, args string
		ok                        bool
	}{
		{"/start", "start", "", "", true},
		{"/start a b", "start", "", "a b", true},
		{"/start@my_bot a b", "start", "my_bot", "a b", true},
		{"/start\nsecond line", "start", "", "second line", true},
		{"/start\r\nsecond line", "start", "", "second line", true},
		{"/start@my_bot a", "start", "my_bot", "a", true},
		{"/set_name_2", "set_name_2", "", "", true},
		{"start", "", "", "", false},
		{"/", "", "", "", false},
		{"/ start", "", "", "", false},
		{"/sta-rt", "", "", "", false},
		{"/" + strings.Repeat("a", 33), "", "", "", false},
		{" /start", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			name, mention, args, ok := parseCommand(tt.text)
			if name != tt.name || mention != tt.mention || args != tt.args || ok != tt.ok {
				t.Errorf("parseCommand(%q) = %q, %q, %q, %v; want %q, %q, %q, %v",
					tt.text, name, mention, args, ok, tt.name, tt.mention, tt.args, tt.ok)
			}
		})
	}
}
