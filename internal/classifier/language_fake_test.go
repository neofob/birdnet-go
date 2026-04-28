package classifier

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/conf"
)

type testAudioPipeline struct {
	classification PipelineClassification
}

func (p testAudioPipeline) Classify(_ context.Context, _ []float32) (PipelineClassification, error) {
	return p.classification, nil
}

func TestLanguagePipelineClassifier_PredictUsesPipelineConfidence(t *testing.T) {
	t.Parallel()

	classifier := newLanguagePipelineClassifier(testAudioPipeline{
		classification: PipelineClassification{Label: fakeLanguageName, Confidence: 0.75},
	}, nil, nil)

	logits, err := classifier.Predict([]float32{0.1, 0.2, 0.3})
	require.NoError(t, err)
	require.Len(t, logits, 1)
	assert.InDelta(t, 1.0986, float64(logits[0]), 0.001)
}

func TestLanguagePipelineClassifier_AnyLabelUsesConfidence(t *testing.T) {
	t.Parallel()

	classifier := newLanguagePipelineClassifier(testAudioPipeline{
		classification: PipelineClassification{Label: "French", Confidence: 0.99},
	}, nil, nil)

	logits, err := classifier.Predict([]float32{0.1, 0.2, 0.3})
	require.NoError(t, err)
	require.Len(t, logits, 1)
	assert.InDelta(t, confidenceToLogit(0.99), float64(logits[0]), 0.001)
}

func TestLanguagePipelineClassifier_PreservesDynamicLabel(t *testing.T) {
	t.Parallel()

	settings := &conf.Settings{BirdNET: conf.BirdNETConfig{Labels: []string{"placeholder"}}}
	classifier := newLanguagePipelineClassifier(testAudioPipeline{
		classification: PipelineClassification{Label: "fr", Confidence: 0.95},
	}, settings, nil)

	_, err := classifier.Predict([]float32{0.1, 0.2})
	require.NoError(t, err)
	assert.Equal(t, "fr", settings.BirdNET.Labels[0])
}

func TestLanguagePipelineClassifier_NilSettingsDoesNotPanic(t *testing.T) {
	t.Parallel()

	classifier := newLanguagePipelineClassifier(testAudioPipeline{
		classification: PipelineClassification{Label: "de", Confidence: 0.80},
	}, nil, nil)

	logits, err := classifier.Predict([]float32{0.1, 0.2})
	require.NoError(t, err)
	require.Len(t, logits, 1)
}

func TestFakeLanguagePipeline_Classify(t *testing.T) {
	t.Parallel()

	classification, err := fakeLanguagePipeline{}.Classify(t.Context(), []float32{0.1, 0.2, 0.3})
	require.NoError(t, err)
	assert.Equal(t, fakeLanguageName, classification.Label)
	assert.InDelta(t, 0.99, float64(classification.Confidence), 0.001)
}
