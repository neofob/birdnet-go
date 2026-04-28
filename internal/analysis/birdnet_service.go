package analysis

import (
	"context"

	"github.com/tphakala/birdnet-go/internal/app"
	"github.com/tphakala/birdnet-go/internal/classifier"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/errors"
	"github.com/tphakala/birdnet-go/internal/logger"
)

// classifierAnalyzerName is the service name used for logging and diagnostics.
const classifierAnalyzerName = "classifier-analyzer"

// ClassifierAnalyzer wraps classifier initialization as an app.Service and
// implements app.Analyzer for source-to-analyzer routing.
type ClassifierAnalyzer struct {
	settings *conf.Settings
	bn       *classifier.Orchestrator
}

// BirdNETAnalyzer is a compatibility alias for callers not yet migrated to
// ClassifierAnalyzer.
type BirdNETAnalyzer = ClassifierAnalyzer

// NewClassifierAnalyzer creates a new ClassifierAnalyzer with the given settings.
// The analyzer is not started; call Start() to initialize the classifier.
func NewClassifierAnalyzer(settings *conf.Settings) *ClassifierAnalyzer {
	return &ClassifierAnalyzer{settings: settings}
}

// NewBirdNETAnalyzer creates a classifier analyzer for legacy callers.
func NewBirdNETAnalyzer(settings *conf.Settings) *ClassifierAnalyzer {
	return NewClassifierAnalyzer(settings)
}

// Name returns a human-readable identifier for logging and diagnostics.
func (a *ClassifierAnalyzer) Name() string {
	return classifierAnalyzerName
}

// Start initializes the configured classifier and builds the species range
// filter only when BirdNET is explicitly selected. The placeholder language
// pipeline is the default primary classifier.
func (a *ClassifierAnalyzer) Start(_ context.Context) error {
	if classifier.UseRealLanguagePipeline(a.settings) {
		bn, err := classifier.NewRealLanguageOrchestrator(a.settings)
		if err != nil {
			return errors.New(err).
				Component("analysis").
				Category(errors.CategoryModelInit).
				Context("operation", "initialize_real_language_pipeline").
				Build()
		}
		a.bn = bn
		return nil
	}

	if classifier.UseFakeLanguagePipeline(a.settings) {
		bn, err := classifier.NewFakeLanguageOrchestrator(a.settings)
		if err != nil {
			return errors.New(err).
				Component("analysis").
				Category(errors.CategoryModelInit).
				Context("operation", "initialize_language_pipeline").
				Build()
		}
		a.bn = bn
		return nil
	}

	bn, err := classifier.NewOrchestrator(a.settings)
	if err != nil {
		return errors.New(err).
			Component("analysis").
			Category(errors.CategoryModelInit).
			Context("operation", "initialize_birdnet").
			Build()
	}

	if err := classifier.BuildRangeFilter(bn); err != nil {
		bn.Delete()
		return errors.New(err).
			Component("analysis").
			Category(errors.CategoryModelInit).
			Context("operation", "build_range_filter").
			Build()
	}

	a.bn = bn
	return nil
}

// Stop releases classifier resources. It is safe to call before Start() or
// multiple times.
func (a *ClassifierAnalyzer) Stop(_ context.Context) error {
	if a.bn != nil {
		log := GetLogger()
		log.Info("stopping classifier",
			logger.String("service", classifierAnalyzerName))
		a.bn.Delete()
		a.bn = nil
	}
	return nil
}

// Compatible returns true if this analyzer can process audio from the given source.
// ClassifierAnalyzer handles all source types except ultrasonic (bat detection).
func (a *ClassifierAnalyzer) Compatible(source app.AudioSource) bool {
	return source.Type != app.SourceTypeUltrasonic
}

// Classifier returns the underlying classifier orchestrator, or nil if the
// analyzer has not been started. Callers must not use the returned pointer
// after Stop().
func (a *ClassifierAnalyzer) Classifier() *classifier.Orchestrator {
	return a.bn
}

// BirdNET returns the underlying classifier orchestrator, or nil if the analyzer
// has not been started. Callers must not use the returned pointer after Stop().
func (a *ClassifierAnalyzer) BirdNET() *classifier.Orchestrator {
	return a.Classifier()
}
