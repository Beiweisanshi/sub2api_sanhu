package service

import "testing"

import "github.com/stretchr/testify/require"

func TestNormalizeOpenAIMessagesDispatchModelConfig(t *testing.T) {
	t.Parallel()

	cfg := normalizeOpenAIMessagesDispatchModelConfig(OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   " gpt-5.4-high ",
		SonnetMappedModel: "gpt-5.3-codex",
		HaikuMappedModel:  " gpt-5.4-mini-medium ",
		ExactModelMappings: map[string]string{
			" claude-sonnet-4-5-20250929 ": " gpt-5.2-high ",
			"":                             "gpt-5.4",
			"claude-opus-4-6":              " ",
		},
	})

	require.Equal(t, "gpt-5.4-high", cfg.OpusMappedModel)
	require.Equal(t, "gpt-5.3-codex", cfg.SonnetMappedModel)
	require.Equal(t, "gpt-5.4-mini-medium", cfg.HaikuMappedModel)
	require.Equal(t, map[string]string{
		"claude-sonnet-4-5-20250929": "gpt-5.2-high",
	}, cfg.ExactModelMappings)
}

func TestResolveMessagesDispatchModelWithReasoning(t *testing.T) {
	t.Parallel()

	group := &Group{
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:  "gpt-5.5-xhigh",
			HaikuMappedModel: "gpt-5.4-mini-medium",
			ExactModelMappings: map[string]string{
				"claude-sonnet-4-5-20250929": "gpt-5.3-codex-spark-high",
			},
		},
	}

	opus := group.ResolveMessagesDispatchModelWithReasoning("claude-opus-4-7")
	require.Equal(t, "gpt-5.5", opus.Model)
	require.Equal(t, "xhigh", opus.ReasoningEffort)
	require.Equal(t, "gpt-5.5", group.ResolveMessagesDispatchModel("claude-opus-4-7"))

	haiku := group.ResolveMessagesDispatchModelWithReasoning("claude-haiku-4-5")
	require.Equal(t, "gpt-5.4-mini", haiku.Model)
	require.Equal(t, "medium", haiku.ReasoningEffort)

	sonnet := group.ResolveMessagesDispatchModelWithReasoning("claude-sonnet-4-5-20250929")
	require.Equal(t, "gpt-5.3-codex-spark", sonnet.Model)
	require.Equal(t, "high", sonnet.ReasoningEffort)
}
