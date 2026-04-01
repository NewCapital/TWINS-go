import React, { useState, useEffect, useRef, useCallback } from 'react';
import { GetDebugStatus, GetDebugEvents, ClearDebugLog, GetDebugSummary } from '@wailsjs/go/main/App';
import { main } from '@wailsjs/go/models';
import { DebugOverviewPanel, type DebugSummary } from './DebugSummaryPanel';

// Debug refresh interval (3 seconds)
const DEBUG_REFRESH_SECONDS = 3;

// Category colors for visual distinction
const CATEGORY_COLORS: Record<string, string> = {
  sync: '#4a8af4',      // blue
  broadcast: '#4caf50',  // green
  ping: '#f0c040',       // yellow
  status: '#b070d0',     // purple
  winner: '#ff9800',     // orange
  active: '#00bcd4',     // cyan
  network: '#e0e0e0',    // light gray
  session: '#ffffff',    // white — session start markers
};

const getCategoryColor = (category: string): string => {
  return CATEGORY_COLORS[category] || '#888';
};

// Format file size for display
const formatFileSize = (bytes: number): string => {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
};

// Format timestamp for display
const formatTimestamp = (iso: string): string => {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  const ss = String(d.getSeconds()).padStart(2, '0');
  const ms = String(d.getMilliseconds()).padStart(3, '0');
  return `${hh}:${mm}:${ss}.${ms}`;
};

interface DebugEvent {
  timestamp: string;
  type: string;
  category: string;
  source: string;
  summary: string;
  payload: string;
}

interface DebugStatus {
  enabled: boolean;
  total: number;
  byCategory: Record<string, number>;
  fileSize: number;
}

type SubTab = 'overview' | 'events';

