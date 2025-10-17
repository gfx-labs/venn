package subctx

import (
	"context"
	"github.com/gfx-labs/venn/lib/config"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSubCtx(t *testing.T) {
	ctx := context.Background()
	c, err := GetChain(ctx)
	assert.Error(t, err)
	assert.Nil(t, c)
	b := IsInternal(ctx)
	assert.False(t, b)

	ctx = WithChain(ctx, &config.Chain{})
	c, err = GetChain(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	b = IsInternal(ctx)
	assert.False(t, b)

	ctx = WithInternal(ctx, true)
	c, err = GetChain(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	b = IsInternal(ctx)
	assert.True(t, b)
}
