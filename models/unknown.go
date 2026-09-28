package models

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Unknown is a variant of a union type that this version of teleiq does not know.
type Unknown struct {
	Kind string          // the value of the union's discriminator field, such as "type"
	Raw  json.RawMessage // the variant as received
}

// MarshalJSON returns the variant as it was received.
func (u Unknown) MarshalJSON() ([]byte, error) {
	if len(u.Raw) == 0 {
		return nil, errors.New("teleiq: Unknown has no raw value")
	}
	return u.Raw, nil
}

func newUnknown(kind string, data []byte) *Unknown {
	return &Unknown{Kind: kind, Raw: bytes.Clone(data)}
}

func isNull(data []byte) bool {
	data = bytes.TrimSpace(data)
	return len(data) == 0 || string(data) == "null"
}

func decodeEach[T any](raws []json.RawMessage, decode func([]byte) (T, error)) ([]T, error) {
	if raws == nil {
		return nil, nil
	}
	out := make([]T, len(raws))
	for i, raw := range raws {
		v, err := decode(raw)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
