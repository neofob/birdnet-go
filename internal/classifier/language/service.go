package language

import "context"

// Transcriber converts audio PCM data into a text transcription.
type Transcriber interface {
	Transcribe(ctx context.Context, pcmData []byte, sampleRate int) (*TranscriptionResult, error)
	Close() error
}

// LanguageClassifier identifies the language of a given text string.
type LanguageClassifier interface {
	Classify(ctx context.Context, text string) (*LanguageResult, error)
	Close() error
}
