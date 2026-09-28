package teleiqtest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// WebAppInitData returns Mini App init data for fields, signed with token.
func WebAppInitData(token string, fields url.Values) string {
	v := url.Values{}
	for k, vs := range fields {
		if k != "hash" && len(vs) > 0 {
			v.Set(k, vs[0])
		}
	}
	if _, ok := v["auth_date"]; !ok {
		v.Set("auth_date", strconv.FormatInt(time.Now().Unix(), 10))
	}
	// Sorted by key, not by line: "a-b=" sorts before "a=", but key "a" before "a-b".
	keys := slices.Sorted(maps.Keys(v))
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + v.Get(k)
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secret.Write([]byte(token))
	h := hmac.New(sha256.New, secret.Sum(nil))
	_, _ = h.Write([]byte(strings.Join(lines, "\n")))
	v.Set("hash", hex.EncodeToString(h.Sum(nil)))
	return v.Encode()
}
