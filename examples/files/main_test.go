package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// fakeBotAPI starts a fake Bot API; corrupt and noPath break the download.
func fakeBotAPI(t *testing.T, corrupt, noPath bool) teleiq.Option {
	t.Helper()
	srv := teleiqtest.NewServer()
	t.Cleanup(srv.Close)
	switch {
	case corrupt:
		srv.AddFile("changed", []byte("other content"))
		srv.Respond("getFile", models.File{FileID: "changed", FileUniqueID: "u", FilePath: teleiq.Ptr("files/changed")})
	case noPath:
		srv.Respond("getFile", models.File{FileID: "doc", FileUniqueID: "u"})
	}
	return teleiq.WithBaseURL(srv.URL)
}

func TestRun(t *testing.T) {
	file := filepath.Join(t.TempDir(), "report.csv")
	if err := os.WriteFile(file, []byte("a,b\n1,2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		chat      string
		path      string
		corrupt   bool
		noPath    bool
		wantOut   []string
		wantInErr string
	}{
		{name: "built-in content", chat: "42", wantOut: []string{"Uploaded teleiq-example.txt", "the content matches"}},
		{name: "file from disk", chat: "@channel", path: file, wantOut: []string{"Uploaded report.csv", "the content matches"}},
		{name: "changed on the way", chat: "42", corrupt: true, wantInErr: "differs from teleiq-example.txt"},
		{name: "no file path", chat: "42", noPath: true, wantInErr: "cannot be downloaded"},
		{name: "missing file", chat: "42", path: filepath.Join(t.TempDir(), "none.csv"), wantInErr: "no such file"},
		{name: "no chat", wantInErr: "set BOT_TOKEN and CHAT_ID"},
		{name: "invalid chat", chat: "channel", wantInErr: "CHAT_ID must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			var out bytes.Buffer

			err := run(context.Background(), teleiqtest.Token, tt.chat, tt.path, log.New(&out, "", 0), fakeBotAPI(t, tt.corrupt, tt.noPath))

			if left, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "teleiq-*")); len(left) > 0 {
				t.Errorf("temporary files left behind: %v", left)
			}
			if tt.wantInErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
					t.Fatalf("run() error = %v, want one mentioning %q", err, tt.wantInErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("run() error = %v", err)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output %q does not mention %q", out.String(), want)
				}
			}
		})
	}
}
