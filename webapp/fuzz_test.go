package webapp

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// FuzzValidate: whatever the input, Validate does not panic, fails only with its errors, and
// accepts nothing but the data of the first vector, however it is encoded, with the hex digits of
// its hash in either case.
func FuzzValidate(f *testing.F) {
	for _, v := range vectors {
		f.Add(v.initData)
	}
	f.Add("")
	f.Add("a=%zz&hash=00")
	// The hash in upper case, which the fuzzer found valid: the same bytes, so the same data.
	head, hash, _ := strings.Cut(vectors[0].initData, "&hash=")
	f.Add(head + "&hash=" + strings.ToUpper(hash))
	token := vectors[0].token
	want, err := Validate(vectors[0].initData, token, sinceVectors)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, initData string) {
		d, err := Validate(initData, token, sinceVectors)
		if err != nil {
			if !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrExpired) {
				t.Fatalf("Validate() = %v, want ErrInvalid or ErrExpired", err)
			}
			return
		}
		same := *d
		if strings.EqualFold(same.Hash, want.Hash) {
			same.Hash = want.Hash
		}
		if !reflect.DeepEqual(&same, want) {
			t.Fatalf("Validate() accepted %q as %+v", initData, d)
		}
	})
}

// FuzzValidateSignature is FuzzValidate for Ed25519 signatures, with a key made from a fixed seed.
func FuzzValidateSignature(f *testing.F) {
	const botID = 42
	priv := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	fields := url.Values{"user": {`{"id":7,"first_name":"Ann"}`}, "auth_date": {strconv.FormatInt(time.Now().Unix(), 10)}}
	message := strconv.Itoa(botID) + ":WebAppData\n" + checkString(flatten(fields), "hash", "signature")
	fields.Set("signature", base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(message))))
	valid := fields.Encode()
	key := priv.Public().(ed25519.PublicKey)
	want, err := ValidateSignature(valid, botID, key, time.Hour)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add(vectors[1].initData)
	f.Fuzz(func(t *testing.T, initData string) {
		d, err := ValidateSignature(initData, botID, key, time.Hour)
		if err != nil {
			if !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrExpired) {
				t.Fatalf("ValidateSignature() = %v, want ErrInvalid or ErrExpired", err)
			}
			return
		}
		if d.User == nil || !reflect.DeepEqual(d.User, want.User) || !d.AuthDate.Equal(want.AuthDate) {
			t.Fatalf("ValidateSignature() accepted %q as %+v", initData, d)
		}
	})
}
