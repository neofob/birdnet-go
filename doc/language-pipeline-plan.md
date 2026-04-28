# Language Classification Pipeline — Implementation Plan

## Session Learnings

### FastText lid.176.ftz

- Classifies **text**, not audio. Input: UTF-8 string. Output: `__label__<iso_code>` + probability.
- 176 languages using mixed ISO 639-1/2/3 codes (e.g., `en`, `fr`, `yue`, `wuu`).
- Model file: `/modelfiles/lid.176.ftz` (~917 KB, quantized/compressed).
- No pure-Go library can load `.ftz` files. CGO wrappers exist but are low quality. No official Go bindings.
- For this project: FastText runs as an external HTTP service (Python/FastAPI wrapping `fasttext.load_model()`).

### Whisper

- OpenAI's ASR model, ported to C++ as `whisper.cpp` by Georgi Gerganov (49k+ stars, MIT).
- **Input**: 16kHz mono float32 PCM. BirdNET-Go's pipeline produces 48kHz 16-bit int PCM — resampling + format conversion needed.
- **Output**: Transcribed text, language detection (99 languages), segments with timestamps, word-level timestamps, per-token probabilities.
- **Language detection**: Built-in via `detect_language = true` or `whisper_lang_auto_detect()`. Returns probability distribution over all 99 languages.
- **Model sizes**: tiny (75 MiB) → large-v3-turbo (~1.6 GiB). Quantized variants available.
- **Go integration options**:
  - Official CGO bindings (`github.com/ggerganov/whisper.cpp/bindings/go`) — compile-time static link to `libwhisper.a`.
  - HTTP service via `whisper-server` (built-in server in whisper.cpp, OpenAI-compatible API) — zero CGO, Docker images available.
  - purego wrapper (theoretically possible but impractical — complex opaque C structs).
- **Chosen approach**: HTTP service. whisper.cpp runs as a separate process; Go app calls it via `httpclient.New()`.

### Existing BirdNET-Go Scaffolding

The codebase already has significant infrastructure for a language pipeline:

| Component | File | Status |
|-----------|------|--------|
| `AudioPipeline` interface | `internal/classifier/audio_pipeline.go:14` | `Classify(ctx, samples) → (Label, Confidence)` |
| `PipelineClassification` type | `internal/classifier/audio_pipeline.go:6` | Ready |
| Fake language orchestrator | `internal/classifier/language_fake.go` | Placeholder — always returns "English" @ 0.99 |
| `languagePipelineClassifier` adapter | `internal/classifier/language_fake.go:80` | Adapts `AudioPipeline` → `inference.Classifier` |
| `ClassificationTypeLanguage` | `internal/detection/classification.go:12` | Ready |
| `ModelRegistry["Language_Fake"]` | `internal/classifier/model_registry.go:99` | Placeholder entry |
| `ModelNameLanguage` constant | `internal/classifier/model_registry.go:22` | `"Language Pipeline"` |
| `confidenceToLogit()` helper | `internal/classifier/language_fake.go:105` | Converts confidence → logit for classifier adapter |

### Audio Format in BirdNET-Go Pipeline

| Property | BirdNET-Go | Whisper | Conversion |
|----------|-----------|---------|------------|
| Sample rate | 48000 Hz | 16000 Hz | 3:1 downsampling via `internal/audiocore/resample/` |
| Bit depth | 16-bit int (S16 LE) | 32-bit float | int16 → float32, divide by 32768.0 |
| Channels | 1 (mono) | 1 (mono) | No change |
| Buffer size | 288000 bytes (3s @ 48kHz S16) | N/A | Resampled to 96000 bytes (3s @ 16kHz S16) |

### External Service Patterns in BirdNET-Go

- **HTTP client**: `internal/httpclient/client.go` — `httpclient.New(cfg)` creates tuned client with connection pooling, HTTP/2, context-based timeouts (default 30s). Only consumer is `push_webhook.go`.
- **Error handling**: Always `internal/errors` package with `.Component()`, `.Category()`, `.Context()`. Never stdlib `errors`.
- **Retry**: eBird uses linear backoff (500ms increments, 3 max). Weather uses fixed delay (2s, 3 max). BirdWeather uses circuit breaker (5 failures, 10min reset).
- **Circuit breaker**: Shared `notification.PushCircuitBreaker` from `internal/notification/circuit_breaker.go`.
- **Config for external services**: Lives in `Settings.Realtime.*` (BirdWeather, eBird, MQTT, Weather).

### Key Architectural Insight

The existing `AudioPipeline` interface is the integration point. The fake pipeline implements it and gets adapted to `inference.Classifier` via `languagePipelineClassifier`. The real pipeline just needs to replace `fakeLanguagePipeline` with one that calls Whisper + FastText. Everything else (orchestration, buffering, ResultsQueue, processor, DB, SSE, events) plugs in automatically.

---

## Architecture

