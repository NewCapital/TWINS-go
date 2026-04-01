import React, { useState, useEffect, useRef, useCallback } from 'react';
import { GetNetworkTraffic, GetTrafficHistory, SaveCSVFile } from '@wailsjs/go/main/App';
import type { TrafficInfo, TrafficSample, TrafficTimeRange } from '@/shared/types/tools.types';
import { formatBytes, formatRate } from '@/shared/utils/format';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DESIRED_SAMPLES = 800; // target density per visible range (controls polling interval)
const MAX_BUFFER_AGE_MS = 24 * 60 * 60 * 1000; // prune samples older than 24h
const MAX_BUFFER_SIZE = 10000; // safety cap on total stored samples

const TIME_RANGES: { label: string; value: TrafficTimeRange; minutes: number }[] = [
  { label: '5m', value: '5m', minutes: 5 },
  { label: '15m', value: '15m', minutes: 15 },
  { label: '30m', value: '30m', minutes: 30 },
  { label: '1h', value: '1h', minutes: 60 },
  { label: '6h', value: '6h', minutes: 360 },
  { label: '24h', value: '24h', minutes: 1440 },
];

const MARGIN = { left: 65, right: 10, top: 10, bottom: 25 };

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getPollingInterval(minutes: number): number {
  return Math.floor((minutes * 60 * 1000) / DESIRED_SAMPLES);
}

/** Log10-based Y-axis grid matching legacy C++ trafficgraphwidget.cpp:76-104 */
function drawYAxisGrid(
  ctx: CanvasRenderingContext2D,
  px: number, py: number, pw: number, ph: number,
  fMax: number,
) {
  if (fMax <= 0) return;

  const base = Math.floor(Math.log10(fMax));
  const majorStep = Math.pow(10, base);

  // Major grid lines
  ctx.strokeStyle = '#444';
  ctx.lineWidth = 0.5;
  ctx.fillStyle = '#888';
  ctx.font = '10px monospace';
  ctx.textAlign = 'right';

  for (let val = majorStep; val < fMax; val += majorStep) {
    const y = py + ph - (val / fMax) * ph;
    ctx.beginPath();
    ctx.moveTo(px, y);
    ctx.lineTo(px + pw, y);
    ctx.stroke();
    ctx.fillText(formatRate(val * 1024), px - 4, y + 3);
  }

  // Minor subdivisions if few major lines (matching C++ fMax/val <= 3.0f)
  if (fMax / majorStep <= 3.0) {
    const minorStep = Math.pow(10, base - 1);
    ctx.strokeStyle = '#333';
    let count = 0;
    for (let val = minorStep; val < fMax; val += minorStep) {
      count++;
      if (count % 10 === 0) continue; // skip where major line is
      const y = py + ph - (val / fMax) * ph;
      ctx.beginPath();
      ctx.moveTo(px, y);
      ctx.lineTo(px + pw, y);
      ctx.stroke();
    }
  }

  // Top scale label
  ctx.fillStyle = '#888';
  ctx.fillText(formatRate(fMax * 1024), px - 4, py + 10);

  // Zero label
  ctx.fillText('0 B/s', px - 4, py + ph + 3);
}

/** X-axis time labels */
function drawXAxisLabels(
  ctx: CanvasRenderingContext2D,
  px: number, py: number, pw: number, ph: number,
  rangeMinutes: number,
) {
  const bottomY = py + ph + 16;
  ctx.fillStyle = '#666';
  ctx.font = '10px monospace';
  ctx.textAlign = 'center';

  const count = rangeMinutes <= 5 ? 5 : rangeMinutes <= 60 ? 6 : 8;
  for (let i = 0; i <= count; i++) {
    const fraction = i / count;
    const xPos = px + pw * (1 - fraction);
    const ago = rangeMinutes * fraction;
    let label: string;
    if (ago === 0) label = 'now';
    else if (ago < 60) label = `-${Math.round(ago)}m`;
    else label = `-${(ago / 60).toFixed(0)}h`;
    ctx.fillText(label, xPos, bottomY);
  }

  // Baseline
  ctx.strokeStyle = '#555';
  ctx.lineWidth = 0.5;
  ctx.beginPath();
  ctx.moveTo(px, py + ph);
  ctx.lineTo(px + pw, py + ph);
  ctx.stroke();
}

