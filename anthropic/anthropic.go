package anthropic

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"


	"go_proxy/api"
)

type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Type      string `json:"type"`
	Error     Error  `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

func NewError(code int, message string) ErrorResponse {
	var etype string
	switch code {
	case http.StatusBadRequest:
		etype = "invalid_request_error"
	case http.StatusUnauthorized:
		etype = "authentication_error"
	case http.StatusForbidden:
		etype = "permission_error"
	case http.StatusNotFound:
		etype = "not_found_error"
	case http.StatusTooManyRequests:
		etype = "rate_limit_error"
	case http.StatusServiceUnavailable, 529:
		etype = "overloaded_error"
	default:
		etype = "api_error"
	}

	return ErrorResponse{
		Type:      "error",
		Error:     Error{Type: etype, Message: message},
		RequestID: generateID("req"),
	}
}

type MessagesRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	Messages      []MessageParam  `json:"messages"`
	System        any             `json:"system,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []Tool          `json:"tools,omitempty"`
	ToolChoice    *ToolChoice     `json:"tool_choice,omitempty"`
	Thinking      *ThinkingConfig `json:"thinking,omitempty"`
}

type MessageParam struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ContentBlock struct {
	Type      string       `json:"type"`
	Text      *string      `json:"text,omitempty"`
	Source    *ImageSource `json:"source,omitempty"`
	ID        string       `json:"id,omitempty"`
	Name      string       `json:"name,omitempty"`
	Input     any          `json:"input,omitempty"`
	ToolUseID string       `json:"tool_use_id,omitempty"`
	Content   any          `json:"content,omitempty"`
	IsError   bool         `json:"is_error,omitempty"`
	Thinking  *string      `json:"thinking,omitempty"`
	Signature string       `json:"signature,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type Tool struct {
	Type        string          `json:"type,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type ToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type MessagesResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type StreamEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type MessageStartEvent struct {
	Type    string           `json:"type"`
	Message MessagesResponse `json:"message"`
}

type ContentBlockStartEvent struct {
	Type         string       `json:"type"`
	Index        int          `json:"index"`
	ContentBlock ContentBlock `json:"content_block"`
}

type ContentBlockDeltaEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta Delta  `json:"delta"`
}

type Delta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

type ContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type MessageDeltaEvent struct {
	Type  string       `json:"type"`
	Delta MessageDelta `json:"delta"`
	Usage DeltaUsage   `json:"usage"`
}

