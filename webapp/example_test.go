package webapp_test

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/DhurghamAhmed/teleiq/teleiqtest"
	"github.com/DhurghamAhmed/teleiq/webapp"
)

// The server of a Mini App checks the data that its page posted before using it. The example
// builds the data with package teleiqtest, as a test of the server would; in production, the page
// reads it from Telegram.WebApp.initData.
func ExampleValidate() {
	initData := teleiqtest.WebAppInitData(teleiqtest.Token, url.Values{
		"query_id": {"AAHdF6IQ"},
		"user":     {`{"id":7,"first_name":"Ann","language_code":"en"}`},
	})

	d, err := webapp.Validate(initData, teleiqtest.Token, 24*time.Hour)
	if err != nil {
		fmt.Println("rejected:", err)
		return
	}
	fmt.Println(d.User.FirstName, d.User.LanguageCode, d.QueryID)

	// Data changed on its way to the server is rejected.
	_, err = webapp.Validate(strings.Replace(initData, "Ann", "Eve", 1), teleiqtest.Token, 24*time.Hour)
	fmt.Println(errors.Is(err, webapp.ErrInvalid))
	// Output:
	// Ann en AAHdF6IQ
	// true
}
