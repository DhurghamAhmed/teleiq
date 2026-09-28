// Command gen generates the Bot API types and methods of package teleiq.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

// The directive lives here, not in teleiq, where the generator deletes stale files.
//go:generate go run . -schema schema/botapi.json -out ../..

func main() {
	schemaPath := flag.String("schema", "internal/gen/schema/botapi.json", "the schema snapshot to read")
	out := flag.String("out", ".", "the directory of package teleiq")
	check := flag.Bool("check", false, "report generated files that differ from the snapshot instead of writing them")
	flag.Parse()
	if err := run(*schemaPath, *out, *check); err != nil {
		fmt.Fprintln(os.Stderr, "teleiq-gen:", err)
		os.Exit(1)
	}
}

func run(schemaPath, out string, check bool) error {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	api, err := schema.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: %w", schemaPath, err)
	}
	m, err := newModel(api)
	if err != nil {
		return err
	}
	files, err := generate(m)
	if err != nil {
		return err
	}
	if err := checkContextNames(filepath.Join(out, filepath.Dir(contextFile)), files[contextFile]); err != nil {
		return err
	}
	if check {
		return verify(out, files)
	}
	return write(out, files)
}

// write stores files in dir, rewriting only changed ones, and removes stale ones.
func write(dir string, files map[string][]byte) error {
	stale, err := staleFiles(dir, files)
	if err != nil {
		return err
	}
	for _, name := range stale {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		path := filepath.Join(dir, name)
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, files[name]) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// verify reports the generated files in dir that are missing, edited or stale.
func verify(dir string, files map[string][]byte) error {
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		old, err := os.ReadFile(filepath.Join(dir, name))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			problems = append(problems, name+" is missing")
		case err != nil:
			return err
		case !bytes.Equal(old, files[name]):
			problems = append(problems, name+" differs")
		}
	}
	stale, err := staleFiles(dir, files)
	if err != nil {
		return err
	}
	for _, name := range stale {
		problems = append(problems, name+" is no longer generated")
	}
	if len(problems) > 0 {
		return fmt.Errorf("generated files are out of date (run go generate ./...): %s", strings.Join(problems, ", "))
	}
	return nil
}

// staleFiles returns the generated files in dir that files does not contain.
func staleFiles(dir string, files map[string][]byte) ([]string, error) {
	var stale []string
	// A section that the page no longer has leaves its file in models/.
	for _, pattern := range []string{"models/*.gen.go"} {
		paths, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return nil, err
			}
			if _, ok := files[filepath.ToSlash(rel)]; !ok {
				stale = append(stale, filepath.ToSlash(rel))
			}
		}
	}
	slices.Sort(stale)
	return stale, nil
}
