package schema

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bots/api" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<html>page</html>"))
	}))
	t.Cleanup(srv.Close)
	tests := []struct {
		name, url, in, want, wantErr string
	}{
		{name: "from a server", url: srv.URL + "/bots/api", want: "<html>page</html>"},
		{name: "from a file", url: srv.URL + "/missing", in: "testdata/page.html", want: "<!DOCTYPE html>"},
		{name: "a server error", url: srv.URL + "/missing", wantErr: "404"},
		{name: "a missing file", in: "no-such-page.html", wantErr: "no such file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadPage(tt.url, tt.in)
			switch {
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ReadPage() error = %v, want %q", err, tt.wantErr)
				}
			case err != nil:
				t.Fatal(err)
			case !strings.HasPrefix(string(got), tt.want):
				t.Errorf("ReadPage() = %.40q, want it to start with %q", got, tt.want)
			}
		})
	}
}
