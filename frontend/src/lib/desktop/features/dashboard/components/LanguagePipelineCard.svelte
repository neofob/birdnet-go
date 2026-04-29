<script lang="ts">
  import { t } from '$lib/i18n';
  import type { Detection } from '$lib/types/detection.types';
  import type { PendingDetection } from '$lib/types/pending.types';
  import { isLanguageDetection } from '$lib/utils/speciesUtils';

  interface Props {
    detections: Detection[];
    pendingDetections?: PendingDetection[];
    loading?: boolean;
    className?: string;
  }

  let { detections = [], pendingDetections = [], loading = false, className = '' }: Props = $props();

  type LanguageMetric = {
    language: string;
    recordings: number;
    avgConfidence: number;
    activeDurationSeconds: number;
  };

  type SourceMetric = {
    source: string;
    recordings: number;
    avgConfidence: number;
    totalDurationSeconds: number;
    topLanguage: string;
  };

  let languageRows = $derived.by(() => {
    const byLanguage = new Map<string, LanguageMetric>();

    for (const d of detections) {
      if (!isLanguageDetection(d.commonName, d.scientificName)) {
        continue;
      }

      const language = (d.commonName ?? '').trim().toLowerCase();
      if (language.length === 0) {
        continue;
      }

      const recordings = 1;
      const durationSeconds = parseClipDurationSeconds(d.beginTime, d.endTime);
      const confidence =
        typeof d.confidence === 'number' && Number.isFinite(d.confidence)
          ? Math.max(0, Math.min(1, d.confidence))
          : 0;

      const existing = byLanguage.get(language);
      if (existing) {
        existing.recordings += recordings;
        existing.avgConfidence += confidence * recordings;
        existing.activeDurationSeconds += durationSeconds;
      } else {
        byLanguage.set(language, {
          language,
          recordings,
          avgConfidence: confidence * recordings,
          activeDurationSeconds: durationSeconds,
        });
      }
    }

    const rows = Array.from(byLanguage.values()).map(row => ({
      ...row,
      avgConfidence: row.recordings > 0 ? row.avgConfidence / row.recordings : 0,
    }));

    rows.sort((a, b) => b.recordings - a.recordings || a.language.localeCompare(b.language));
    return rows;
  });

  let activeLanguageCount = $derived(
    pendingDetections.filter(d => isLanguageDetection(d.species, d.scientificName)).length
  );

  let hourlyLanguageBuckets = $derived.by(() => {
    const buckets = Array.from({ length: 24 }, (_, hour) => ({ hour, recordings: 0 }));

    for (const d of detections) {
      if (!isLanguageDetection(d.commonName, d.scientificName)) {
        continue;
      }
      const ts = d.timestamp ? Date.parse(d.timestamp) : Number.NaN;
      if (!Number.isFinite(ts)) {
        continue;
      }
      const hour = new Date(ts).getHours();
      const bucket = buckets[hour];
      if (bucket) {
        bucket.recordings += 1;
      }
    }

    return buckets;
  });

  let maxHourlyRecordings = $derived(
    Math.max(1, ...hourlyLanguageBuckets.map(bucket => bucket.recordings))
  );

  type LanguageAnomaly = {
    severity: 'info' | 'warning' | 'critical';
    title: string;
    detail: string;
  };

  let anomalyAlerts = $derived.by(() => {
    const alerts: LanguageAnomaly[] = [];
    const now = new Date();
    const currentHour = now.getHours();

    const currentBucket = hourlyLanguageBuckets.find(bucket => bucket.hour === currentHour);
    const currentCount = currentBucket?.recordings ?? 0;
    const baselineBuckets = hourlyLanguageBuckets.filter(bucket => bucket.hour !== currentHour);
    const baselineValues = baselineBuckets.map(bucket => bucket.recordings);
    const baselineMean =
      baselineValues.length > 0
        ? baselineValues.reduce((sum, value) => sum + value, 0) / baselineValues.length
        : 0;
    const baselineVariance =
      baselineValues.length > 0
        ? baselineValues.reduce((sum, value) => sum + (value - baselineMean) ** 2, 0) /
          baselineValues.length
        : 0;
    const baselineStdDev = Math.sqrt(baselineVariance);
    const burstThreshold = baselineMean + Math.max(2, 2 * baselineStdDev);

    if (currentCount >= burstThreshold && currentCount >= 4) {
      alerts.push({
        severity: currentCount >= burstThreshold * 1.5 ? 'critical' : 'warning',
        title: 'Language activity burst',
        detail: `Current hour has ${currentCount} recordings (baseline ${baselineMean.toFixed(1)}).`,
      });
    }

    const currentHourDetections = detections.filter(d => {
      if (!isLanguageDetection(d.commonName, d.scientificName)) {
        return false;
      }
      const ts = d.timestamp ? Date.parse(d.timestamp) : Number.NaN;
      if (!Number.isFinite(ts)) {
        return false;
      }
      return new Date(ts).getHours() === currentHour;
    });

    const previousHourDetections = detections.filter(d => {
      if (!isLanguageDetection(d.commonName, d.scientificName)) {
        return false;
      }
      const ts = d.timestamp ? Date.parse(d.timestamp) : Number.NaN;
      if (!Number.isFinite(ts)) {
        return false;
      }
      return new Date(ts).getHours() !== currentHour;
    });

    const previousLanguages = new Set(
      previousHourDetections.map(d => (d.commonName ?? '').trim().toLowerCase()).filter(Boolean)
    );
    const newLanguages = new Set(
      currentHourDetections
        .map(d => (d.commonName ?? '').trim().toLowerCase())
        .filter(label => label.length > 0 && !previousLanguages.has(label))
    );

    if (newLanguages.size > 0) {
      alerts.push({
        severity: 'info',
        title: 'New language labels this hour',
        detail: Array.from(newLanguages).slice(0, 5).join(', '),
      });
    }

    const currentHourAvgConfidence =
      currentHourDetections.length > 0
        ? currentHourDetections.reduce((sum, d) => sum + (Number.isFinite(d.confidence) ? d.confidence : 0), 0) /
          currentHourDetections.length
        : 0;

    if (currentHourDetections.length >= 4 && currentHourAvgConfidence < 0.45) {
      alerts.push({
        severity: 'warning',
        title: 'Confidence drop',
        detail: `Current-hour average confidence is ${formatPercent(currentHourAvgConfidence)}.`,
      });
    }

    return alerts;
  });

  let sourceRows = $derived.by(() => {
    const bySource = new Map<
      string,
      {
        source: string;
        recordings: number;
        confidenceWeightedSum: number;
        totalDurationSeconds: number;
        languageCounts: Map<string, number>;
      }
    >();

    for (const d of detections) {
      if (!isLanguageDetection(d.commonName, d.scientificName)) {
        continue;
      }

      const source =
        (d.source?.displayName && d.source.displayName.trim()) ||
        (d.source?.id && d.source.id.trim()) ||
        'unknown';
      const language = (d.commonName ?? '').trim().toLowerCase() || 'und';
      const duration = parseClipDurationSeconds(d.beginTime, d.endTime);
      const confidence =
        typeof d.confidence === 'number' && Number.isFinite(d.confidence)
          ? Math.max(0, Math.min(1, d.confidence))
          : 0;

      const existing = bySource.get(source);
      if (existing) {
        existing.recordings += 1;
        existing.confidenceWeightedSum += confidence;
        existing.totalDurationSeconds += duration;
        existing.languageCounts.set(language, (existing.languageCounts.get(language) ?? 0) + 1);
      } else {
        bySource.set(source, {
          source,
          recordings: 1,
          confidenceWeightedSum: confidence,
          totalDurationSeconds: duration,
          languageCounts: new Map([[language, 1]]),
        });
      }
    }

    const rows: SourceMetric[] = [];
    for (const item of bySource.values()) {
      let topLanguage = 'und';
      let topCount = 0;
      for (const [language, count] of item.languageCounts.entries()) {
        if (count > topCount) {
          topCount = count;
          topLanguage = language;
        }
      }

      rows.push({
        source: item.source,
        recordings: item.recordings,
        avgConfidence: item.recordings > 0 ? item.confidenceWeightedSum / item.recordings : 0,
        totalDurationSeconds: item.totalDurationSeconds,
        topLanguage,
      });
    }

    rows.sort((a, b) => b.recordings - a.recordings || a.source.localeCompare(b.source));
    return rows;
  });

  let totalRecordings = $derived(languageRows.reduce((sum, row) => sum + row.recordings, 0));
  let totalActiveDuration = $derived(
    languageRows.reduce((sum, row) => sum + row.activeDurationSeconds, 0)
  );
  let weightedAverageConfidence = $derived.by(() => {
    if (totalRecordings === 0) {
      return 0;
    }
    const weightedSum = languageRows.reduce(
      (sum, row) => sum + row.avgConfidence * row.recordings,
      0
    );
    return weightedSum / totalRecordings;
  });

  function formatPercent(confidence: number): string {
    const value = Math.max(0, Math.min(1, confidence));
    return `${(value * 100).toFixed(1)}%`;
  }

  function formatDuration(seconds: number): string {
    const safeSeconds = Math.max(0, Math.floor(seconds));
    const hours = Math.floor(safeSeconds / 3600);
    const minutes = Math.floor((safeSeconds % 3600) / 60);
    const secs = safeSeconds % 60;

    if (hours > 0) {
      return `${hours}h ${minutes}m`;
    }
    return `${minutes}m ${secs}s`;
  }

  function parseClipDurationSeconds(beginTime: string, endTime: string): number {
    const begin = parseClockSeconds(beginTime);
    const end = parseClockSeconds(endTime);
    if (begin === null || end === null) {
      return 0;
    }
    return Math.max(0, end - begin);
  }

  function parseClockSeconds(value: string): number | null {
    const match = /^(\d{2}):(\d{2}):(\d{2})$/.exec(value ?? '');
    if (!match) {
      return null;
    }
    const hours = Number.parseInt(match[1], 10);
    const minutes = Number.parseInt(match[2], 10);
    const seconds = Number.parseInt(match[3], 10);
    return hours * 3600 + minutes * 60 + seconds;
  }

  function getAlertClass(severity: LanguageAnomaly['severity']): string {
    if (severity === 'critical') {
      return 'border-[var(--color-error)]/30 bg-[var(--color-error)]/10';
    }
    if (severity === 'warning') {
      return 'border-[var(--color-warning)]/30 bg-[var(--color-warning)]/10';
    }
    return 'border-[var(--color-info)]/30 bg-[var(--color-info)]/10';
  }
