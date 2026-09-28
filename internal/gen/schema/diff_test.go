package schema

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	field := func(name, typ string, required bool) Field {
		return Field{Name: name, Types: []string{typ}, Required: required}
	}
	old := &API{Version: "1.0", Methods: []Method{
		{Name: "a", Params: []Field{field("p1", "String", true)}, Returns: []string{"True"}},
		{Name: "b", Params: []Field{field("p1", "String", false)}, Returns: []string{"True"}},
		{Name: "gone", Returns: []string{"True"}},
		{Name: "same", Description: "Old words.", Returns: []string{"True"}},
	}, Types: []Type{
		{Name: "T", Fields: []Field{field("f1", "Integer", false), field("f2", "String", false)}},
		{Name: "U", Members: []string{"A", "B"}},
		{Name: "Dropped"},
	}}
	new := &API{Version: "1.1", ReleaseDate: "2026-10-01", Methods: []Method{
		{Name: "a", Params: []Field{field("p1", "Integer", true), field("p2", "String", false)}, Returns: []string{"True"}},
		{Name: "b", Params: []Field{field("p1", "String", true)}, Returns: []string{"Message"}},
		{Name: "same", Description: "New words.", Returns: []string{"True"}},
		{Name: "c", Returns: []string{"True"}},
	}, Types: []Type{
		{Name: "T", Fields: []Field{field("f1", "Integer", false), field("f3", "Boolean", true)}},
		{Name: "U", Members: []string{"A", "C"}},
		{Name: "V"},
	}}
	c := Compare(old, new)
	check := func(what string, got, want []string) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Errorf("%s = %q, want %q", what, got, want)
		}
	}
	check("added methods", c.AddedMethods, []string{"c"})
	check("removed methods", c.RemovedMethods, []string{"gone"})
	check("added types", c.AddedTypes, []string{"V"})
	check("removed types", c.RemovedTypes, []string{"Dropped"})
	if len(c.ChangedMethods) != 2 || len(c.ChangedTypes) != 2 || c.Described != 1 {
		t.Fatalf("changed methods %v, types %v, described %d; want 2, 2 and 1", c.ChangedMethods, c.ChangedTypes, c.Described)
	}
	a, _ := c.Changed("a")
	check("a added", a.Added, []string{"p2 (String, optional)"})
	check("a modified", a.Modified, []string{"p1: String -> Integer"})
	b, _ := c.Changed("b")
	check("b modified", b.Modified, []string{"p1: optional -> required", "returns: True -> Message"})
	tt, _ := c.Changed("T")
	check("T added", tt.Added, []string{"f3 (Boolean, required)"})
	check("T removed", tt.Removed, []string{"f2"})
	u, _ := c.Changed("U")
	check("U added", u.Added, []string{"member C"})
	check("U removed", u.Removed, []string{"member B"})
	if _, ok := c.Changed("same"); ok || c.Empty() {
		t.Errorf("a description alone counts as a change of the method, or no change at all")
	}

	var report bytes.Buffer
	c.Report(&report)
	for _, want := range []string{"Bot API 1.0 -> 1.1 (2026-10-01)", "new methods (1): c", "removed types (1): Dropped",
		"    a: +p2 (String, optional); ~p1: String -> Integer", "    U: +member C; -member B", "descriptions changed: 1"} {
		if !strings.Contains(report.String(), want) {
			t.Errorf("the report has no %q:\n%s", want, report.String())
		}
	}
}

func TestCompareSame(t *testing.T) {
	api := &API{Version: "1.0", Methods: []Method{{Name: "a", Returns: []string{"True"}}}, Types: []Type{{Name: "T"}}}
	if c := Compare(api, api); !c.Empty() {
		t.Errorf("Compare(api, api) = %+v, want no change", c)
	}
	moved := &API{Version: "1.1", Methods: api.Methods, Types: api.Types}
	if c := Compare(api, moved); c.Empty() {
		t.Errorf("a new version alone is a change")
	}
	// Telegram also edits the page within a release; the code keeps its descriptions.
	reworded := &API{Version: "1.0", Methods: []Method{{Name: "a", Description: "Reworded.", Returns: []string{"True"}}}, Types: api.Types}
	if c := Compare(api, reworded); c.Empty() || c.Described != 1 {
		t.Errorf("Compare() of a reworded description = %+v, want a change", c)
	}
}
