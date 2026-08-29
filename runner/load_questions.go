package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"benchmark_runner/types"
)

func loadQuestions(filename, sliceNotation string) ([]types.Question, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read benchmark %s: %w", filename, err)
	}

	var questions []types.Question
	if err := json.Unmarshal(data, &questions); err != nil {
		return nil, fmt.Errorf("parse benchmark JSON: %w", err)
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("benchmark contains no questions")
	}
	for i, question := range questions {
		if question.ID == "" || question.Question == "" || question.CorrectAnswerID == "" {
			return nil, fmt.Errorf("question %d is missing id, question, or correct_answer_id", i+1)
		}
		if len(question.Answers) == 0 {
			return nil, fmt.Errorf("question %s has no answers", question.ID)
		}
		correctAnswerFound := false
		for _, answer := range question.Answers {
			if answer.ID == question.CorrectAnswerID {
				correctAnswerFound = true
			}
		}
		if !correctAnswerFound {
			return nil, fmt.Errorf("question %s correct_answer_id does not match an answer", question.ID)
		}
	}
	return applySlice(questions, sliceNotation)
}

func applySlice(questions []types.Question, notation string) ([]types.Question, error) {
	if notation == "" {
		return questions, nil
	}
	parts := strings.Split(notation, ":")
	if len(parts) > 2 {
		return nil, fmt.Errorf("invalid slice %q", notation)
	}

	start, end := 0, len(questions)
	var err error
	if len(parts) == 1 {
		end, err = strconv.Atoi(parts[0])
	} else {
		if parts[0] != "" {
			start, err = strconv.Atoi(parts[0])
		}
		if err == nil && parts[1] != "" {
			end, err = strconv.Atoi(parts[1])
		}
	}
	if err != nil || start < 0 || end < 0 || start > end || start > len(questions) {
		return nil, fmt.Errorf("invalid slice %q for %d questions", notation, len(questions))
	}
	if end > len(questions) {
		end = len(questions)
	}
	return questions[start:end], nil
}