type MessageDelta struct {
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

type DeltaUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type MessageStopEvent struct {
	Type string `json:"type"`
}

func FromMessagesRequest(r MessagesRequest) (*api.ChatRequest, error) {
	var messages []api.Message

	if r.System != nil {
		switch sys := r.System.(type) {
		case string:
			if sys != "" {
				messages = append(messages, api.Message{Role: "system", Content: sys})
			}
		case []any:
			var content strings.Builder
			for _, block := range sys {
				if blockMap, ok := block.(map[string]any); ok {
					if blockMap["type"] == "text" {
						if text, ok := blockMap["text"].(string); ok {
							content.WriteString(text)
						}
					}
				}
			}
			if content.Len() > 0 {
				messages = append(messages, api.Message{Role: "system", Content: content.String()})
			}
		}
	}

	for _, msg := range r.Messages {
		converted, err := convertMessage(msg)
		if err != nil {
			return nil, err
		}
		messages = append(messages, converted...)
	}

	options := make(map[string]any)
	options["num_predict"] = r.MaxTokens
	if r.Temperature != nil {
		options["temperature"] = *r.Temperature
	}
	if r.TopP != nil {
		options["top_p"] = *r.TopP
	}
	if r.TopK != nil {
		options["top_k"] = *r.TopK
	}
	if len(r.StopSequences) > 0 {
		options["stop"] = r.StopSequences
	}

	stream := r.Stream
	convertedRequest := &api.ChatRequest{
		Model:    r.Model,
		Messages: messages,
		Options:  options,
		Stream:   &stream,
	}

	return convertedRequest, nil
}

func convertMessage(msg MessageParam) ([]api.Message, error) {
	var messages []api.Message
	role := strings.ToLower(msg.Role)

	switch content := msg.Content.(type) {
	case string:
		messages = append(messages, api.Message{Role: role, Content: content})
	case []any:
		var textContent strings.Builder
		var images []api.ImageData
		var toolCalls []api.ToolCall
		var thinking string
		var toolResults []api.Message

		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				return nil, errors.New("invalid content block format")
			}
			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "text":
				if text, ok := blockMap["text"].(string); ok {
					textContent.WriteString(text)
				}
			case "image":
				source, ok := blockMap["source"].(map[string]any)
				if !ok {
					return nil, errors.New("invalid image source")
				}
				sourceType, _ := source["type"].(string)
				if sourceType == "base64" {
					data, _ := source["data"].(string)
					decoded, err := base64.StdEncoding.DecodeString(data)
					if err != nil {
						return nil, fmt.Errorf("invalid base64 image data: %w", err)
					}
					images = append(images, decoded)
				}
			case "tool_use":
				id, _ := blockMap["id"].(string)
				name, _ := blockMap["name"].(string)
				tc := api.ToolCall{ID: id, Function: api.ToolCallFunction{Name: name}}
				if input, ok := blockMap["input"].(map[string]any); ok {
					tc.Function.Arguments = mapToArgs(input)
				}
				toolCalls = append(toolCalls, tc)
			case "tool_result":
				toolUseID, _ := blockMap["tool_use_id"].(string)
				var resultContent string
				switch c := blockMap["content"].(type) {
				case string:
					resultContent = c
				case []any:
					for _, cb := range c {
						if cbMap, ok := cb.(map[string]any); ok {
							if cbMap["type"] == "text" {
								if text, ok := cbMap["text"].(string); ok {
									resultContent += text
								}
							}
						}
					}
				}
				toolResults = append(toolResults, api.Message{
					Role:       "tool",
					Content:    resultContent,
					ToolCallID: toolUseID,
				})
			case "thinking":
				if t, ok := blockMap["thinking"].(string); ok {
					thinking = t
				}
			}
		}
		if textContent.Len() > 0 || len(images) > 0 || len(toolCalls) > 0 || thinking != "" {
			messages = append(messages, api.Message{
				Role:      role,
				Content:   textContent.String(),
				Images:    images,
				ToolCalls: toolCalls,
				Thinking:  thinking,
			})
		}
		messages = append(messages, toolResults...)
	}
	return messages, nil
}

func mapToArgs(m map[string]any) api.ToolCallFunctionArguments {
	args := api.NewToolCallFunctionArguments()
	for k, v := range m {
		args.Set(k, v)
	}
	return args
}

func ToMessagesResponse(id string, r api.ChatResponse) MessagesResponse {
	var content []ContentBlock
	if r.Message.Thinking != "" {
		content = append(content, ContentBlock{Type: "thinking", Thinking: &r.Message.Thinking})
	}
	if r.Message.Content != "" {
		content = append(content, ContentBlock{Type: "text", Text: &r.Message.Content})
	}
	for _, tc := range r.Message.ToolCalls {
		content = append(content, ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		})
	}

	stopReason := mapStopReason(r.Done, len(r.Message.ToolCalls) > 0)

	return MessagesResponse{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Model:      r.Model,
		Content:    content,
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  r.Metrics.PromptEvalCount,
			OutputTokens: r.Metrics.EvalCount,
		},
	}
}

func mapStopReason(done bool, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_use"
	}
	if done {
		return "end_turn"
	}
	return ""
}

func generateID(prefix string) string {
	b := make([]byte, 12)
	rand.Read(b)
	return fmt.Sprintf("%s_%x", prefix, b)
}

func GenerateMessageID() string {
	return generateID("msg")
}