```
Audio (48kHz S16 mono, from BirdNET-Go analysis buffers)
  ↓ resample to 16kHz + convert to float32 + encode as WAV
  ↓
Whisper HTTP Server (whisper.cpp /inference endpoint)
  → TranscriptionResult { Text, Language, Confidence, Segments, Duration }
  ↓
FastText HTTP Server (Python/FastAPI wrapping lid.176.ftz)
  → LanguageResult { Label: "en", Confidence: 0.98, TopN: [...] }
  ↓
PipelineClassification { Label: "en", Confidence: 0.98 }
  ↓
languagePipelineClassifier adapter → inference.Classifier → Orchestrator
  ↓
ResultsQueue → Processor → DB / SSE / MQTT / Events
```

---

## Implementation Phases

### Phase 1: Fix Build Error

**File: `internal/api/v2/classifications.go`** (new)

Implement `func (c *Controller) initClassificationRoutes()` as a stub that registers no routes. This unblocks compilation — `api.go:591` already references this method.

### Phase 2: Define Service Interfaces

**File: `internal/classifier/language/service.go`** (new)

```go
type Transcriber interface {
    Transcribe(ctx context.Context, pcmData []byte, sampleRate int) (*TranscriptionResult, error)
    Close() error
}

type LanguageClassifier interface {
    Classify(ctx context.Context, text string) (*LanguageResult, error)
    Close() error
}
```

**File: `internal/classifier/language/types.go`** (new)

Shared types: `TranscriptionResult`, `LanguageResult`, `Segment`, `Config`.

### Phase 3: Configuration

**File: `internal/conf/config.go`** (modify)

Add `LanguagePipeline LanguagePipelineSettings` to `Settings`:

```go
type LanguagePipelineSettings struct {
    Enabled  bool
    Whisper  WhisperSettings
    FastText FastTextSettings
}

type WhisperSettings struct {
    Endpoint string        // e.g., "http://localhost:8080"
    Timeout  time.Duration // default: 30s
    Model    string        // model name hint (for logging)
    Language string        // "auto" or specific ISO code
}

type FastTextSettings struct {
    Endpoint string        // e.g., "http://localhost:8000"
    Timeout  time.Duration // default: 5s
    MaxTopN  int           // default: 5
}
```

Wire YAML tags. Config file path: `language_pipeline` (or `language`).

### Phase 4: Audio Conversion Helpers

**File: `internal/classifier/language/audio_convert.go`** (new)

- `ResamplePCM16(pcmData []byte, fromRate, toRate int) ([]byte, error)` — reuse `internal/audiocore/resample/`
- `PCM16ToFloat32(pcmData []byte) []float32` — int16 LE → float32 [-1.0, 1.0]
- `Float32ToWAV(samples []float32, sampleRate int) []byte` — encode as WAV for HTTP upload
- `PCM16ToWAV(pcmData []byte, sampleRate int) []byte` — combined resample + convert + encode

### Phase 5: Whisper HTTP Client

**File: `internal/classifier/language/whisper_client.go`** (new)

Implements `Transcriber` interface.

- Calls whisper.cpp server at `{endpoint}/inference` with multipart form (WAV file + params)
- Request params: `response_format=verbose_json`, `language=auto`, optional model selection
- Response parsing: extract text, language, confidence, segments from verbose_json
- Uses `httpclient.New()` with `DefaultTimeout: 30s`
- Error handling via `internal/errors` with `.Component("classifier.language.whisper")`
- Graceful degradation: if Whisper fails, return error (no fallback to fake)

### Phase 6: FastText HTTP Client

**File: `internal/classifier/language/fasttext_client.go`** (new)

Implements `LanguageClassifier` interface.

- Calls FastText HTTP server with transcription text as POST body
- Request: `{ "text": "transcribed text here" }`
- Response: `{ "label": "en", "confidence": 0.98, "top_n": [{"label": "en", "confidence": 0.98}, ...] }`
- Strips `__label__` prefix from FastText output
- Uses `httpclient.New()` with `DefaultTimeout: 5s` (text classification is fast)
- Error handling via `internal/errors` with `.Component("classifier.language.fasttext")`
- If FastText fails, fall back to Whisper's detected language (it supports 99 of the 176)

### Phase 7: Real Pipeline Implementation

**File: `internal/classifier/language/pipeline.go`** (new, replaces `language_fake.go`)

Implements `AudioPipeline` interface: `Classify(ctx, samples []float32) → (PipelineClassification, error)`.

Orchestrates:
1. Convert float32 samples → int16 PCM bytes (48kHz)
2. Call `Transcriber.Transcribe(pcmData, 48000)` → `TranscriptionResult`
3. Call `LanguageClassifier.Classify(result.Text)` → `LanguageResult`
4. Return `PipelineClassification{Label: langCode, Confidence: confidence}`

Error strategy:
- Whisper fails → return error (cannot classify without transcription)
- FastText fails → use Whisper's detected language as fallback

**File: `internal/classifier/language/orchestrator.go`** (new)

