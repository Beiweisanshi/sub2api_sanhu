package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","size":"1024x1024","quality":"high","stream":true}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, "/v1/images/generations", parsed.Endpoint)
	require.Equal(t, "gpt-image-2", parsed.Model)
	require.Equal(t, "draw a cat", parsed.Prompt)
	require.True(t, parsed.Stream)
	require.Equal(t, "1024x1024", parsed.Size)
	require.Equal(t, "1K", parsed.SizeTier)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
	require.False(t, parsed.Multipart)
}

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_MultipartEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "replace background"))
	require.NoError(t, writer.WriteField("size", "1536x1024"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-image-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body.Bytes())
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, "/v1/images/edits", parsed.Endpoint)
	require.True(t, parsed.Multipart)
	require.Equal(t, "gpt-image-2", parsed.Model)
	require.Equal(t, "replace background", parsed.Prompt)
	require.Equal(t, "1536x1024", parsed.Size)
	require.Equal(t, "2K", parsed.SizeTier)
	require.Len(t, parsed.Uploads, 1)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
}

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_PromptOnlyDefaultsRemainBasic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"prompt":"draw a cat"}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, "gpt-image-2", parsed.Model)
	require.Equal(t, OpenAIImagesCapabilityBasic, parsed.RequiredCapability)
}

func TestParseChatGPTDPLFromHTML(t *testing.T) {
	t.Run("extracts dpl from data-build and clientBuildNumber from data-seq (current chatgpt.com)", func(t *testing.T) {
		html := `<!DOCTYPE html><html lang="zh-CN" data-build="prod-2b36f43ae6b1e9017a8e524e83254dd18e2eaae5" data-seq="6097956"><head></head></html>`
		fp := parseChatGPTDPLFromHTML(html)
		require.Equal(t, "prod-2b36f43ae6b1e9017a8e524e83254dd18e2eaae5", fp.dpl)
		require.Equal(t, "6097956", fp.clientBuildNumber)
		require.Empty(t, fp.scripts)
	})

	t.Run("collects legacy c/<hash>/_ scripts when present", func(t *testing.T) {
		html := `<html data-build="prod-current" data-seq="123"><head>
<script src="https://cdn.oaistatic.com/c/abc123XYZ/_/vendor.js"></script>
<script src="https://cdn.oaistatic.com/c/abc123XYZ/_/main.js"></script>
<script src="https://example.com/third-party.js"></script>
<script src="https://cdn.oaistatic.com/c/def456OTHER/_/other.js"></script>
</head></html>`
		fp := parseChatGPTDPLFromHTML(html)
		// 新逻辑：dpl 始终取 data-build
		require.Equal(t, "prod-current", fp.dpl)
		require.Equal(t, "123", fp.clientBuildNumber)
		// 老格式 scripts 仍会被收集（取第一个 legacy segment 对应的脚本）
		require.Len(t, fp.scripts, 2)
		for _, src := range fp.scripts {
			require.Contains(t, src, "abc123XYZ")
		}
	})

	t.Run("falls back to legacy dpl when data-build missing", func(t *testing.T) {
		html := `<html><head>
<script src="https://cdn.oaistatic.com/c/legacyBuild/_/main.js"></script>
</head></html>`
		fp := parseChatGPTDPLFromHTML(html)
		require.Equal(t, "c/legacyBuild/_", fp.dpl)
		require.Len(t, fp.scripts, 1)
		require.Empty(t, fp.clientBuildNumber)
	})

	t.Run("empty input returns empty fingerprint", func(t *testing.T) {
		fp := parseChatGPTDPLFromHTML("")
		require.Empty(t, fp.dpl)
		require.Empty(t, fp.scripts)
		require.Empty(t, fp.clientBuildNumber)
	})
}

func TestFilterOpenAIInputPointerInfos(t *testing.T) {
	inputs := map[string]struct{}{
		"file_input_1": {},
		"file_input_2": {},
	}
	items := []openAIImagePointerInfo{
		{Pointer: "sediment://file_input_1", Prompt: "input"},
		{Pointer: "file-service://file_input_2", Prompt: "input-but-wrapped"},
		{Pointer: "file-service://file_output_3", Prompt: "output"},
		{Pointer: "sediment://file_output_4", Prompt: "another output"},
	}
	got := filterOpenAIInputPointerInfos(items, inputs)
	require.Len(t, got, 2)
	require.Equal(t, "file-service://file_output_3", got[0].Pointer)
	require.Equal(t, "sediment://file_output_4", got[1].Pointer)
}

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_ExplicitSizeRequiresNativeCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"prompt":"draw a cat","size":"1024x1024"}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
}

// 作者: mkx  变更: 2026/04/26
// 429 不再在 Layer 1 重试，统一交给 handler 层 openAISameAccountRetryDelay 决策。
func TestOpenAIImageBackendRetryDelay_429NotRetriedAtLayer1(t *testing.T) {
	for _, retryAfter := range []string{"1", "30", ""} {
		err := &openAIImageStatusError{
			StatusCode: http.StatusTooManyRequests,
			Message:    "rate limited",
			ResponseHeaders: http.Header{
				"Retry-After": []string{retryAfter},
			},
		}
		_, ok := openAIImageBackendRetryDelay(err, 1)
		require.False(t, ok, "429 with Retry-After=%q should not be retried at layer 1", retryAfter)
	}
}

