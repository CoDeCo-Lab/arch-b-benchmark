package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Question represents a single benchmark question
type Question struct {
	ID                  string   `json:"id"`
	Question            string   `json:"question"`
	QuestionImages      []string `json:"question_images"`
	QuestionImageHashes []string `json:"-"`
	Answers             []Answer `json:"answers"`
	CorrectAnswerID     string   `json:"correct_answer_id"`
}

// Answer represents a possible answer choice
type Answer struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Images      []string `json:"images"`
	ImageHashes []string `json:"-"`
}

// UnmarshalJSON reads released {"url": "...", "hash": "..."} image objects.
func (q *Question) UnmarshalJSON(data []byte) error {
	type questionAlias Question
	var decoded struct {
		*questionAlias
		QuestionImages []json.RawMessage `json:"question_images"`
	}
	decoded.questionAlias = (*questionAlias)(q)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	urls, hashes, err := decodeImageReferences(decoded.QuestionImages)
	if err != nil {
		return fmt.Errorf("invalid question_images: %w", err)
	}
	q.QuestionImages = urls
	q.QuestionImageHashes = hashes
	return nil
}

func (a *Answer) UnmarshalJSON(data []byte) error {
	type answerAlias Answer
	var decoded struct {
		*answerAlias
		Images []json.RawMessage `json:"images"`
	}
	decoded.answerAlias = (*answerAlias)(a)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	urls, hashes, err := decodeImageReferences(decoded.Images)
	if err != nil {
		return fmt.Errorf("invalid images: %w", err)
	}
	a.Images = urls
	a.ImageHashes = hashes
	return nil
}

func decodeImageReferences(images []json.RawMessage) ([]string, []string, error) {
	urls := make([]string, len(images))
	hashes := make([]string, len(images))
	for i, raw := range images {
		var image struct {
			URL  string `json:"url"`
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(raw, &image); err != nil {
			return nil, nil, fmt.Errorf("image %d must be an object with url and hash: %w", i+1, err)
		}
		if image.URL == "" {
			return nil, nil, fmt.Errorf("image %d has an empty url", i+1)
		}
		if len(image.Hash) != 64 {
			return nil, nil, fmt.Errorf("image %d hash must be a 64-character SHA-256 hex string", i+1)
		}
		if _, err := hex.DecodeString(image.Hash); err != nil {
			return nil, nil, fmt.Errorf("image %d hash must be a valid SHA-256 hex string: %w", i+1, err)
		}
		urls[i] = image.URL
		hashes[i] = image.Hash
	}
	return urls, hashes, nil
}
