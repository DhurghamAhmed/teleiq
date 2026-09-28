package teleiqtest_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq/teleiqtest"
	"github.com/DhurghamAhmed/teleiq/webapp"
)

func TestWebAppInitData(t *testing.T) {
	// The hash of the same data computed with Python's hmac and hashlib.
	old := url.Values{"auth_date": {"1790000000"}, "some_new_field": {"kept in the check"}}
	got, err := url.ParseQuery(teleiqtest.WebAppInitData("2:B", old))
	if err != nil || got.Get("hash") != "47017f886adae69ae9c1d315946526c2ae0805c0acaee541e27de052563a83d5" {
		t.Errorf("WebAppInitData() = %v, %v; want the hash computed independently", got, err)
	}

	tests := []struct {
		name   string
		fields url.Values
		check  func(*webapp.InitData) bool
	}{
		{"no fields", nil, func(d *webapp.InitData) bool { return time.Since(d.AuthDate) < time.Minute }},
		{"user and query", url.Values{"query_id": {"Q"}, "user": {`{"id":7,"first_name":"User7"}`}},
			func(d *webapp.InitData) bool { return d.QueryID == "Q" && d.User.ID == 7 }},
		{"keys that prefix others", url.Values{"a": {"1"}, "a-b": {"2"}, "a_b": {"3"}, "start_param": {"x y+z"}},
			func(d *webapp.InitData) bool { return d.StartParam == "x y+z" }},
		{"a hash given is replaced", url.Values{"hash": {"00"}, "chat_type": {"sender"}},
			func(d *webapp.InitData) bool { return d.ChatType == "sender" }},
		{"only the first value counts", url.Values{"chat_type": {"group", "private"}},
			func(d *webapp.InitData) bool { return d.ChatType == "group" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := url.Values{}
			for k, v := range tt.fields {
				before[k] = append([]string(nil), v...)
			}
			d, err := webapp.Validate(teleiqtest.WebAppInitData(teleiqtest.Token, tt.fields), teleiqtest.Token, time.Minute)
			if err != nil || !tt.check(d) {
				t.Fatalf("webapp.Validate() = %+v, %v", d, err)
			}
			if tt.fields != nil && tt.fields.Encode() != before.Encode() {
				t.Errorf("fields changed to %v, want %v", tt.fields, before)
			}
		})
	}
}
