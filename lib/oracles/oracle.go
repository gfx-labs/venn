package oracles

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"go.uber.org/multierr"
	"golang.org/x/sync/singleflight"
)

type OracleFunc[T any] func(ctx context.Context) (T, error)

func (o OracleFunc[T]) Report(ctx context.Context) (T, error) {
	return o(ctx)
}

type Oracle[T any] interface {
	Report(ctx context.Context) (T, error)
}

// MaxTrimOracle is MaxOracle, except it trims the min and max when there are more than 4 oracles
func MaxTrimOracle[T cmp.Ordered](oracles []Oracle[T]) Oracle[T] {
	return OracleFunc[T](func(ctx context.Context) (t T, err error) {
		var merr error
		responses := []T{}
		for _, v := range oracles {
			ans, err := v.Report(ctx)
			if err != nil {
				merr = multierr.Append(merr, err)
				continue
			}
			responses = append(responses, ans)
		}
		if len(responses) == 0 {
			return t, merr
		}
		slices.Sort(responses)
		if len(responses) >= 5 {
			responses = responses[1 : len(responses)-1]
		}
		var best T
		for _, v := range responses {
			if v > best {
				best = v
			}
		}
		// return the highest. there has to be 1 response here.
		return best, nil
	})
}

// more complicated oracles
func MaxOracle[T cmp.Ordered](oracles []Oracle[T]) Oracle[T] {
	return OracleFunc[T](func(ctx context.Context) (t T, err error) {
		var merr error
		var best T
		var hit bool
		for _, v := range oracles {
			ans, err := v.Report(ctx)
			if err != nil {
				merr = multierr.Append(merr, err)
				continue
			}
			if ans > best {
				hit = true
				best = ans
			}
		}
		if !hit {
			return t, merr
		}
		return best, nil
	})
}

func MedianOracle[T cmp.Ordered](oracles []Oracle[T]) Oracle[T] {
	return OracleFunc[T](func(ctx context.Context) (t T, err error) {
		var merr error
		responses := []T{}
		for _, v := range oracles {
			ans, err := v.Report(ctx)
			if err != nil {
				merr = multierr.Append(merr, err)
				continue
			}
			responses = append(responses, ans)
		}
		if len(responses) == 0 {
			return t, merr
		}
		slices.Sort(responses)
		return responses[len(responses)/2], nil
	})
}

// basic oracles

// adds a timeout to the context
func TimeoutOracle[T any](primary Oracle[T], timeout time.Duration) Oracle[T] {
	return OracleFunc[T](func(ctx context.Context) (t T, err error) {
		ctx, cn := context.WithTimeout(ctx, timeout)
		defer cn()
		return primary.Report(ctx)
	})
}

// FallbackOracle tries fallback if primary fails
func FallbackOracle[T any](primary, fallback Oracle[T]) Oracle[T] {
	return OracleFunc[T](func(ctx context.Context) (t T, err error) {
		res, err1 := primary.Report(ctx)
		if err1 == nil {
			return res, nil
		}
		res, err = fallback.Report(ctx)
		if err != nil {
			return t, multierr.Append(err1, err)
		}
		return res, nil
	})
}

func MutexOracle[T any](primary Oracle[T]) Oracle[T] {
	var mu sync.Mutex
	return OracleFunc[T](func(ctx context.Context) (t T, err error) {
		mu.Lock()
		defer mu.Unlock()
		res, err := primary.Report(ctx)
		if err != nil {
			return t, err
		}
		return res, nil
	})
}

func SingleFlightOracle[T any](primary Oracle[T]) Oracle[T] {
	var sf singleflight.Group
	return OracleFunc[T](func(ctx context.Context) (t T, err error) {
		res, err, _ := sf.Do("", func() (interface{}, error) {
			return primary.Report(ctx)
		})
		if err != nil {
			return t, err
		}
		return res.(T), nil
	})
}

// CacheOracle caches non-error results
func TtlOracle[T any](primary Oracle[T], ttl time.Duration) Oracle[T] {
	expireTime := time.Time{}
	var lastValue T
	return OracleFunc[T](func(ctx context.Context) (t T, err error) {
		// check expiration
		if expireTime.Sub(time.Now()) > 0 {
			return lastValue, nil
		}
		res, err := primary.Report(ctx)
		if err != nil {
			expireTime = time.Time{}
			return t, err
		}
		expireTime = time.Now().Add(ttl)
		lastValue = res
		return res, nil
	})
}
