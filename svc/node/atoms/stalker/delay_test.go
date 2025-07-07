package stalker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelayTracker(t *testing.T) {
	d := newDelayTracker(3, 0, 100)

	d.Add(3)
	require.Equal(t, d.Mean(), 3)
	d.Add(3)
	require.Equal(t, d.Mean(), 3)
	d.Add(3)
	require.Equal(t, d.Mean(), 3)

	d.Add(6)
	require.Equal(t, d.Mean(), 4)
	d.Add(6)
	require.Equal(t, d.Mean(), 5)
	d.Add(6)
	require.Equal(t, d.Mean(), 6)

	d.Add(3)
	require.Equal(t, d.Mean(), 5)
	d.Add(3)
	require.Equal(t, d.Mean(), 4)
	d.Add(3)
	require.Equal(t, d.Mean(), 3)
}