- `NewLanguageOrchestrator(settings) → (*classifier.Orchestrator, error)`
- Creates real pipeline with Whisper + FastText clients
- Sets up proper labels (176 language codes from FastText model)
- Registers as primary model in the Orchestrator

### Phase 8: Model Registry Update

**File: `internal/classifier/model_registry.go`** (modify)

Replace `Language_Fake` entry:

```go
"Language": {
    ID:               "Language",
    Name:             ModelNameLanguage,
    Backend:          "External",
    DetectionName:    "Language",
    DetectionVersion: "1.0",
    Description:      "Language classification pipeline (Whisper + FastText lid.176)",
    Spec:             ModelSpec{SampleRate: 48000, ClipLength: 3 * time.Second},
    ConfigAliases:    []string{"language"},
    NumSpecies:       176,
},
```

Keep `Language_Fake` for testing behind a build tag or separate config flag.

### Phase 9: Wire Into Orchestrator

**File: `internal/classifier/orchestrator.go`** (modify)

- When language model is enabled in settings, call `NewLanguageOrchestrator()` instead of `NewFakeLanguageOrchestrator()`
- `language_fake.go`: keep for unit tests, gate behind `UseFakeLanguagePipeline()` check

### Phase 10: API Routes

**File: `internal/api/v2/classifications.go`** (expand Phase 1 stub)

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| GET | `/api/v2/classifications` | ListRecentClassifications | No |
| GET | `/api/v2/classifications/:id` | GetClassification | No |
| GET | `/api/v2/classifications/stats` | GetLanguageStats | No |

Follow existing patterns from `detections.go`: pagination, query params, DTO conversion, error handling.

### Phase 11: External Services Setup (User Responsibility)

**Whisper server** (Docker):
```bash
docker run -d -p 8080:8080 \
  -v /path/to/models:/models \
  ghcr.io/ggml-org/whisper.cpp:main \
  "whisper-server --host 0.0.0.0 -m /models/ggml-base.bin"
```

**FastText server** (Python/FastAPI):
```python
# fasttext_server.py
from fastapi import FastAPI
from pydantic import BaseModel
import fasttext

app = FastAPI()
model = fasttext.load_model("lid.176.ftz")

class Request(BaseModel):
    text: str

class Prediction(BaseModel):
    label: str
    confidence: float

class Response(BaseModel):
    label: str
    confidence: float
    top_n: list[Prediction]

@app.post("/classify")
def classify(req: Request):
    labels, probs = model.predict(req.text, k=5)
    top_n = [
        Prediction(label=l.replace("__label__", ""), confidence=float(p))
        for l, p in zip(labels, probs)
    ]
    return Response(
        label=top_n[0].label,
        confidence=top_n[0].confidence,
        top_n=top_n,
    )
```

Config in BirdNET-Go settings:
```yaml
language_pipeline:
  enabled: true
  whisper:
    endpoint: "http://localhost:8080"
    timeout: 30s
    language: "auto"
  fasttext:
    endpoint: "http://localhost:8000"
    timeout: 5s
    max_top_n: 5
```

---

## File Summary

| Action | File | Phase |
|--------|------|-------|
| New | `internal/api/v2/classifications.go` | 1, 10 |
| New | `internal/classifier/language/service.go` | 2 |
| New | `internal/classifier/language/types.go` | 2 |
| New | `internal/classifier/language/audio_convert.go` | 4 |
| New | `internal/classifier/language/whisper_client.go` | 5 |
| New | `internal/classifier/language/fasttext_client.go` | 6 |
| New | `internal/classifier/language/pipeline.go` | 7 |
| New | `internal/classifier/language/orchestrator.go` | 7 |
| Modify | `internal/conf/config.go` | 3 |
| Modify | `internal/classifier/model_registry.go` | 8 |
| Modify | `internal/classifier/orchestrator.go` | 9 |
| Modify | `internal/classifier/language_fake.go` | 9 |

## Implementation Order

```
Phase 1 (fix build)
  ↓
Phase 2 (interfaces + types)
  ↓
Phase 3 (config)
  ↓
Phase 4 (audio conversion)
  ↓
Phase 5 (whisper client)
  ↓
Phase 6 (fasttext client)
  ↓
Phase 7 (pipeline + orchestrator)
  ↓
Phase 8 (model registry)
  ↓
Phase 9 (wire into orchestrator)
  ↓
Phase 10 (API routes)
  ↓
Phase 11 (external services)
```

## Open Questions

- **Transcription storage**: Store in existing `Note.Comments` field, or add a dedicated `transcription` column to the detections table?
- **Per-segment vs per-clip language**: Whisper returns segments with timestamps. Should each segment get its own FastText classification, or classify the full clip text at once?
- **FastText 176 vs Whisper 99**: Whisper detects 99 languages, FastText covers 176. When FastText returns a language not in Whisper's set (e.g., `yue`), how should this be displayed?
- **Latency budget**: Whisper inference on base model is ~100-500ms per 3s clip. FastText is ~1-5ms. Total pipeline latency: ~200-600ms per clip. Is this acceptable for real-time use?
