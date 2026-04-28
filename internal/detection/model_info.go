package detection

// Default model constants.
const (
	DefaultModelName    = "Language"
	DefaultModelVersion = "1.0"
	DefaultModelVariant = "default"
)

// ModelInfo describes the AI model used for detection.
type ModelInfo struct {
	Name           string  // e.g., "Language"
	Version        string  // e.g., "1.0"
	Variant        string  // e.g., "default"
	ClassifierPath *string // path to custom classifier file, nil for default
}

// DefaultModelInfo returns the default Language pipeline model info.
func DefaultModelInfo() ModelInfo {
	return ModelInfo{
		Name:           DefaultModelName,
		Version:        DefaultModelVersion,
		Variant:        DefaultModelVariant,
		ClassifierPath: nil,
	}
}
