package chat_completions

import (
	"bytes"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertOpenAIRequestToTrae converts an OpenAI Chat Completions request (raw JSON)
// into a Trae-compatible request JSON.
func ConvertOpenAIRequestToTrae(modelName string, inputRawJSON []byte, _ bool) []byte {
	rawJSON := bytes.Clone(inputRawJSON)
	
	// Base envelope for Trae API
	// The exact structure might vary, but based on typical ByteDance/Trae APIs:
	out := []byte(`{"model":"","messages":[],"stream":false}`)

	// Set model mapping if needed, otherwise use passed modelName
	out, _ = sjson.SetBytes(out, "model", modelName)

	// Stream mapping
	if gjson.GetBytes(rawJSON, "stream").Bool() {
		out, _ = sjson.SetBytes(out, "stream", true)
	}

	// Temperature/TopP/MaxTokens passthrough
	if v := gjson.GetBytes(rawJSON, "temperature"); v.Exists() {
		out, _ = sjson.SetBytes(out, "temperature", v.Num)
	}
	if v := gjson.GetBytes(rawJSON, "top_p"); v.Exists() {
		out, _ = sjson.SetBytes(out, "top_p", v.Num)
	}
	if v := gjson.GetBytes(rawJSON, "max_tokens"); v.Exists() {
		out, _ = sjson.SetBytes(out, "max_tokens", v.Int())
	}

	// Messages mapping
	messages := gjson.GetBytes(rawJSON, "messages")
	if messages.IsArray() {
		var traeMessages []interface{}
		for _, m := range messages.Array() {
			msg := map[string]interface{}{
				"role":    m.Get("role").String(),
				"content": m.Get("content").String(),
			}
			// Handle complex content (multimodal/array) if needed
			if m.Get("content").IsArray() {
                var combinedText strings.Builder
                for _, part := range m.Get("content").Array() {
                    if part.Get("type").String() == "text" {
                        combinedText.WriteString(part.Get("text").String())
                    }
                    // TODO: handle images for Trae if supported
                }
                msg["content"] = combinedText.String()
			}
			traeMessages = append(traeMessages, msg)
		}
		out, _ = sjson.SetBytes(out, "messages", traeMessages)
	}

	return out
}
