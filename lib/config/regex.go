package config

import (
	"github.com/wasilibs/go-re2"
)

// Regexp adds unmarshalling from json for regexp.Regexp
type Regexp struct {
	*re2.Regexp
}

// UnmarshalText unmarshals json into a regexp.Regexp
func (r *Regexp) UnmarshalText(b []byte) error {
	regex, err := re2.Compile(string(b))
	if err != nil {
		return err
	}
	r.Regexp = regex
	return nil
}
