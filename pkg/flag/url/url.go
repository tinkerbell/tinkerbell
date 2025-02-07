package url

import (
	"fmt"
	"net/url"

	"github.com/go-playground/validator/v10"
)

type URL struct{ *url.URL }

func (u *URL) String() string {
	if u.URL == nil {
		return ""
	}
	return u.URL.String()
}

func (u *URL) Set(s string) error {
	if s == "" {
		return nil
	}
	v := validator.New()
	if err := v.Var(s, "http_url"); err != nil {
		return fmt.Errorf("invalid URL: %q", s)
	}
	ur, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %q", s)
	}
	*u.URL = *ur

	return nil
}

func (u *URL) Reset() error {
	*u.URL = url.URL{}

	return nil
}

func (u *URL) Type() string {
	return "url"
}
