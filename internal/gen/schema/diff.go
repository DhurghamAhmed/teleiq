package schema

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// Changes are the differences between two snapshots of the Bot API.
type Changes struct {
	OldVersion, NewVersion, NewDate string
	AddedMethods, RemovedMethods    []string
	AddedTypes, RemovedTypes        []string
	ChangedMethods, ChangedTypes    []Change
	// Described counts the methods and types whose description alone changed.
	Described int
}

// Change is what changed in one method or type: its parameters, fields or members.
type Change struct {
	Name                     string
	Added, Removed, Modified []string
}

// Compare returns the changes from old to new, in the order of the new page.
func Compare(old, new *API) Changes {
	c := Changes{OldVersion: old.Version, NewVersion: new.Version, NewDate: new.ReleaseDate}
	oldMethods := map[string]Method{}
	for _, m := range old.Methods {
		oldMethods[m.Name] = m
	}
	for _, m := range new.Methods {
		was, ok := oldMethods[m.Name]
		switch {
		case !ok:
			c.AddedMethods = append(c.AddedMethods, m.Name)
		default:
			ch := compareFields(m.Name, was.Params, m.Params)
			ch.Modified = append(ch.Modified, compareReturns(was.Returns, m.Returns)...)
			if ch.changed() {
				c.ChangedMethods = append(c.ChangedMethods, ch)
			} else if was.Description != m.Description || descriptionsDiffer(was.Params, m.Params) {
				c.Described++
			}
		}
		delete(oldMethods, m.Name)
	}
	for _, m := range old.Methods {
		if _, gone := oldMethods[m.Name]; gone {
			c.RemovedMethods = append(c.RemovedMethods, m.Name)
		}
	}
	oldTypes := map[string]Type{}
	for _, t := range old.Types {
		oldTypes[t.Name] = t
	}
	for _, t := range new.Types {
		was, ok := oldTypes[t.Name]
		switch {
		case !ok:
			c.AddedTypes = append(c.AddedTypes, t.Name)
		default:
			ch := compareFields(t.Name, was.Fields, t.Fields)
			for _, m := range t.Members {
				if !slices.Contains(was.Members, m) {
					ch.Added = append(ch.Added, "member "+m)
				}
			}
			for _, m := range was.Members {
				if !slices.Contains(t.Members, m) {
					ch.Removed = append(ch.Removed, "member "+m)
				}
			}
			if ch.changed() {
				c.ChangedTypes = append(c.ChangedTypes, ch)
			} else if was.Description != t.Description || descriptionsDiffer(was.Fields, t.Fields) {
				c.Described++
			}
		}
		delete(oldTypes, t.Name)
	}
	for _, t := range old.Types {
		if _, gone := oldTypes[t.Name]; gone {
			c.RemovedTypes = append(c.RemovedTypes, t.Name)
		}
	}
	return c
}

// Empty reports whether the two snapshots are the same release with the same content.
func (c Changes) Empty() bool {
	return c.OldVersion == c.NewVersion && len(c.AddedMethods)+len(c.RemovedMethods)+len(c.AddedTypes)+
		len(c.RemovedTypes)+len(c.ChangedMethods)+len(c.ChangedTypes)+c.Described == 0
}

// Changed returns the change of the method or type name, if it changed.
func (c Changes) Changed(name string) (Change, bool) {
	for _, ch := range slices.Concat(c.ChangedMethods, c.ChangedTypes) {
		if ch.Name == name {
			return ch, true
		}
	}
	return Change{}, false
}

// Report writes the changes for a reader.
func (c Changes) Report(w io.Writer) {
	// Write errors are ignored: the output is only a report for a reader.
	out := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	if c.OldVersion == c.NewVersion {
		out("Bot API %s\n", c.NewVersion)
	} else {
		out("Bot API %s -> %s (%s)\n", c.OldVersion, c.NewVersion, c.NewDate)
	}
	list := func(label string, names []string) {
		if len(names) > 0 {
			out("  %s (%d): %s\n", label, len(names), strings.Join(names, ", "))
		}
	}
	list("new methods", c.AddedMethods)
	list("removed methods", c.RemovedMethods)
	list("new types", c.AddedTypes)
	list("removed types", c.RemovedTypes)
	for _, group := range []struct {
		label   string
		changes []Change
	}{{"changed methods", c.ChangedMethods}, {"changed types", c.ChangedTypes}} {
		if len(group.changes) == 0 {
			continue
		}
		out("  %s (%d):\n", group.label, len(group.changes))
		for _, ch := range group.changes {
			var parts []string
			for _, a := range ch.Added {
				parts = append(parts, "+"+a)
			}
			for _, r := range ch.Removed {
				parts = append(parts, "-"+r)
			}
			for _, m := range ch.Modified {
				parts = append(parts, "~"+m)
			}
			out("    %s: %s\n", ch.Name, strings.Join(parts, "; "))
		}
	}
	if c.Described > 0 {
		out("  descriptions changed: %d\n", c.Described)
	}
}

func (ch Change) changed() bool { return len(ch.Added)+len(ch.Removed)+len(ch.Modified) > 0 }

// compareFields compares the parameters of a method or the fields of a type.
func compareFields(name string, old, new []Field) Change {
	ch := Change{Name: name}
	was := map[string]Field{}
	for _, f := range old {
		was[f.Name] = f
	}
	for _, f := range new {
		o, ok := was[f.Name]
		switch {
		case !ok:
			ch.Added = append(ch.Added, describe(f))
		case !slices.Equal(o.Types, f.Types):
			ch.Modified = append(ch.Modified, fmt.Sprintf("%s: %s -> %s", f.Name, strings.Join(o.Types, " or "), strings.Join(f.Types, " or ")))
		case o.Required != f.Required:
			ch.Modified = append(ch.Modified, fmt.Sprintf("%s: %s -> %s", f.Name, requirement(o), requirement(f)))
		}
		delete(was, f.Name)
	}
	for _, f := range old {
		if _, gone := was[f.Name]; gone {
			ch.Removed = append(ch.Removed, f.Name)
		}
	}
	return ch
}

func compareReturns(old, new []string) []string {
	if slices.Equal(old, new) {
		return nil
	}
	return []string{fmt.Sprintf("returns: %s -> %s", strings.Join(old, " or "), strings.Join(new, " or "))}
}

func descriptionsDiffer(old, new []Field) bool {
	was := map[string]string{}
	for _, f := range old {
		was[f.Name] = f.Description
	}
	for _, f := range new {
		if was[f.Name] != f.Description {
			return true
		}
	}
	return false
}

func describe(f Field) string {
	return fmt.Sprintf("%s (%s, %s)", f.Name, strings.Join(f.Types, " or "), requirement(f))
}

func requirement(f Field) string {
	if f.Required {
		return "required"
	}
	return "optional"
}
