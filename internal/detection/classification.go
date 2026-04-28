package detection

import "strings"

// ClassificationType identifies what kind of label a classifier produced.
type ClassificationType string

const (
	// ClassificationTypeSpecies is a bird/species label from BirdNET-compatible models.
	ClassificationTypeSpecies ClassificationType = "species"
	// ClassificationTypeLanguage is a spoken-language label from the language pipeline.
	ClassificationTypeLanguage ClassificationType = "language"
	// ClassificationTypeUnknown is used when a model cannot be mapped yet.
	ClassificationTypeUnknown ClassificationType = "unknown"
)

// Label is the neutral domain representation of a classifier output label.
// Existing persistence still stores species-shaped fields; this type is the
// bridge for new classifier code that should not depend on bird taxonomy terms.
type Label struct {
	Name        string
	DisplayName string
	Code        string
	Type        ClassificationType
}

// Classification represents a single classifier prediction.
type Classification struct {
	Label      Label
	Confidence float64
}

// ClassificationTypeForModel maps model metadata to a neutral classification type.
func ClassificationTypeForModel(model ModelInfo) ClassificationType {
	if strings.EqualFold(model.Name, "Language") || strings.EqualFold(model.Name, DefaultModelName) {
		return ClassificationTypeLanguage
	}
	if model.Name != "" {
		return ClassificationTypeSpecies
	}
	return ClassificationTypeUnknown
}

// LabelFromSpecies adapts the legacy species-shaped label into the neutral form.
func LabelFromSpecies(species Species, labelType ClassificationType) Label {
	name := species.ScientificName
	if name == "" {
		name = species.CommonName
	}

	displayName := species.CommonName
	if displayName == "" {
		displayName = name
	}

	return Label{
		Name:        name,
		DisplayName: displayName,
		Code:        species.Code,
		Type:        labelType,
	}
}

// Classification returns the neutral classifier result for this detection.
func (r *Result) Classification() Classification {
	if r == nil {
		return Classification{}
	}
	labelType := ClassificationTypeForModel(r.Model)
	return Classification{
		Label:      LabelFromSpecies(r.Species, labelType),
		Confidence: r.Confidence,
	}
}

// Classification returns the neutral classifier result for this additional result.
func (r AdditionalResult) Classification(labelType ClassificationType) Classification {
	return Classification{
		Label:      LabelFromSpecies(r.Species, labelType),
		Confidence: r.Confidence,
	}
}
