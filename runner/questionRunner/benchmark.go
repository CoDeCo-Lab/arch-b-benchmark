package questionRunner

import (
	"benchmark_runner/llm"
	"benchmark_runner/types"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BenchmarkRunner orchestrates the benchmark process
type BenchmarkRunner struct {
	client llm.Client
}

// NewBenchmarkRunner creates a new benchmark runner
func NewBenchmarkRunner(modelString string) *BenchmarkRunner {
	return &BenchmarkRunner{
		client: llm.NewLLMClient(modelString),
	}
}

// RunBenchmark processes all questions and returns results (backward compatibility)
func (br *BenchmarkRunner) RunBenchmark(questions []types.Question) ([]types.BenchmarkResult, error) {
	return br.RunBenchmarkWithMode(questions, "answer-and-reason")
}

// RunBenchmarkWithMode processes all questions with specified mode and returns results
func (br *BenchmarkRunner) RunBenchmarkWithMode(questions []types.Question, mode string) ([]types.BenchmarkResult, error) {
	var results []types.BenchmarkResult

	switch mode {
	case "both":
		// For 'both' mode, we need to double the expected results
		results = make([]types.BenchmarkResult, 0, len(questions)*2)
	default:
		results = make([]types.BenchmarkResult, 0, len(questions))
	}

	fmt.Printf("Starting benchmark with %d questions in mode '%s'...\n", len(questions), mode)

	for i, question := range questions {
		if mode == "both" {
			fmt.Printf("Processing question %d/%d (ID: %s) - both modes...\n", i+1, len(questions), question.ID)

			// First query: answer-only
			fmt.Printf("  Mode: answer-only\n")
			result, err := br.processQuestion(question, false)
			if err != nil {
				return results, err
			}
			results = append(results, result)

			// Second query: answer-and-reason
			fmt.Printf("  Mode: answer-and-reason\n")
			result, err = br.processQuestion(question, true)
			if err != nil {
				return results, err
			}
			results = append(results, result)

		} else {
			// Single mode processing
			fmt.Printf("Processing question %d/%d (ID: %s) - mode: %s...\n", i+1, len(questions), question.ID, mode)

			result, err := br.processQuestion(question, mode == "answer-and-reason")
			if err != nil {
				return results, err
			}
			results = append(results, result)
		}

		// Small delay to avoid rate limiting
		time.Sleep(1 * time.Second)
	}

	return results, nil
}

func convertQuestionToContent(question types.Question, withReasoning bool) ([]llm.ContentPart, error) {
	// Process images for the question
	questionImages, err := processImageURLs(question.QuestionImages, question.QuestionImageHashes)
	if err != nil {
		return nil, fmt.Errorf("failed to process question images: %w", err)
	}

	// Build the prompt
	content := []llm.ContentPart{}

	// Add question text and images
	promptText := fmt.Sprintf("Question: %s", question.Question)
	content = append(content, llm.ContentPart{
		Type: llm.ContentTypeText,
		Text: promptText,
	})

	// Add question images if any
	for _, imgBytes := range questionImages {
		content = append(content, llm.ContentPart{
			Type: llm.ContentTypeImage,
			Data: imgBytes,
		})
	}

	// Add each answer choice with its associated images
	content = append(content, llm.ContentPart{
		Type: llm.ContentTypeText,
		Text: "\nAnswer choices:",
	})

	for _, answer := range question.Answers {
		// Add answer choice header
		answerText := fmt.Sprintf("\n\nAnswer choice ID: %s", answer.ID)
		if answer.Text != "" {
			answerText += fmt.Sprintf("\nText: %s", answer.Text)
		}

		content = append(content, llm.ContentPart{
			Type: llm.ContentTypeText,
			Text: answerText,
		})

		// Add images for this specific answer choice
		answerImages, err := processImageURLs(answer.Images, answer.ImageHashes)
		if err != nil {
			return nil, fmt.Errorf("failed to process answer images for %s: %w", answer.ID, err)
		}

		for _, imgBytes := range answerImages {
			content = append(content, llm.ContentPart{
				Type: llm.ContentTypeImage,
				Data: imgBytes,
			})
		}
	}

	// Add instruction based on mode
	instructionText := "\n\nThere is exactly one correct answer. Please respond with only the answer choice ID. No explanation needed.\n\nFormat:\nAnswer: [answer_choice_id]\n\nFor example:\nAnswer: ahz6bkqz"
	if withReasoning {
		instructionText = "\n\nThere is exactly one correct answer. Please respond in the following format:\nAnswer: [answer_choice_id]\nExplanation: [brief explanation of why you chose this answer]\n\nFor example:\nAnswer: ahz6bkqz\nExplanation: This image shows a different architectural style compared to the others."
	}

	content = append(content, llm.ContentPart{
		Type: "text",
		Text: instructionText,
	})

	return content, nil
}

func (br *BenchmarkRunner) processQuestion(question types.Question, withReasoning bool) (types.BenchmarkResult, error) {
	mode := "answer-only"
	if withReasoning {
		mode = "answer-and-reason"
	}

	questionContent, err := convertQuestionToContent(question, withReasoning)
	if err != nil {
		if errors.Is(err, ErrImageHashMismatch) {
			return types.BenchmarkResult{}, fmt.Errorf("question %s: %w", question.ID, err)
		}
		fmt.Printf("Error processing question %s: %v\n", question.ID, err)
		// Still record the result with error info
		return types.BenchmarkResult{
			QuestionID:     question.ID,
			QuestionText:   question.Question,
			LLMAnswer:      fmt.Sprintf("ERROR: %v", err),
			LLMExplanation: "Error occurred during processing",
			CorrectAnswer:  question.CorrectAnswerID,
			IsCorrect:      false,
			Mode:           mode,
		}, nil
	}

	// Query the LLM
	start := time.Now()
	llmResponse, err := br.client.Query(
		[]llm.Message{
			{
				Role:    llm.MessageRoleUser,
				Content: questionContent,
			},
		})
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("Error processing question %s: %v\n", question.ID, err)
		// Still record the result with error info
		return types.BenchmarkResult{
			QuestionID:     question.ID,
			QuestionText:   question.Question,
			LLMAnswer:      fmt.Sprintf("ERROR: %v", err),
			LLMExplanation: "Error occurred during processing",
			CorrectAnswer:  question.CorrectAnswerID,
			IsCorrect:      false,
			Mode:           mode,
		}, nil
	}

	// Check if answer is correct
	isCorrect := llmResponse.AnswerID == question.CorrectAnswerID

	fmt.Printf("    LLM Answer: %s | Correct Answer: %s | Correct: %v | Duration: %v\n",
		llmResponse.AnswerID, question.CorrectAnswerID, isCorrect, duration)
	reasoning := ""
	if withReasoning {
		reasoning = llmResponse.Explanation
		fmt.Printf("    Explanation: %s\n", llmResponse.Explanation)
	}

	return types.BenchmarkResult{
		QuestionID:     question.ID,
		QuestionText:   question.Question,
		LLMAnswer:      llmResponse.AnswerID,
		LLMExplanation: reasoning,
		CorrectAnswer:  question.CorrectAnswerID,
		IsCorrect:      isCorrect,
		Mode:           mode,
	}, nil
}

