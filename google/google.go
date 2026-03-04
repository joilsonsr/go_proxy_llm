package google

import (
	"go_proxy/openai"
)

// GooglePayload representa o formato de requisição do Google Gemini
type GooglePayload struct {
	Contents          []Content       `json:"contents"`
	SystemInstruction *SystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *GenerationConfig  `json:"generationConfig,omitempty"`
}

type Content struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type SystemInstruction struct {
	Parts []Part `json:"parts"`
}

type GenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
}

// GoogleResponse representa o formato de resposta do Google Gemini
type GoogleResponse struct {
	Candidates []Candidate `json:"candidates"`
	UsageMetadata UsageMetadata `json:"usageMetadata"`
}

type Candidate struct {
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason"`
}

type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// FromOpenAIRequest converte uma requisição OpenAI para o formato Google
func FromOpenAIRequest(data openai.ChatCompletionRequest) GooglePayload {
	var contents []Content
	var systemParts []Part

	for _, msg := range data.Messages {
		role := msg.Role
		contentStr := ""
		if s, ok := msg.Content.(string); ok {
			contentStr = s
		}

		parts := []Part{{Text: contentStr}}

		if role == "system" {
			systemParts = append(systemParts, parts...)
		} else {
			gRole := "user"
			if role == "assistant" {
				gRole = "model"
			}
			contents = append(contents, Content{Role: gRole, Parts: parts})
		}
	}

	payload := GooglePayload{Contents: contents}

	if len(systemParts) > 0 {
		payload.SystemInstruction = &SystemInstruction{Parts: systemParts}
	}

	genCfg := &GenerationConfig{}
	if data.Temperature != nil {
		genCfg.Temperature = data.Temperature
	}
	if data.MaxTokens != nil {
		genCfg.MaxOutputTokens = data.MaxTokens
	}
	
	if genCfg.Temperature != nil || genCfg.MaxOutputTokens != nil {
		payload.GenerationConfig = genCfg
	}

	return payload
}

// ToOpenAIResponse converte uma resposta Google para o formato OpenAI
func ToOpenAIResponse(data GoogleResponse, model string) openai.ChatCompletion {
	var choices []openai.Choice
	for i, cand := range data.Candidates {
		text := ""
		for _, p := range cand.Content.Parts {
			text += p.Text
		}
		
		finishReason := cand.FinishReason
		if finishReason == "" {
			finishReason = "stop"
		}

		choices = append(choices, openai.Choice{
			Index: i,
			Message: openai.Message{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: &finishReason,
		})
	}

	return openai.ChatCompletion{
		Id:      "chatcmpl-proxy",
		Object:  "chat.completion",
		Model:   model,
		Choices: choices,
		Usage: openai.Usage{
			PromptTokens:     data.UsageMetadata.PromptTokenCount,
			CompletionTokens: data.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      data.UsageMetadata.TotalTokenCount,
		},
	}
}