</script>

<section
  class="card col-span-12 flex h-full flex-col rounded-2xl border border-border-100 bg-[var(--color-base-100)] shadow-sm {className}"
>
  <div class="flex items-center gap-2 border-b border-[var(--color-base-200)] px-6 py-4">
    <div class="flex flex-col">
      <h3 class="font-semibold">Language Pipeline</h3>
      <p class="text-sm text-[var(--color-base-content)]/60">
        Last 24h language metrics
      </p>
    </div>
  </div>

  {#if loading}
    <div class="flex flex-1 items-center justify-center px-6 py-8">
      <p class="text-sm text-[var(--color-base-content)]/40">Loading language analytics...</p>
    </div>
  {:else if languageRows.length > 0}
    <div class="grid grid-cols-1 gap-2 border-b border-[var(--color-base-200)] p-4 sm:grid-cols-3">
      <div class="rounded-lg bg-[var(--color-base-200)] px-3 py-2">
        <div class="text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">Languages</div>
        <div class="text-sm font-semibold text-[var(--color-base-content)]">{languageRows.length}</div>
      </div>
      <div class="rounded-lg bg-[var(--color-base-200)] px-3 py-2">
        <div class="text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">Recordings</div>
        <div class="text-sm font-semibold text-[var(--color-base-content)]">{totalRecordings}</div>
      </div>
      <div class="rounded-lg bg-[var(--color-base-200)] px-3 py-2">
        <div class="text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">Avg Confidence</div>
        <div class="text-sm font-semibold text-[var(--color-base-content)]">
          {formatPercent(weightedAverageConfidence)}
        </div>
      </div>
      <div class="rounded-lg bg-[var(--color-base-200)] px-3 py-2 sm:col-span-3">
        <div class="text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">Active Now</div>
        <div class="text-sm font-semibold text-[var(--color-base-content)]">{activeLanguageCount}</div>
      </div>
    </div>

    <div class="p-4">
      <div class="mb-3 text-xs text-[var(--color-base-content)]/60">
        Total active duration: {formatDuration(totalActiveDuration)}
      </div>

      <div class="mb-4 rounded-lg border border-[var(--color-base-200)] p-3">
        <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-base-content)]/60">
          Language Distribution by Hour (24h)
        </div>
        <div class="grid grid-cols-12 gap-1 sm:grid-cols-24">
          {#each hourlyLanguageBuckets as bucket (bucket.hour)}
            <div class="flex flex-col items-center">
              <div
                class="w-full rounded-sm bg-[var(--color-primary)]/20"
                style={`height: ${Math.max(6, (bucket.recordings / maxHourlyRecordings) * 40)}px`}
                title={`${bucket.hour.toString().padStart(2, '0')}:00 · ${bucket.recordings}`}
              ></div>
              <span class="mt-1 text-[10px] text-[var(--color-base-content)]/50">
                {bucket.hour.toString().padStart(2, '0')}
              </span>
            </div>
          {/each}
        </div>
      </div>

      <div class="mb-4 rounded-lg border border-[var(--color-base-200)] p-3">
        <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-base-content)]/60">
          Burst and Anomaly Alerts
        </div>
        {#if anomalyAlerts.length === 0}
          <p class="text-sm text-[var(--color-base-content)]/60">
            No anomalies detected in the current 24h window.
          </p>
        {:else}
          <div class="space-y-2">
            {#each anomalyAlerts as alert (alert.title + alert.detail)}
              <div
                class={`rounded-lg border px-3 py-2 text-sm ${getAlertClass(alert.severity)}`}
              >
                <div class="font-medium">{alert.title}</div>
                <div class="text-[var(--color-base-content)]/70">{alert.detail}</div>
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-[var(--color-base-200)] text-left text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">
              <th class="pb-2 pr-3 font-medium">Language</th>
              <th class="pb-2 pr-3 font-medium">Recordings</th>
              <th class="pb-2 pr-3 font-medium">Avg Confidence</th>
              <th class="pb-2 font-medium">Active Duration</th>
            </tr>
          </thead>
          <tbody>
            {#each languageRows as row (row.language)}
              <tr class="border-b border-[var(--color-base-200)]/60 last:border-b-0">
                <td class="py-2 pr-3 font-medium uppercase tracking-wide">{row.language}</td>
                <td class="py-2 pr-3">{row.recordings}</td>
                <td class="py-2 pr-3">{formatPercent(row.avgConfidence)}</td>
                <td class="py-2">{formatDuration(row.activeDurationSeconds)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <div class="mt-4 overflow-x-auto">
        <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-base-content)]/60">
          Source Breakdown
        </div>
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-[var(--color-base-200)] text-left text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">
              <th class="pb-2 pr-3 font-medium">Source</th>
              <th class="pb-2 pr-3 font-medium">Recordings</th>
              <th class="pb-2 pr-3 font-medium">Top Language</th>
              <th class="pb-2 pr-3 font-medium">Avg Confidence</th>
              <th class="pb-2 font-medium">Duration</th>
            </tr>
          </thead>
          <tbody>
            {#each sourceRows as row (row.source)}
              <tr class="border-b border-[var(--color-base-200)]/60 last:border-b-0">
                <td class="py-2 pr-3 font-medium">{row.source}</td>
                <td class="py-2 pr-3">{row.recordings}</td>
                <td class="py-2 pr-3 uppercase tracking-wide">{row.topLanguage}</td>
                <td class="py-2 pr-3">{formatPercent(row.avgConfidence)}</td>
                <td class="py-2">{formatDuration(row.totalDurationSeconds)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {:else}
    <div class="flex flex-1 items-center justify-center px-6 py-8">
      <p class="text-sm text-[var(--color-base-content)]/40">{t('dashboard.currentlyHearing.empty')}</p>
    </div>
  {/if}
</section>
