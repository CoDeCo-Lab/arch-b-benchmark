package llm

import (
	"context"
	"fmt"
	"google.golang.org/genai"
	"os"
	"time"
)

type GeminiClient struct {
	Model  string
	Client *genai.Client
}

func NewGeminiClient(model string) GeminiClient {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		fmt.Println("Warning: GEMINI_API_KEY environment variable not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		panic(err)
	}

	return GeminiClient{
		Model:  model,
		Client: client,
	}
}

func (c GeminiClient) Query(messages []Message) (LLMResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	contents := make([]*genai.Content, len(messages))
	for i, message := range messages {
		contents[i] = convertMessageToGenaiContent(message)
	}

	var result *genai.GenerateContentResponse
	var err error

	// Retry up to 3 times
	for attempt := 0; attempt < 3; attempt++ {
		result, err = c.Client.Models.GenerateContent(
			ctx,
			c.Model,
			contents,
			&genai.GenerateContentConfig{
				Temperature: func() *float32 { v := float32(0.0); return &v }(),
			},
		)

		if err == nil {
			break
		}

		// Wait before retrying (exponential backoff)
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}

	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed after 3 attempts: %w", err)
	}

	if result == nil {
		return LLMResponse{}, fmt.Errorf("received nil result from GenerateContent")
	}

	return parseStructuredResponse(result.Text())
}

func convertMessageToGenaiContent(message Message) *genai.Content {
	var role genai.Role = genai.RoleUser
	if message.Role == MessageRoleSystem {
		role = genai.RoleModel
	}

	parts := make([]*genai.Part, len(message.Content))
	for i, content := range message.Content {
		parts[i] = convertContentPartToGenaiPart(content)
	}

	return genai.NewContentFromParts(parts, role)
}

func convertContentPartToGenaiPart(content ContentPart) *genai.Part {
	switch content.Type {
	case ContentTypeText:
		return genai.NewPartFromText(content.Text)
	case ContentTypeImage:
		return genai.NewPartFromBytes(content.Data, "image/jpeg")
	default:
		panic("unimplemented content type")
	}
}
