package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

// BedrockClient uses Amazon Bedrock's standard OpenAI-compatible runtime
// endpoint and Bedrock bearer-token authentication.
type BedrockClient struct {
	Model  string
	Client openai.Client
}

func NewBedrockClient(provider, model string) BedrockClient {
	apiKey := os.Getenv("AWS_BEARER_TOKEN_BEDROCK")
	if apiKey == "" {
		fmt.Println("Warning: AWS_BEARER_TOKEN_BEDROCK environment variable not set")
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	modelID := provider + "." + model
	return BedrockClient{
		Model: modelID,
		Client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/v1", region)),
		),
	}
}

func (c BedrockClient) Query(messages []Message) (LLMResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	converted := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, message := range messages {
		converted[i] = convertMessageToOpenAIMessage(message)
	}

	completion, err := c.Client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages:    converted,
		Model:       openai.ChatModel(c.Model),
		Temperature: openai.Float(0),
	})
	if err != nil {
		return LLMResponse{}, err
	}
	if len(completion.Choices) == 0 {
		return LLMResponse{}, fmt.Errorf("no choices in Bedrock response")
	}
	return parseStructuredResponse(completion.Choices[0].Message.Content)
}
