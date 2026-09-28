package webapp

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// vectors were signed by code independent of this package, Python's hmac and hashlib following
// the official algorithm. Their auth_date is 2026-09-21.
var vectors = []struct{ name, token, initData string }{
	{"basic", "111:fake-token-basic", "query_id=AAHdF6IQAAAAAN0XohDhrOrc&user=%7B%22id%22%3A279058397%2C%22first_name%22%3A%22%D8%B3%D9%84%D9%85%D9%89%22%2C%22last_name%22%3A%22Test%20%F0%9F%9A%80%22%2C%22username%22%3A%22salma_test%22%2C%22language_code%22%3A%22ar%22%2C%22is_premium%22%3Atrue%2C%22allows_write_to_pm%22%3Atrue%2C%22photo_url%22%3A%22https%3A%2F%2Ft.me%2Fi%2Fuserpic%2F320%2Fa%2Bb.svg%22%7D&auth_date=1790000000&hash=01b18f48032d1d414d2382292484816f83bad61894c5a2171be7f9224f236b79"},
	{"signature is part of the hash", "222:fake-token-signature", "user=%7B%22id%22%3A279058397%2C%22first_name%22%3A%22%D8%B3%D9%84%D9%85%D9%89%22%2C%22last_name%22%3A%22Test%20%F0%9F%9A%80%22%2C%22username%22%3A%22salma_test%22%2C%22language_code%22%3A%22ar%22%2C%22is_premium%22%3Atrue%2C%22allows_write_to_pm%22%3Atrue%2C%22photo_url%22%3A%22https%3A%2F%2Ft.me%2Fi%2Fuserpic%2F320%2Fa%2Bb.svg%22%7D&chat_instance=-3788475317572404878&chat_type=private&auth_date=1790000000&signature=zL-ucjNyREiHDE8aihFwpfR9aggP2xiAo3NSpfe-p7IbCisNlDKlo7Kb6G4D0Ao2mBrSgEk4maLSdv6MLIlADQ&hash=686e0d04f31b39c78cd451ceef78422c4cab28acd7f9b82d9ffc6c16c0435e66"},
	{"attachment menu", "333:fake_token-attachment", "query_id=AAE1&user=%7B%22id%22%3A279058397%2C%22first_name%22%3A%22%D8%B3%D9%84%D9%85%D9%89%22%2C%22last_name%22%3A%22Test%20%F0%9F%9A%80%22%2C%22username%22%3A%22salma_test%22%2C%22language_code%22%3A%22ar%22%2C%22is_premium%22%3Atrue%2C%22allows_write_to_pm%22%3Atrue%2C%22photo_url%22%3A%22https%3A%2F%2Ft.me%2Fi%2Fuserpic%2F320%2Fa%2Bb.svg%22%7D&receiver=%7B%22id%22%3A100%2C%22is_bot%22%3Atrue%2C%22first_name%22%3A%22Other%20bot%22%7D&chat=%7B%22id%22%3A-1001234567890%2C%22type%22%3A%22supergroup%22%2C%22title%22%3A%22Group%20%26%20friends%22%7D&chat_type=supergroup&chat_instance=42&start_param=ref_2024&can_send_after=5&auth_date=1790000000&hash=6f318cc36c9bb33ddb9df2c8cb2bed0b0a2d6e88a4bc91282621be3bf09f36a6"},
	{"reserved characters in values", "1:A", "start_param=a%2Bb%26c%3Dd%25e%20f%2Fg%3Fh%23i&auth_date=1790000000&hash=a58b908c051a58f4d59ca17bfb51c91cad018abff71a25e5130d1def8b53c1e1"},
	{"a field unknown today", "2:B", "auth_date=1790000000&some_new_field=kept%20in%20the%20check&hash=47017f886adae69ae9c1d315946526c2ae0805c0acaee541e27de052563a83d5"},
}

// sinceVectors is a maxAge that accepts the auth_date of the vectors.
var sinceVectors = time.Since(time.Unix(1790000000, 0)) + time.Hour

