import React, { useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { AlertTriangle, Clock, Layers, Zap } from 'lucide-react';
import { core } from '@/shared/types/wallet.types';

interface SyncProgressRowProps {
  blockchainInfo: core.BlockchainInfo | null;
}

// Ring buffer size for the speed estimator. At a 10-second poll interval this
// gives ~60s of smoothing — enough to dampen spikes without lagging too far
// behind real speed changes (peer drop, parallel header download, etc.).
const SPEED_BUFFER_SIZE = 6;

interface SpeedSample {
  blocks: number;
  timestamp: number;
}

// Compact single-line card: ~45px tall (down from the prior ~130px label-above-value
// layout). Padding tightened from 16px 20px → 10px 16px; gap between metrics row
// and progress bar reduced 10px → 8px; bar thinned 6px → 4px. The behind_time
// warning is rendered as an inline chip in the metrics row instead of a separate
// full-width Banner row, eliminating ~40px of vertical chrome.
const cardStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  padding: '10px 16px',
  display: 'flex',
  flexDirection: 'column',
  gap: '8px',
};

// 3-zone metrics row via CSS Grid `1fr auto 1fr` template.
// Layout per user spec: empty left cell · centered cluster · right cluster.
// Grid chosen over a flex + spacer pattern to align with the canonical 3-zone
// codebase convention established by `shared/components/PaginationFooter.tsx:211`
// (3-zone footer) — also used by Transactions.tsx, AddressView.tsx, and
// TransactionDetailsDialog.tsx. The grid template declaratively guarantees that
// the `auto`-sized center cell lands at the geometric center of the row because
// the two `1fr` cells absorb equal slack. No invisible spacer markup needed.
// Narrow-width behavior: Wails desktop MinWidth keeps the row comfortably above
// the threshold where the center cluster + right cluster would collide; below
// ~520px the right cluster's `justify-self: end` keeps it pinned to the right
// edge and the center cluster stays centered until horizontal overflow forces
// the user agent to truncate or scroll. A media-query single-column collapse
// (like PaginationFooter's `@media (max-width: 780px)`) was considered but
// rejected because the entire metric strip is dense by design (icons + values,
// no labels) and stacking would defeat the height reduction (SC14).
const metricsRowStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr auto 1fr',
  alignItems: 'center',
  gap: '12px',
};

// Center cluster — auto-sized grid cell content; holds Speed + ETA + optional
// warning chip. Sits in the middle grid column so it lands at the geometric
// center of the row regardless of right-cluster content width.
const centerClusterStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '12px',
};

// Left cluster — fills the first 1fr grid column and left-aligns its child
// (the percentage) via `justifyContent: 'flex-start'`. Percentage acts as the
// row's primary anchor at the left edge per user spec.
const leftClusterStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'flex-start',
  gap: '12px',
};

// Right cluster — fills the third 1fr grid column and right-aligns its child
// (the Block counter) via `justifyContent: 'flex-end'`. `justifySelf: 'end'`
// is implicit because the inner flex pins content to the right; the column
// itself spans the full 1fr.
const rightClusterStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'flex-end',
  gap: '12px',
};

// One icon+value pair. The wrapper carries the `title` tooltip so the icon's
// meaning surfaces on hover for accessibility (lucide icons by themselves have
// no implicit semantics for screen readers without an aria-label).
const metricPairStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '6px',
  color: '#ddd',
  fontFamily: 'monospace',
  fontVariantNumeric: 'tabular-nums',
  fontSize: '13px',
};

// Percentage joins the Block counter in the right cluster (per user spec).
// `marginLeft: auto` is intentionally NOT used here — the parent right cluster
// already right-aligns via `justify-content: flex-end`.
const percentStyle: React.CSSProperties = {
  fontSize: '14px',
  fontWeight: 600,
  color: '#ddd',
  fontFamily: 'monospace',
  fontVariantNumeric: 'tabular-nums',
};

// Inline warning chip — replaces the prior full-width `<Banner variant="warning">`.
// Palette is hand-coded here (not consumed via `<StatusPill tone="warning">`)
// because StatusPill's API takes `label: string` only — there's no way to slot
// the AlertTriangle icon inside the pill chrome without extending the shared
// primitive. To avoid a speculative shared-component change, the palette is
// duplicated here verbatim:
//   - source of truth: `shared/components/StatusPill.tsx` PILL_COLORS.warning = '#ff9966'
//   - background = hexToRGBA('#ff9966', 0.15) = rgba(255, 153, 102, 0.15)
//   - border     = hexToRGBA('#ff9966', 0.4)  = rgba(255, 153, 102, 0.4)
// If a future task retones the warning palette in StatusPill (e.g., shifts
// `#ff9966` to a different orange), update these three rgba literals in
// lockstep — or migrate to a shared `<Chip icon={...} tone="warning" />` API
// once a second icon-bearing chip consumer materializes.
const warningChipStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: '4px',
  padding: '2px 8px',
  borderRadius: '999px',
  backgroundColor: 'rgba(255, 153, 102, 0.15)',
  border: '1px solid rgba(255, 153, 102, 0.4)',
  color: '#ff9966',
  fontSize: '11px',
  fontWeight: 500,
  whiteSpace: 'nowrap',
};

