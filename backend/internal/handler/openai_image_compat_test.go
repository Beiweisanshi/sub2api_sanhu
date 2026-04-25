package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildOpenAIImageCompatRequestBody_ChatCompletions(t *testing.T) {
	body := []byte(`{
		"model":"gpt-image-2",
		"messages":[
			{"role":"system","content":"You are concise."},
			{"role":"user","content":[
				{"type":"text","text":"draw a red bicycle"},
				{"type":"image_url","image_url":{"url":"https://example.com/input.png"}}
			]}
		],
		"size":"1024x1024",
		"n":2,
		"stream":false,
		"max_tokens":128
	}`)

	got, err := buildOpenAIImageCompatRequestBody(body, openAIImageCompatSourceChatCompletions, "gpt-image-2")
	require.NoError(t, err)
	require.True(t, gjson.ValidBytes(got))
	require.Equal(t, "gpt-image-2", gjson.GetBytes(got, "model").String())
	require.Equal(t, "draw a red bicycle", gjson.GetBytes(got, "prompt").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(got, "size").String())
	require.Equal(t, int64(2), gjson.GetBytes(got, "n").Int())
	require.False(t, gjson.GetBytes(got, "messages").Exists())
	require.False(t, gjson.GetBytes(got, "max_tokens").Exists())
}

func TestBuildOpenAIImageCompatRequestBody_Responses(t *testing.T) {
	body := []byte(`{
		"model":"gpt-image-2",
		"previous_response_id":"resp_should_be_dropped",
		"input":[
			{"role":"developer","content":[{"type":"input_text","text":"high contrast"}]},
			{"role":"user","content":[{"type":"input_text","text":"a clean product render"}]}
		],
		"quality":"high",
		"response_format":"b64_json"
	}`)

	got, err := buildOpenAIImageCompatRequestBody(body, openAIImageCompatSourceResponses, "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "a clean product render", gjson.GetBytes(got, "prompt").String())
	require.Equal(t, "high", gjson.GetBytes(got, "quality").String())
	require.Equal(t, "b64_json", gjson.GetBytes(got, "response_format").String())
	require.False(t, gjson.GetBytes(got, "input").Exists())
	require.False(t, gjson.GetBytes(got, "previous_response_id").Exists())
}

func TestBuildOpenAIImageCompatRequestBody_DropsChatResponseFormatObject(t *testing.T) {
	body := []byte(`{
		"model":"gpt-image-2",
		"input":"draw a glass teapot",
		"response_format":{"type":"json_schema","json_schema":{"name":"ignored"}}
	}`)

	got, err := buildOpenAIImageCompatRequestBody(body, openAIImageCompatSourceResponses, "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "draw a glass teapot", gjson.GetBytes(got, "prompt").String())
	require.False(t, gjson.GetBytes(got, "response_format").Exists())
}

func TestBuildOpenAIImageCompatRequestBody_KeepsImageResponseFormatString(t *testing.T) {
	body := []byte(`{
		"model":"gpt-image-2",
		"input":"draw a glass teapot",
		"response_format":"url"
	}`)

	got, err := buildOpenAIImageCompatRequestBody(body, openAIImageCompatSourceResponses, "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "url", gjson.GetBytes(got, "response_format").String())
}

func TestBuildOpenAIImageCompatRequestBody_Messages(t *testing.T) {
	body := []byte(`{
		"model":"gpt-image-2",
		"messages":[
			{"role":"user","content":[{"type":"text","text":"cinematic city skyline"}]}
		],
		"style":"vivid"
	}`)

	got, err := buildOpenAIImageCompatRequestBody(body, openAIImageCompatSourceMessages, "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "cinematic city skyline", gjson.GetBytes(got, "prompt").String())
	require.Equal(t, "vivid", gjson.GetBytes(got, "style").String())
	require.False(t, gjson.GetBytes(got, "messages").Exists())
}

func TestBuildOpenAIImageCompatRequestBody_RequiresPrompt(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x"}}]}]}`)

	_, err := buildOpenAIImageCompatRequestBody(body, openAIImageCompatSourceChatCompletions, "gpt-image-2")
	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt is required")
}
