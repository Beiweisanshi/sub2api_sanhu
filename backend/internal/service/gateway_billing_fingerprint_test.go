package service

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectFP mirrors zhima2api's algorithm so we can assert from first principles
// instead of repeating the hash output in each test case.
func expectFP(first, version string) string {
	runes := []rune(first)
	at := func(i int) string {
		if i < 0 || i >= len(runes) {
			return undefinedMarker
		}
		return string(runes[i])
	}
	sum := sha256.Sum256([]byte(billingFingerprintSalt + at(4) + at(7) + at(20) + version))
	return hex.EncodeToString(sum[:])[:3]
}

func TestComputeCCVersionFingerprint(t *testing.T) {
	t.Run("long ascii message indexes all three positions", func(t *testing.T) {
		first := "The quick brown fox jumps over the lazy dog"
		body := []byte(`{"messages":[{"role":"user","content":"` + first + `"}]}`)
		got := ComputeCCVersionFingerprint(body, "2.1.22")
		require.Equal(t, expectFP(first, "2.1.22"), got)
	})

	t.Run("content blocks: text block is extracted", func(t *testing.T) {
		first := "hello world from structured content"
		body := []byte(`{"messages":[{"role":"assistant","content":"ignored"},{"role":"user","content":[{"type":"image","source":{}},{"type":"text","text":"` + first + `"}]}]}`)
		got := ComputeCCVersionFingerprint(body, "2.1.81")
		require.Equal(t, expectFP(first, "2.1.81"), got)
	})

	t.Run("short message: indexes beyond length fall back to 'undefined'", func(t *testing.T) {
		first := "hi" // length 2 → all three indices are undefined
		body := []byte(`{"messages":[{"role":"user","content":"` + first + `"}]}`)
		got := ComputeCCVersionFingerprint(body, "2.1.22")
		require.Equal(t, expectFP(first, "2.1.22"), got)

		expectedRaw := billingFingerprintSalt + undefinedMarker + undefinedMarker + undefinedMarker + "2.1.22"
		sum := sha256.Sum256([]byte(expectedRaw))
		require.Equal(t, hex.EncodeToString(sum[:])[:3], got, "must match the undefined-collapsed form")
	})

	t.Run("no user message: behaves like empty firstMsg", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"assistant","content":"only assistant here"}]}`)
		got := ComputeCCVersionFingerprint(body, "2.1.22")
		require.Equal(t, expectFP("", "2.1.22"), got)
	})

	t.Run("empty messages array", func(t *testing.T) {
		got := ComputeCCVersionFingerprint([]byte(`{"messages":[]}`), "2.1.22")
		require.Equal(t, expectFP("", "2.1.22"), got)
	})

	t.Run("no messages field at all", func(t *testing.T) {
		got := ComputeCCVersionFingerprint([]byte(`{}`), "2.1.22")
		require.Equal(t, expectFP("", "2.1.22"), got)
	})

	t.Run("image-only content yields empty firstMsg", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"xxx"}}]}]}`)
		got := ComputeCCVersionFingerprint(body, "2.1.22")
		require.Equal(t, expectFP("", "2.1.22"), got)
	})

	t.Run("version participates in hash", func(t *testing.T) {
		first := "The quick brown fox jumps over the lazy dog"
		body := []byte(`{"messages":[{"role":"user","content":"` + first + `"}]}`)
		a := ComputeCCVersionFingerprint(body, "2.1.22")
		b := ComputeCCVersionFingerprint(body, "2.1.23")
		assert.NotEqual(t, a, b, "different versions must hash differently")
	})

	t.Run("output is always 3 lowercase hex chars", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"anything"}]}`)
		got := ComputeCCVersionFingerprint(body, "2.1.22")
		require.Regexp(t, `^[0-9a-f]{3}$`, got)
	})
}

func TestNormalizeCCFingerprint(t *testing.T) {
	t.Run("valid lowercase passes through", func(t *testing.T) {
		require.Equal(t, "a1b", normalizeCCFingerprint("a1b"))
	})
	t.Run("uppercase is lowered", func(t *testing.T) {
		require.Equal(t, "a1b", normalizeCCFingerprint("A1B"))
	})
	t.Run("wrong length falls back to random hex", func(t *testing.T) {
		got := normalizeCCFingerprint("abcd")
		require.Regexp(t, `^[0-9a-f]{3}$`, got)
	})
	t.Run("non-hex characters trigger fallback", func(t *testing.T) {
		got := normalizeCCFingerprint("xyz")
		require.Regexp(t, `^[0-9a-f]{3}$`, got)
	})
	t.Run("empty triggers fallback", func(t *testing.T) {
		got := normalizeCCFingerprint("")
		require.Regexp(t, `^[0-9a-f]{3}$`, got)
	})
}