const trackStyle: React.CSSProperties = {
  height: '4px',
  backgroundColor: '#3a3a3a',
  borderRadius: '2px',
  overflow: 'hidden',
};

// Format ETA seconds into a coarse human-readable string. Returns '—' when
// input is null (insufficient samples or unknown). Bucketing matches the
// project's existing formatTimeAgo / formatStakeAge conventions.
function formatEta(seconds: number | null): string {
  if (seconds === null || !Number.isFinite(seconds) || seconds <= 0) return '—';
  if (seconds < 60) return '<1m';
  if (seconds < 3600) {
    const m = Math.round(seconds / 60);
    return `${m}m`;
  }
  if (seconds < 86400) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return m > 0 ? `${h}h ${m}m` : `${h}h`;
  }
  if (seconds < 2592000) {
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    return h > 0 ? `${d}d ${h}h` : `${d}d`;
  }
  if (seconds < 31536000) {
    const mo = Math.floor(seconds / 2592000);
    return `${mo}mo`;
  }
  const y = Math.floor(seconds / 31536000);
  const mo = Math.floor((seconds % 31536000) / 2592000);
  return mo > 0 ? `${y}y ${mo}mo` : `${y}y`;
}

// Format sync speed in blocks/sec. Below 1 b/s shows '<1 b/s' so the user
// can still see that progress exists. Null = '—' (first poll, no buffer yet).
function formatSpeed(bps: number | null): string {
  if (bps === null || !Number.isFinite(bps) || bps <= 0) return '—';
  if (bps < 1) return '<1 b/s';
  return `~${Math.round(bps)} b/s`;
}