// PrintSummary prints a summary of the benchmark results
func (br *BenchmarkRunner) PrintSummary(results []types.BenchmarkResult) {
	total := len(results)
	correct := 0

	// Track separate stats for different modes
	answerOnlyResults := 0
	answerWithReasonResults := 0
	answerOnlyCorrect := 0
	answerWithReasonCorrect := 0

	// Track stats by question type (grouped by question text)
	questionTypeStats := make(map[string]struct {
		total   int
		correct int
	})

	for _, result := range results {
		if result.IsCorrect {
			correct++
		}

		if result.Mode == "answer-only" {
			answerOnlyResults++
			if result.IsCorrect {
				answerOnlyCorrect++
			}
		} else if result.Mode == "answer-and-reason" {
			answerWithReasonResults++
			if result.IsCorrect {
				answerWithReasonCorrect++
			}
		}

		// Track by question type (grouped by Question field)
		questionText := result.QuestionText
		stats := questionTypeStats[questionText]
		stats.total++
		if result.IsCorrect {
			stats.correct++
		}
		questionTypeStats[questionText] = stats
	}

	accuracy := float64(correct) / float64(total) * 100

	fmt.Printf("\n=== BENCHMARK SUMMARY ===\n")
	fmt.Printf("Total Results: %d\n", total)
	fmt.Printf("Correct Answers: %d\n", correct)
	fmt.Printf("Incorrect Answers: %d\n", total-correct)
	fmt.Printf("Accuracy: %.2f%%\n", accuracy)

	// Show breakdown by mode only if we have both types
	if answerOnlyResults > 0 && answerWithReasonResults > 0 {
		fmt.Printf("\n--- MODE BREAKDOWN ---\n")
		fmt.Printf("Answer-only results: %d/%d correct (%.2f%%)\n",
			answerOnlyCorrect, answerOnlyResults,
			float64(answerOnlyCorrect)/float64(answerOnlyResults)*100)
		fmt.Printf("Answer-and-reason results: %d/%d correct (%.2f%%)\n",
			answerWithReasonCorrect, answerWithReasonResults,
			float64(answerWithReasonCorrect)/float64(answerWithReasonResults)*100)
	}

	// Show breakdown by question type
	if len(questionTypeStats) > 1 {
		fmt.Printf("\n--- QUESTION TYPE BREAKDOWN ---\n")

		// Sort question texts for consistent ordering
		questionTexts := make([]string, 0, len(questionTypeStats))
		for questionText := range questionTypeStats {
			questionTexts = append(questionTexts, questionText)
		}
		sort.Strings(questionTexts)

		for _, questionText := range questionTexts {
			stats := questionTypeStats[questionText]
			accuracy := float64(stats.correct) / float64(stats.total) * 100
			fmt.Printf("Question: \"%s\"\n", questionText)
			fmt.Printf("  Results: %d/%d correct (%.2f%%)\n\n", stats.correct, stats.total, accuracy)
		}
	}

	fmt.Printf("========================\n\n")
}

