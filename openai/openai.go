package openai

import (
	"encoding/json"
	"net/http"
	"time"

	"go_proxy/api"
)

type Error struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   any     `json:"param"`
	Code    *string `json:"code"`
}

type ErrorResponse struct {
	Error Error `json:"error"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason *string `json:"finish_reason"`
}

type ChunkChoice struct {
	Index        int     `json:"index"`
	Delta        Message `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   *int      `json:"max_tokens"`
	Temperature *float64  `json:"temperature"`
	TopP        *float64  `json:"top_p"`
}

type ChatCompletion struct {
	Id      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage,omitempty"`
}

type ChatCompletionChunk struct {
	Id      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func NewError(code int, message string) ErrorResponse {
	var etype string
	switch code {
	case http.StatusBadRequest:
		etype = "invalid_request_error"
	case http.StatusNotFound:
		etype = "not_found_error"
	default:
		etype = "api_error"
	}
	return ErrorResponse{Error{Type: etype, Message: message}}
}

func ToUsage(r api.ChatResponse) Usage {
	return Usage{
		PromptTokens:     r.Metrics.PromptEvalCount,
		CompletionTokens: r.Metrics.EvalCount,
		TotalTokens:      r.Metrics.PromptEvalCount + r.Metrics.EvalCount,
	}
}

func ToToolCalls(tc []api.ToolCall) []ToolCall {
	toolCalls := make([]ToolCall, len(tc))
	for i, tc := range tc {
		toolCalls[i].ID = tc.ID
		toolCalls[i].Type = "function"
			toolCalls[i].Function.Name = tc.Function.Name
			args, _ := json.Marshal(tc.Function.Arguments)
		toolCalls[i].Function.Arguments = string(args)
	}
	return toolCalls
}

func ToChatCompletion(id string, r api.ChatResponse) ChatCompletion {
	toolCalls := ToToolCalls(r.Message.ToolCalls)
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	return ChatCompletion{
		Id:      id,
		Object:  "chat.completion",
		Created: r.CreatedAt.Unix(),
		Model:   r.Model,
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:      r.Message.Role,
				Content:   r.Message.Content,
				ToolCalls: toolCalls,
				Reasoning: r.Message.Thinking,
			},
			FinishReason: &finishReason,
		}},
		Usage: ToUsage(r),
	}
}

func ToChunk(id string, r api.ChatResponse) ChatCompletionChunk {
	toolCalls := ToToolCalls(r.Message.ToolCalls)
	var finishReason *string
	if r.Done {
		fr := "stop"
		if len(toolCalls) > 0 {
			fr = "tool_calls"
		}
		finishReason = &fr
	}

	return ChatCompletionChunk{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   r.Model,
		Choices: []ChunkChoice{{
			Index: 0,
			Delta: Message{
				Role:      "assistant",
				Content:   r.Message.Content,
				ToolCalls: toolCalls,
				Reasoning: r.Message.Thinking,
			},
			FinishReason: finishReason,
		}},
	}
}
