# Plan: Replace BirdNET with Language Pipeline (Whisper + FastText)

## Goal

Make the existing "Language" pipeline (Whisper transcription + FastText language-id) the only classifier. BirdNET is removed entirely: no TFLite model loading, no taxonomy assumptions, no range filter, no BirdWeather, no species tracker.

## Server Compatibility

Both servers match the existing client code — no client changes needed:

- **Whisper** (`whisper.cpp/examples/server`): `POST {endpoint}/inference` multipart `file=@audio.wav` → `{text, language, segments}`
- **FastText** (`fasttext_server.py`): `POST {endpoint}/classify` JSON `{"text": "..."}` → `{label, confidence, top_n}`

## Changes

### 1. Make "Language" the only classifier initialization path

**File:** `internal/analysis/birdnet_service.go`

- Update `(*ClassifierAnalyzer).Start()` to always initialize the real language pipeline orchestrator via `classifier.NewRealLanguageOrchestrator(settings)`.
- Remove/disable these code paths:
  - `classifier.NewOrchestrator(settings)` — BirdNET primary model loader
  - `classifier.BuildRangeFilter(...)` — BirdNET geographic range filter
  - `classifier.NewFakeLanguageOrchestrator(settings)` — placeholder/fake pipeline

The app should always start the Language pipeline regardless of `language_pipeline.enabled`. That flag becomes a no-op (or is removed).

### 2. Stop the Processor from requiring BirdNET taxonomy

**File:** `internal/analysis/processor/processor.go`

Right now `parseAndValidateSpecies()` calls `p.Bn.EnrichResultWithTaxonomy(result.Species)` and **skips detections** when `scientificName == ""`. Language labels like `"en"`, `"fr"` have no taxonomy, so all detections get dropped.

Changes:
- In `parseAndValidateSpecies(...)`, when the active model is `Language` (or `settings.LanguagePipeline.Enabled`), treat `result.Species` as the label directly:
  - `commonName = result.Species`
  - `scientificName = result.Species` (or empty — but must not cause skipping)
  - `speciesCode = ""`
  - `speciesLowercase = strings.ToLower(result.Species)`
- Keep BirdNET-only features disabled under language mode (these are already gated behind `isLanguagePipelineMode()`):
  - Occurrence lookup (`GetSpeciesOccurrenceAtTime`)
  - Range filter update actions
  - BirdWeather integration
  - Species tracker "new species" logic
  - Dog/human detection filters

### 3. Replace hardcoded "BirdNET" defaults

Several places default to `"BirdNET"` when a model ID/name is missing. These must be changed to `"Language"`.

**File:** `internal/analysis/processor/dynamic_threshold.go`
- Change `const defaultModelID = "BirdNET"` → `"Language"`

**File:** `internal/detection/model_info.go`
- Change default model constants:
  - `DefaultModelName = "BirdNET"` → `"Language"`
  - `DefaultModelVersion = "2.4"` → `"1.0"`
  - `DefaultModelVariant` stays `"default"`
- This affects v2 DB seeding in `internal/datastore/v2/manager.go` and `internal/datastore/v2/mysql_manager.go` (both use `detection.DefaultModel*` constants).

### 4. Remove BirdNET timing assumptions

**File:** `internal/analysis/process.go`
- Replace `bufferDuration := 3 * time.Second` and `settings.BirdNET.Overlap` usage with the active model's `ModelSpec.ClipLength` (Language model spec is `3s @ 48kHz`, matching the current hardcoded value — but this removes the last BirdNET coupling in the hot path).

### 5. (Later) Transcript storage

When ready, extend the pipeline result to carry transcript text (already available from Whisper response) and persist it:
- Add a transcript field to the detection result payload in v2 tables (or a new related table).
- Plumb through API v2 + UI.

## Verification

1. Start Whisper + FastText servers.
2. Run Go tests: `task test`
3. Run the app and confirm:
   - No BirdNET model load logs in startup output.
   - Detections are produced with labels like `en`, `fr`, etc., and are **not skipped**.
   - No range-filter update attempts.
   - Dynamic thresholds work with model ID `"Language"`.

## Out of Scope (Cleanup)

After the above works end-to-end, these can be done as separate follow-ups:

- Stop embedding TFLite files (`internal/classifier/models_embedded.go`).
- Remove BirdNET model registry entries from `internal/classifier/model_registry.go`.
- Remove BirdNET config surface (`BirdNETConfig` struct, `birdnet.*` viper defaults).
- Remove `models.enabled` config (no longer needed — only one model).
- Remove embedded BirdNET label files (`internal/classifier/data/labels/V2.4/`).
