package config

import (
	"encoding/json"
	"net/url"
	"os"
)

type EnvExpandable string

func (T *EnvExpandable) MarshalText() ([]byte, error) {
	if T == nil {
		return []byte("<nil>"), nil
	}
	return []byte(*T), nil
}

func (T *EnvExpandable) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(*T))
}

func (T *EnvExpandable) UnmarshalJSON(bts []byte) error {
	var s string
	if err := json.Unmarshal(bts, &s); err != nil {
		return err
	}
	*T = EnvExpandable(os.ExpandEnv(s))
	return nil
}

type SafeUrl string

func (u *SafeUrl) MarshalText() (text []byte, err error) {
	if u == nil {
		return []byte("<nil>"), nil
	}
	urls, err := url.Parse(string(*u))
	if err != nil {
		return nil, err
	}
	return []byte(urls.Scheme + "://" + urls.Host), nil
}

func (u *SafeUrl) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(*u))
}

func (u *SafeUrl) UnmarshalJSON(bts []byte) error {
	var s EnvExpandable
	if err := json.Unmarshal(bts, &s); err != nil {
		return err
	}
	urls, err := url.Parse(string(s))
	if err != nil {
		return err
	}
	urlString := SafeUrl(urls.String())
	*u = urlString
	return nil
}
