# Language Intelligence Dashboard Ideas

This document captures practical dashboard/chart ideas for language-pipeline outputs:

- language label
- confidence
- transcript
- timestamps and recording duration
- source / sensor
- hit or recording count

## High-Value Additions

1. **Language Distribution (time-bucketed stacked area)**
   - Show language mix over time (hour/day).
   - Fast way to spot shifts in the operating environment.

2. **Confidence Quality Panel**
   - Median and percentile confidence by language and by source.
   - Trend low-confidence rate to separate noise from signal.

3. **Language Activity Heatmap**
   - Hour-of-day vs day-of-week matrix by language count/duration.
   - Reveals recurring communication windows.

4. **Source Intelligence Map/Table**
   - Per source: top languages, active duration, confidence, transcript volume.
   - Useful for sensor placement and reliability comparisons.

5. **Burst / Anomaly Detection**
   - Baseline vs current activity.
   - Alert on spikes, new language appearances, sudden confidence drop.

6. **Top Phrases / Keyword Trends (transcripts)**
   - Frequent terms and emerging terms by language and time window.
   - Add co-occurrence view for context.

7. **Session Timeline**
   - Group contiguous detections into sessions.
   - Show start/end, duration, and switch points.

8. **Language Switch Graph**
   - Transition graph (Sankey/chord) between language labels.
   - Highlights multilingual sequence patterns.

9. **Coverage and Gaps**
   - Uptime vs activity vs silence windows.
   - Finds passive collection blind spots.

10. **Short-Term Forecast**
    - Forecast volume by source/time bucket.
    - Useful for staffing and analyst workflow planning.

## If Translation Is Added Later

- Pair ASR confidence with translation confidence.
- Add entity timeline (people/places/orgs) with source and confidence.
- Add cross-language concept clustering for topic-level tracking.

## Recommended Rollout Sequence

1. Distribution + Heatmap + Source breakdown
2. Burst/anomaly alerts
3. Session timeline + language-switch graph
4. Transcript keyword/entity analytics
5. Forecasting

## Current Implementation Status

- A `Language Analytics` dashboard card already exists.
- Initial enhancement started for rollout item 1:
  - language distribution summary
  - source-level breakdown
  - hourly activity view for the last 24h
