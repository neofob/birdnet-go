package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/detection"
)

func TestModelTypeFromDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		info     detection.ModelInfo
		expected entities.ModelType
	}{
		{
			name: "bird model",
			info: detection.ModelInfo{
				Name:    "BirdNET",
				Version: "2.4",
				Variant: "default",
			},
			expected: entities.ModelTypeBird,
		},
		{
			name: "language model",
			info: detection.ModelInfo{
				Name:    "Language",
				Version: "fake",
				Variant: "default",
			},
			expected: entities.ModelTypeLanguage,
		},
		{
			name: "language model case insensitive",
			info: detection.ModelInfo{
				Name:    "language",
				Version: "fake",
				Variant: "default",
			},
			expected: entities.ModelTypeLanguage,
		},
		{
			name: "empty model name defaults to bird",
			info:  detection.ModelInfo{},
			expected: entities.ModelTypeBird,
		},
		{
			name: "unknown model defaults to bird",
			info: detection.ModelInfo{
				Name:    "Perch",
				Version: "V2",
				Variant: "default",
			},
			expected: entities.ModelTypeBird,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := modelTypeFromDetection(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLabelTypeIDForModel(t *testing.T) {
	t.Parallel()

	deps := &ConversionDeps{
		SpeciesLabelTypeID:  1,
		LanguageLabelTypeID: 2,
	}

	assert.Equal(t, uint(1), labelTypeIDForModel(entities.ModelTypeBird, deps))
	assert.Equal(t, uint(2), labelTypeIDForModel(entities.ModelTypeLanguage, deps))
	assert.Equal(t, uint(1), labelTypeIDForModel(entities.ModelTypeMulti, deps))
	assert.Equal(t, uint(1), labelTypeIDForModel(entities.ModelTypeBat, deps))
}