// WriteSummaryCSV writes the summary data to a CSV file
func (br *BenchmarkRunner) WriteSummaryCSV(results []types.BenchmarkResult, outputFile string) error {
	// Generate summary filename by replacing "results_" with "results_summary_"
	summaryFile := strings.Replace(outputFile, "results_", "results_summary_", 1)

	file, err := os.Create(summaryFile)
	if err != nil {
		return fmt.Errorf("failed to create summary CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Calculate overall stats
	total := len(results)
	correct := 0
	answerOnlyResults := 0
	answerWithReasonResults := 0
	answerOnlyCorrect := 0
	answerWithReasonCorrect := 0

	// Track stats by question type
	questionTypeStats := make(map[string]struct {
		total   int
		correct int
	})

	for _, result := range results {
		if result.IsCorrect {
			correct++
		}

		if result.Mode == "answer-only" {
			answerOnlyResults++
			if result.IsCorrect {
				answerOnlyCorrect++
			}
		} else if result.Mode == "answer-and-reason" {
			answerWithReasonResults++
			if result.IsCorrect {
				answerWithReasonCorrect++
			}
		}

		// Track by question type
		questionText := result.QuestionText
		stats := questionTypeStats[questionText]
		stats.total++
		if result.IsCorrect {
			stats.correct++
		}
		questionTypeStats[questionText] = stats
	}

	// Write CSV headers
	headers := []string{"metric_type", "category", "total", "correct", "accuracy_percent"}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write overall summary
	accuracy := float64(correct) / float64(total) * 100
	overallRow := []string{
		"overall",
		"all_questions",
		strconv.Itoa(total),
		strconv.Itoa(correct),
		fmt.Sprintf("%.2f", accuracy),
	}
	if err := writer.Write(overallRow); err != nil {
		return fmt.Errorf("failed to write overall row: %w", err)
	}

	// Write mode breakdown only if we have both types
	if answerOnlyResults > 0 && answerWithReasonResults > 0 {
		answerOnlyAccuracy := float64(answerOnlyCorrect) / float64(answerOnlyResults) * 100
		modeRow := []string{
			"mode",
			"answer-only",
			strconv.Itoa(answerOnlyResults),
			strconv.Itoa(answerOnlyCorrect),
			fmt.Sprintf("%.2f", answerOnlyAccuracy),
		}
		if err := writer.Write(modeRow); err != nil {
			return fmt.Errorf("failed to write mode row: %w", err)
		}

		answerWithReasonAccuracy := float64(answerWithReasonCorrect) / float64(answerWithReasonResults) * 100
		modeRow = []string{
			"mode",
			"answer-and-reason",
			strconv.Itoa(answerWithReasonResults),
			strconv.Itoa(answerWithReasonCorrect),
			fmt.Sprintf("%.2f", answerWithReasonAccuracy),
		}
		if err := writer.Write(modeRow); err != nil {
			return fmt.Errorf("failed to write mode row: %w", err)
		}
	}

	// Write question type breakdown with consistent ordering
	questionTexts := make([]string, 0, len(questionTypeStats))
	for questionText := range questionTypeStats {
		questionTexts = append(questionTexts, questionText)
	}
	sort.Strings(questionTexts)

	for _, questionText := range questionTexts {
		stats := questionTypeStats[questionText]
		accuracy := float64(stats.correct) / float64(stats.total) * 100
		questionRow := []string{
			"question_type",
			questionText,
			strconv.Itoa(stats.total),
			strconv.Itoa(stats.correct),
			fmt.Sprintf("%.2f", accuracy),
		}
		if err := writer.Write(questionRow); err != nil {
			return fmt.Errorf("failed to write question type row: %w", err)
		}
	}

	fmt.Printf("Summary written to %s\n", summaryFile)
	return nil
}
