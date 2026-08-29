package types

// MultiBenchmarkResult represents the result of testing one question
type MultiBenchmarkResult struct {
	QuestionID    string
	QuestionText  string
	CorrectAnswer string
	Mode          string // "answer-only" or "answer-and-reason" only
	ModelResults  map[string]MultiBenchmarkPerModelResult
}

type MultiBenchmarkPerModelResult struct {
	Answer    string
	Correct   bool
	Reasoning string
}

func (m MultiBenchmarkResult) CalculatePercentCorrect() float64 {
	// Calculate percent correct
	correctCount := 0
	totalModels := len(m.ModelResults)
	for _, modelResult := range m.ModelResults {
		if modelResult.Correct {
			correctCount++
		}
	}
	return float64(correctCount) / float64(totalModels) * 100
}

func AggregateResultsIntoMultiBenchmarkResult(resultMap map[string][]BenchmarkResult) []MultiBenchmarkResult {
	// Verify all []BenchmarkResult have the same length
	var expectedLength int
	var firstModel string
	for modelName, results := range resultMap {
		if firstModel == "" {
			firstModel = modelName
			expectedLength = len(results)
		} else if len(results) != expectedLength {
			panic("All models must have the same number of benchmark results")
		}
	}

	questionMap := make(map[string]*MultiBenchmarkResult)

	for modelName, results := range resultMap {
		for _, result := range results {
			questionKey := result.QuestionID + "|" + result.Mode

			if questionMap[questionKey] == nil {
				questionMap[questionKey] = &MultiBenchmarkResult{
					QuestionID:    result.QuestionID,
					QuestionText:  result.QuestionText,
					CorrectAnswer: result.CorrectAnswer,
					Mode:          result.Mode,
					ModelResults:  make(map[string]MultiBenchmarkPerModelResult),
				}
			}

			questionMap[questionKey].ModelResults[modelName] = MultiBenchmarkPerModelResult{
				Answer:    result.LLMAnswer,
				Correct:   result.IsCorrect,
				Reasoning: result.LLMExplanation,
			}
		}
	}

	var aggregatedResults []MultiBenchmarkResult
	for _, multiBenchmarkResult := range questionMap {
		aggregatedResults = append(aggregatedResults, *multiBenchmarkResult)
	}

	return aggregatedResults
}
