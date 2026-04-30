package language

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/httpclient"
	"github.com/tphakala/birdnet-go/internal/logger"
)

// fastTextRequest is the JSON body sent to the FastText classification server.
type fastTextRequest struct {
	Text string `json:"text"`
}

// fastTextResponse is the JSON response from the FastText classification server.
type fastTextResponse struct {
	Label      string       `json:"label"`
	Confidence float32      `json:"confidence"`
	TopN       []labelScore `json:"top_n"`
}

// labelScore pairs a language label with its confidence score.
type labelScore struct {
	Label      string  `json:"label"`
	Confidence float32 `json:"confidence"`
}

// FastTextClient implements LanguageClassifier by calling a FastText HTTP server.
type FastTextClient struct {
	client   *httpclient.Client
	endpoint string
	timeout  time.Duration
	maxTopN  int
}

// NewFastTextClient creates a FastText HTTP client.
func NewFastTextClient(cfg FastTextConfig) *FastTextClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if cfg.Endpoint == "" {
		server := os.Getenv("FASTTEXT_SERVER")
		if server == "" {
			server = "localhost"
		}
		port := os.Getenv("FASTTEXT_PORT")
		if port == "" {
			port = "8000"
		}
		cfg.Endpoint = fmt.Sprintf("http://%s:%s", server, port)
	}
	if cfg.MaxTopN <= 0 {
		cfg.MaxTopN = 5
	}

	return &FastTextClient{
		client:   httpclient.New(&httpclient.Config{DefaultTimeout: timeout}),
		endpoint: cfg.Endpoint,
		timeout:  timeout,
		maxTopN:  cfg.MaxTopN,
	}
}

// Classify sends text to the FastText server and returns the language
// identification result.
func (f *FastTextClient) Classify(ctx context.Context, text string) (*LanguageResult, error) {
	log := GetLogger()
	log.Info("fasttext Classify called",
		logger.String("endpoint", f.endpoint),
		logger.Int("text_length", len(text)))

	if text == "" {
		return nil, errors.Newf("empty text provided for language classification").
			Component("classifier.language.fasttext").
			Category(errors.CategoryValidation).
			Build()
	}

	reqBody := fastTextRequest{Text: text}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.Newf("failed to marshal fasttext request: %w", err).
			Component("classifier.language.fasttext").
			Category(errors.CategoryHTTP).
			Build()
	}

	url := fmt.Sprintf("%s/classify", f.endpoint)
	resp, err := f.client.Post(ctx, url, "application/json", reqBytes)
	if err != nil {
		return nil, errors.New(err).
			Component("classifier.language.fasttext").
			Category(errors.CategoryNetwork).
			Context("url", url).
			Build()
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Newf("failed to read fasttext response: %w", err).
			Component("classifier.language.fasttext").
			Category(errors.CategoryHTTP).
			Build()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Newf("fasttext server returned status %d: %s", resp.StatusCode, string(respBody)).
			Component("classifier.language.fasttext").
			Category(errors.CategoryHTTP).
			Context("status_code", resp.StatusCode).
			Build()
	}

	var ftResp fastTextResponse
	if err := json.Unmarshal(respBody, &ftResp); err != nil {
		return nil, errors.Newf("failed to parse fasttext response: %w", err).
			Component("classifier.language.fasttext").
			Category(errors.CategoryHTTP).
			Context("response_body", string(respBody)).
			Build()
	}

	topN := make([]LabelScore, 0, len(ftResp.TopN))
	for _, s := range ftResp.TopN {
		topN = append(topN, LabelScore{
			Label:      s.Label,
			Confidence: s.Confidence,
		})
	}

	return &LanguageResult{
		Label:      ftResp.Label,
		Confidence: ftResp.Confidence,
		TopN:       topN,
	}, nil
}

// Close releases the HTTP client resources.
func (f *FastTextClient) Close() error {
	f.client.Close()
	GetLogger().Info("fasttext client closed",
		logger.String("endpoint", f.endpoint))
	return nil
}
