package language

import (
	"context"
	"math"
	"strings"

	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/logger"
)

// Result is the output of language classification from the pipeline.
type Result struct {
	Label      string
	Confidence float32
	Transcript string
}

// Pipeline orchestrates Whisper transcription and FastText language
// classification to produce a language identification from audio samples.
type Pipeline struct {
	transcriber        Transcriber
	languageClassifier LanguageClassifier
	minConfidence      float32
}

// NewPipeline creates a language classification pipeline.
func NewPipeline(transcriber Transcriber, languageClassifier LanguageClassifier, minConfidence float32) *Pipeline {
	if minConfidence <= 0 || math.IsNaN(float64(minConfidence)) {
		minConfidence = 0.60
	}
	if minConfidence > 1 {
		minConfidence = 1
	}

	return &Pipeline{
		transcriber:        transcriber,
		languageClassifier: languageClassifier,
		minConfidence:      minConfidence,
	}
}

// Classify converts float32 audio samples to PCM16, transcribes them with
// Whisper, and identifies the language with FastText. If FastText fails,
// it falls back to Whisper's detected language.
func (p *Pipeline) Classify(ctx context.Context, samples []float32) (Result, error) {
	log := GetLogger()

	log.Info("language pipeline Classify called",
		logger.Int("sample_count", len(samples)))

	pcmData := Float32ToPCM16(samples)

	log.Info("sending audio to whisper",
		logger.Int("pcm_bytes", len(pcmData)),
		logger.Int("original_samples", len(samples)))

	transcription, err := p.transcriber.Transcribe(ctx, pcmData, 48000)
	if err != nil {
		log.Error("whisper transcription failed",
			logger.Error(err))
		return Result{}, errors.New(err).
			Component("classifier.language").
			Category(errors.CategoryNetwork).
			Context("operation", "transcribe").
			Build()
	}

	log.Info("whisper transcription complete",
		logger.String("text", truncate(transcription.Text, 100)),
		logger.String("language", transcription.Language),
		logger.Int("segment_count", len(transcription.Segments)))

	cleanText := strings.Join(strings.Fields(transcription.Text), " ")

	if cleanText == "" {
		// Avoid dropping the entire pipeline output when Whisper can't produce text.
		// Emit an explicit "und" (undetermined) label so downstream SSE/UI stays alive.
		log.Warn("whisper returned empty transcription; emitting undetermined language",
			logger.String("whisper_language", transcription.Language))
		return Result{Label: "und", Confidence: 0, Transcript: ""}, nil
	}

	langResult, err := p.languageClassifier.Classify(ctx, cleanText)
	if err != nil {
		log.Warn("fasttext classification failed, falling back to whisper language",
			logger.Error(err),
			logger.String("whisper_language", transcription.Language))

		if transcription.Language == "" {
			return Result{}, errors.Newf("both fasttext and whisper language detection failed").
				Component("classifier.language").
				Category(errors.CategoryProcessing).
				Build()
		}

		log.Info("language pipeline result (whisper fallback)",
			logger.String("language", transcription.Language),
			logger.String("transcription", truncate(transcription.Text, 200)))

		fallbackConfidence := float32(0.5)
		if fallbackConfidence < p.minConfidence {
			log.Info("whisper fallback below confidence threshold; emitting undetermined label",
				logger.String("raw_language", transcription.Language),
				logger.Float64("raw_confidence", float64(fallbackConfidence)),
				logger.Float64("min_confidence", float64(p.minConfidence)))
			return Result{
				Label:      "und",
				Confidence: fallbackConfidence,
				Transcript: cleanText,
			}, nil
		}

		return Result{
			Label:      transcription.Language,
			Confidence: fallbackConfidence,
			Transcript: cleanText,
		}, nil
	}

	log.Info("language pipeline result",
		logger.String("language", langResult.Label),
		logger.Float64("confidence", float64(langResult.Confidence)),
		logger.String("transcription", truncate(transcription.Text, 200)),
		logger.Int("top_n_count", len(langResult.TopN)))

	if langResult.Confidence < p.minConfidence {
		log.Info("language below confidence threshold; emitting undetermined label",
			logger.String("raw_language", langResult.Label),
			logger.Float64("raw_confidence", float64(langResult.Confidence)),
			logger.Float64("min_confidence", float64(p.minConfidence)))
		return Result{
			Label:      "und",
			Confidence: langResult.Confidence,
			Transcript: cleanText,
		}, nil
	}

	for i, tn := range langResult.TopN {
		log.Info("fasttext top_n",
			logger.Int("rank", i+1),
			logger.String("label", tn.Label),
			logger.Float64("confidence", float64(tn.Confidence)))
	}

	return Result{
		Label:      langResult.Label,
		Confidence: langResult.Confidence,
		Transcript: cleanText,
	}, nil
}

// Close releases resources held by the pipeline and its clients.
func (p *Pipeline) Close() error {
	errs := make([]error, 0, 2)

	if err := p.transcriber.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := p.languageClassifier.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	suffix := "..."
	if maxLen <= len(suffix) {
		return suffix[:maxLen]
	}
	return s[:maxLen-len(suffix)] + suffix
}