func TestValidateVectors(t *testing.T) {
	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			d, err := Validate(v.initData, v.token, sinceVectors)
			if err != nil {
				t.Fatalf("Validate() = %v", err)
			}
			if d.AuthDate.Unix() != 1790000000 {
				t.Errorf("AuthDate = %v", d.AuthDate)
			}
		})
	}
	d, _ := Validate(vectors[0].initData, vectors[0].token, sinceVectors)
	if u := d.User; d.QueryID != "AAHdF6IQAAAAAN0XohDhrOrc" || u == nil || u.ID != 279058397 || u.FirstName != "سلمى" ||
		u.LastName != "Test 🚀" || !u.IsPremium || !u.AllowsWriteToPM || u.PhotoURL != "https://t.me/i/userpic/320/a+b.svg" {
		t.Errorf("basic = %+v, user %+v", d, d.User)
	}
	d, _ = Validate(vectors[1].initData, vectors[1].token, sinceVectors)
	if d.Signature == "" || d.ChatType != "private" || d.ChatInstance != "-3788475317572404878" {
		t.Errorf("signature case = %+v", d)
	}
	d, _ = Validate(vectors[2].initData, vectors[2].token, sinceVectors)
	if d.Chat == nil || d.Chat.ID != -1001234567890 || d.Chat.Title != "Group & friends" || d.Receiver == nil || !d.Receiver.IsBot ||
		d.CanSendAfter != 5*time.Second || d.StartParam != "ref_2024" {
		t.Errorf("attachment menu = %+v, chat %+v, receiver %+v", d, d.Chat, d.Receiver)
	}
	d, _ = Validate(vectors[3].initData, vectors[3].token, sinceVectors)
	if d.StartParam != "a+b&c=d%e f/g?h#i" {
		t.Errorf("StartParam = %q", d.StartParam)
	}
}

// signHMAC signs fields as Telegram does, for variants the vectors do not cover.
func signHMAC(token string, fields url.Values) string {
	check := checkString(flatten(fields), "hash")
	fields.Set("hash", hex.EncodeToString(mac(mac([]byte("WebAppData"), token), check)))
	return fields.Encode()
}

func flatten(v url.Values) map[string]string {
	m := map[string]string{}
	for k := range v {
		m[k] = v.Get(k)
	}
	return m
}

func now(offset time.Duration) string { return strconv.FormatInt(time.Now().Add(offset).Unix(), 10) }

func TestValidateRejects(t *testing.T) {
	const token = "123:secret"
	fresh := func(extra ...string) url.Values {
		v := url.Values{"query_id": {"Q1"}, "user": {`{"id":7,"first_name":"Ann"}`}, "auth_date": {now(0)}}
		for i := 0; i+1 < len(extra); i += 2 {
			v.Set(extra[i], extra[i+1])
		}
		return v
	}
	valid := signHMAC(token, fresh())
	tests := []struct {
		name     string
		initData string
		token    string
		maxAge   time.Duration
		want     error // nil for a usage error that matches neither sentinel
	}{
		{"another token", valid, "123:other", time.Hour, ErrInvalid},
		{"value changed", strings.Replace(valid, "Ann", "Bob", 1), token, time.Hour, ErrInvalid},
		{"field removed", strings.Replace(valid, "query_id=Q1&", "", 1), token, time.Hour, ErrInvalid},
		{"field added", valid + "&admin=true", token, time.Hour, ErrInvalid},
		{"no hash", strings.Split(valid, "&hash=")[0], token, time.Hour, ErrInvalid},
		{"hash not hex", strings.Split(valid, "&hash=")[0] + "&hash=zz", token, time.Hour, ErrInvalid},
		{"hash twice", valid + "&hash=" + strings.Split(valid, "&hash=")[1], token, time.Hour, ErrInvalid},
		{"not a query string", "a=%zz", token, time.Hour, ErrInvalid},
		{"empty", "", token, time.Hour, ErrInvalid},
		{"expired", signHMAC(token, fresh("auth_date", now(-2*time.Hour))), token, time.Hour, ErrExpired},
		{"from the future", signHMAC(token, fresh("auth_date", now(5*time.Minute))), token, time.Hour, ErrInvalid},
		{"no auth_date", signHMAC(token, url.Values{"query_id": {"Q1"}}), token, time.Hour, ErrInvalid},
		{"user not an object", signHMAC(token, fresh("user", "42")), token, time.Hour, ErrInvalid},
		{"can_send_after not a number", signHMAC(token, fresh("can_send_after", "soon")), token, time.Hour, ErrInvalid},
		{"empty token", valid, "", time.Hour, nil},
		{"no maxAge", valid, token, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Validate(tt.initData, tt.token, tt.maxAge)
			switch {
			case d != nil || err == nil:
				t.Fatalf("Validate() = %+v, %v; want an error", d, err)
			case tt.want != nil && !errors.Is(err, tt.want):
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			case tt.want == nil && (errors.Is(err, ErrInvalid) || errors.Is(err, ErrExpired)):
				t.Errorf("Validate() = %v, want a usage error", err)
			}
			if strings.Contains(err.Error(), "Ann") || strings.Contains(err.Error(), "Q1") {
				t.Errorf("the error quotes the data: %v", err)
			}
		})
	}
	if d, err := Validate(valid, token, time.Hour); err != nil || d.User.FirstName != "Ann" {
		t.Errorf("Validate(valid) = %+v, %v", d, err)
	}
}

