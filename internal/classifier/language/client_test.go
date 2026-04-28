package language

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhisperClient_Transcribe_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/inference", r.URL.Path)
		require.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"text": "hello world",
			"language": "en",
			"segments": [{"id": 0, "start": 0.0, "end": 1.5, "text": "hello world"}]
		}`))
	}))
	defer srv.Close()

	client := NewWhisperClient(WhisperConfig{
		Endpoint: srv.URL,
		Timeout:  5 * time.Second,
		Language: "auto",
	})

	samples := make([]float32, 144000)
	result, err := client.Transcribe(context.Background(), Float32ToPCM16(samples), 48000)

	require.NoError(t, err)
	assert.Equal(t, "hello world", result.Text)
	assert.Equal(t, "en", result.Language)
	assert.Len(t, result.Segments, 1)
}

func TestWhisperClient_Transcribe_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewWhisperClient(WhisperConfig{
		Endpoint: srv.URL,
		Timeout:  5 * time.Second,
	})

	samples := make([]float32, 144000)
	_, err := client.Transcribe(context.Background(), Float32ToPCM16(samples), 48000)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFastTextClient_Classify_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/classify", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"label": "en",
			"confidence": 0.98,
			"top_n": [{"label": "en", "confidence": 0.98}, {"label": "fr", "confidence": 0.01}]
		}`))
	}))
	defer srv.Close()

	client := NewFastTextClient(FastTextConfig{
		Endpoint: srv.URL,
		Timeout:  5 * time.Second,
		MaxTopN:  5,
	})

	result, err := client.Classify(context.Background(), "hello world")

	require.NoError(t, err)
	assert.Equal(t, "en", result.Label)
	assert.InDelta(t, 0.98, float64(result.Confidence), 0.001)
	require.Len(t, result.TopN, 2)
	assert.Equal(t, "en", result.TopN[0].Label)
}

func TestFastTextClient_Classify_EmptyText(t *testing.T) {
	t.Parallel()

	client := NewFastTextClient(FastTextConfig{
		Endpoint: "http://localhost:9999",
		Timeout:  5 * time.Second,
	})

	_, err := client.Classify(context.Background(), "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty text")
}

func TestFastTextClient_Classify_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := NewFastTextClient(FastTextConfig{
		Endpoint: srv.URL,
		Timeout:  5 * time.Second,
	})

	_, err := client.Classify(context.Background(), "some text")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}