/** Draw filled area series with timestamp-based X positioning */
function drawFilledSeries(
  ctx: CanvasRenderingContext2D,
  px: number, py: number, pw: number, ph: number,
  data: { rate: number; timestamp: number }[],
  fMax: number, rangeMs: number, now: number,
  strokeColor: string, fillColor: string,
) {
  if (data.length < 2 || fMax <= 0) return;

  const xForTime = (ts: number) => px + pw - pw * (now - ts) / rangeMs;

  // Extend newest rate to the right edge so graph reaches "now"
  const newestRate = data[data.length - 1].rate;
  const newestY = py + ph - (ph * Math.min(newestRate, fMax)) / fMax;

  // Fill path: bottom-right → right edge at newest rate → trace data → bottom-left
  ctx.beginPath();
  ctx.moveTo(px + pw, py + ph);
  ctx.lineTo(px + pw, newestY);
  for (let i = data.length - 1; i >= 0; i--) {
    const x = xForTime(data[i].timestamp);
    const y = py + ph - (ph * Math.min(data[i].rate, fMax)) / fMax;
    ctx.lineTo(x, y);
  }
  ctx.lineTo(xForTime(data[0].timestamp), py + ph);
  ctx.closePath();
  ctx.fillStyle = fillColor;
  ctx.fill();

  // Stroke top edge
  ctx.beginPath();
  ctx.moveTo(px + pw, newestY);
  for (let i = data.length - 1; i >= 0; i--) {
    const x = xForTime(data[i].timestamp);
    const y = py + ph - (ph * Math.min(data[i].rate, fMax)) / fMax;
    ctx.lineTo(x, y);
  }
  ctx.strokeStyle = strokeColor;
  ctx.lineWidth = 1.5;
  ctx.stroke();
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export const NetworkTrafficTab: React.FC = () => {
  const [samples, setSamples] = useState<TrafficSample[]>([]);
  const [totals, setTotals] = useState<TrafficInfo>({ totalBytesRecv: 0, totalBytesSent: 0, peerCount: 0 });
  const [timeRange, setTimeRange] = useState<TrafficTimeRange>('5m');
  const [peakRateIn, setPeakRateIn] = useState(0);
  const [peakRateOut, setPeakRateOut] = useState(0);
  const [currentRateIn, setCurrentRateIn] = useState(0);
  const [currentRateOut, setCurrentRateOut] = useState(0);
  const [tooltip, setTooltip] = useState<{ x: number; y: number; idx: number } | null>(null);

  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const lastTotalsRef = useRef<TrafficInfo | null>(null);
  const lastSampleTimeRef = useRef<number>(0);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const mountedRef = useRef(true);
  const visibleSamplesRef = useRef<TrafficSample[]>([]);

  const rangeMinutes = TIME_RANGES.find((r) => r.value === timeRange)!.minutes;
  const intervalMs = getPollingInterval(rangeMinutes);
  const historyLoadedRef = useRef(false);

  // --- Load backend history on mount and when time range changes ---
  const loadHistory = useCallback(async (minutes: number) => {
    try {
      const history = (await GetTrafficHistory(minutes, DESIRED_SAMPLES)) as TrafficSample[];
      if (!mountedRef.current || !history || history.length === 0) return;

      setSamples((prev) => {
        // Merge: backend history + any live samples collected after history's latest timestamp
        const latestHistoryTs = history[history.length - 1].timestamp;
        const liveSamples = prev.filter((s) => s.timestamp > latestHistoryTs);
        return [...history, ...liveSamples];
      });

      // Recompute peaks from history
      let peakIn = 0;
      let peakOut = 0;
      for (const s of history) {
        if (s.rateIn > peakIn) peakIn = s.rateIn;
        if (s.rateOut > peakOut) peakOut = s.rateOut;
      }
      setPeakRateIn((prev) => Math.max(prev, peakIn));
      setPeakRateOut((prev) => Math.max(prev, peakOut));
    } catch {
      // Backend not ready yet, will retry on next range change
    }
  }, []);

  // Load history on mount
  useEffect(() => {
    if (!historyLoadedRef.current) {
      historyLoadedRef.current = true;
      loadHistory(rangeMinutes);
    }
  }, [loadHistory, rangeMinutes]);

  // --- Data fetching (live polling for new samples) ---
  const fetchTraffic = useCallback(async () => {
    try {
      const data = (await GetNetworkTraffic()) as TrafficInfo;
      if (!mountedRef.current) return;
      setTotals(data);

      const last = lastTotalsRef.current;
      const now = Date.now();
      if (last && lastSampleTimeRef.current > 0) {
        const elapsed = now - lastSampleTimeRef.current;
        if (elapsed <= 0) return;

        const bytesIn = Math.max(0, data.totalBytesRecv - last.totalBytesRecv);
        const bytesOut = Math.max(0, data.totalBytesSent - last.totalBytesSent);
        // KB/s  (matching C++: bytes/1024 * 1000/interval)
        const rateIn = (bytesIn / 1024) * (1000 / elapsed);
        const rateOut = (bytesOut / 1024) * (1000 / elapsed);

        setCurrentRateIn(rateIn);
        setCurrentRateOut(rateOut);
        setPeakRateIn((prev) => Math.max(prev, rateIn));
        setPeakRateOut((prev) => Math.max(prev, rateOut));

        setSamples((prev) => {
          const next = [...prev, { timestamp: now, bytesIn, bytesOut, rateIn, rateOut }];
          // Prune samples older than 24h
          const cutoff = now - MAX_BUFFER_AGE_MS;
          let start = 0;
          while (start < next.length && next[start].timestamp < cutoff) start++;
          const pruned = start > 0 ? next.slice(start) : next;
          return pruned.length > MAX_BUFFER_SIZE ? pruned.slice(-MAX_BUFFER_SIZE) : pruned;
        });
      }
      lastTotalsRef.current = data;
      lastSampleTimeRef.current = now;
    } catch {
      // Silently handle transient errors
    }
  }, []);

  // --- Timer management ---
  const startTimer = useCallback(() => {
    if (timerRef.current) clearInterval(timerRef.current);
    fetchTraffic();
    timerRef.current = setInterval(fetchTraffic, intervalMs);
  }, [fetchTraffic, intervalMs]);

  useEffect(() => {
    mountedRef.current = true;
    startTimer();
    return () => {
      mountedRef.current = false;
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [startTimer]);

  // --- Canvas resize ---
  useEffect(() => {
    const container = containerRef.current;
    const canvas = canvasRef.current;
    if (!container || !canvas) return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const w = Math.floor(entry.contentRect.width);
        if (w > 0 && canvas.width !== w) canvas.width = w;
      }
    });
    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  // --- Canvas rendering ---
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const w = canvas.width;
    const h = canvas.height;
    const px = MARGIN.left;
    const py = MARGIN.top;
    const pw = w - MARGIN.left - MARGIN.right;
    const ph = h - MARGIN.top - MARGIN.bottom;

    // Background
    ctx.fillStyle = '#1a1a2e';
    ctx.fillRect(0, 0, w, h);

    if (pw <= 0 || ph <= 0) return;

    // Filter to visible time range
    const now = Date.now();
    const rangeMs = rangeMinutes * 60 * 1000;
    const visible = samples.filter((s) => s.timestamp >= now - rangeMs);
    visibleSamplesRef.current = visible;

    if (visible.length < 2) {
      ctx.fillStyle = '#666';
      ctx.font = '13px monospace';
      ctx.textAlign = 'center';
      ctx.fillText('Collecting data...', w / 2, h / 2);
      return;
    }

    // Compute fMax (KB/s) across visible samples
    let fMax = 0;
    for (const s of visible) {
      if (s.rateIn > fMax) fMax = s.rateIn;
      if (s.rateOut > fMax) fMax = s.rateOut;
    }
    if (fMax < 1) fMax = 1; // minimum 1 KB/s scale

    // Y-axis grid + labels
    drawYAxisGrid(ctx, px, py, pw, ph, fMax);

    // X-axis time labels
    drawXAxisLabels(ctx, px, py, pw, ph, rangeMinutes);

    // Y-axis left edge line
    ctx.strokeStyle = '#555';
    ctx.lineWidth = 0.5;
    ctx.beginPath();
    ctx.moveTo(px, py);
    ctx.lineTo(px, py + ph);
    ctx.stroke();

    // Plot area clip
    ctx.save();
    ctx.beginPath();
    ctx.rect(px, py, pw, ph);
    ctx.clip();

    // Filled series: received (green) then sent (red)
    const visibleIn = visible.map((s) => ({ rate: s.rateIn, timestamp: s.timestamp }));
    const visibleOut = visible.map((s) => ({ rate: s.rateOut, timestamp: s.timestamp }));
    drawFilledSeries(ctx, px, py, pw, ph, visibleIn, fMax, rangeMs, now, '#00ff00', 'rgba(0, 255, 0, 0.25)');
    drawFilledSeries(ctx, px, py, pw, ph, visibleOut, fMax, rangeMs, now, '#ff4444', 'rgba(255, 68, 68, 0.25)');

    ctx.restore();
  }, [samples, rangeMinutes]);

  // --- Handlers ---
  const handleTimeRangeChange = (range: TrafficTimeRange) => {
    setTimeRange(range);
    setTooltip(null);
    // Re-fetch history from backend for the new range
    const minutes = TIME_RANGES.find((r) => r.value === range)!.minutes;
    loadHistory(minutes);
  };

  const handleClear = () => {
    setSamples([]);
    setPeakRateIn(0);
    setPeakRateOut(0);
    setCurrentRateIn(0);
    setCurrentRateOut(0);
    lastTotalsRef.current = null;
    lastSampleTimeRef.current = 0;
    setTooltip(null);
  };

  const handleExportCSV = async () => {
    const visible = visibleSamplesRef.current;
    if (visible.length === 0) return;
    const header = 'Timestamp,Received (KB/s),Sent (KB/s)\n';
    const rows = visible
      .map((s) => `${new Date(s.timestamp).toISOString()},${s.rateIn.toFixed(3)},${s.rateOut.toFixed(3)}`)
      .join('\n');
    try {
      await SaveCSVFile(header + rows, 'network_traffic.csv', 'Export Network Traffic');
    } catch {
      // user cancelled
    }
  };

  const handleCanvasMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    const visible = visibleSamplesRef.current;
    if (!canvas || visible.length < 2) { setTooltip(null); return; }
    const rect = canvas.getBoundingClientRect();
    const mouseX = (e.clientX - rect.left) * (canvas.width / rect.width);
    const mouseY = (e.clientY - rect.top) * (canvas.height / rect.height);
    const pw = canvas.width - MARGIN.left - MARGIN.right;
    const ph = canvas.height - MARGIN.top - MARGIN.bottom;

    if (mouseX < MARGIN.left || mouseX > MARGIN.left + pw || mouseY < MARGIN.top || mouseY > MARGIN.top + ph) {
      setTooltip(null);
      return;
    }

    // Map mouse X to a target timestamp
    const now = Date.now();
    const rangeMs = rangeMinutes * 60 * 1000;
    const fraction = (MARGIN.left + pw - mouseX) / pw; // 0 at right ("now"), 1 at left
    const targetTime = now - fraction * rangeMs;

    // Find closest visible sample (sorted by timestamp ascending)
    let closestIdx = 0;
    let closestDist = Math.abs(visible[0].timestamp - targetTime);
    for (let i = 1; i < visible.length; i++) {
      const dist = Math.abs(visible[i].timestamp - targetTime);
      if (dist < closestDist) {
        closestDist = dist;
        closestIdx = i;
      } else {
        break; // sorted, so distance only increases from here
      }
    }

    setTooltip({ x: e.clientX - rect.left, y: e.clientY - rect.top, idx: closestIdx });
  };

  // --- Render ---
  const btnStyle = (active: boolean, first: boolean, last: boolean): React.CSSProperties => ({
    padding: '4px 10px',
    fontSize: '12px',
    border: '1px solid #555',
    borderRight: last ? '1px solid #555' : 'none',
    borderRadius: first ? '4px 0 0 4px' : last ? '0 4px 4px 0' : '0',
    backgroundColor: active ? '#4a9eff' : 'transparent',
    color: active ? '#fff' : '#aaa',
    cursor: 'pointer',
    outline: 'none',
  });

  const actionBtn: React.CSSProperties = {
    padding: '4px 12px',
    fontSize: '12px',
    border: '1px solid #555',
    borderRadius: '4px',
    backgroundColor: 'transparent',
    color: '#aaa',
    cursor: 'pointer',
    outline: 'none',
  };

  const tooltipSample = tooltip ? visibleSamplesRef.current[tooltip.idx] : null;

  return (
    <div style={{ padding: '16px 20px', display: 'flex', flexDirection: 'column', height: '100%', overflow: 'auto' }}>
      {/* Control bar */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
        <div style={{ display: 'flex' }}>
          {TIME_RANGES.map((r, i) => (
            <button
              key={r.value}
              style={btnStyle(timeRange === r.value, i === 0, i === TIME_RANGES.length - 1)}
              onClick={() => handleTimeRangeChange(r.value)}
            >
              {r.label}
            </button>
          ))}
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button style={actionBtn} onClick={handleClear}>Clear</button>
          <button style={actionBtn} onClick={handleExportCSV}>Export CSV</button>
        </div>
      </div>

      {/* Graph */}
      <div ref={containerRef} style={{ width: '100%', position: 'relative' }}>
        <canvas
          ref={canvasRef}
          height={280}
          style={{ width: '100%', height: '280px', borderRadius: '4px', border: '1px solid #444' }}
          onMouseMove={handleCanvasMouseMove}
          onMouseLeave={() => setTooltip(null)}
        />
        {/* Tooltip overlay */}
        {tooltipSample && (
          <div
            style={{
              position: 'absolute',
              left: Math.min(tooltip!.x + 12, (containerRef.current?.clientWidth ?? 300) - 160),
              top: tooltip!.y - 50,
              backgroundColor: '#2a2a3a',
              border: '1px solid #555',
              borderRadius: '4px',
              padding: '6px 10px',
              fontSize: '11px',
              color: '#ddd',
              pointerEvents: 'none',
              zIndex: 10,
              whiteSpace: 'nowrap',
            }}
          >
            <div><span style={{ color: '#00ff00' }}>In:</span> {formatRate(tooltipSample.rateIn * 1024)}</div>
            <div><span style={{ color: '#ff4444' }}>Out:</span> {formatRate(tooltipSample.rateOut * 1024)}</div>
            <div style={{ color: '#888', marginTop: '2px', fontSize: '10px' }}>
              {new Date(tooltipSample.timestamp).toLocaleTimeString()}
            </div>
          </div>
        )}
      </div>

      {/* Stats row: legend + current rates + peak */}
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '12px', fontSize: '13px', flexWrap: 'wrap', gap: '8px' }}>
        {/* Legend */}
        <div style={{ display: 'flex', gap: '16px' }}>
          <span>
            <span style={{ color: '#00ff00' }}>&#9632;</span>
            <span style={{ color: '#aaa', marginLeft: '6px' }}>Received</span>
          </span>
          <span>
            <span style={{ color: '#ff4444' }}>&#9632;</span>
            <span style={{ color: '#aaa', marginLeft: '6px' }}>Sent</span>
          </span>
        </div>
        {/* Current rates */}
        <div style={{ display: 'flex', gap: '16px', color: '#aaa' }}>
          <span>In: <span style={{ color: '#00ff00', fontFamily: 'monospace' }}>{formatRate(currentRateIn * 1024)}</span></span>
          <span>Out: <span style={{ color: '#ff4444', fontFamily: 'monospace' }}>{formatRate(currentRateOut * 1024)}</span></span>
        </div>
        {/* Peak rates */}
        <div style={{ display: 'flex', gap: '16px', color: '#888' }}>
          <span>Peak In: <span style={{ color: '#fff' }}>{formatRate(peakRateIn * 1024)}</span></span>
          <span>Peak Out: <span style={{ color: '#fff' }}>{formatRate(peakRateOut * 1024)}</span></span>
        </div>
      </div>

      {/* Totals row */}
      <div style={{ display: 'flex', gap: '20px', marginTop: '8px', color: '#aaa', fontSize: '13px' }}>
        <span>Total In: <span style={{ color: '#fff' }}>{formatBytes(totals.totalBytesRecv)}</span></span>
        <span>Total Out: <span style={{ color: '#fff' }}>{formatBytes(totals.totalBytesSent)}</span></span>
        <span>Peers: <span style={{ color: '#fff' }}>{totals.peerCount}</span></span>
      </div>

      {/* Peer Traffic Breakdown - lazy loaded */}
      <PeerBreakdownSection />
    </div>
  );
};

