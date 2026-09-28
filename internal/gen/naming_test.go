package main

import "testing"

func TestGoName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"id", "ID"},
		{"chat_id", "ChatID"},
		{"file_unique_id", "FileUniqueID"},
		{"message_ids", "MessageIDs"},
		{"url", "URL"},
		{"thumbnail_url", "ThumbnailURL"},
		{"ip_address", "IPAddress"},
		{"html", "HTML"},
		{"is_rtl", "IsRTL"},
		{"mpeg4_url", "Mpeg4URL"},
		{"gif_width", "GifWidth"},
		{"street_line1", "StreetLine1"},
		{"text", "Text"},
		{"x", "X"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := goName(tt.in); got != tt.want {
				t.Errorf("goName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFirstSentence(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"single", "Unique identifier for this user or bot", "Unique identifier for this user or bot"},
		{"two sentences", "Text of the message. Must be 1-4096 characters.", "Text of the message."},
		{"abbreviation", "Pass a URL, e.g. the one of a photo. Then more.", "Pass a URL, e.g. the one of a photo."},
		{"quoted start", "Type of the chat. “private” or “group”.", "Type of the chat."},
		{"second paragraph", "First line.\nSecond paragraph.", "First line."},
		{"ends with period", "Only one.", "Only one."},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstSentence(tt.in); got != tt.want {
				t.Errorf("firstSentence(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTypeDoc(t *testing.T) {
	tests := []struct{ name, typ, in, want string }{
		{"this object", "User", "This object represents a Telegram user or bot. More text.", "User represents a Telegram user or bot."},
		{"other wording", "MessageOriginUser", "The message was originally sent by a known user.", "The message was originally sent by a known user."},
		{"no description", "EphemeralMessageParameters", "", "EphemeralMessageParameters is a Bot API object."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := typeDoc(tt.typ, tt.in); got != tt.want {
				t.Errorf("typeDoc(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
