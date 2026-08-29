package llm

import (
	"fmt"
	"strings"
)

type Client interface {
	Query(messages []Message) (LLMResponse, error)
}

func NewLLMClient(modelString string) Client {
	parts := strings.SplitN(modelString, ":", 2)
	if len(parts) != 2 {
		panic("bad model string format")
	}

	switch parts[0] {
	case "openai", "ollama", "xai":
		return NewOpenAIClient(parts[0], parts[1])
	case "gemini":
		return NewGeminiClient(parts[1])
	case "anthropic-batch":
		return NewAnthropicBatchClient(parts[1])
	case "amazon":
		return NewBedrockClient(parts[0], parts[1])
	case "meta", "anthropic", "mistral":
		return NewBedrockClient("us."+parts[0], parts[1])
	default:
		panic("bad model string provider format")
	}
}

type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
)

type MessageRole string

const (
	MessageRoleUser   MessageRole = "user"
	MessageRoleSystem MessageRole = "system"
)

type Message struct {
	Role    MessageRole
	Content []ContentPart
}

type ContentPart struct {
	Type ContentType
	Text string
	Data []byte
}

type LLMResponse struct {
	AnswerID    string
	Explanation string
}

// parseStructuredResponse parses the LLM response in the format:
// Answer: [answer_id]
// Explanation: [explanation]
func parseStructuredResponse(response string) (LLMResponse, error) {
	lines := strings.Split(response, "\n")

	var answerID, explanation string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "answer:") {
			answerID = strings.TrimSpace(strings.TrimPrefix(line, "Answer:"))
			answerID = strings.TrimSpace(strings.TrimPrefix(answerID, "answer:"))
		} else if strings.HasPrefix(strings.ToLower(line), "explanation:") {
			explanation = strings.TrimSpace(strings.TrimPrefix(line, "Explanation:"))
			explanation = strings.TrimSpace(strings.TrimPrefix(explanation, "explanation:"))
		}
	}

	// Fallback: if structured format not found, try to extract answer from first word
	if answerID == "" {
		fields := strings.Fields(response)
		if len(fields) > 0 {
			answerID = fields[0]
			explanation = "No structured explanation provided"
		} else {
			return LLMResponse{}, fmt.Errorf("could not extract answer ID from response")
		}
	}

	// Clean up answer ID
	answerID = strings.Trim(answerID, "\"'.,")

	if answerID == "" {
		return LLMResponse{}, fmt.Errorf("empty answer ID after parsing")
	}

	return LLMResponse{
		AnswerID:    answerID,
		Explanation: explanation,
	}, nil
}
