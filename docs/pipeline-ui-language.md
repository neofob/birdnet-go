# Pipeline Output to Web UI (Language Classifier)

This document describes how to connect the **Language (Whisper + FastText)** pipeline output to the **web UI dashboard** in a way that matches the existing BirdNET-Go architecture.

## Goal

Show, in real time on the Dashboard:

1. Predicted language code (e.g. `en`)
2. Confidence formatted as a percentage (e.g. `99.5%`)
3. One-line transcript preview
4. Model ID (to make it explicit this is the language pipeline)

## Key Constraint

The dashboard already consumes real-time updates via SSE at `GET /api/v2/detections/stream` and listens for `event: pending` messages.

The lowest-risk integration is to reuse this existing SSE connection and stream language pipeline output as a specialized pending payload.

## Architecture (End-to-End)

1. **Classifier**
   - Whisper generates transcript
   - FastText predicts language + confidence
   - Results are published into `classifier.ResultsQueue`

2. **Processor**
   - `processor.processDetections()` consumes `classifier.ResultsQueue`
   - Updates/creates entries in the pending detection map
   - Produces a *pending snapshot* and calls `PendingBroadcaster(snapshot)`

3. **API v2 SSE**
   - `PendingBroadcaster` is wired to `apiController.BroadcastPending(snapshot)`
   - SSE pushes the snapshot as `event: pending` over `/api/v2/detections/stream`

4. **Frontend Dashboard**
   - `DashboardPage.svelte` connects to `/api/v2/detections/stream`
   - Listens to `pending` events and updates `pendingDetections`
   - `CurrentlyHearingCard.svelte` renders pending items

## Backend Changes

### 1. Pending SSE DTO

Extend the pending SSE payload DTO used by the processor (the snapshot payload sent via `event: pending`).

Add fields:

1. `confidence` (float 0..1)
2. `transcriptPreview` (string, single-line, truncated)
3. `modelID` (already present in this repo's DTO; keep it)

Where:

1. `internal/analysis/processor/pending_broadcast.go`

### 2. Language Mode Shaping Rules

The frontend has a language-mode heuristic:

`isLanguageDetection(commonName, scientificName)` is true when:

1. `commonName` is 2-3 lowercase letters (`en`, `fr`, ...)
2. `scientificName` is empty

To make language output render correctly:

1. Set `species` to the predicted language code (e.g. `en`)
2. Force `scientificName` to `""` in language mode
3. Force `thumbnail` to `""` in language mode to avoid bird image lookups
4. Populate `confidence` from the pending detection's stored confidence
5. Populate `transcriptPreview` from `Detection.Result.Transcript` after normalization

Transcript preview normalization:

1. Trim whitespace
2. Replace `\r\n`/`\n` with spaces
3. Collapse repeated whitespace
4. Truncate to a safe max length (recommend 180-240 chars)

### 3. Logging/Verification Signals

Backend logs to confirm the pipeline is flowing:

1. `operation=process_detections_entry`
2. `operation=create_pending_detection`
3. `operation=pending_broadcast` ("Broadcasting pending detections")
4. SSE connection: `SSE client connected` for `/api/v2/detections/stream`

## Frontend Changes

### 1. Type Updates

Update the pending detection TS interface:

1. `confidence?: number`
2. `transcriptPreview?: string`
3. `modelID?: string`

Where:

1. `frontend/src/lib/types/pending.types.ts`

### 2. Rendering in CurrentlyHearingCard

For language items:

1. Primary line: `Language` badge + language code (e.g. `en`)
2. Secondary line: `99.5% · model: <modelID>` (+ source if multiple sources)
3. Optional third line: one-line transcript preview (CSS `truncate`)

Confidence formatting:

1. UI should display `((confidence ?? 0) * 100).toFixed(1) + '%'`

Where:

1. `frontend/src/lib/desktop/features/dashboard/components/CurrentlyHearingCard.svelte`

### 3. Browser Verification

Browser console expectations:

1. Dashboard logs `Connecting to SSE stream at /api/v2/detections/stream`
2. No JSON parse errors on `pending` events
3. Pending items update when new language results arrive

## Failure Modes (What To Check)

1. No pending detections created:
   - Look for `operation=create_pending_detection` in `analysis.processor` logs

2. Pending detections created but not broadcast:
   - Look for `operation=pending_broadcast`
   - Confirm `Pending broadcaster connected to processor` during startup

3. Broadcast happens but UI not updating:
   - Confirm `SSE client connected` for `/api/v2/detections/stream`
   - Check browser console for SSE reconnect loop / parse errors

4. UI receives pending payload but renders incorrectly:
   - Ensure language items have `scientificName=""` so language-mode UI triggers
   - Ensure `confidence` is present and is a number

## Open Choice

Transcript preview gating (optional):

1. Always show transcript preview when present
2. Only show when `confidence >= 0.8`

This is a UX preference; the data model supports either.
