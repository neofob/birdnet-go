package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tphakala/birdnet-go/internal/app"
	"github.com/tphakala/birdnet-go/internal/conf"
)

// Compile-time interface compliance check.
var _ app.Analyzer = (*ClassifierAnalyzer)(nil)

func TestClassifierAnalyzer_Name(t *testing.T) {
	t.Parallel()

	a := NewClassifierAnalyzer(&conf.Settings{})
	assert.Equal(t, "classifier-analyzer", a.Name())
}

func TestClassifierAnalyzer_Compatible(t *testing.T) {
	t.Parallel()

	a := NewClassifierAnalyzer(&conf.Settings{})

	tests := []struct {
		name       string
		sourceType app.SourceType
		want       bool
	}{
		{"audio card is compatible", app.SourceTypeAudioCard, true},
		{"RTSP is compatible", app.SourceTypeRTSP, true},
		{"ultrasonic is not compatible", app.SourceTypeUltrasonic, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := app.AudioSource{Type: tt.sourceType}
			assert.Equal(t, tt.want, a.Compatible(src))
		})
	}
}

func TestClassifierAnalyzer_Classifier_NilBeforeStart(t *testing.T) {
	t.Parallel()

	a := NewClassifierAnalyzer(&conf.Settings{})
	assert.Nil(t, a.BirdNET(), "BirdNET() should return nil before Start()")
	assert.Nil(t, a.Classifier(), "Classifier() should return nil before Start()")
}

func TestClassifierAnalyzer_Start_UsesFakeLanguagePipeline(t *testing.T) {
	t.Parallel()

	a := NewClassifierAnalyzer(&conf.Settings{})
	err := a.Start(t.Context())
	assert.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, a.Stop(t.Context())) })

	bn := a.Classifier()
	assert.NotNil(t, bn)
	assert.Equal(t, "Language_Fake", bn.ModelInfo.ID)
	assert.Equal(t, bn, a.BirdNET(), "BirdNET() should remain a compatibility alias")
}

func TestClassifierAnalyzer_Stop_NilSafe(t *testing.T) {
	t.Parallel()

	a := NewClassifierAnalyzer(&conf.Settings{})
	// Stop before Start should not panic and should return nil.
	assert.NotPanics(t, func() {
		err := a.Stop(t.Context())
		assert.NoError(t, err)
	})
}
