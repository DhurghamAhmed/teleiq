// Package schema is TeleIQ's machine-readable description of the Telegram Bot API.
package schema

import (
	"encoding/json"
	"io"
)

// API describes one release of the Bot API.
type API struct {
	Version     string   `json:"version"`
	ReleaseDate string   `json:"release_date"`
	Methods     []Method `json:"methods"`
	Types       []Type   `json:"types"`
}

// Method describes one Bot API method.
type Method struct {
	Name        string   `json:"name"`
	Section     string   `json:"section"`
	Description string   `json:"description"`
	Params      []Field  `json:"params"`
	Returns     []string `json:"returns"`
}

// Type describes one Bot API object type or union of types.
type Type struct {
	Name          string   `json:"name"`
	Section       string   `json:"section"`
	Description   string   `json:"description"`
	Fields        []Field  `json:"fields"`
	Members       []string `json:"members,omitempty"`
	Discriminator string   `json:"discriminator,omitempty"`
}

// Field describes a method parameter or a field of a type.
type Field struct {
	Name        string   `json:"name"`
	Types       []string `json:"types"`
	Required    bool     `json:"required"`
	Const       string   `json:"const,omitempty"`
	Description string   `json:"description"`
}

// Encode writes api as the snapshot's indented JSON.
func Encode(w io.Writer, api *API) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(api)
}

// Decode reads a snapshot written by Encode.
func Decode(r io.Reader) (*API, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var api API
	if err := dec.Decode(&api); err != nil {
		return nil, err
	}
	return &api, nil
}