// ---------------------------------------------------------------------------
// Peer Traffic Breakdown (inline to avoid circular import issues)
// ---------------------------------------------------------------------------

const PeerBreakdownSection: React.FC = () => {
  const [expanded, setExpanded] = useState(false);
  const [peers, setPeers] = useState<import('@/shared/types/tools.types').PeerDetail[]>([]);
  const mountedRef = useRef(true);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!expanded) return;
    mountedRef.current = true;

    const fetch = async () => {
      try {
        const { GetPeerList } = await import('@wailsjs/go/main/App');
        const data = await GetPeerList();
        if (mountedRef.current) {
          const sorted = [...(data || [])].sort(
            (a, b) => (b.bytesSent + b.bytesReceived) - (a.bytesSent + a.bytesReceived),
          );
          setPeers(sorted);
        }
      } catch { /* ignore */ }
    };

    fetch();
    timerRef.current = setInterval(fetch, 5000);
    return () => {
      mountedRef.current = false;
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [expanded]);

  const thStyle: React.CSSProperties = {
    padding: '4px 8px', textAlign: 'left', fontSize: '12px',
    fontWeight: 500, color: '#aaa', borderBottom: '1px solid #444',
    position: 'sticky', top: 0, backgroundColor: '#333', zIndex: 1,
  };
  const tdStyle: React.CSSProperties = {
    padding: '4px 8px', fontSize: '12px', color: '#ddd',
    borderBottom: '1px solid #333',
  };

  return (
    <div style={{ marginTop: '16px', borderTop: '1px solid #444', paddingTop: '12px' }}>
      <div
        onClick={() => setExpanded(!expanded)}
        style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '8px', userSelect: 'none' }}
      >
        <span style={{ color: '#aaa', fontSize: '13px' }}>
          {expanded ? '\u25BC' : '\u25B6'} Peer Traffic Breakdown
        </span>
      </div>
      {expanded && (
        <div style={{ maxHeight: '200px', overflowY: 'auto', marginTop: '8px' }}>
          {peers.length === 0 ? (
            <div style={{ color: '#666', fontSize: '12px', padding: '8px' }}>No peers connected</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  <th style={thStyle}>Peer Address</th>
                  <th style={{ ...thStyle, width: '50px', textAlign: 'center' }}>Dir</th>
                  <th style={{ ...thStyle, width: '90px', textAlign: 'right' }}>Received</th>
                  <th style={{ ...thStyle, width: '90px', textAlign: 'right' }}>Sent</th>
                  <th style={{ ...thStyle, width: '90px', textAlign: 'right' }}>Total</th>
                </tr>
              </thead>
              <tbody>
                {peers.map((p) => (
                  <tr key={p.id} style={{ cursor: 'default' }}
                    onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.backgroundColor = '#3a3a3a'; }}
                    onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.backgroundColor = 'transparent'; }}
                  >
                    <td style={{ ...tdStyle, fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '200px' }}>
                      {p.address}
                    </td>
                    <td style={{ ...tdStyle, textAlign: 'center' }}>
                      <span style={{ color: p.inbound ? '#ffaa00' : '#00ff00', fontSize: '11px', fontWeight: 600 }}>
                        {p.inbound ? 'IN' : 'OUT'}
                      </span>
                    </td>
                    <td style={{ ...tdStyle, textAlign: 'right', fontFamily: 'monospace' }}>{formatBytes(p.bytesReceived)}</td>
                    <td style={{ ...tdStyle, textAlign: 'right', fontFamily: 'monospace' }}>{formatBytes(p.bytesSent)}</td>
                    <td style={{ ...tdStyle, textAlign: 'right', fontFamily: 'monospace' }}>{formatBytes(p.bytesSent + p.bytesReceived)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
};
