package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

const page = "../schema/testdata/page.html"

// module writes a module root whose snapshot is the test page as it was in Bot API 9.4: without
// getChatMemberCount, and without the entities of Message. It returns the root.
func module(t *testing.T, readme, changelog string) string {
	t.Helper()
	data, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	api, err := schema.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	api.Version, api.ReleaseDate = "9.4", "2026-01-01"
	api.Methods = slices.DeleteFunc(api.Methods, func(m schema.Method) bool { return m.Name == "getChatMemberCount" })
	for i, typ := range api.Types {
		if typ.Name == "Message" {
			api.Types[i].Fields = slices.DeleteFunc(typ.Fields, func(f schema.Field) bool { return f.Name == "entities" })
		}
	}
	root := t.TempDir()
	var buf bytes.Buffer
	if err := schema.Encode(&buf, api); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{snapshotFile: buf.String(), "README.md": readme, "CHANGELOG.md": changelog} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const (
	readme = "[![Bot API 9.4](https://img.shields.io/badge/Bot%20API-9.4-2CA5E0)](x)\nEvery method of Bot API 9.4; Bot API 9.40 is another.\n"
	log    = "# Changelog\n\n## [Unreleased]\n\n### Fixed\n\n- A fix.\n\n## [0.1.0] - 2026-09-28\n\n### Added\n\n- The first release.\n"
)

func read(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUpdate(t *testing.T) {
	root := module(t, readme, log)
	var out bytes.Buffer
	if err := run(&out, options{in: page, root: root}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Bot API 9.4 -> 9.5 (2026-03-01)", "new methods (1): getChatMemberCount",
		"    Message: +entities (Array of MessageEntity, optional)", "Message has new fields", "set Bot API 9.5"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the output has no %q:\n%s", want, out.String())
		}
	}
	snapshot, err := schema.Decode(strings.NewReader(read(t, root, snapshotFile)))
	if err != nil || snapshot.Version != "9.5" || len(snapshot.Methods) != 5 {
		t.Errorf("snapshot = version %v, %d methods, %v; want the page", snapshot.Version, len(snapshot.Methods), err)
	}
	wantReadme := "[![Bot API 9.5](https://img.shields.io/badge/Bot%20API-9.5-2CA5E0)](x)\nEvery method of Bot API 9.5; Bot API 9.40 is another.\n"
	if got := read(t, root, "README.md"); got != wantReadme {
		t.Errorf("README.md =\n%s", got)
	}
	wantLog := "## [Unreleased]\n\n### Added\n\n- Bot API 9.5 (2026-03-01): the methods `getChatMemberCount`; changes to `Message`.\n\n### Fixed\n"
	if got := read(t, root, "CHANGELOG.md"); !strings.Contains(got, wantLog) || strings.Count(got, "### Added") != 2 {
		t.Errorf("CHANGELOG.md =\n%s\nwant a new Added section in Unreleased", got)
	}

	// The same page again changes nothing.
	out.Reset()
	if err := run(&out, options{in: page, root: root}); err != nil || !strings.Contains(out.String(), "Bot API 9.5 is up to date.") {
		t.Errorf("a second run = %v:\n%s", err, out.String())
	}
}

func TestUpdateAddedSection(t *testing.T) {
	root := module(t, readme, "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- Something.\n")
	if err := run(&bytes.Buffer{}, options{in: page, root: root}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, "CHANGELOG.md"); !strings.Contains(got, "### Added\n\n- Bot API 9.5 (2026-03-01): the methods `getChatMemberCount`; changes to `Message`.\n- Something.\n") {
		t.Errorf("CHANGELOG.md =\n%s\nwant the entry first in the existing Added section", got)
	}
}

func TestUpdateCheck(t *testing.T) {
	root := module(t, readme, log)
	before := read(t, root, snapshotFile)
	var out bytes.Buffer
	if err := run(&out, options{in: page, root: root, check: true}); !errors.Is(err, errOutdated) {
		t.Errorf("run -check = %v, want errOutdated", err)
	}
	if read(t, root, snapshotFile) != before || read(t, root, "README.md") != readme || read(t, root, "CHANGELOG.md") != log {
		t.Errorf("run -check changed files")
	}
	if !strings.Contains(out.String(), "new methods (1): getChatMemberCount") {
		t.Errorf("run -check did not report the changes:\n%s", out.String())
	}
}

func TestUpdateErrors(t *testing.T) {
	tests := []struct {
		name, readme, changelog, in, want string
	}{
		{"a README without the version", "Every method.\n", log, page, "does not mention Bot API 9.4"},
		{"a CHANGELOG without Unreleased", readme, "# Changelog\n", page, "no \"## [Unreleased]\" section"},
		{"a page that is missing", readme, log, "no-such-page.html", "no such file"},
		{"not a documentation page", readme, log, "main.go", "no release found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := module(t, tt.readme, tt.changelog)
			err := run(&bytes.Buffer{}, options{in: tt.in, root: root})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("run() = %v, want an error with %q", err, tt.want)
			}
		})
	}
}