export const MasternodeDebugPanel: React.FC = () => {
  const [status, setStatus] = useState<DebugStatus | null>(null);
  const [events, setEvents] = useState<DebugEvent[]>([]);
  const [expandedTs, setExpandedTs] = useState<string | null>(null);
  const [categoryFilter, setCategoryFilter] = useState('');
  const [searchFilter, setSearchFilter] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [activeSubTab, setActiveSubTab] = useState<SubTab>('overview');
  const [summary, setSummary] = useState<DebugSummary | null>(null);

  const mountedRef = useRef(true);
  const eventListRef = useRef<HTMLDivElement>(null);
  const prevFirstEventRef = useRef<string | null>(null);

  // Fetch debug status
  const fetchStatus = useCallback(async () => {
    try {
      const result = await GetDebugStatus();
      if (mountedRef.current && result) {
        setStatus({
          enabled: result.enabled,
          total: result.total,
          byCategory: result.byCategory || {},
          fileSize: result.fileSize,
        });
      }
    } catch (err) {
      console.error('Failed to fetch debug status:', err);
    }
  }, []);

  // Fetch debug events
  const fetchEvents = useCallback(async () => {
    if (activeSubTab !== 'events') return;
    try {
      const filter = new main.DebugFilter({
        category: categoryFilter || undefined,
        search: searchFilter || undefined,
        limit: 500,
      });
      const result = await GetDebugEvents(filter);
      if (mountedRef.current && result) {
        setEvents(result as DebugEvent[]);
      }
    } catch (err) {
      console.error('Failed to fetch debug events:', err);
    }
  }, [activeSubTab, categoryFilter, searchFilter]);

  // Fetch debug summary
  const fetchSummary = useCallback(async () => {
    if (activeSubTab !== 'overview') return;
    try {
      const result = await GetDebugSummary();
      if (mountedRef.current && result) {
        setSummary(result as unknown as DebugSummary);
      }
    } catch (err) {
      console.error('Failed to fetch debug summary:', err);
    }
  }, [activeSubTab]);

  // Stable refs to avoid stale closures in timer
  const fetchStatusRef = useRef(fetchStatus);
  fetchStatusRef.current = fetchStatus;
  const fetchEventsRef = useRef(fetchEvents);
  fetchEventsRef.current = fetchEvents;
  const fetchSummaryRef = useRef(fetchSummary);
  fetchSummaryRef.current = fetchSummary;

  // Initial fetch and auto-refresh
  useEffect(() => {
    mountedRef.current = true;
    fetchStatusRef.current();
    fetchEventsRef.current();
    fetchSummaryRef.current();

    const interval = setInterval(() => {
      fetchStatusRef.current();
      fetchEventsRef.current();
      fetchSummaryRef.current();
    }, DEBUG_REFRESH_SECONDS * 1000);

    return () => {
      mountedRef.current = false;
      clearInterval(interval);
    };
  }, []);

  // Refetch when filters change
  useEffect(() => {
    fetchEventsRef.current();
  }, [categoryFilter, searchFilter]);

  // Fetch data when sub-tab changes
  useEffect(() => {
    if (activeSubTab === 'overview') {
      fetchSummaryRef.current();
    } else {
      fetchEventsRef.current();
    }
  }, [activeSubTab]);

  // Scroll to top only when a new event arrives AND the user is already near the top.
  // If the user has scrolled down to investigate older events, preserve their position.
  useEffect(() => {
    const firstTs = events.length > 0 ? events[0].timestamp : null;
    if (firstTs !== prevFirstEventRef.current) {
      prevFirstEventRef.current = firstTs;
      if (eventListRef.current && firstTs !== null) {
        const isNearTop = eventListRef.current.scrollTop < 100;
        if (isNearTop) {
          eventListRef.current.scrollTop = 0;
        }
      }
    }
  }, [events]);

  // Clear log
  const handleClear = async () => {
    try {
      await ClearDebugLog();
      setEvents([]);
      setSummary(null);
      setExpandedTs(null);
      setError(null);
      await fetchStatus();
      if (activeSubTab === 'overview') {
        const result = await GetDebugSummary();
        if (mountedRef.current && result) {
          setSummary(result as unknown as DebugSummary);
        }
      } else {
        await fetchEvents();
      }
    } catch (err) {
      setError(String(err));
    }
  };

  // Row click to expand/collapse payload (composite key avoids collision when
  // two events share the same millisecond timestamp)
  const makeRowKey = (event: DebugEvent, idx: number) =>
    `${event.timestamp}-${event.type}-${event.source}-${idx}`;

  const handleRowClick = (key: string) => {
    setExpandedTs(expandedTs === key ? null : key);
  };

  // All available categories from status
  const categories = status?.byCategory ? Object.keys(status.byCategory).sort() : [];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', gap: '8px' }}>
      {/* Header bar — line 1: controls + stats */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '4px 0',
      }}>
        <span
          style={{
            padding: '4px 12px',
            fontSize: '11px',
            backgroundColor: status?.enabled ? '#4caf50' : '#555',
            color: '#fff',
            borderRadius: '2px',
          }}
        >
          {status?.enabled ? 'Enabled' : 'Disabled'}
        </span>

        <button
          onClick={handleClear}
          style={{
            padding: '4px 12px',
            fontSize: '11px',
            backgroundColor: '#3a3a3a',
            color: '#ccc',
            border: '1px solid #555',
            borderRadius: '2px',
            cursor: 'pointer',
          }}
        >
          Clear
        </button>

        <div style={{ flex: 1 }} />

        <span style={{ fontSize: '11px', color: '#999' }}>
          {status ? `${status.total} events` : '...'}
          {status?.fileSize ? ` | ${formatFileSize(status.fileSize)}` : ''}
        </span>
      </div>

      {/* Header bar — line 2: sub-tab buttons */}
      <div style={{
        display: 'flex',
        gap: '8px',
        padding: '0 0 4px 0',
      }}>
        <SubTabButton label="Overview" active={activeSubTab === 'overview'} onClick={() => setActiveSubTab('overview')} />
        <SubTabButton label="Events" active={activeSubTab === 'events'} onClick={() => setActiveSubTab('events')} />
      </div>

      {/* Error message */}
      {error && (
        <div style={{
          padding: '4px 8px',
          fontSize: '11px',
          color: '#ff6666',
          backgroundColor: '#3a1a1a',
          border: '1px solid #ff6666',
          borderRadius: '2px',
        }}>
          {error}
        </div>
      )}

      {/* Sub-tab content */}
      {activeSubTab === 'overview' ? (
        <DebugOverviewPanel summary={summary} />
      ) : (
        <>
          {/* Category summary chips */}
          {status?.byCategory && Object.keys(status.byCategory).length > 0 && (
            <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
              {Object.entries(status.byCategory)
                .sort(([, a], [, b]) => b - a)
                .map(([cat, count]) => (
                  <span
                    key={cat}
                    style={{
                      padding: '2px 8px',
                      fontSize: '10px',
                      backgroundColor: getCategoryColor(cat) + '22',
                      color: getCategoryColor(cat),
                      border: `1px solid ${getCategoryColor(cat)}44`,
                      borderRadius: '10px',
                      cursor: 'pointer',
                      opacity: categoryFilter === cat ? 1 : 0.7,
                      fontWeight: categoryFilter === cat ? 'bold' : 'normal',
                    }}
                    onClick={() => setCategoryFilter(categoryFilter === cat ? '' : cat)}
                    title={`Filter by ${cat} (click to toggle)`}
                  >
                    {cat}: {count}
                  </span>
                ))}
            </div>
          )}

          {/* Filter bar */}
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <select
              value={categoryFilter}
              onChange={(e) => setCategoryFilter(e.target.value)}
              style={{
                padding: '4px 8px',
                fontSize: '11px',
                backgroundColor: '#2b2b2b',
                color: '#ccc',
                border: '1px solid #555',
                borderRadius: '2px',
              }}
            >
              <option value="">All Categories</option>
              {categories.map((cat) => (
                <option key={cat} value={cat}>{cat}</option>
              ))}
            </select>

            <input
              type="text"
              placeholder="Search events..."
              value={searchFilter}
              onChange={(e) => setSearchFilter(e.target.value)}
              style={{
                flex: 1,
                padding: '4px 8px',
                fontSize: '11px',
                backgroundColor: '#2b2b2b',
                color: '#ccc',
                border: '1px solid #555',
                borderRadius: '2px',
              }}
            />

            {(categoryFilter || searchFilter) && (
              <button
                onClick={() => { setCategoryFilter(''); setSearchFilter(''); }}
                style={{
                  padding: '4px 8px',
                  fontSize: '11px',
                  backgroundColor: '#3a3a3a',
                  color: '#ccc',
                  border: '1px solid #555',
                  borderRadius: '2px',
                  cursor: 'pointer',
                }}
              >
                Clear Filters
              </button>
            )}
          </div>

          {/* Event list */}
          <div
            ref={eventListRef}
            style={{
              flex: 1,
              overflow: 'auto',
              border: '1px solid #3a3a3a',
              borderRadius: '2px',
              backgroundColor: '#1e1e1e',
              minHeight: 0,
            }}
          >
            {/* Table header */}
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '11px' }}>
              <thead style={{ position: 'sticky', top: 0, zIndex: 1 }}>
                <tr style={{ backgroundColor: '#3a3a3a' }}>
                  <th style={{ ...thStyle, width: '85px' }}>Time</th>
                  <th style={{ ...thStyle, width: '70px' }}>Category</th>
                  <th style={{ ...thStyle, width: '140px' }}>Type</th>
                  <th style={{ ...thStyle, width: '130px' }}>Source</th>
                  <th style={{ ...thStyle }}>Summary</th>
                </tr>
              </thead>
              <tbody>
                {events.length === 0 ? (
                  <tr>
                    <td colSpan={5} style={{ padding: '20px', textAlign: 'center', color: '#666' }}>
                      {!status?.enabled
                        ? 'Debug collector is disabled. Enable it to capture events.'
                        : 'No events yet. Masternode activity will appear here.'}
                    </td>
                  </tr>
                ) : (
                  events.map((event, idx) => {
                    const rowKey = makeRowKey(event, idx);
                    const isExpanded = expandedTs === rowKey;
                    return (
                    <React.Fragment key={rowKey}>
                      <tr
                        onClick={() => handleRowClick(rowKey)}
                        style={{
                          cursor: 'pointer',
                          backgroundColor: isExpanded ? '#2a2a3a' : (idx % 2 === 0 ? '#1e1e1e' : '#232323'),
                          borderBottom: '1px solid #2a2a2a',
                        }}
                        onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.backgroundColor = '#2a2a3a'; }}
                        onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.backgroundColor = isExpanded ? '#2a2a3a' : (idx % 2 === 0 ? '#1e1e1e' : '#232323'); }}
                      >
                        <td style={{ ...tdStyle, fontFamily: 'monospace', color: '#888' }}>
                          {formatTimestamp(event.timestamp)}
                        </td>
                        <td style={{ ...tdStyle }}>
                          <span style={{
                            color: getCategoryColor(event.category),
                            fontWeight: 'bold',
                          }}>
                            {event.category}
                          </span>
                        </td>
                        <td style={{ ...tdStyle, color: '#aaa', fontFamily: 'monospace', fontSize: '10px' }}>
                          {event.type}
                        </td>
                        <td style={{
                          ...tdStyle,
                          color: '#999',
                          maxWidth: '130px',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}
                        title={event.source}
                        >
                          {event.source || '-'}
                        </td>
                        <td style={{ ...tdStyle, color: '#ccc' }}>
                          {event.summary}
                        </td>
                      </tr>
                      {isExpanded && event.payload && event.payload !== '{}' && event.payload !== 'null' && (
                        <tr>
                          <td colSpan={5} style={{
                            padding: '4px 12px 8px 12px',
                            backgroundColor: '#1a1a2a',
                            borderBottom: '1px solid #333',
                          }}>
                            <pre style={{
                              margin: 0,
                              fontSize: '10px',
                              color: '#aaa',
                              fontFamily: 'monospace',
                              whiteSpace: 'pre-wrap',
                              wordBreak: 'break-all',
                              maxHeight: '200px',
                              overflow: 'auto',
                            }}>
                              {formatPayload(event.payload)}
                            </pre>
                          </td>
                        </tr>
                      )}
                    </React.Fragment>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>

          {/* Footer */}
          <div style={{
            display: 'flex',
            justifyContent: 'space-between',
            fontSize: '10px',
            color: '#666',
            padding: '2px 0',
          }}>
            <span>
              Showing {events.length} events
              {categoryFilter && ` (filtered: ${categoryFilter})`}
              {searchFilter && ` (search: "${searchFilter}")`}
            </span>
            <span>
              Latest events first
            </span>
          </div>
        </>
      )}
    </div>
  );
};

// Sub-tab button component
const SubTabButton: React.FC<{ label: string; active: boolean; onClick: () => void }> = ({ label, active, onClick }) => (
  <button
    onClick={onClick}
    style={{
      padding: '4px 12px',
      fontSize: '11px',
      backgroundColor: active ? '#4a8af4' : '#3a3a3a',
      color: active ? '#fff' : '#ccc',
      border: `1px solid ${active ? '#4a8af4' : '#555'}`,
      borderRadius: '2px',
      cursor: 'pointer',
    }}
  >
    {label}
  </button>
);

// Format JSON payload for display
const formatPayload = (payload: string): string => {
  try {
    const parsed = JSON.parse(payload);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return payload;
  }
};

// Shared table cell styles
const thStyle: React.CSSProperties = {
  padding: '6px 8px',
  textAlign: 'left',
  color: '#aaa',
  fontWeight: 'bold',
  borderBottom: '1px solid #444',
  backgroundColor: '#3a3a3a',
};

const tdStyle: React.CSSProperties = {
  padding: '4px 8px',
  verticalAlign: 'top',
};
