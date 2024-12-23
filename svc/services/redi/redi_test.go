package redi

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"gfx.cafe/gfx/venn/lib/config"
)

func TestRedis_CompareAndSwapIfGreater(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	rediResult, err := New(RedisParams{
		Log: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Config: &config.Redis{
			Namespace: "test",
		},
		Lc: lc,
	})
	require.NoError(t, err)
	redi := rediResult.Redis
	lc.RequireStart()
	defer lc.RequireStop()
	_, err = redi.C().Set(context.Background(), "test", 1, 0).Result()
	require.NoError(t, err)
	old, err := redi.CompareAndSwapIfGreater(context.Background(), "test", 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, old)
	res, err := redi.C().Get(context.Background(), "test").Result()
	require.NoError(t, err)
	require.EqualValues(t, "1", res)
	old, err = redi.CompareAndSwapIfGreater(context.Background(), "test", 2)
	if err != nil {
		t.Error(err)
		return
	}
	if old != 1 {
		t.Errorf("expected old to be 1 but got %d", old)
		return
	}
	res, err = redi.C().Get(context.Background(), "test").Result()
	if err != nil {
		t.Error(err)
		return
	}
	// make sure res changed
	if res != "2" {
		t.Errorf("expected get to be 2 but got %s", res)
		return
	}
}
