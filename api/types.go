package api

import (
	"encoding/json"

	"time"

	"go_proxy/internal/orderedmap"
)

type ImageData []byte

type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	Thinking   string      `json:"thinking,omitempty"`
	Images     []ImageData `json:"images,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolName   string      `json:"tool_name,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string                    `json:"name"`
	Arguments ToolCallFunctionArguments `json:"arguments"`
}

type ToolCallFunctionArguments struct {
	om *orderedmap.Map[string, any]
}

func NewToolCallFunctionArguments() ToolCallFunctionArguments {
	return ToolCallFunctionArguments{om: orderedmap.New[string, any]()}
}

func (t *ToolCallFunctionArguments) UnmarshalJSON(data []byte) error {
	t.om = orderedmap.New[string, any]()
	return json.Unmarshal(data, t.om)
}
func (t *ToolCallFunctionArguments) Set(key string, value any) {
	if t.om == nil {
		t.om = orderedmap.New[string, any]()
	}
	t.om.Set(key, value)
}

func (t *ToolCallFunctionArguments) MarshalJSON() ([]byte, error) {
	if t.om == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(t.om)
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

type ToolParameters struct {
	Type       string         `json:"type"`
	Required   []string       `json:"required"`
	Properties map[string]any `json:"properties"`
}

type ChatRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   *bool          `json:"stream,omitempty"`
	Format   json.RawMessage `json:"format,omitempty"`
	Options  map[string]any `json:"options"`
}

type ChatResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Message   Message   `json:"message"`
	Done      bool      `json:"done"`
	Metrics   Metrics   `json:"metrics,omitempty"`
}

type Metrics struct {
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}
