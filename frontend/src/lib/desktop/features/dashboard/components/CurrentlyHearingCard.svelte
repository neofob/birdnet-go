<!--
CurrentlyHearingCard.svelte - Real-time pending detection display

Purpose:
- Shows species currently being detected by BirdNET in real-time
- Provides visual feedback when detections are approved or rejected
- Retains terminal (approved/rejected) states for a few seconds before fading out
- Hidden entirely when no pending detections exist

Props:
- detections: PendingDetection[] - Current pending detection snapshot from SSE
- className?: string - Additional CSS classes (default: '')
-->
<script lang="ts">
  import { Check, X } from '@lucide/svelte';
  import { fade } from 'svelte/transition';
  import { untrack } from 'svelte';
  import { t } from '$lib/i18n';
  import type { PendingDetection } from '$lib/types/pending.types';
  import { isLanguageDetection } from '$lib/utils/speciesUtils';

  interface Props {
    detections: PendingDetection[];
    className?: string;
  }

  let { detections = [], className = '' }: Props = $props();

  // How long terminal (approved/rejected) detections remain visible (ms)
  const TERMINAL_RETENTION_MS = 3000;

  // Retained terminal detections kept visible after backend stops sending them
  let retainedKeys = $state<string[]>([]);
  let retainedData: Record<string, PendingDetection> = {};
  let removalTimers: Record<string, ReturnType<typeof setTimeout>> = {};

  function detectionKey(d: PendingDetection): string {
    // Use sourceID when available (stable identifier), and fall back to species
    // for language detections where scientificName may be empty.
    return `${d.sourceID}:${d.scientificName || d.species}`;
  }

  // Track terminal detections and schedule their removal.
  // Use untrack() when reading retainedKeys to avoid a read-write loop
  // (this effect should only re-run when detections changes, not retainedKeys).
  $effect(() => {
    for (const d of detections) {
      const key = detectionKey(d);
      if ((d.status === 'approved' || d.status === 'rejected') && !(key in removalTimers)) {
        /* eslint-disable security/detect-object-injection -- key is derived from detectionKey(), a controlled string */
        retainedData[key] = d;
        removalTimers[key] = setTimeout(() => {
          delete retainedData[key];
          delete removalTimers[key];
          /* eslint-enable security/detect-object-injection */
          retainedKeys = retainedKeys.filter(k => k !== key);
        }, TERMINAL_RETENTION_MS);
        if (!untrack(() => retainedKeys).includes(key)) {
          retainedKeys = [...untrack(() => retainedKeys), key];
        }
      }
    }
  });

  // Merge incoming detections with retained terminal ones
  let displayDetections = $derived.by(() => {
    // Read retainedKeys to establish reactive dependency
    const retained = retainedKeys;

    const incomingByKey = new Set<string>();
    for (const d of detections) {
      incomingByKey.add(detectionKey(d));
    }

    const result: PendingDetection[] = [...detections];
    for (const key of retained) {
      if (!incomingByKey.has(key)) {
        // eslint-disable-next-line security/detect-object-injection -- key is from retainedKeys, a controlled string array
        const data = retainedData[key];
        if (data) {
          result.push(data);
        }
      }
    }

    // Sort newest first so new detections appear on the left
    result.sort((a, b) => b.firstDetected - a.firstDetected);
    return result;
  });

  let hasDisplayDetections = $derived(displayDetections.length > 0);

  // Compute relative time string from Unix timestamp
  function getElapsedText(firstDetected: number): string {
    const elapsed = Math.max(0, Math.floor(Date.now() / 1000 - firstDetected));
    if (elapsed < 60) return `${elapsed}s`;
    const minutes = Math.floor(elapsed / 60);
    if (minutes < 60) return `${minutes}m`;
    const hours = Math.floor(minutes / 60);
    return `${hours}h`;
  }

  // Refresh elapsed times every second
  let tick = $state(0);
  $effect(() => {
    if (!hasDisplayDetections) return;
    const interval = setInterval(() => {
      tick++;
    }, 1000);
    return () => clearInterval(interval);
  });

  // Force re-evaluation of elapsed text when tick changes
  let elapsedTexts = $derived.by(() => {
    void tick;
    const result: Record<string, string> = {};
    for (const d of displayDetections) {
      result[detectionKey(d)] = getElapsedText(d.firstDetected);
    }
    return result;
  });

  function getElapsedForKey(key: string): string {
    // eslint-disable-next-line security/detect-object-injection -- key is a controlled detection key string
    return elapsedTexts[key] ?? '';
  }

  // Show source column only when multiple sources are present
  let hasMultipleSources = $derived(new Set(displayDetections.map(d => d.source)).size > 1);

  // Show transcript line only when at least one item has it
  let hasAnyTranscript = $derived(displayDetections.some(d => (d.transcript ?? '').length > 0));

  let languageDetections = $derived(
    displayDetections.filter(d => isLanguageDetection(d.species, d.scientificName))
  );

  let hasLanguageDetections = $derived(languageDetections.length > 0);

  type LanguageAggregate = {
    label: string;
    recordings: number;
    avgConfidence: number;
    totalDurationSeconds: number;
  };

  let languageAggregates = $derived.by(() => {
    void tick;
    const byLabel = new Map<string, LanguageAggregate>();

    for (const d of languageDetections) {
      const label = (d.species ?? '').trim().toLowerCase();
      if (label.length === 0) {
        continue;
      }

      const recordings = Math.max(1, d.hitCount ?? 1);
      const durationSeconds = Math.max(0, Math.floor(Date.now() / 1000 - d.firstDetected));
      const weightedConfidence =
        typeof d.confidence === 'number' && Number.isFinite(d.confidence)
          ? Math.max(0, Math.min(1, d.confidence)) * recordings
          : 0;

      const current = byLabel.get(label);
      if (current) {
        current.recordings += recordings;
        current.avgConfidence += weightedConfidence;
        current.totalDurationSeconds += durationSeconds;
      } else {
        byLabel.set(label, {
          label,
          recordings,
          avgConfidence: weightedConfidence,
          totalDurationSeconds: durationSeconds,
        });
      }
    }

    const aggregates = Array.from(byLabel.values()).map(item => ({
      ...item,
      avgConfidence: item.recordings > 0 ? item.avgConfidence / item.recordings : 0,
    }));

    aggregates.sort((a, b) => b.recordings - a.recordings || a.label.localeCompare(b.label));
    return aggregates;
  });

  let totalLanguageRecordings = $derived(
    languageAggregates.reduce((sum, item) => sum + item.recordings, 0)
  );

  let averageLanguageConfidence = $derived.by(() => {
    if (totalLanguageRecordings === 0) {
      return 0;
    }
    const weightedSum = languageAggregates.reduce(
      (sum, item) => sum + item.avgConfidence * item.recordings,
      0
    );
    return weightedSum / totalLanguageRecordings;
  });

  let totalLanguageDurationSeconds = $derived(
    languageAggregates.reduce((sum, item) => sum + item.totalDurationSeconds, 0)
  );

  function formatConfidence(conf?: number): string {
    if (typeof conf !== 'number' || !Number.isFinite(conf)) return '';
    const pct = Math.max(0, Math.min(1, conf)) * 100;
    return `${pct.toFixed(1)}%`;
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

  // Clean up pending timers on component destroy
  $effect(() => {
    return () => {
      for (const key in removalTimers) {
        // eslint-disable-next-line security/detect-object-injection -- key is from for-in over own Record
        clearTimeout(removalTimers[key]);
      }
    };
  });
</script>

<section
  class="card col-span-12 flex h-full flex-col rounded-2xl border border-border-100 bg-[var(--color-base-100)] shadow-sm {className}"
>
  <!-- Card Header -->
  <div class="flex items-center gap-2 border-b border-[var(--color-base-200)] px-6 py-4">
    <div class="flex flex-col">
      <h3 class="font-semibold">{t('dashboard.currentlyHearing.title')}</h3>
      <p class="text-sm text-[var(--color-base-content)]/60">
        {t('dashboard.currentlyHearing.subtitle')}
      </p>
    </div>
  </div>

  <!-- Card Content -->
  {#if hasDisplayDetections}
    {#if hasLanguageDetections}
      <div class="grid grid-cols-1 gap-2 border-b border-[var(--color-base-200)] px-4 py-3 sm:grid-cols-3">
        <div class="rounded-lg bg-[var(--color-base-200)] px-3 py-2">
          <div class="text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">
            Languages
          </div>
          <div class="text-sm font-semibold text-[var(--color-base-content)]">
            {languageAggregates.length}
          </div>
        </div>
        <div class="rounded-lg bg-[var(--color-base-200)] px-3 py-2">
          <div class="text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">
            Recordings
          </div>
          <div class="text-sm font-semibold text-[var(--color-base-content)]">
            {totalLanguageRecordings}
          </div>
        </div>
        <div class="rounded-lg bg-[var(--color-base-200)] px-3 py-2">
          <div class="text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">
            Avg Confidence
          </div>
          <div class="text-sm font-semibold text-[var(--color-base-content)]">
            {formatConfidence(averageLanguageConfidence)}
          </div>
        </div>
        <div class="rounded-lg bg-[var(--color-base-200)] px-3 py-2 sm:col-span-3">
          <div class="text-xs uppercase tracking-wide text-[var(--color-base-content)]/60">
            Total Active Duration
          </div>
          <div class="text-sm font-semibold text-[var(--color-base-content)]">
            {formatDuration(totalLanguageDurationSeconds)}
          </div>
          <div class="mt-2 flex flex-wrap gap-2">
            {#each languageAggregates.slice(0, 6) as item (item.label)}
              <span
                class="rounded-full border border-[var(--color-primary)]/25 bg-[var(--color-primary)]/10 px-2.5 py-1 text-xs text-[var(--color-base-content)]"
              >
                {item.label} · {item.recordings}
              </span>
            {/each}
          </div>
        </div>
      </div>
    {/if}

    <div class="flex flex-wrap gap-3 p-4">
      {#each displayDetections as detection (detectionKey(detection))}
        {@const key = detectionKey(detection)}
        {@const elapsedText = getElapsedForKey(key)}
        <div
          class="flex items-center gap-2 rounded-lg px-3 py-2 transition-colors duration-300
            {detection.status === 'approved'
            ? 'border border-[var(--color-success)]/30 bg-[var(--color-success)]/15'
            : detection.status === 'rejected'
              ? 'border border-[var(--color-error)]/30 bg-[var(--color-error)]/15 opacity-60'
              : 'border border-transparent bg-[var(--color-base-200)]'}"
          transition:fade={{ duration: 200 }}
        >
          <!-- Thumbnail -->
          {#if detection.thumbnail}
            <img
              src={detection.thumbnail}
              alt={detection.species}
              class="h-8 aspect-[4/3] rounded-md object-cover"
            />
          {:else}
            <div
              class="flex h-8 aspect-[4/3] items-center justify-center rounded-md bg-[var(--color-base-content)]/10 text-xs font-bold text-[var(--color-base-content)]/50"
            >
              {detection.species.slice(0, 2).toUpperCase()}
            </div>
          {/if}

          <!-- Species info -->
          <div class="flex flex-col">
            <span class="text-sm font-medium leading-tight text-[var(--color-base-content)]">
              {#if isLanguageDetection(detection.species, detection.scientificName)}
                <span
                  class="text-xs uppercase tracking-wider text-[var(--color-primary)] font-semibold mr-1"
                  >Language</span
                >
              {/if}
              {detection.species}
            </span>
            <span class="text-xs text-[var(--color-base-content)]/60">
              {elapsedText}
              {#if hasMultipleSources}
                · {detection.source}
              {/if}

              {#if isLanguageDetection(detection.species, detection.scientificName)}
                {@const confText = formatConfidence(detection.confidence)}
                {#if confText}
                  · {confText}
                {/if}
                {#if (detection.hitCount ?? 0) > 0}
                  · {detection.hitCount} rec
                {/if}
              {/if}
            </span>

            {#if hasAnyTranscript && (detection.transcript ?? '').length > 0}
              <span class="text-xs text-[var(--color-base-content)]/60 truncate max-w-[42ch]">
                {detection.transcript}
              </span>
            {/if}
          </div>

          <!-- Status indicator -->
          {#if detection.status === 'approved'}
            <Check
              aria-label={t('dashboard.approved')}
              class="ml-1 h-4 w-4 text-[var(--color-success)]"
            />
          {:else if detection.status === 'rejected'}
            <X
              aria-label={t('dashboard.rejected')}
              class="ml-1 h-4 w-4 text-[var(--color-error)]"
            />
          {/if}
        </div>
      {/each}
    </div>
  {:else}
    <div class="flex flex-1 items-center justify-center px-6 py-8">
      <p class="text-sm text-[var(--color-base-content)]/40">
        {t('dashboard.currentlyHearing.empty')}
      </p>
    </div>
  {/if}
</section>