func TestOpenAIImageGenerationRetryableClassification(t *testing.T) {
	require.True(t, isOpenAIImageGenerationRetryable(&openAIImageNoDownloadableError{
		PollTimeout: 120 * time.Second,
	}))

	require.True(t, isOpenAIImageGenerationRetryable(&openAIImageStatusError{
		StatusCode: http.StatusBadGateway,
		Message:    "temporary bad gateway",
	}))

	require.False(t, isOpenAIImageGenerationRetryable(newOpenAIImageStageError(
		"conversation_poll",
		openAIChatGPTConversationURL,
		context.Canceled,
	)))

	require.False(t, isOpenAIImageGenerationRetryable(&openAIImageStatusError{
		StatusCode: http.StatusBadRequest,
		Message:    "invalid request",
	}))

	require.False(t, isOpenAIImageGenerationRetryable(newOpenAIImageStageError(
		"download_result",
		openAIChatGPTFilesURL,
		fmt.Errorf("unsupported image pointer: bad://id"),
	)))
}

func TestDetectOpenAIImageTerminalFailure(t *testing.T) {
	err := detectOpenAIImageTerminalFailure([]byte(`{"error":{"message":"image generation failed upstream"}}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "image generation failed upstream")

	err = detectOpenAIImageTerminalFailure([]byte(`{
		"mapping": {
			"tool-1": {
				"message": {
					"author": {"role": "tool"},
					"status": "failed",
					"metadata": {"async_task_type": "image_gen"},
					"content": {"content_type": "multimodal_text", "parts": []}
				}
			}
		}
	}`))
	require.Error(t, err)
	var terminalErr *openAIImageTerminalFailureError
	require.ErrorAs(t, err, &terminalErr)
	require.Equal(t, "failed", terminalErr.Status)

	err = detectOpenAIImageTerminalFailure([]byte(`{
		"mapping": {
			"tool-1": {
				"message": {
					"author": {"role": "tool"},
					"status": "finished_successfully",
					"metadata": {"async_task_type": "image_gen"},
					"content": {"content_type": "multimodal_text", "parts": [{"asset_pointer":"file-service://file_ok"}]}
				}
			}
		}
	}`))
	require.NoError(t, err)
}

func TestReadOpenAIImageConversationStreamStopsOnContextDeadline(t *testing.T) {
	body := &blockingOpenAIImageStreamBody{done: make(chan struct{})}
	resp := &req.Response{
		Response: &http.Response{Body: body},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, _, _, err := readOpenAIImageConversationStream(ctx, resp, time.Now())

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
}

type blockingOpenAIImageStreamBody struct {
	once sync.Once
	done chan struct{}
}

func (b *blockingOpenAIImageStreamBody) Read(_ []byte) (int, error) {
	<-b.done
	return 0, io.EOF
}

func (b *blockingOpenAIImageStreamBody) Close() error {
	b.once.Do(func() {
		close(b.done)
	})
	return nil
}

// 作者: mkx  变更: 2026/04/23 - 回归之前 PoW answer 的两个 bug：
//  1. part2 没去除尾部 ']' 导致拼接后多一个闭合符
//  2. 字段错位（core+3008 字符串、0.123456 小数、UTC RFC1123 等），生成的不是合法浏览器指纹
//
// 低难度下能解出，且解出的答案 base64 解码后必须是合法 JSON，字段数=18，索引 3/9 分别被 i/i>>1 填入。
func TestGenerateOpenAIChallengeAnswer_StructuredPayload(t *testing.T) {
	// 准备缓存，让 buildOpenAIPowConfig 拿到稳定的 dpl/script
	storeChatGPTFingerprint(chatGPTClientFingerprint{
		dpl:               "prod-test-dpl",
		scripts:           []string{"https://cdn.oaistatic.com/sentinel.js"},
		clientBuildNumber: "123",
	})
	defer storeChatGPTFingerprint(chatGPTClientFingerprint{})

	config := buildOpenAIPowConfig("Mozilla/5.0 TestUA")
	require.Len(t, config, 18, "config must have exactly 18 fields to match chat2api")

	// 难度 "ff"（一字节高位即可满足）—— 几次循环内一定能解出
	answer, solved := generateOpenAIChallengeAnswer("test-seed", "ff", config)
	require.True(t, solved, "low-difficulty challenge must solve quickly")

	decoded, err := base64.StdEncoding.DecodeString(answer)
	require.NoError(t, err, "answer must be valid base64")

	var arr []any
	require.NoError(t, json.Unmarshal(decoded, &arr), "decoded payload must be valid JSON array")
	require.Len(t, arr, 18, "payload array must also be 18 elements")

	// 索引 3/9 在 generate_answer 里被 i / i>>1 覆盖 —— 必须是数字
	_, ok := arr[3].(float64)
	require.True(t, ok, "index 3 (i) must be numeric")
	_, ok = arr[9].(float64)
	require.True(t, ok, "index 9 (i>>1) must be numeric")

	// 第 6 项 dpl 应该是从缓存里取出来的（不是空串）
	require.Equal(t, "prod-test-dpl", arr[6])
}
