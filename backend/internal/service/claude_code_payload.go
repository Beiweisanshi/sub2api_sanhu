package service

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/google/uuid"
)

// generateSessionString generates a Claude Code style session string.
// The output format is determined by the provided UA header value (falling back
// to claude.DefaultHeaders), ensuring consistency between the user_id format
// and the UA sent to upstream.
func generateSessionString(uaHeader string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	hex64 := hex.EncodeToString(b)
	sessionUUID := uuid.New().String()
	if uaHeader == "" {
		uaHeader = claude.DefaultHeaders["User-Agent"]
	}
	uaVersion := ExtractCLIVersion(uaHeader)
	return FormatMetadataUserID(hex64, "", sessionUUID, uaVersion), nil
}

// createClaudeCodeStylePayload creates a Claude Code style request payload.
// uaHeader provides the UA value used to derive the metadata user_id suffix.
func createClaudeCodeStylePayload(modelID string, uaHeader string, prompt string, maxTokens int, stream bool) (map[string]any, error) {
	sessionID, err := generateSessionString(uaHeader)
	if err != nil {
		return nil, err
	}
	if prompt == "" {
		prompt = "hi"
	}
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	return map[string]any{
		"model": modelID,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": prompt,
						"cache_control": map[string]string{
							"type": "ephemeral",
						},
					},
				},
			},
		},
		"system": []map[string]any{
			{
				"type": "text",
				"text": claudeCodeSystemPrompt,
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		},
		"metadata": map[string]string{
			"user_id": sessionID,
		},
		"max_tokens":  maxTokens,
		"temperature": 1,
		"stream":      stream,
	}, nil
}

// createTestPayload creates a Claude Code style test request payload.
// uaHeader provides the UA value used to derive the metadata user_id suffix.
func createTestPayload(modelID string, uaHeader string) (map[string]any, error) {
	return createClaudeCodeStylePayload(modelID, uaHeader, "hi", 1024, true)
}
