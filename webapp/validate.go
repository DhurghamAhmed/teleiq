package webapp

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrInvalid is returned for data that is malformed or that Telegram did not sign.
	ErrInvalid = errors.New("webapp: init data is not valid")
	// ErrExpired is returned for data older than the maximum age given.
	ErrExpired = errors.New("webapp: init data has expired")
)

// ProductionKey and TestKey are Telegram's public keys for Mini App data.
var (
	ProductionKey = mustKey("e7bf03a2fa4602af4580703d88dda5bb59f32ed8b02a56c187fe7d34caed242d")
	TestKey       = mustKey("40055058a4ee38156a06562e52eece92a771bcd8346a8c4615cb7376eddf72ec")
)

// futureSkew is how far ahead of the server clock auth_date may be.
const futureSkew = time.Minute

// Validate checks initData with the token of the bot and returns its fields.
func Validate(initData, token string, maxAge time.Duration) (*InitData, error) {
	switch {
	case token == "":
		return nil, errors.New("webapp: Validate: empty token")
	case maxAge <= 0:
		return nil, errors.New("webapp: Validate: maxAge must be positive")
	}
	fields, err := parse(initData)
	if err != nil {
		return nil, err
	}
	hash, err := hex.DecodeString(fields["hash"])
	if err != nil || len(hash) != sha256.Size {
		return nil, fmt.Errorf("%w: no hash of 64 hex digits", ErrInvalid)
	}
	secret := mac([]byte("WebAppData"), token)
	if !hmac.Equal(hash, mac(secret, checkString(fields, "hash"))) {
		return nil, fmt.Errorf("%w: the hash does not match", ErrInvalid)
	}
	return decode(fields, maxAge)
}

// ValidateSignature checks the signature of initData and returns its fields.
func ValidateSignature(initData string, botID int64, key ed25519.PublicKey, maxAge time.Duration) (*InitData, error) {
	switch {
	case botID <= 0:
		return nil, errors.New("webapp: ValidateSignature: the bot ID must be positive")
	case len(key) != ed25519.PublicKeySize:
		return nil, errors.New("webapp: ValidateSignature: the key is not an Ed25519 public key")
	case maxAge <= 0:
		return nil, errors.New("webapp: ValidateSignature: maxAge must be positive")
	}
	fields, err := parse(initData)
	if err != nil {
		return nil, err
	}
	// Telegram omits the base64url padding; padded input is accepted too.
	signature, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(fields["signature"], "="))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: no Ed25519 signature", ErrInvalid)
	}
	message := strconv.FormatInt(botID, 10) + ":WebAppData\n" + checkString(fields, "hash", "signature")
	if !ed25519.Verify(key, []byte(message), signature) {
		return nil, fmt.Errorf("%w: the signature does not match", ErrInvalid)
	}
	return decode(fields, maxAge)
}

// parse splits the query string into its fields, each of which must appear once.
func parse(initData string) (map[string]string, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("%w: not a query string", ErrInvalid)
	}
	fields := make(map[string]string, len(values))
	for k, v := range values {
		if len(v) != 1 {
			return nil, fmt.Errorf("%w: a field appears more than once", ErrInvalid)
		}
		fields[k] = v[0]
	}
	return fields, nil
}

// checkString returns the data-check-string of fields, without those left out.
func checkString(fields map[string]string, leaveOut ...string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if !slices.Contains(leaveOut, k) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(fields[k])
	}
	return b.String()
}

// mac returns the HMAC-SHA-256 of data with key.
func mac(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(data))
	return h.Sum(nil)
}

func mustKey(h string) ed25519.PublicKey {
	key, err := hex.DecodeString(h)
	if err != nil || len(key) != ed25519.PublicKeySize {
		panic("webapp: bad built-in key")
	}
	return key
}
