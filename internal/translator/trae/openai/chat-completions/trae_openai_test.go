package chat_completions

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertOpenAIRequestToTrae(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"stream": true,
		"temperature": 0.7
	}`)

	outputJSON := ConvertOpenAIRequestToTrae("gpt-4o", input, true)
	output := gjson.ParseBytes(outputJSON)

	if output.Get("model").String() != "gpt-4o" {
		t.Errorf("Expected model gpt-4o, got %s", output.Get("model").String())
	}

	if output.Get("stream").Bool() != true {
		t.Errorf("Expected stream true, got %v", output.Get("stream").Bool())
	}

	if output.Get("temperature").Num != 0.7 {
		t.Errorf("Expected temperature 0.7, got %v", output.Get("temperature").Num)
	}

	messages := output.Get("messages").Array()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Get("role").String() != "user" {
		t.Errorf("Expected role user, got %s", messages[0].Get("role").String())
	}

	if messages[0].Get("content").String() != "Hello" {
		t.Errorf("Expected content Hello, got %s", messages[0].Get("content").String())
	}
}

func TestConvertTraeResponseToOpenAINonStream(t *testing.T) {
	traeResponse := []byte(`{
		"id": "res-123",
		"choices": [
			{
				"message": {
					"role": "assistant",
					"content": "Hi there!"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 5,
			"completion_tokens": 10,
			"total_tokens": 15
		}
	}`)

	outputJSON := ConvertTraeResponseToOpenAINonStream(nil, "gpt-4o", nil, nil, traeResponse, nil)
	
	if outputJSON == "" {
		t.Fatal("Expected non-empty output")
	}

	output := gjson.Parse(outputJSON)
	if output.Get("choices.0.message.content").String() != "Hi there!" {
		t.Errorf("Expected content 'Hi there!', got %s", output.Get("choices.0.message.content").String())
	}

	if output.Get("usage.total_tokens").Int() != 15 {
		t.Errorf("Expected total_tokens 15, got %d", output.Get("usage.total_tokens").Int())
	}
}
