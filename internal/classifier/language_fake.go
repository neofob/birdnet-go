package classifier

import (
	"context"
	"math"

	"github.com/tphakala/birdnet-go/internal/classifier/language"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/logger"
)

const (
	fakeLanguageModelID      = "Language_Fake"
	realLanguageModelID      = "Language"
	fakeLanguageName         = "English"
	fakeLanguageLabel        = fakeLanguageName + "_" + fakeLanguageName
	fakeLanguageModelVersion = "fake"
	realLanguageModelVersion = "1.0"
	realLanguageNumSpecies   = 176
	minPipelineConfidence    = 0.0001
	maxPipelineConfidence    = 0.9999
)

// UseFakeLanguagePipeline reports whether settings select the placeholder
// language pipeline instead of the normal BirdNET classifier.
func UseFakeLanguagePipeline(settings *conf.Settings) bool {
	if settings == nil {
		return false
	}
	if len(settings.Models.Enabled) == 0 {
		return true
	}
	for _, configID := range settings.Models.Enabled {
		registryID, ok := ResolveConfigModelID(configID)
		if ok && registryID == fakeLanguageModelID {
			return true
		}
	}
	return false
}

// UseRealLanguagePipeline reports whether settings enable the real language
// classification pipeline (Whisper + FastText).
func UseRealLanguagePipeline(settings *conf.Settings) bool {
	if settings == nil {
		GetLogger().Info("UseRealLanguagePipeline: settings is nil")
		return false
	}
	GetLogger().Info("UseRealLanguagePipeline check",
		logger.Bool("enabled", settings.LanguagePipeline.Enabled),
		logger.String("whisper_endpoint", settings.LanguagePipeline.Whisper.Endpoint))
	return settings.LanguagePipeline.Enabled
}

// NewFakeLanguageOrchestrator creates a BirdNET-compatible orchestrator backed
// by a placeholder language classifier. It keeps the current audio pipeline
// operational until the real language pipeline is available.
func NewFakeLanguageOrchestrator(settings *conf.Settings) (*Orchestrator, error) {
	info, ok := ModelRegistry[fakeLanguageModelID]
	if !ok {
		return nil, errors.Newf("fake language model %s not found in registry", fakeLanguageModelID).
			Component("classifier.language").
			Category(errors.CategoryModelInit).
			Build()
	}

	settings.BirdNET.Labels = []string{fakeLanguageLabel}

	bn := &BirdNET{
		classifier:       newLanguagePipelineClassifier(fakeLanguagePipeline{}, nil, nil),
		Settings:         settings,
		ModelInfo:        info,
		modelVersion:     fakeLanguageModelVersion,
		resultsBuffer:    make([]datastore.Results, info.NumSpecies),
		confidenceBuffer: make([]float32, info.NumSpecies),
		speciesCache:     make(map[string]*speciesCacheEntry),
	}

	return &Orchestrator{
		Settings:  settings,
		ModelInfo: info,
		models: map[string]*modelEntry{
			info.ID: {instance: bn},
		},
		primary: bn,
	}, nil
}

