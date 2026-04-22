package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateOpenAIChatCompletionsTestPayload(t *testing.T) {
	payload := createOpenAIChatCompletionsTestPayload("doubao-seed-code")

	require.Equal(t, "doubao-seed-code", payload["model"])
	require.Equal(t, true, payload["stream"])
	require.Equal(t, 32, payload["max_tokens"])

	messages, ok := payload["messages"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0]["role"])
	require.Equal(t, "hi", messages[0]["content"])

	_, hasInput := payload["input"]
	require.False(t, hasInput)
	_, hasInstructions := payload["instructions"]
	require.False(t, hasInstructions)
}
