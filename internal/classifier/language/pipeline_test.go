package language

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTranscriber struct {
	result *TranscriptionResult
	err    error
}

func (m *mockTranscriber) Transcribe(_ context.Context, _ []byte, _ int) (*TranscriptionResult, error) {
	return m.result, m.err
}

func (m *mockTranscriber) Close() error { return nil }

type mockLanguageClassifier struct {
	result *LanguageResult
	err    error
}

func (m *mockLanguageClassifier) Classify(_ context.Context, _ string) (*LanguageResult, error) {
	return m.result, m.err
}

func (m *mockLanguageClassifier) Close() error { return nil }

func TestPipeline_Classify_Success(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(
		&mockTranscriber{
			result: &TranscriptionResult{
				Text:     "hello world",
				Language: "en",
			},
		},
		&mockLanguageClassifier{
			result: &LanguageResult{
				Label:      "en",
				Confidence: 0.98,
			},
		},
	)

	samples := make([]float32, 100)
	result, err := pipeline.Classify(context.Background(), samples)

	require.NoError(t, err)
	assert.Equal(t, "en", result.Label)
	assert.InDelta(t, 0.98, float64(result.Confidence), 0.001)
}

func TestPipeline_Classify_WhisperFails(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(
		&mockTranscriber{err: errors.New("connection refused")},
		&mockLanguageClassifier{},
	)

	samples := make([]float32, 100)
	_, err := pipeline.Classify(context.Background(), samples)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestPipeline_Classify_EmptyTranscription(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(
		&mockTranscriber{
			result: &TranscriptionResult{Text: "", Language: ""},
		},
		&mockLanguageClassifier{},
	)

	samples := make([]float32, 100)
	_, err := pipeline.Classify(context.Background(), samples)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty transcription")
}

func TestPipeline_Classify_FastTextFallsBackToWhisper(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(
		&mockTranscriber{
			result: &TranscriptionResult{
				Text:     "bonjour le monde",
				Language: "fr",
			},
		},
		&mockLanguageClassifier{err: errors.New("fasttext error")},
	)

	samples := make([]float32, 100)
	result, err := pipeline.Classify(context.Background(), samples)

	require.NoError(t, err)
	assert.Equal(t, "fr", result.Label)
	assert.InDelta(t, 0.5, float64(result.Confidence), 0.001)
}

func TestPipeline_Classify_FastTextAndWhisperLanguageFail(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(
		&mockTranscriber{
			result: &TranscriptionResult{
				Text:     "bonjour le monde",
				Language: "",
			},
		},
		&mockLanguageClassifier{err: errors.New("fasttext error")},
	)

	samples := make([]float32, 100)
	_, err := pipeline.Classify(context.Background(), samples)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "both fasttext and whisper language detection failed")
}

func TestPipeline_Close(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(
		&mockTranscriber{},
		&mockLanguageClassifier{},
	)

	err := pipeline.Close()
	assert.NoError(t, err)
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "he...", truncate("hello world", 5))
}
