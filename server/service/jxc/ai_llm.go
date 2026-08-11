package jxc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/config"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/utils/request"
)

// LLMMessage 对话消息
type LLMMessage struct {
	Role    string `json:"role"` // system / user / assistant
	Content string `json:"content"`
}

// LLMClient LLM 客户端（OpenAI 兼容），抽接口便于测试 mock
type LLMClient interface {
	// Chat 同步对话，返回完整回复文本；jsonMode 开启 JSON 输出模式
	Chat(ctx context.Context, messages []LLMMessage, jsonMode bool) (string, error)
	// ChatStream 流式对话，返回 SSE 响应体（调用方负责 Close），流的生命周期由 ctx 控制
	ChatStream(ctx context.Context, messages []LLMMessage) (*http.Response, error)
}

// DeepSeekClient DeepSeek 官方 API 实现（OpenAI 兼容 /chat/completions）
type DeepSeekClient struct{}

// chatCompletionRequest OpenAI 兼容请求体
type chatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []LLMMessage    `json:"messages"`
	Stream         bool            `json:"stream"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat 同步对话
func (c *DeepSeekClient) Chat(ctx context.Context, messages []LLMMessage, jsonMode bool) (string, error) {
	cfg, err := llmConfig()
	if err != nil {
		return "", err
	}
	payload := chatCompletionRequest{
		Model:       cfg.Model,
		Messages:    messages,
		Stream:      false,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	}
	if jsonMode {
		payload.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	res, err := request.HttpRequestWithContextAndTimeout(
		ctx,
		chatCompletionURL(cfg.BaseURL),
		http.MethodPost,
		llmHeaders(cfg.APIKey),
		nil,
		payload,
		time.Duration(cfg.Timeout)*time.Second,
	)
	if err != nil {
		return "", fmt.Errorf("调用大模型失败: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("读取大模型响应失败: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("大模型服务返回非 2xx: status=%d body=%s", res.StatusCode, previewLLMBody(body))
	}
	var resp chatCompletionResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("解析大模型响应失败: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("大模型服务错误: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return "", errors.New("大模型未返回内容")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// ChatStream 流式对话，SSE 响应体直接透传给调用方
func (c *DeepSeekClient) ChatStream(ctx context.Context, messages []LLMMessage) (*http.Response, error) {
	cfg, err := llmConfig()
	if err != nil {
		return nil, err
	}
	payload := chatCompletionRequest{
		Model:       cfg.Model,
		Messages:    messages,
		Stream:      true,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	}
	headers := llmHeaders(cfg.APIKey)
	headers["Accept"] = "text/event-stream"
	headers["Accept-Encoding"] = "identity" // 禁止 gzip，避免 SSE 流被压缩缓冲卡住
	headers["Cache-Control"] = "no-cache"
	res, err := request.HttpRequestWithContextAndTimeout(
		ctx,
		chatCompletionURL(cfg.BaseURL),
		http.MethodPost,
		headers,
		nil,
		payload,
		-1, // 不设置 client.Timeout，SSE 流的生命周期由 ctx 控制
	)
	if err != nil {
		return nil, fmt.Errorf("调用大模型流式服务失败: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("大模型流式服务返回非 2xx: status=%d body=%s", res.StatusCode, previewLLMBody(body))
	}
	return res, nil
}

// llmConfig 读取 AI 配置并填充默认值；apiKey 支持环境变量 GVA_AI_APIKEY 兜底
func llmConfig() (config.LLM, error) {
	cfg := global.GVA_CONFIG.AI.LLM
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2048
	}
	if cfg.Temperature <= 0 {
		cfg.Temperature = 0.7
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("GVA_AI_APIKEY")
	}
	if cfg.APIKey == "" {
		return cfg, errors.New("请先配置 AI API Key：config.yaml 的 ai.llm.api_key 或环境变量 GVA_AI_APIKEY")
	}
	return cfg, nil
}

// chatCompletionURL 拼接 /chat/completions 端点
func chatCompletionURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/chat/completions"
}

func llmHeaders(apiKey string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + apiKey,
	}
}

// previewLLMBody 截断响应体便于错误日志展示
func previewLLMBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "<empty>"
	}
	runes := []rune(text)
	if len(runes) > 300 {
		return string(runes[:300]) + "..."
	}
	return text
}
