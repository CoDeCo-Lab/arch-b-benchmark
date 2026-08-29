package questionRunner

// LLMRequest represents the request sent to the LLM
type LLMRequest struct {
	Question       string         `json:"question"`
	QuestionImages []string       `json:"question_images,omitempty"`
	Answers        []AnswerForLLM `json:"answers"`
}

// AnswerForLLM represents an answer choice for the LLM (without metadata)
type AnswerForLLM struct {
	ID     string   `json:"id"`
	Text   string   `json:"text,omitempty"`
	Images []string `json:"images,omitempty"`
}
