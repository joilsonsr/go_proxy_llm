package openai

import (
	"encoding/json"
	"net/http"
	"strings"
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
	Stop        any       `json:"stop,omitempty"`
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

func FromChatRequest(r ChatCompletionRequest) (*api.ChatRequest, error) {
	var messages []api.Message
	for _, msg := range r.Messages {
		toolName := ""
		if strings.ToLower(msg.Role) == "tool" {
			toolName = msg.Name
			if toolName == "" && msg.ToolCallID != "" {
				toolName = nameFromToolCallID(r.Messages, msg.ToolCallID)
			}
		}

		switch content := msg.Content.(type) {
		case string:
			toolCalls, err := FromCompletionToolCall(msg.ToolCalls)
			if err != nil {
				return nil, err
			}
			messages = append(messages, api.Message{
				Role:       msg.Role,
				Content:    content,
				Thinking:   msg.Reasoning,
				ToolCalls:  toolCalls,
				ToolName:   toolName,
				ToolCallID: msg.ToolCallID,
			})
		case []any:
			// Simplificado para o proxy
			for _, c := range content {
				data, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if data["type"] == "text" {
					text, _ := data["text"].(string)
					messages = append(messages, api.Message{Role: msg.Role, Content: text})
				}
			}
			if len(messages) > 0 && len(msg.ToolCalls) > 0 {
				toolCalls, _ := FromCompletionToolCall(msg.ToolCalls)
				idx := len(messages) - 1
				messages[idx].ToolCalls = toolCalls
				messages[idx].ToolName = toolName
				messages[idx].ToolCallID = msg.ToolCallID
				messages[idx].Thinking = msg.Reasoning
			}
		default:
			if msg.ToolCalls != nil {
				toolCalls, _ := FromCompletionToolCall(msg.ToolCalls)
				messages = append(messages, api.Message{
					Role:       msg.Role,
					Thinking:   msg.Reasoning,
					ToolCalls:  toolCalls,
					ToolCallID: msg.ToolCallID,
					ToolName:   toolName,
				})
			}
		}
	}

	options := make(map[string]any)
	if r.MaxTokens != nil {
		options["num_predict"] = *r.MaxTokens
	}
	if r.Temperature != nil {
		options["temperature"] = *r.Temperature
	}
	if r.TopP != nil {
		options["top_p"] = *r.TopP
	}

	stream := r.Stream
	return &api.ChatRequest{
		Model:    r.Model,
		Messages: messages,
		Options:  options,
		Stream:   &stream,
	}, nil
}

func FromCompletionToolCall(tc []ToolCall) ([]api.ToolCall, error) {
	if tc == nil {
		return nil, nil
	}
	res := make([]api.ToolCall, len(tc))
	for i, t := range tc {
		res[i] = api.ToolCall{
			ID: t.ID,
			Function: api.ToolCallFunction{
				Name: t.Function.Name,
			},
		}
		if t.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(t.Function.Arguments), &res[i].Function.Arguments); err != nil {
				return nil, err
			}
		}
	}
	return res, nil
}

func nameFromToolCallID(messages []Message, toolCallID string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		for _, tc := range msg.ToolCalls {
			if tc.ID == toolCallID {
				return tc.Function.Name
			}
		}
	}
	return ""
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
