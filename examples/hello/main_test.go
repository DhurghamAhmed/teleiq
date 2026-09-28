package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// program returns main.go from its package clause on.
func program(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(src, []byte("package main\n"))
	if i < 0 {
		t.Fatal("main.go has no package clause")
	}
	return string(src[i:])
}

// readmeProgram returns the Go block of Getting started in the README.
func readmeProgram(t *testing.T) string {
	t.Helper()
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(readme), "\n## Getting started\n")
	if !ok {
		t.Fatal("README.md has no Getting started section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	_, block, ok := strings.Cut(section, "\n```go\n")
	if !ok {
		t.Fatal("the Getting started section of README.md has no Go block")
	}
	block, _, _ = strings.Cut(block, "\n```\n")
	return block + "\n"
}

// TestProgramIsDocumented keeps the first bot of the README identical to this program.
func TestProgramIsDocumented(t *testing.T) {
	if got, want := readmeProgram(t), program(t); got != want {
		t.Errorf("the first bot of README.md differs from examples/hello/main.go:\n%s", got)
	}
}

// TestProgramIsShort keeps the program within 25 lines of code.
func TestProgramIsShort(t *testing.T) {
	lines := 0
	for line := range strings.Lines(program(t)) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "//") {
			lines++
		}
	}
	if lines > 25 {
		t.Errorf("the program has %d lines, want at most 25", lines)
	}
}
