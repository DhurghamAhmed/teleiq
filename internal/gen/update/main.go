// Command update brings TeleIQ to the latest release of the Bot API.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

// snapshotFile is where the snapshot lives, from the module root.
const snapshotFile = "internal/gen/schema/botapi.json"

// errOutdated reports, with -check, a page that differs from the snapshot.
var errOutdated = errors.New("the snapshot is behind the official page; run go run ./internal/gen/update")

type options struct {
	url, in, root   string
	check, generate bool
}

func main() {
	var o options
	flag.StringVar(&o.url, "url", schema.PageURL, "the documentation page to read")
	flag.StringVar(&o.in, "in", "", "read the page from this file instead of fetching it")
	flag.StringVar(&o.root, "root", ".", "the root of the module")
	flag.BoolVar(&o.check, "check", false, "only report the changes, and fail if there are any")
	flag.BoolVar(&o.generate, "generate", true, "run go generate after writing the snapshot")
	flag.Parse()
	if err := run(os.Stdout, o); err != nil {
		fmt.Fprintln(os.Stderr, "update:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, o options) error {
	// Write errors are ignored: the output is only a report.
	say := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format+"\n", args...) }
	path := filepath.Join(o.root, snapshotFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	old, err := schema.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	page, err := schema.ReadPage(o.url, o.in)
	if err != nil {
		return err
	}
	api, err := schema.Parse(bytes.NewReader(page))
	if err != nil {
		return err
	}
	changes := schema.Compare(old, api)
	if changes.Empty() {
		say("Bot API %s is up to date.", api.Version)
		return nil
	}
	changes.Report(w)
	if ch, ok := changes.Changed("Message"); ok && len(ch.Added) > 0 {
		say("Message has new fields: classify each in filter/service.go, as a service message or not;")
		say("TestServiceClassifiesEveryField fails until then.")
	}
	if o.check {
		return errOutdated
	}

	var buf bytes.Buffer
	if err := schema.Encode(&buf, api); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return err
	}
	say("wrote %s", snapshotFile)
	if o.generate {
		cmd := exec.Command("go", "generate", "./internal/gen")
		cmd.Dir, cmd.Stdout, cmd.Stderr = o.root, w, w
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go generate: %w; a new section of the page needs a file in internal/gen/sections.go", err)
		}
		say("generated the code")
	}
	if old.Version != api.Version {
		if err := setReadmeVersion(filepath.Join(o.root, "README.md"), old.Version, api.Version); err != nil {
			return err
		}
		if err := addChangelogEntry(filepath.Join(o.root, "CHANGELOG.md"), changes); err != nil {
			return err
		}
		say("set Bot API %s in README.md and CHANGELOG.md", api.Version)
	}
	say("next: check the new methods and types against the page, then go vet ./... && go test ./...")
	return nil
}

// setReadmeVersion replaces old with new in every mention of the Bot API.
func setReadmeVersion(path, old, new string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`(Bot(?: |%20)API[ -])` + regexp.QuoteMeta(old) + `\b`)
	if !re.Match(data) {
		return fmt.Errorf("%s does not mention Bot API %s", path, old)
	}
	return os.WriteFile(path, re.ReplaceAll(data, []byte("${1}"+new)), 0o644)
}

// addChangelogEntry adds the new Bot API release under Added in Unreleased.
func addChangelogEntry(path string, c schema.Changes) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(data)
	const unreleased = "## [Unreleased]\n"
	i := strings.Index(text, unreleased)
	if i < 0 {
		return fmt.Errorf("%s has no %q section", path, strings.TrimSpace(unreleased))
	}
	start := i + len(unreleased)
	end := len(text)
	if j := strings.Index(text[start:], "\n## ["); j >= 0 {
		end = start + j + 1
	}
	entry := changelogEntry(c)
	section := text[start:end]
	if k := strings.Index(section, "### Added\n\n"); k >= 0 {
		at := start + k + len("### Added\n\n")
		text = text[:at] + entry + text[at:]
	} else {
		text = text[:start] + "\n### Added\n\n" + entry + text[start:]
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

func changelogEntry(c schema.Changes) string {
	var parts []string
	quote := func(names []string) string {
		q := make([]string, len(names))
		for i, n := range names {
			q[i] = "`" + n + "`"
		}
		return strings.Join(q, ", ")
	}
	if len(c.AddedMethods) > 0 {
		parts = append(parts, "the methods "+quote(c.AddedMethods))
	}
	if len(c.AddedTypes) > 0 {
		parts = append(parts, "the types "+quote(c.AddedTypes))
	}
	var changed []string
	for _, ch := range append(c.ChangedMethods, c.ChangedTypes...) {
		changed = append(changed, ch.Name)
	}
	if len(changed) > 0 {
		parts = append(parts, "changes to "+quote(changed))
	}
	entry := "- Bot API " + c.NewVersion
	if c.NewDate != "" {
		entry += " (" + c.NewDate + ")"
	}
	if len(parts) > 0 {
		entry += ": " + strings.Join(parts, "; ")
	}
	if len(c.RemovedMethods)+len(c.RemovedTypes) > 0 {
		entry += ". Removed by Telegram: " + quote(append(c.RemovedMethods, c.RemovedTypes...))
	}
	return entry + ".\n"
}
