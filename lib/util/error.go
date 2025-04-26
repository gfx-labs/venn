package util

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"gfx.cafe/open/jrpc/pkg/jsonrpc"
)

var ErrChainNotFound = errors.New("chain not found")

func NewErrChainNotFound(chain string) error {
	return fmt.Errorf("%w: %s", ErrChainNotFound, chain)
}

func GetChain[T any, U ~map[string]T](chain string, m U) (t T, err error) {
	c, ok := m[chain]
	if !ok {
		return t, NewErrChainNotFound(chain)
	}
	return c, nil
}

var UserError = errors.New("user error")

func IsUserError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, UserError) {
		return true
	}

	if errors.Is(err, subscription.ErrNotificationsUnsupported) {
		return true
	}

	var codecError jsonrpc.Error
	if errors.As(err, &codecError) {
		// from eip-1474
		switch codecError.ErrorCode() {
		case -32601, // method not found
			-32603, // internal error
			-32001, // resource not found
			-32002, // resource unavailable
			-32005: // limit exceeded
			return false
		case -32000: // invalid params (special case because most invalid params are user error)
			return !strings.Contains(codecError.Error(), "not found")
		default:
			return true
		}
	}

	// request took too long
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	// by default just assume that it's something up with the endpoint

	return false
}

func startsWithIgnoreCase(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}

	for _, expected := range needle {
		actual, size := utf8.DecodeRuneInString(haystack)
		if actual == utf8.RuneError {
			return false
		}
		haystack = haystack[size:]

		if unicode.ToUpper(actual) != unicode.ToUpper(expected) {
			return false
		}
	}

	return true
}

func containsIgnoreCase(haystack, needle string) bool {
	for {
		if len(needle) > len(haystack) {
			return false
		}

		if startsWithIgnoreCase(haystack, needle) {
			return true
		}

		// advance
		r, size := utf8.DecodeRuneInString(haystack)
		if r == utf8.RuneError {
			return false
		}
		haystack = haystack[size:]
	}
}

func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	var codecError jsonrpc.Error
	if errors.As(err, &codecError) {
		switch codecError.ErrorCode() {
		case -32005: // limit exceeded
			return true
		}
	}

	var httpError *jsonrpc.HTTPError
	if errors.As(err, &httpError) {
		switch httpError.StatusCode {
		case http.StatusTooManyRequests:
			return true
		}
	}

	if containsIgnoreCase(err.Error(), "limit") || containsIgnoreCase(err.Error(), "rate") {
		return true
	}
	return false
}

func IsNodeError(err error) bool {
	if err == nil {
		return false
	}

	var jsonRpcError jsonrpc.Error
	if errors.As(err, &jsonRpcError) {
		return false
	}

	var httpError *jsonrpc.HTTPError
	if errors.As(err, &httpError) {
		switch httpError.StatusCode {
		case http.StatusTooManyRequests, http.StatusBadRequest, http.StatusRequestEntityTooLarge:
			return false
		default:
			return true
		}
	}

	return true
}
