package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassificationTypeForModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model ModelInfo
		want  ClassificationType
	}{
		{"language model", ModelInfo{Name: "Language"}, ClassificationTypeLanguage},
		{"language model case insensitive", ModelInfo{Name: "language"}, ClassificationTypeLanguage},
		{"default language model", DefaultModelInfo(), ClassificationTypeLanguage},
		{"custom named model defaults to species compatibility", ModelInfo{Name: "Custom"}, ClassificationTypeSpecies},
		{"empty model", ModelInfo{}, ClassificationTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ClassificationTypeForModel(tt.model))
		})
	}
}

func TestLabelFromSpecies(t *testing.T) {
	t.Parallel()

	label := LabelFromSpecies(Species{
		ScientificName: "English",
		CommonName:     "English",
		Code:           "",
	}, ClassificationTypeLanguage)

	assert.Equal(t, "English", label.Name)
	assert.Equal(t, "English", label.DisplayName)
	assert.Equal(t, ClassificationTypeLanguage, label.Type)
}

func TestResultClassification(t *testing.T) {
	t.Parallel()

	result := &Result{
		Species:    Species{ScientificName: "English", CommonName: "English"},
		Confidence: 0.99,
		Model:      ModelInfo{Name: "Language", Version: "fake"},
	}

	classification := result.Classification()
	assert.Equal(t, ClassificationTypeLanguage, classification.Label.Type)
	assert.Equal(t, "English", classification.Label.Name)
	assert.InDelta(t, 0.99, classification.Confidence, 0.001)
}

func TestAdditionalResultClassification(t *testing.T) {
	t.Parallel()

	result := AdditionalResult{
		Species:    Species{ScientificName: "French", CommonName: "French"},
		Confidence: 0.42,
	}

	classification := result.Classification(ClassificationTypeLanguage)
	assert.Equal(t, ClassificationTypeLanguage, classification.Label.Type)
	assert.Equal(t, "French", classification.Label.DisplayName)
	assert.InDelta(t, 0.42, classification.Confidence, 0.001)
}
