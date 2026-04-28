package language

import "time"

// TranscriptionResult holds the output from a speech-to-text transcription.
type TranscriptionResult struct {
	Text       string              `json:"text"`
	Language   string              `json:"language"`
	Confidence float32             `json:"confidence"`
	Segments   []TranscriptSegment `json:"segments,omitempty"`
	Duration   float32             `json:"duration"`
}

// TranscriptSegment is a single timestamped segment of a transcription.
type TranscriptSegment struct {
	ID         int     `json:"id"`
	Start      float32 `json:"start"`
	End        float32 `json:"end"`
	Text       string  `json:"text"`
	Confidence float32 `json:"confidence,omitempty"`
}

// LanguageResult holds the output from language identification.
type LanguageResult struct {
	Label      string       `json:"label"`
	Confidence float32      `json:"confidence"`
	TopN       []LabelScore `json:"top_n,omitempty"`
}

// LabelScore pairs a language label with its confidence score.
type LabelScore struct {
	Label      string  `json:"label"`
	Confidence float32 `json:"confidence"`
}

// Config holds the settings for the language classification pipeline.
type Config struct {
	Whisper  WhisperConfig
	FastText FastTextConfig
}

// WhisperConfig holds settings for the Whisper transcription service.
type WhisperConfig struct {
	Endpoint string
	Timeout  time.Duration
	Language string
}

// FastTextConfig holds settings for the FastText language identification service.
type FastTextConfig struct {
	Endpoint string
	Timeout  time.Duration
	MaxTopN  int
}
