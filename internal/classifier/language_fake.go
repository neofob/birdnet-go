package classifier

import (
	"context"
	"math"
	"strings"

	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore"
	"github.com/tphakala/birdnet-go/internal/errors"
)

const (
	fakeLanguageModelID      = "Language_Fake"
	fakeLanguageName         = "English"
	fakeLanguageLabel        = fakeLanguageName + "_" + fakeLanguageName
	fakeLanguageModelVersion = "fake"
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
		classifier:       newLanguagePipelineClassifier(fakeLanguagePipeline{}),
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

type fakeLanguagePipeline struct{}

func (fakeLanguagePipeline) Classify(_ context.Context, _ []float32) (PipelineClassification, error) {
	return PipelineClassification{Label: fakeLanguageName, Confidence: 0.99}, nil
}

type languagePipelineClassifier struct {
	pipeline AudioPipeline
}

func newLanguagePipelineClassifier(pipeline AudioPipeline) languagePipelineClassifier {
	return languagePipelineClassifier{pipeline: pipeline}
}

func (c languagePipelineClassifier) Predict(samples []float32) ([]float32, error) {
	classification, err := c.pipeline.Classify(context.Background(), samples)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(classification.Label, fakeLanguageName) {
		return []float32{confidenceToLogit(minPipelineConfidence)}, nil
	}

	return []float32{confidenceToLogit(classification.Confidence)}, nil
}

func (languagePipelineClassifier) NumSpecies() int { return 1 }

func (languagePipelineClassifier) Close() {}

func confidenceToLogit(confidence float32) float32 {
	confidence = min(max(confidence, minPipelineConfidence), maxPipelineConfidence)
	p := float64(confidence)
	return float32(math.Log(p / (1 - p)))
}