// NewRealLanguageOrchestrator creates a BirdNET-compatible orchestrator backed
// by the real language classification pipeline (Whisper + FastText).
func NewRealLanguageOrchestrator(settings *conf.Settings) (*Orchestrator, error) {
	info, ok := ModelRegistry[realLanguageModelID]
	if !ok {
		return nil, errors.Newf("language model %s not found in registry", realLanguageModelID).
			Component("classifier.language").
			Category(errors.CategoryModelInit).
			Build()
	}

	cfg := settings.LanguagePipeline

	whisperClient := language.NewWhisperClient(language.WhisperConfig{
		Endpoint: cfg.Whisper.Endpoint,
		Timeout:  cfg.Whisper.Timeout,
		Language: cfg.Whisper.Language,
	})

	fastTextClient := language.NewFastTextClient(language.FastTextConfig{
		Endpoint: cfg.FastText.Endpoint,
		Timeout:  cfg.FastText.Timeout,
		MaxTopN:  cfg.FastText.MaxTopN,
	})

	pipeline := language.NewPipeline(whisperClient, fastTextClient)
	adapter := newRealPipelineAdapter(pipeline)

	settings.BirdNET.Labels = []string{"language"}
	// The language pipeline produces a single top-1 label per inference (the detected
	// language code). We size the reusable buffers to match that 1-element output.
	// This keeps BirdNET.Predict's reuse helpers happy (buffer length must match
	// labels/predictions length) while still allowing FastText to compute top-N
	// internally.
	const languagePipelineOutputSize = 1

	bn := &BirdNET{
		classifier:       newLanguagePipelineClassifier(adapter, settings, pipeline),
		Settings:         settings,
		ModelInfo:        info,
		modelVersion:     realLanguageModelVersion,
		resultsBuffer:    make([]datastore.Results, languagePipelineOutputSize),
		confidenceBuffer: make([]float32, languagePipelineOutputSize),
		speciesCache:     make(map[string]*speciesCacheEntry),
	}

	GetLogger().Info("language classification pipeline initialized",
		logger.String("whisper_endpoint", cfg.Whisper.Endpoint),
		logger.String("fasttext_endpoint", cfg.FastText.Endpoint))

	return &Orchestrator{
		Settings:  settings,
		ModelInfo: info,
		models: map[string]*modelEntry{
			info.ID: {instance: bn},
		},
		primary: bn,
	}, nil
}

type fakeLanguagePipeline struct{}

func (fakeLanguagePipeline) Classify(_ context.Context, _ []float32) (PipelineClassification, error) {
	return PipelineClassification{Label: fakeLanguageName, Confidence: 0.99}, nil
}

// realPipelineAdapter adapts a language.Pipeline (which returns language.Result)
// to the AudioPipeline interface (which returns PipelineClassification).
type realPipelineAdapter struct {
	pipeline *language.Pipeline
}

func newRealPipelineAdapter(pipeline *language.Pipeline) realPipelineAdapter {
	return realPipelineAdapter{pipeline: pipeline}
}

func (a realPipelineAdapter) Classify(ctx context.Context, samples []float32) (PipelineClassification, error) {
	GetLogger().Info("realPipelineAdapter.Classify called",
		logger.Int("sample_count", len(samples)))

	result, err := a.pipeline.Classify(ctx, samples)
	if err != nil {
		GetLogger().Error("realPipelineAdapter.Classify failed",
			logger.Error(err))
		return PipelineClassification{}, err
	}
	return PipelineClassification{
		Label:      result.Label,
		Confidence: result.Confidence,
		Transcript: result.Transcript,
	}, nil
}

type languagePipelineClassifier struct {
	pipeline        AudioPipeline
	settings        *conf.Settings
	pipelineCloser  *language.Pipeline
	lastTranscript  string
}

func newLanguagePipelineClassifier(pipeline AudioPipeline, settings *conf.Settings, pipelineCloser *language.Pipeline) *languagePipelineClassifier {
	return &languagePipelineClassifier{pipeline: pipeline, settings: settings, pipelineCloser: pipelineCloser}
}

func (c *languagePipelineClassifier) Predict(samples []float32) ([]float32, error) {
	GetLogger().Info("languagePipelineClassifier.Predict called",
		logger.Int("sample_count", len(samples)))

	classification, err := c.pipeline.Classify(context.Background(), samples)
	if err != nil {
		GetLogger().Error("languagePipelineClassifier.Predict failed",
			logger.Error(err))
		return nil, err
	}

	c.lastTranscript = classification.Transcript

	if c.settings != nil && len(c.settings.BirdNET.Labels) > 0 {
		c.settings.BirdNET.Labels[0] = classification.Label
	}

	return []float32{confidenceToLogit(classification.Confidence)}, nil
}

func (c *languagePipelineClassifier) GetTranscript() string {
	return c.lastTranscript
}

func (languagePipelineClassifier) NumSpecies() int { return 1 }

func (c languagePipelineClassifier) Close() {
	if c.pipelineCloser != nil {
		if err := c.pipelineCloser.Close(); err != nil {
			GetLogger().Warn("failed to close language pipeline",
				logger.Error(err))
		}
	}
}

func confidenceToLogit(confidence float32) float32 {
	confidence = min(max(confidence, minPipelineConfidence), maxPipelineConfidence)
	p := float64(confidence)
	return float32(math.Log(p / (1 - p)))
}