func TestValidateSignature(t *testing.T) {
	const botID = 5768337691
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(botID int64, fields url.Values) url.Values {
		message := strconv.FormatInt(botID, 10) + ":WebAppData\n" + checkString(flatten(fields), "hash", "signature")
		fields.Set("signature", base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(message))))
		fields.Set("hash", strings.Repeat("ab", 32)) // a third party cannot check it, and does not need to
		return fields
	}
	fields := func() url.Values {
		return url.Values{"user": {`{"id":7,"first_name":"Ann"}`}, "chat_type": {"sender"}, "auth_date": {now(0)}}
	}
	valid := sign(botID, fields()).Encode()
	padded := sign(botID, fields())
	padded.Set("signature", padded.Get("signature")+"==")
	tests := []struct {
		name     string
		initData string
		botID    int64
		key      ed25519.PublicKey
		want     error // nil for success, or a usage error when usage is set
		usage    bool
	}{
		{"valid", valid, botID, pub, nil, false},
		{"padded signature", padded.Encode(), botID, pub, nil, false},
		{"another bot", valid, botID + 1, pub, ErrInvalid, false},
		{"Telegram's key on our signature", valid, botID, ProductionKey, ErrInvalid, false},
		{"value changed", strings.Replace(valid, "Ann", "Bob", 1), botID, pub, ErrInvalid, false},
		{"no signature", signHMAC("1:x", fields()), botID, pub, ErrInvalid, false},
		{"expired", sign(botID, url.Values{"auth_date": {now(-2 * time.Hour)}}).Encode(), botID, pub, ErrExpired, false},
		{"no bot ID", valid, 0, pub, nil, true},
		{"short key", valid, botID, pub[:16], nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := ValidateSignature(tt.initData, tt.botID, tt.key, time.Hour)
			switch {
			case tt.usage:
				if err == nil || errors.Is(err, ErrInvalid) || errors.Is(err, ErrExpired) {
					t.Errorf("ValidateSignature() = %v, want a usage error", err)
				}
			case tt.want == nil:
				if err != nil || d.User == nil || d.User.FirstName != "Ann" {
					t.Errorf("ValidateSignature() = %+v, %v", d, err)
				}
			case !errors.Is(err, tt.want):
				t.Errorf("ValidateSignature() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestKeys(t *testing.T) {
	for name, tt := range map[string]struct {
		key ed25519.PublicKey
		hex string
	}{
		"production": {ProductionKey, "e7bf03a2fa4602af4580703d88dda5bb59f32ed8b02a56c187fe7d34caed242d"},
		"test":       {TestKey, "40055058a4ee38156a06562e52eece92a771bcd8346a8c4615cb7376eddf72ec"},
	} {
		if hex.EncodeToString(tt.key) != tt.hex {
			t.Errorf("%s key = %x, want the key published by Telegram", name, []byte(tt.key))
		}
	}
}
