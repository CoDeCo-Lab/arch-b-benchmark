package questionRunner

import (
	"benchmark_runner/types"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// WriteCSVResults writes benchmark results to a CSV file
func WriteCSVResults(results []types.BenchmarkResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"question_id", "question_text", "llm_answer", "llm_explanation", "correct_answer", "is_correct", "mode"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, result := range results {
		row := []string{
			result.QuestionID,
			result.QuestionText,
			result.LLMAnswer,
			result.LLMExplanation,
			result.CorrectAnswer,
			strconv.FormatBool(result.IsCorrect),
			result.Mode,
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

// PrintCSVResults prints CSV formatted results to stdout
func PrintCSVResults(results []types.BenchmarkResult) {
	fmt.Println("question_id,question_text,llm_answer,llm_explanation,correct_answer,is_correct,mode")

	for _, result := range results {
		// Escape quotes in question text and explanation for CSV
		questionText := escapeCSVField(result.QuestionText)
		explanation := escapeCSVField(result.LLMExplanation)

		fmt.Printf("%s,%s,%s,%s,%s,%v,%s\n",
			result.QuestionID,
			questionText,
			result.LLMAnswer,
			explanation,
			result.CorrectAnswer,
			result.IsCorrect,
			result.Mode,
		)
	}
}

// escapeCSVField properly escapes a CSV field
func escapeCSVField(field string) string {
	// Replace internal quotes with double quotes and wrap in quotes
	escaped := strings.ReplaceAll(field, "\"", "\"\"")
	return fmt.Sprintf("\"%s\"", escaped)
}

// WriteMultiBenchmarkCSVResults writes multi benchmark results to a CSV file
func WriteMultiBenchmarkCSVResults(results []types.MultiBenchmarkResult, filename string) error {
	if len(results) == 0 {
		return fmt.Errorf("no results to write")
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Get all unique model names
	modelNames := make(map[string]bool)
	for _, result := range results {
		for modelName := range result.ModelResults {
			modelNames[modelName] = true
		}
	}

	// Convert to sorted slice for consistent ordering
	var sortedModelNames []string
	for modelName := range modelNames {
		sortedModelNames = append(sortedModelNames, modelName)
	}
	sort.Strings(sortedModelNames)

	// Build header
	header := []string{"question_id", "question_text", "correct_answer", "mode", "percent_models_correct"}

	// Add model answer columns
	for _, modelName := range sortedModelNames {
		header = append(header, modelName+"_answer")
	}

	// Add model correct columns
	for _, modelName := range sortedModelNames {
		header = append(header, modelName+"_correct")
	}

	// Add model reasoning columns
	for _, modelName := range sortedModelNames {
		header = append(header, modelName+"_reasoning")
	}

	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, result := range results {

		row := []string{
			result.QuestionID,
			result.QuestionText,
			result.CorrectAnswer,
			result.Mode,
			fmt.Sprintf("%.1f", result.CalculatePercentCorrect()),
		}

		// Add model answers
		for _, modelName := range sortedModelNames {
			if modelResult, exists := result.ModelResults[modelName]; exists {
				row = append(row, modelResult.Answer)
			} else {
				row = append(row, "")
			}
		}

		// Add model correctness
		for _, modelName := range sortedModelNames {
			if modelResult, exists := result.ModelResults[modelName]; exists {
				row = append(row, strconv.FormatBool(modelResult.Correct))
			} else {
				row = append(row, "")
			}
		}

		// Add model correctness
		for _, modelName := range sortedModelNames {
			if modelResult, exists := result.ModelResults[modelName]; exists {
				row = append(row, modelResult.Reasoning)
			} else {
				row = append(row, "")
			}
		}

		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}
