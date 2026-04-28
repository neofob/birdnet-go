package classifier

import "context"

// PipelineClassification is the output from a generic audio classification pipeline.
type PipelineClassification struct {
	Label      string
	Confidence float32
}

// AudioPipeline classifies audio samples without exposing model-runtime details.
// The current language implementation is fake; the real pipeline should satisfy
// this interface when it is available.
type AudioPipeline interface {
	Classify(ctx context.Context, samples []float32) (PipelineClassification, error)
}
