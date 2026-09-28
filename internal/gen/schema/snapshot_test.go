package schema

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

var primitives = map[string]bool{"Integer": true, "Float": true, "String": true, "Boolean": true, "True": true}

func loadSnapshot(t *testing.T) ([]byte, *API) {
	t.Helper()
	raw, err := os.ReadFile("botapi.json")
	if err != nil {
		t.Fatal(err)
	}
	api, err := Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Decode(botapi.json) error = %v", err)
	}
	return raw, api
}

func TestSnapshotIsCanonical(t *testing.T) {
	raw, api := loadSnapshot(t)
	var buf bytes.Buffer
	if err := Encode(&buf, api); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), raw) {
		t.Error("botapi.json differs from its canonical encoding; regenerate it instead of editing it by hand")
	}
}

func TestSnapshotIsConsistent(t *testing.T) {
	_, api := loadSnapshot(t)
	if !regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`).MatchString(api.Version) {
		t.Errorf("version = %q", api.Version)
	}
	if _, err := time.Parse(time.DateOnly, api.ReleaseDate); err != nil {
		t.Errorf("release date = %q: %v", api.ReleaseDate, err)
	}
	types := map[string]*Type{}
	for i := range api.Types {
		ty := &api.Types[i]
		if types[ty.Name] != nil {
			t.Errorf("type %s is defined twice", ty.Name)
		}
		types[ty.Name] = ty
	}
	known := func(expr string) bool {
		for {
			rest, ok := strings.CutPrefix(expr, "Array of ")
			if !ok {
				break
			}
			expr = rest
		}
		return primitives[expr] || types[expr] != nil
	}
	check := func(where string, exprs []string) {
		if len(exprs) == 0 {
			t.Errorf("%s has no type", where)
		}
		for _, e := range exprs {
			if !known(e) {
				t.Errorf("%s refers to unknown type %q", where, e)
			}
		}
	}
	methods := map[string]bool{}
	for _, m := range api.Methods {
		if methods[m.Name] {
			t.Errorf("method %s is defined twice", m.Name)
		}
		methods[m.Name] = true
		check(m.Name+" result", m.Returns)
		for _, p := range m.Params {
			check(m.Name+"."+p.Name, p.Types)
		}
	}
	for _, ty := range api.Types {
		for _, f := range ty.Fields {
			check(ty.Name+"."+f.Name, f.Types)
		}
		for _, member := range ty.Members {
			check(ty.Name+" member", []string{member})
			if m := types[member]; ty.Discriminator != "" && m != nil && (len(m.Fields) == 0 || m.Fields[0].Name != ty.Discriminator || m.Fields[0].Const == "") {
				t.Errorf("%s member %s has no %q value", ty.Name, member, ty.Discriminator)
			}
		}
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	if _, err := Decode(strings.NewReader(`{"version":"1.0","typo":true}`)); err == nil {
		t.Error("Decode() accepted an unknown field")
	}
}
