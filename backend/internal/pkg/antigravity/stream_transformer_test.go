//go:build unit

package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplySimulateCacheDeterministic_PreservesUpstreamCacheRead(t *testing.T) {
	newInput, newCacheRead, newCacheCreation := applySimulateCacheDeterministic(
		40, 60, 0.8, true, 0.5, 0,
	)

	require.Equal(t, 20, newInput)
	require.Equal(t, 70, newCacheRead)
	require.Equal(t, 10, newCacheCreation)
}

func TestApplySimulateCacheDeterministic_UpperJitterOnly(t *testing.T) {
	newInput, newCacheRead, newCacheCreation := applySimulateCacheDeterministic(
		100, 0, 0.6, false, 0, 0.02,
	)

	require.Equal(t, 38, newInput)
	require.Equal(t, 62, newCacheRead)
	require.Equal(t, 0, newCacheCreation)
}
