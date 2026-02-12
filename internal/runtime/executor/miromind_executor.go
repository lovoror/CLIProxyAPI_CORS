package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/auth/miromind"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	"github.com/tidwall/gjson"
)

// MiroMindExecutor handles request execution for the MiroMind provider.
type MiroMindExecutor struct {
	cfg *config.Config
}

// NewMiroMindExecutor creates a new MiroMindExecutor instance.
func NewMiroMindExecutor(cfg *config.Config) *MiroMindExecutor {
	return &MiroMindExecutor{
		cfg: cfg,
	}
}

func (e *MiroMindExecutor) Identifier() string {
	return "miromind"
}

// Refresh handles token refresh for MiroMind.
func (e *MiroMindExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	// MiroMind tokens are manual session tokens (JWTs), auto-refresh not supported yet.
	return auth, nil
}

// PrepareRequest sets up the necessary headers for MiroMind API requests.
func (e *MiroMindExecutor) PrepareRequest(req *http.Request, auth *cliproxyauth.Auth) error {
	if req == nil {
		return nil
	}
	token, _ := miromindCreds(auth)
	if token == "" {
		return fmt.Errorf("miromind executor: missing session token")
	}

	// Masquerade as a legitimate browser session.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Origin", "https://dr.miromind.ai")
	req.Header.Set("Referer", "https://dr.miromind.ai/")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Authorization", "Bearer "+token)

	// Clean up any cookie headers that might have been set incorrectly
	req.Header.Del("Cookie")

	return nil
}

// HttpRequest injects MiroMind credentials into the request and executes it.
func (e *MiroMindExecutor) HttpRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("miromind executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)
	if err := e.PrepareRequest(httpReq, auth); err != nil {
		return nil, err
	}
	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	return httpClient.Do(httpReq)
}

// miromindRequest represents the payload expected by MiroMind API
type miromindRequest struct {
	Messages []map[string]interface{} `json:"messages"`
	Debug    bool                     `json:"debug"`
	Mode     string                   `json:"mode"`
}

func (e *MiroMindExecutor) buildMiroMindRequest(req cliproxyexecutor.Request) ([]byte, error) {
	// Default mode
	mode := "pro"
	modelName := thinking.ParseSuffix(req.Model).ModelName
	if strings.Contains(strings.ToLower(modelName), "fast") {
		mode = "fast"
	}
	// Allow direct mode override from model name if it matches "miromind-MODE"
	if strings.HasPrefix(modelName, "miromind-") {
		parts := strings.Split(modelName, "-")
		if len(parts) > 1 && parts[1] != "chat" {
			mode = parts[1]
		}
	}

	// Extract messages using gjson
	messagesResult := gjson.GetBytes(req.Payload, "messages")
	if !messagesResult.Exists() || !messagesResult.IsArray() {
		return nil, fmt.Errorf("invalid messages in request payload")
	}

	var messages []map[string]interface{}
	for _, msg := range messagesResult.Array() {
		role := msg.Get("role").String()
		content := msg.Get("content").String()
		messages = append(messages, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}

	mmReq := miromindRequest{
		Messages: messages,
		Debug:    false,
		Mode:     mode,
	}

	return json.Marshal(mmReq)
}

// Execute performs the API request to MiroMind and returns the response (non-streaming).
func (e *MiroMindExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	// Verify if we can just use the stream endpoint and buffer it
	stream, err := e.ExecuteStream(ctx, auth, req, opts)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}

	var contentBuilder strings.Builder
	for chunk := range stream {
		if chunk.Err != nil {
			return cliproxyexecutor.Response{}, chunk.Err
		}
		// Parse OpenAI chunk format to extract content
		line := string(chunk.Payload)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(data) == "[DONE]" {
			break
		}
		
		val := gjson.Parse(data)
		delta := val.Get("choices.0.delta.content").String()
		contentBuilder.WriteString(delta)
	}

	// Construct a full OpenAI response
	responseBody := map[string]interface{}{
		"id":      generateID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": contentBuilder.String(),
				},
				"finish_reason": "stop",
			},
		},
	}
	
	respBytes, _ := json.Marshal(responseBody)
	return cliproxyexecutor.Response{Payload: respBytes}, nil
}

