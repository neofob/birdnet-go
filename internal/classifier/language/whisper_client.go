package language

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/httpclient"
	"github.com/tphakala/birdnet-go/internal/logger"
)

// whisperResponse models the verbose_json response from the whisper.cpp server.
type whisperResponse struct {
	Text     string                `json:"text"`
	Language string                `json:"language"`
	Segments []whisperSegment      `json:"segments,omitempty"`
}

// whisperSegment is a single transcription segment from whisper.cpp.
type whisperSegment struct {
	ID        int     `json:"id"`
	Start     float32 `json:"start"`
	End       float32 `json:"end"`
	Text      string  `json:"text"`
}

// WhisperClient implements Transcriber by calling a whisper.cpp HTTP server.
type WhisperClient struct {
	client   *httpclient.Client
	endpoint string
	timeout  time.Duration
	language string
}

// NewWhisperClient creates a Whisper HTTP client.
func NewWhisperClient(cfg WhisperConfig) *WhisperClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:8080"
	}
	if cfg.Language == "" {
		cfg.Language = "auto"
	}

	return &WhisperClient{
		client:   httpclient.New(&httpclient.Config{DefaultTimeout: timeout}),
		endpoint: cfg.Endpoint,
		timeout:  timeout,
		language: cfg.Language,
	}
}

// Transcribe sends audio PCM data to the whisper.cpp server and returns
// the transcription result.
func (w *WhisperClient) Transcribe(ctx context.Context, pcmData []byte, sampleRate int) (*TranscriptionResult, error) {
	wavData, err := PCM16ToWAVResampled(pcmData, sampleRate, 16000)
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return nil, errors.Newf("failed to create multipart form: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}

	if _, err := io.Copy(part, bytes.NewReader(wavData)); err != nil {
		return nil, errors.Newf("failed to write audio data to multipart form: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}

	_ = writer.WriteField("response_format", "verbose_json")
	if w.language != "auto" {
		_ = writer.WriteField("language", w.language)
	}

	if err := writer.Close(); err != nil {
		return nil, errors.Newf("failed to close multipart writer: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}

	url := fmt.Sprintf("%s/inference", w.endpoint)
	resp, err := w.client.Post(ctx, url, writer.FormDataContentType(), body.Bytes())
	if err != nil {
		return nil, errors.New(err).
			Component("classifier.language.whisper").
			Category(errors.CategoryNetwork).
			Context("url", url).
			Build()
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Newf("failed to read whisper response: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Newf("whisper server returned status %d: %s", resp.StatusCode, string(respBody)).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Context("status_code", resp.StatusCode).
			Build()
	}

	var wResp whisperResponse
	if err := json.Unmarshal(respBody, &wResp); err != nil {
		return nil, errors.Newf("failed to parse whisper response: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Context("response_body", string(respBody)).
			Build()
	}

	segments := make([]TranscriptSegment, 0, len(wResp.Segments))
	for _, s := range wResp.Segments {
		segments = append(segments, TranscriptSegment{
			ID:    s.ID,
			Start: s.Start,
			End:   s.End,
			Text:  s.Text,
		})
	}

	return &TranscriptionResult{
		Text:     wResp.Text,
		Language: wResp.Language,
		Segments: segments,
	}, nil
}

// Close releases the HTTP client resources.
func (w *WhisperClient) Close() error {
	w.client.Close()
	GetLogger().Info("whisper client closed",
		logger.String("endpoint", w.endpoint))
	return nil
}
