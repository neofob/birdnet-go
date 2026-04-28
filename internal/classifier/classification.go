package classifier

import "github.com/tphakala/birdnet-go/internal/datastore"

// Classification is the classifier-facing name for a single prediction result.
// It aliases datastore.Results while persistence and UI code still use the
// legacy BirdNET/species-shaped schema.
type Classification = datastore.Results
