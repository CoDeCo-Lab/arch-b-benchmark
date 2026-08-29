package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const anthropicAPIVersion = "2023-06-01"

type APIMessage struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Content struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	Source *Image `json:"source,omitempty"`
}

type Image struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicBatchClient struct {
	Model   string
	APIKey  string
	BaseURL string
	Client  *http.Client
}

func NewAnthropicBatchClient(model string) AnthropicBatchClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("Warning: ANTHROPIC_API_KEY environment variable not set")
	}

	return AnthropicBatchClient{
		Model:   model,
		APIKey:  apiKey,
		BaseURL: "https://api.anthropic.com/v1",
		Client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type AnthropicBatchCreateRequest struct {
	Requests []AnthropicBatchRequest `json:"requests"`
}

type AnthropicBatchRequest struct {
	CustomID string                 `json:"custom_id"`
	Params   AnthropicMessageParams `json:"params"`
}

type AnthropicMessageParams struct {
	Model       string       `json:"model"`
	MaxTokens   int          `json:"max_tokens"`
	Temperature float32      `json:"temperature,omitempty"`
	System      string       `json:"system,omitempty"`
	Messages    []APIMessage `json:"messages"`
}

type AnthropicBatch struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	ProcessingStatus string                 `json:"processing_status"`
	RequestCounts    AnthropicRequestCounts `json:"request_counts"`
}

type AnthropicRequestCounts struct {
	Processing int `json:"processing"`
	Succeeded  int `json:"succeeded"`
	Errored    int `json:"errored"`
	Canceled   int `json:"canceled"`
	Expired    int `json:"expired"`
}

type AnthropicBatchResultLine struct {
	CustomID string               `json:"custom_id"`
	Result   AnthropicBatchResult `json:"result"`
}

type AnthropicBatchResult struct {
	Type    string                    `json:"type"`
	Message *AnthropicMessageResponse `json:"message,omitempty"`
	Error   json.RawMessage           `json:"error,omitempty"`
}

type AnthropicMessageResponse struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Role    string            `json:"role"`
	Model   string            `json:"model"`
	Content []ResponseContent `json:"content"`
}

type AnthropicAPIError struct {
	Type    string             `json:"type"`
	Message string             `json:"message"`
	Error   *AnthropicAPIError `json:"error,omitempty"`
}

func (c AnthropicBatchClient) Query(messages []Message) (LLMResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	customID := "request-" + uuid.NewString()
	apiMessages, systemPrompt := c.buildMessages(messages)
	createReq := AnthropicBatchCreateRequest{
		Requests: []AnthropicBatchRequest{
			{
				CustomID: customID,
				Params: AnthropicMessageParams{
					Model:       c.Model,
					MaxTokens:   4096,
					Temperature: 0.0,
					System:      systemPrompt,
					Messages:    apiMessages,
				},
			},
		},
	}

	batch, err := c.createBatch(ctx, createReq)
	if err != nil {
		return LLMResponse{}, err
	}

	batch, err = c.waitForBatch(ctx, batch.ID)
	if err != nil {
		return LLMResponse{}, err
	}

	responseText, err := c.fetchBatchResult(ctx, batch.ID, customID)
	if err != nil {
		return LLMResponse{}, err
	}

	return parseStructuredResponse(responseText)
}

func (c AnthropicBatchClient) createBatch(ctx context.Context, reqBody AnthropicBatchCreateRequest) (AnthropicBatch, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return AnthropicBatch{}, fmt.Errorf("error marshaling Anthropic batch request: %w", err)
	}

	var batch AnthropicBatch
	if err := c.doJSON(ctx, http.MethodPost, "/messages/batches", body, &batch); err != nil {
		return AnthropicBatch{}, err
	}
	if batch.ID == "" {
		return AnthropicBatch{}, fmt.Errorf("Anthropic batch response missing id")
	}
	return batch, nil
}

