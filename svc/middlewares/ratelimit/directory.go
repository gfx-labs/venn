package ratelimit

import (
	"encoding"
	"sync"
	"time"
)

// we implement a directory which stores whether or not users are banned
// and how long if so they are banned for
type Directory struct {
	mu     sync.RWMutex
	untils map[string]int64
}

type TextSerializable struct {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
}

type GenericEntry[T ~string | TextSerializable] struct {
	User   T      `gtrs:"user"`
	Action string `gtrs:"action"`
	Until  int64  `gtrs:"until"`
}

type Entry struct {
	User   string `gtrs:"user"`
	Action string `gtrs:"action"`
	Until  int64  `gtrs:"until"`
}

func NewDirectory() *Directory {
	return &Directory{
		untils: make(map[string]int64),
	}
}

// if the returned time is not a zero time, it means they are rate limited  until the provided time
func (d *Directory) Check(user string) time.Time {
	d.mu.RLock()
	val, ok := d.untils[user]
	d.mu.RUnlock()
	if !ok {
		return time.Time{}
	}
	return time.Unix(val, 0)
}

// sending 0 value for until is an unban
func (d *Directory) Ban(e *Entry) {
	// use the zero value - zero value of until means to unban
	if e.Until == 0 {
		d.mu.Lock()
		delete(d.untils, e.User)
		d.mu.Unlock()
		return
	}
	now := time.Now()
	// this is a no-op (don't ban or unban if supplied with invalid time)
	if e.Until < now.Unix() {
		return
	}
	// set the rate limit
	d.mu.Lock()
	d.untils[e.User] = e.Until
	d.mu.Unlock()
}