// ExecuteStream handles streaming requests.
func (e *MiroMindExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (stream <-chan cliproxyexecutor.StreamChunk, err error) {
	url := "https://dr.miromind.ai/api/chat/stream"
	if override := e.cfg.SDKConfig.MiroMindAPIURL; override != "" {
		url = override
	}

	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := newUsageReporter(ctx, e.Identifier(), baseModel, auth)
	defer reporter.trackFailure(ctx, &err)

	body, err := e.buildMiroMindRequest(req)
	if err != nil {
		return nil, fmt.Errorf("miromind executor: build request: %w", err)
	}

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("miromind executor: create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      body,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	// httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0) // Removed unused variable
	resp, err := e.HttpRequest(ctx, auth, httpReq)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return nil, fmt.Errorf("miromind executor: http call failed: %w", err)
	}

	recordAPIResponseMetadata(ctx, e.cfg, resp.StatusCode, resp.Header.Clone())

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		resp.Body.Close()
		
		errMsg := string(b)
		if gjson.Valid(errMsg) {
			// Extract cleaner error message from upstream JSON
			if v := gjson.Get(errMsg, "error.message").String(); v != "" {
				errMsg = v
			} else if v := gjson.Get(errMsg, "message").String(); v != "" {
				errMsg = v
			} else if v := gjson.Get(errMsg, "detail").String(); v != "" {
				errMsg = v
			}
		}
		
		err = statusErr{code: resp.StatusCode, msg: errMsg}
		return nil, err
	}

	out := make(chan cliproxyexecutor.StreamChunk)
	stream = out

	go func() {
		defer close(out)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer size for large events
		scanner.Buffer(make([]byte, 4096), 10*1024*1024)

		id := generateID()
		created := time.Now().Unix()

		sendChunk := func(content, reasoning, finishReason string) {
			chunk := map[string]interface{}{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   baseModel,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]string{},
						"finish_reason": nil,
					},
				},
			}
			
			if finishReason != "" {
				chunk["choices"].([]map[string]interface{})[0]["finish_reason"] = finishReason
			}
			
			delta := chunk["choices"].([]map[string]interface{})[0]["delta"].(map[string]string)
			if content != "" {
				delta["content"] = content
			}
			if reasoning != "" {
				delta["reasoning_content"] = reasoning
			}
			
			// Use Encoder to avoid HTML escaping
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			_ = enc.Encode(chunk)
			out <- cliproxyexecutor.StreamChunk{Payload: bytes.TrimSpace(buf.Bytes())}
		}

		// Initial role chunk
		initialChunk := map[string]interface{}{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   baseModel,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]string{
						"role": "assistant",
					},
					"finish_reason": nil,
				},
			},
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(initialChunk)
		out <- cliproxyexecutor.StreamChunk{Payload: bytes.TrimSpace(buf.Bytes())}

		inThinking := false
		currentEvent := "" // 跟踪当前 SSE 事件类型
		var totalPromptTokens, totalCompletionTokens int64

		for scanner.Scan() {
			line := scanner.Bytes()
			appendAPIResponseChunk(ctx, e.cfg, line)

			text := string(line)

			// 跟踪 SSE 事件类型（event: 行总是在对应的 data: 行之前）
			if strings.HasPrefix(text, "event: ") {
				currentEvent = strings.TrimPrefix(text, "event: ")
				continue
			}

			// 跳过非 data 行（id:、空行等）
			if !strings.HasPrefix(text, "data: ") {
				continue
			}

			dataStr := strings.TrimPrefix(text, "data: ")
			if !gjson.Valid(dataStr) {
				continue
			}

			switch currentEvent {
			case "message":
				// reporter agent 的正式回复内容
				content := gjson.Get(dataStr, "delta.content").String()
				if content != "" {
					sendChunk(content, "", "")
				}

			case "tool_call":
				// 工具调用事件 - 只处理 show_text 中的 <think> 内容
				toolName := gjson.Get(dataStr, "tool_name").String()
				if toolName == "show_text" {
					deltaText := gjson.Get(dataStr, "delta_input.text").String()
					if deltaText != "" {
						// State machine for <think> parsing
						toContent := ""
						toReasoning := ""

						remaining := deltaText

						// Safety loop to process multiple tags in one chunk
						for len(remaining) > 0 {
							if !inThinking {
								// Look for <think>
								if idx := strings.Index(remaining, "<think>"); idx >= 0 {
									toContent += remaining[:idx]
									inThinking = true
									// Skip <think> tag (7 chars)
									if len(remaining) > idx+7 {
										remaining = remaining[idx+7:]
									} else {
										remaining = ""
									}
									// Only one skip of newline if present immediately?
									if strings.HasPrefix(remaining, "\n") {
										remaining = remaining[1:]
									}
								} else {
									toContent += remaining
									remaining = ""
								}
							} else {
								// Look for </think>
								if idx := strings.Index(remaining, "</think>"); idx >= 0 {
									toReasoning += remaining[:idx]
									inThinking = false
									// Skip </think> tag (8 chars)
									if len(remaining) > idx+8 {
										remaining = remaining[idx+8:]
									} else {
										remaining = ""
									}
								} else {
									toReasoning += remaining
									remaining = ""
								}
							}
						}

						if toContent != "" || toReasoning != "" {
							sendChunk(toContent, toReasoning, "")
						}
					}
				}

			case "usage_info":
				// 提取 token 用量信息
				scene := gjson.Get(dataStr, "scene").String()
				if scene == "main_agent_end" || scene == "summary_llm_end" {
					pt := gjson.Get(dataStr, "usage.total_prompt_tokens").Int()
					ct := gjson.Get(dataStr, "usage.total_completion_tokens").Int()
					totalPromptTokens += pt
					totalCompletionTokens += ct
				}

			case "done", "end_of_workflow":
				// 流结束信号，干净退出
				goto streamDone

			default:
				// ping, heartbeat, history, start_of_agent, end_of_agent 等 - 忽略
			}
		}

	streamDone:
		// 上报 token 用量（如果有）
		if totalPromptTokens > 0 || totalCompletionTokens > 0 {
			reporter.publish(ctx, usage.Detail{
				InputTokens:  totalPromptTokens,
				OutputTokens: totalCompletionTokens,
				TotalTokens:  totalPromptTokens + totalCompletionTokens,
			})
		}
		sendChunk("", "", "stop")
	}()

	return stream, nil
}

// CountTokens provides a placeholder for token counting.
func (e *MiroMindExecutor) CountTokens(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	// Simple approximation: string length / 4
	reqLen := len(req.Payload)
	count := reqLen / 4
	return cliproxyexecutor.Response{
		Payload: []byte(fmt.Sprintf(`{"total_tokens": %d}`, count)),
	}, nil
}

func miromindCreds(auth *cliproxyauth.Auth) (string, string) {
	if auth == nil {
		return "", ""
	}
	// Try Storage first
	if auth.Storage != nil {
		if s, ok := auth.Storage.(*miromind.MiroMindTokenStorage); ok && s != nil {
			return s.SessionToken, s.Email
		}
	}
	// Fallback to Metadata
	if auth.Metadata != nil {
		token, _ := auth.Metadata["session_token"].(string)
		email, _ := auth.Metadata["email"].(string)
		return token, email
	}
	return "", ""
}

func generateID() string {
	return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
}