// Renders the sync progress card above the Overview 2x2 grid as a compact
// single-line metric strip + thin shimmer-animated progress bar. The four
// metrics (Block X/Y, speed, ETA, percentage) are rendered as icon+value pairs
// — icons (Layers/Zap/Clock) replace the prior uppercase text labels, halving
// vertical space. The behind_time warning is an inline pill chip in the same
// row, not a separate Banner. Returns null when fully synced. Speed is computed
// entirely on the frontend from a ring buffer of recent `blocks` polls — no
// backend changes.
export const SyncProgressRow: React.FC<SyncProgressRowProps> = ({ blockchainInfo }) => {
  const { t } = useTranslation(['wallet']);

  // Ring buffer for sync speed estimation. Persisted across renders via ref
  // so polling updates accumulate without re-creating the array. Reset when
  // the component leaves the syncing state (see effect below).
  const bufferRef = useRef<SpeedSample[]>([]);

  const currentBlocks = blockchainInfo?.blocks ?? 0;
  const behindBlocks = blockchainInfo?.behind_blocks ?? 0;
  const syncPercentage = blockchainInfo?.sync_percentage ?? 0;
  const isSyncing = blockchainInfo?.is_syncing ?? false;
  const isOutOfSync = blockchainInfo?.is_out_of_sync ?? false;
  const behindTime = blockchainInfo?.behind_time ?? '';
  const isActive = isSyncing || isOutOfSync;

  // Sample collection: append (blocks, now) on every blocks change while the
  // sync gate is open. Trim to the last SPEED_BUFFER_SIZE entries. Reset when
  // the gate closes so a future re-entry starts with a clean window.
  useEffect(() => {
    if (!isActive) {
      bufferRef.current = [];
      return;
    }
    if (currentBlocks <= 0) return;
    const last = bufferRef.current[bufferRef.current.length - 1];
    // Guard against duplicate samples for the same blocks value (the 10s poll
    // can fire on unchanged data); only the timestamp moves, which would skew
    // the speed denominator without adding signal.
    if (last && last.blocks === currentBlocks) return;
    bufferRef.current.push({ blocks: currentBlocks, timestamp: Date.now() });
    if (bufferRef.current.length > SPEED_BUFFER_SIZE) {
      bufferRef.current = bufferRef.current.slice(-SPEED_BUFFER_SIZE);
    }
  }, [currentBlocks, isActive]);

  // Compute speed (blocks/sec) from the ring buffer. Requires >= 2 samples
  // spanning > 0 seconds; returns null otherwise so the UI shows '—'.
  const syncSpeed = useMemo<number | null>(() => {
    const buf = bufferRef.current;
    if (buf.length < 2) return null;
    const oldest = buf[0];
    const newest = buf[buf.length - 1];
    const blockDelta = newest.blocks - oldest.blocks;
    const timeDeltaSec = (newest.timestamp - oldest.timestamp) / 1000;
    if (timeDeltaSec <= 0 || blockDelta <= 0) return null;
    const bps = blockDelta / timeDeltaSec;
    return Number.isFinite(bps) ? bps : null;
    // Recompute when the latest sample changes — currentBlocks is the trigger
    // that the bufferRef just gained a new entry.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentBlocks]);

  // ETA in seconds = remaining / speed. Returns null when speed is unknown
  // or remaining is non-positive (synced or unknown).
  const etaSeconds = useMemo<number | null>(() => {
    if (syncSpeed === null || syncSpeed <= 0) return null;
    if (behindBlocks <= 0) return null;
    return behindBlocks / syncSpeed;
  }, [syncSpeed, behindBlocks]);

  // Cached locale-aware integer formatter for the Block X / Y display.
  // 'en-US' matches the rest of the wallet's thousand-separator convention.
  const numberFmt = useMemo(() => new Intl.NumberFormat('en-US'), []);

  if (!blockchainInfo) return null;
  if (!isActive) return null;

  const targetBlocks = currentBlocks + behindBlocks;
  const fillClass = isOutOfSync ? 'sync-progress-fill-warning' : 'sync-progress-fill';
  const clampedPercentage = Math.min(Math.max(syncPercentage, 0), 100);

  return (
    <div style={cardStyle}>
      {/*
        3-zone grid layout: percentage left, centered cluster (Speed/ETA/Warning),
        right cluster (Block counter). Each metric pair carries BOTH `title`
        (hover tooltip, sighted-user discoverability of what the icon represents)
        AND `aria-label` (screen reader announcement context — `title` alone is
        announced inconsistently across SR engines: VoiceOver often skips it,
        NVDA/JAWS vary). Without aria-label, an SR user gets just the raw value
        ("500 / 1000") with no context. Pattern mirrors
        `shared/components/IconButton.tsx` which accepts both props and forwards
        both for icon-only affordances.

        Reading-order note: in DOM/screen-reader order this row announces
        Percentage → Speed → ETA → (behind_time warning) → Block counter. The
        percentage acts as the row's primary anchor at the left edge per user
        spec; the warning chip sits inside centerCluster (between live-metrics
        and the right-edge block counter), which puts the "context" callout
        naturally between the per-tick metrics and the absolute progress
        position.
      */}
      <div style={metricsRowStyle}>
        <div style={leftClusterStyle}>
          <span style={percentStyle}>{syncPercentage.toFixed(2)}%</span>
        </div>

        <div style={centerClusterStyle}>
          <div
            style={metricPairStyle}
            title={t('wallet:sync.speedLabel')}
            aria-label={t('wallet:sync.speedLabel')}
          >
            <Zap size={12} color="#888" />
            <span>{formatSpeed(syncSpeed)}</span>
          </div>
          <div
            style={metricPairStyle}
            title={t('wallet:sync.etaLabel')}
            aria-label={t('wallet:sync.etaLabel')}
          >
            <Clock size={12} color="#888" />
            <span>{formatEta(etaSeconds)}</span>
          </div>
          {behindTime && (
            // role="status" (polite live region) preserves the prior Banner's
            // a11y convention — shared `Banner.tsx:22` auto-derives 'alert' for
            // error variants and 'status' for warning/info. "Wallet is behind
            // by X" is an informational freshness signal (user is already
            // syncing and aware of the state), not an actionable interrupt, so
            // polite announcement is the correct default. If product intent
            // shifts (e.g., behind_time > 1h should interrupt), upgrade to
            // role="alert" — this is the same deferred a11y consideration the
            // round-2 reviewer flagged on the original Banner usage.
            <span style={warningChipStyle} role="status">
              <AlertTriangle size={11} />
              {behindTime}
            </span>
          )}
        </div>

        <div style={rightClusterStyle}>
          <div
            style={metricPairStyle}
            title={t('wallet:sync.blockCounterLabel')}
            aria-label={t('wallet:sync.blockCounterLabel')}
          >
            <Layers size={12} color="#888" />
            <span>
              {numberFmt.format(currentBlocks)} / {numberFmt.format(targetBlocks)}
            </span>
          </div>
        </div>
      </div>
      <div style={trackStyle}>
        <div
          className={fillClass}
          style={{
            width: `${clampedPercentage}%`,
            height: '100%',
            transition: 'width 0.3s ease',
          }}
        />
      </div>
    </div>
  );
};
