package bot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DhurghamAhmed/teleiq/models"
)

// Callback builds and reads the callback data of buttons, and matches their presses.
type Callback struct {
	prefix string // escaped
}

// NewCallback returns the Callback of the buttons whose data starts with prefix.
func NewCallback(prefix string) Callback {
	return Callback{prefix: escapeField(prefix)}
}

// Pack returns the callback data of the prefix and fields.
func (cb Callback) Pack(fields ...any) (string, error) {
	var sb strings.Builder
	sb.WriteString(cb.prefix)
	for i, f := range fields {
		var s string
		switch v := f.(type) {
		case string:
			s = escapeField(v)
		case bool:
			s = "0"
			if v {
				s = "1"
			}
		case int:
			s = strconv.FormatInt(int64(v), 10)
		case int8:
			s = strconv.FormatInt(int64(v), 10)
		case int16:
			s = strconv.FormatInt(int64(v), 10)
		case int32:
			s = strconv.FormatInt(int64(v), 10)
		case int64:
			s = strconv.FormatInt(v, 10)
		case uint:
			s = strconv.FormatUint(uint64(v), 10)
		case uint8:
			s = strconv.FormatUint(uint64(v), 10)
		case uint16:
			s = strconv.FormatUint(uint64(v), 10)
		case uint32:
			s = strconv.FormatUint(uint64(v), 10)
		case uint64:
			s = strconv.FormatUint(v, 10)
		default:
			return "", fmt.Errorf("bot: Callback.Pack: field %d is a %T; want a string, an integer or a bool", i, f)
		}
		sb.WriteByte(':')
		sb.WriteString(s)
	}
	data := sb.String()
	if len(data) == 0 || len(data) > 64 {
		return data, fmt.Errorf("bot: Callback.Pack: %d bytes of callback data; Telegram allows 1 to 64", len(data))
	}
	return data, nil
}

// Button returns an inline button that sends the data Pack returns for fields.
func (cb Callback) Button(text string, fields ...any) models.InlineKeyboardButton {
	data, _ := cb.Pack(fields...)
	return models.NewCallbackButton(text, data)
}

// Match reports whether the update is the press of a button of cb.
func (cb Callback) Match(c *Context) bool {
	q := c.update.CallbackQuery
	if q == nil || q.Data == nil {
		return false
	}
	rest, ok := strings.CutPrefix(*q.Data, cb.prefix)
	return ok && (rest == "" || rest[0] == ':')
}

// Unpack reads the fields of data, the callback data of a button of cb.
func (cb Callback) Unpack(data string) *CallbackFields {
	parts, err := splitFields(data)
	switch {
	case err != nil:
		return &CallbackFields{err: err}
	case parts[0] != cb.prefix:
		return &CallbackFields{err: fmt.Errorf("bot: callback data %q does not start with the prefix of the Callback", data)}
	}
	fields := make([]string, len(parts)-1)
	for i, p := range parts[1:] {
		fields[i] = unescapeField(p)
	}
	return &CallbackFields{fields: fields}
}

// CallbackFields are the fields of callback data, read with Callback.Unpack.
type CallbackFields struct {
	fields []string
	err    error
}

// Len returns the number of fields.
func (f *CallbackFields) Len() int { return len(f.fields) }

// Err returns the first error of Unpack or of reading a field, or nil.
func (f *CallbackFields) Err() error { return f.err }

// String returns field i.
func (f *CallbackFields) String(i int) string {
	s, _ := f.field(i)
	return s
}

// Int returns field i as an int.
func (f *CallbackFields) Int(i int) int {
	n, _ := parseField(f, i, func(s string) (int, error) { return strconv.Atoi(s) })
	return n
}

// Int64 returns field i as an int64.
func (f *CallbackFields) Int64(i int) int64 {
	n, _ := parseField(f, i, func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) })
	return n
}

// Bool returns field i as a bool, which Pack writes as 1 or 0.
func (f *CallbackFields) Bool(i int) bool {
	b, _ := parseField(f, i, strconv.ParseBool)
	return b
}

func (f *CallbackFields) field(i int) (string, bool) {
	if i < 0 || i >= len(f.fields) {
		f.fail(fmt.Errorf("bot: callback data has %d fields, not a field %d", len(f.fields), i))
		return "", false
	}
	return f.fields[i], true
}

func (f *CallbackFields) fail(err error) {
	if f.err == nil {
		f.err = err
	}
}

func parseField[T any](f *CallbackFields, i int, parse func(string) (T, error)) (T, bool) {
	var zero T
	s, ok := f.field(i)
	if !ok {
		return zero, false
	}
	v, err := parse(s)
	if err != nil {
		f.fail(fmt.Errorf("bot: callback data field %d: %w", i, err))
		return zero, false
	}
	return v, true
}

var fieldEscaper = strings.NewReplacer(`\`, `\\`, `:`, `\:`)

func escapeField(s string) string { return fieldEscaper.Replace(s) }

func unescapeField(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// splitFields splits data at the colons that no backslash escapes, keeping the escapes.
func splitFields(data string) ([]string, error) {
	var parts []string
	start := 0
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '\\':
			if i+1 == len(data) {
				return nil, errors.New("bot: callback data ends with a lone backslash")
			}
			i++
		case ':':
			parts = append(parts, data[start:i])
			start = i + 1
		}
	}
	return append(parts, data[start:]), nil
}
