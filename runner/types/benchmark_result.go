package types

// BenchmarkResult represents the result of testing one question
type BenchmarkResult struct {
	QuestionID     string
	QuestionText   string
	LLMAnswer      string
	LLMExplanation string
	CorrectAnswer  string
	IsCorrect      bool
	Mode           string // "answer-only" or "answer-and-reason" only
}
