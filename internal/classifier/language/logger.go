package language

import (
	"github.com/tphakala/birdnet-go/internal/logger"
)

// GetLogger returns the language pipeline logger scoped to the classifier.language module.
func GetLogger() logger.Logger {
	return logger.Global().Module("classifier.language")
}
