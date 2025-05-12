package config

import (
	"encoding/json"
	"net/url"
)

type URL struct {
	url.URL
}

func (u *URL) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	ur, err := url.Parse(s)
	if err != nil {
		return err
	}
	u.URL = *ur
	return nil
}

func (u *URL) MarshalJSON() ([]byte, error) {
	return []byte(u.String()), nil
}
