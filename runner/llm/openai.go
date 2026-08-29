package llm

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/shared"
	"os"
	"strings"
	"time"
)

// reasoningSuffix marks a gpt-5+ model that should run with reasoning enabled,
// e.g. "gpt-5.5:reasoning". Without it, gpt-5+ models run with reasoning
// disabled so they can use temperature 0 for deterministic benchmarking.
const reasoningSuffix = ":reasoning"

type OpenAIClient struct {
	Model  string
	Client openai.Client
}

func NewOpenAIClient(provider string, model string) OpenAIClient {
	var apiKey string
	var baseUrl string
	switch provider {
	case "openai":
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			fmt.Println("Warning: OPENAI_API_KEY environment variable not set")
		}
		baseUrl = "https://api.openai.com/v1"
	case "ollama":
		// The OpenAI-compatible Ollama endpoint does not require authentication.
		apiKey = "ollama"
		baseUrl = "http://localhost:11434/v1"
	case "xai":
		apiKey = os.Getenv("XAI_API_KEY")
		if apiKey == "" {
			fmt.Println("Warning: XAI_API_KEY environment variable not set")
		}
		baseUrl = "https://api.x.ai/v1"
	default:
		panic("bad model string provider format")
	}
	return OpenAIClient{
		Model: model,
		Client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(baseUrl),
		),
	}
}

func (c OpenAIClient) Query(messages []Message) (LLMResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, message := range messages {
		openaiMessages[i] = convertMessageToOpenAIMessage(message)
	}

	model, temp, effort := resolveModelParams(c.Model)
	params := openai.ChatCompletionNewParams{
		Messages:    openaiMessages,
		Model:       openai.ChatModel(model),
		Temperature: openai.Float(temp),
	}
	if effort != "" {
		params.ReasoningEffort = effort
	}

	chatCompletion, err := c.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		return LLMResponse{}, err
	}

	if len(chatCompletion.Choices) == 0 {
		return LLMResponse{}, fmt.Errorf("no choices in response")
	}

	body := chatCompletion.Choices[0].Message.Content

	return parseStructuredResponse(body)
}

// resolveModelParams maps a model string to the API model name, temperature, and
// reasoning_effort to send.
//
// gpt-5+ reasoning models choose between two modes via a ":reasoning" suffix on
// the model string (e.g. "gpt-5.5:reasoning"):
//   - reasoning ON  -> reasoning_effort="medium", temperature 1.0 (reasoning
//     models reject any non-default temperature).
//   - reasoning OFF -> reasoning_effort="none", temperature 0 for deterministic,
//     reproducible benchmarking.
//
// The ":reasoning" suffix is stripped from the returned model name. Non-gpt-5
// models use temperature 0 and an empty effort (so reasoning_effort is omitted);
// a stray ":reasoning" on a non-gpt-5 model is stripped but otherwise ignored.
func resolveModelParams(modelString string) (model string, temperature float64, effort shared.ReasoningEffort) {
	reasoningOn := strings.HasSuffix(modelString, reasoningSuffix)
	model = strings.TrimSuffix(modelString, reasoningSuffix)

	if !strings.Contains(model, "gpt-5") {
		return model, 0.0, ""
	}

	if reasoningOn {
		return model, 1.0, shared.ReasoningEffortMedium
	}
	return model, 0.0, shared.ReasoningEffort("none")
}

func convertMessageToOpenAIMessage(message Message) openai.ChatCompletionMessageParamUnion {
	switch message.Role {
	case MessageRoleUser:
		if len(message.Content) == 1 && message.Content[0].Type == ContentTypeText {
			return openai.UserMessage(message.Content[0].Text)
		}
		// For multipart messages with images, use the proper content parts
		parts := make([]openai.ChatCompletionContentPartUnionParam, len(message.Content))
		for i, content := range message.Content {
			parts[i] = convertContentPartToOpenAIPart(content)
		}
		return openai.UserMessage(parts)
	case MessageRoleSystem:
		var systemText string
		for _, content := range message.Content {
			if content.Type == ContentTypeText {
				systemText += content.Text
			}
		}
		return openai.SystemMessage(systemText)
	default:
		panic("unimplemented message role")
	}
}

func convertContentPartToOpenAIPart(content ContentPart) openai.ChatCompletionContentPartUnionParam {
	switch content.Type {
	case ContentTypeText:
		return openai.ChatCompletionContentPartUnionParam{
			OfText: &openai.ChatCompletionContentPartTextParam{
				Text: content.Text,
			},
		}
	case ContentTypeImage:
		encoded := base64.StdEncoding.EncodeToString(content.Data)
		imageURL := fmt.Sprintf("data:image/jpeg;base64,%s", encoded)
		return openai.ChatCompletionContentPartUnionParam{
			OfImageURL: &openai.ChatCompletionContentPartImageParam{
				ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
					URL: imageURL,
				},
			},
		}
	default:
		panic("unimplemented content type")
	}
}
