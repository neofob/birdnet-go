package language

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/logger"
)

type whisperResponse struct {
	Text     string           `json:"text"`
	Language string           `json:"language"`
	Segments []whisperSegment `json:"segments,omitempty"`
}

type whisperSegment struct {
	ID    int     `json:"id"`
	Start float32 `json:"start"`
	End   float32 `json:"end"`
	Text  string  `json:"text"`
}

type WhisperClient struct {
	endpoint string
	timeout  time.Duration
	language string
	client   *http.Client
}

func NewWhisperClient(cfg WhisperConfig) *WhisperClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if cfg.Endpoint == "" {
		server := os.Getenv("WHISPER_SERVER")
		port := os.Getenv("WHISPER_PORT")
		if server != "" && port != "" {
			cfg.Endpoint = fmt.Sprintf("http://%s:%s", server, port)
		} else if server != "" {
			cfg.Endpoint = server
		} else {
			cfg.Endpoint = "http://localhost:8010"
		}
	}
	if cfg.Language == "" {
		cfg.Language = "auto"
	}

	return &WhisperClient{
		endpoint: cfg.Endpoint,
		timeout:  timeout,
		language: cfg.Language,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (w *WhisperClient) Transcribe(ctx context.Context, pcmData []byte, sampleRate int) (*TranscriptionResult, error) {
	log := GetLogger()

	wavData, err := PCM16ToWAVResampled(pcmData, sampleRate, 16000)
	if err != nil {
		return nil, err
	}

	tmpFile := fmt.Sprintf("/tmp/birdnet_debug_%d.wav", time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, wavData, 0644); err == nil {
		log.Info("wrote debug WAV",
			logger.String("path", tmpFile),
			logger.Int("wav_bytes", len(wavData)))
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return nil, errors.Newf("failed to create multipart file field: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}
	if _, err := part.Write(wavData); err != nil {
		return nil, errors.Newf("failed to write WAV data to multipart: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}

	if err := writer.WriteField("language", w.language); err != nil {
		return nil, errors.Newf("failed to write language field: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}

	if err := writer.Close(); err != nil {
		return nil, errors.Newf("failed to close multipart writer: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}

	reqURL := w.endpoint + "/inference"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, &body)
	if err != nil {
		return nil, errors.Newf("failed to create whisper request: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
			Build()
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	log.Info("sending audio to whisper",
		logger.Int("pcm_bytes", len(pcmData)),
		logger.Int("original_samples", len(pcmData)/2))

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, errors.Newf("whisper request failed: %w", err).
			Component("classifier.language.whisper").
			Category(errors.CategoryNetwork).
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
		log.Error("whisper server returned error",
			logger.Int("status", resp.StatusCode),
			logger.String("body", string(respBody)))
		return nil, errors.Newf("whisper server returned status %d: %s", resp.StatusCode, string(respBody)).
			Component("classifier.language.whisper").
			Category(errors.CategoryHTTP).
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

	log.Info("whisper transcription complete",
		logger.String("language", wResp.Language),
		logger.Int("text_len", len(wResp.Text)))

	segments := make([]TranscriptSegment, 0, len(wResp.Segments))
	for _, s := range wResp.Segments {
		segments = append(segments, TranscriptSegment{
			ID:    s.ID,
			Start: s.Start,
			End:    s.End,
			Text:  s.Text,
		})
	}

	return &TranscriptionResult{
		Text:     wResp.Text,
		Language: wResp.Language,
		Segments: segments,
	}, nil
}

func (w *WhisperClient) Close() error {
	GetLogger().Info("whisper client closed",
		logger.String("endpoint", w.endpoint))
	return nil
}