func (c AnthropicBatchClient) waitForBatch(ctx context.Context, batchID string) (AnthropicBatch, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		var batch AnthropicBatch
		if err := c.doJSON(ctx, http.MethodGet, "/messages/batches/"+batchID, nil, &batch); err != nil {
			return AnthropicBatch{}, err
		}

		switch batch.ProcessingStatus {
		case "ended":
			return batch, nil
		case "in_progress", "canceling":
			select {
			case <-ctx.Done():
				return AnthropicBatch{}, fmt.Errorf("timed out waiting for Anthropic batch %s: %w", batchID, ctx.Err())
			case <-ticker.C:
			}
		default:
			return AnthropicBatch{}, fmt.Errorf("unexpected Anthropic batch status %q for batch %s", batch.ProcessingStatus, batchID)
		}
	}
}

func (c AnthropicBatchClient) fetchBatchResult(ctx context.Context, batchID string, customID string) (string, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/messages/batches/"+batchID+"/results", nil)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var resultLine AnthropicBatchResultLine
		if err := json.Unmarshal([]byte(line), &resultLine); err != nil {
			return "", fmt.Errorf("error unmarshaling Anthropic batch result: %w", err)
		}
		if resultLine.CustomID != customID {
			continue
		}

		if resultLine.Result.Type != "succeeded" {
			if len(resultLine.Result.Error) > 0 {
				return "", fmt.Errorf("Anthropic batch request %s failed: %s", customID, formatAnthropicBatchError(resultLine.Result.Error))
			}
			return "", fmt.Errorf("Anthropic batch request %s finished with result type %q", customID, resultLine.Result.Type)
		}
		if resultLine.Result.Message == nil {
			return "", fmt.Errorf("Anthropic batch request %s succeeded without a message", customID)
		}

		var text strings.Builder
		for _, content := range resultLine.Result.Message.Content {
			if content.Type == "text" {
				text.WriteString(content.Text)
			}
		}
		if text.Len() == 0 {
			return "", fmt.Errorf("Anthropic batch request %s returned no text content", customID)
		}
		return text.String(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading Anthropic batch results: %w", err)
	}

	return "", fmt.Errorf("Anthropic batch results did not include custom_id %s", customID)
}

func formatAnthropicBatchError(raw json.RawMessage) string {
	var apiError AnthropicAPIError
	if err := json.Unmarshal(raw, &apiError); err == nil {
		if apiError.Error != nil {
			apiError = *apiError.Error
		}
		if apiError.Type != "" && apiError.Message != "" {
			return fmt.Sprintf("%s: %s", apiError.Type, apiError.Message)
		}
		if apiError.Message != "" {
			return apiError.Message
		}
		if apiError.Type != "" {
			return apiError.Type + ": " + string(raw)
		}
	}

	return string(raw)
}

func (c AnthropicBatchClient) doJSON(ctx context.Context, method string, path string, body []byte, out any) error {
	responseBody, err := c.doRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("error unmarshaling Anthropic response: %w", err)
	}
	return nil
}

func (c AnthropicBatchClient) doRequest(ctx context.Context, method string, path string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("error creating Anthropic request: %w", err)
	}

	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making Anthropic request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading Anthropic response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Anthropic API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	return responseBody, nil
}

func (c AnthropicBatchClient) buildMessages(messages []Message) ([]APIMessage, string) {
	apiMessages := make([]APIMessage, 0, len(messages))
	var systemText strings.Builder

	for _, message := range messages {
		if message.Role != MessageRoleSystem {
			continue
		}
		for _, content := range message.Content {
			if content.Type == ContentTypeText {
				systemText.WriteString(content.Text)
			}
		}
	}

	for _, message := range messages {
		if message.Role == MessageRoleSystem {
			continue
		}

		contents := make([]Content, 0, len(message.Content))
		for _, content := range message.Content {
			switch content.Type {
			case ContentTypeText:
				contents = append(contents, Content{
					Type: "text",
					Text: content.Text,
				})
			case ContentTypeImage:
				contents = append(contents, Content{
					Type: "image",
					Source: &Image{
						Type:      "base64",
						MediaType: "image/jpeg",
						Data:      base64.StdEncoding.EncodeToString(content.Data),
					},
				})
			}
		}

		apiMessages = append(apiMessages, APIMessage{
			Role:    "user",
			Content: contents,
		})
	}

	return apiMessages, systemText.String()
}
