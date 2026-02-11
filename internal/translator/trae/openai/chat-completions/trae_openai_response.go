package chat_completions

import (
	"context"
	"fmt"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertTraeResponseToOpenAI translates a single chunk of a streaming response from Trae to OpenAI format.
func ConvertTraeResponseToOpenAI(_ context.Context, _ string, _, _, rawJSON []byte, param *any) []string {
	// Trae's streaming response usually follows OpenAI format closely as well if it's a proxy for GPT-4o/Claude
	// But if it has minor differences, we adjust here.
	// For now, we assume it's OpenAI-like but ensure standard fields.

	// If it's already OpenAI format, we might just pass it through with minor tweaks.
	// Here we implement a basic conversion.

	res := gjson.ParseBytes(rawJSON)
	if !res.Exists() {
		return []string{}
	}

	// OpenAI SSE template
	template := `{"id":"","object":"chat.completion.chunk","created":0,"model":"","choices":[{"index":0,"delta":{},"finish_reason":null}]}`
	
	now := time.Now().Unix()
	template, _ = sjson.Set(template, "created", now)
	template, _ = sjson.Set(template, "id", fmt.Sprintf("trae-%d", time.Now().UnixNano()))

	// If Trae returns content in a specific field, map it
	if content := res.Get("choices.0.delta.content"); content.Exists() {
		template, _ = sjson.Set(template, "choices.0.delta.content", content.String())
	} else if choices := res.Get("choices"); choices.IsArray() && len(choices.Array()) > 0 {
        // Fallback or more complex mapping
    }

	return []string{template}
}

// ConvertTraeResponseToOpenAINonStream converts a non-streaming Trae response to OpenAI format.
func ConvertTraeResponseToOpenAINonStream(_ context.Context, modelName string, _, _, rawJSON []byte, _ *any) string {
	res := gjson.ParseBytes(rawJSON)
	if !res.Exists() {
		return ""
	}

	// OpenAI non-stream template
	template := `{"id":"","object":"chat.completion","created":0,"model":"","choices":[{"index":0,"message":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	
	now := time.Now().Unix()
	template, _ = sjson.Set(template, "created", now)
	template, _ = sjson.Set(template, "model", modelName)
	template, _ = sjson.Set(template, "id", fmt.Sprintf("trae-%d", time.Now().UnixNano()))

	if content := res.Get("choices.0.message.content"); content.Exists() {
		template, _ = sjson.Set(template, "choices.0.message.role", "assistant")
		template, _ = sjson.Set(template, "choices.0.message.content", content.String())
	}
	
	// Map usage if available
	if usage := res.Get("usage"); usage.Exists() {
		template, _ = sjson.SetRaw(template, "usage", usage.Raw)
	}

	return template
}
