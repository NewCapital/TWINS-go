import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { GetPeerList, GetBannedPeers, BanPeer, UnbanPeer, DisconnectPeer, AddPeer, AddPeers, SetPeerAlias, RemovePeerAlias, CopyToClipboard, SaveCSVFile, GetToolsInfo } from '@wailsjs/go/main/App';
import type { PeerDetail, BannedPeerInfo } from '@/shared/types/tools.types';
import { formatBytes } from '@/shared/utils/format';
import { SimpleConfirmDialog } from '@/shared/components/SimpleConfirmDialog';

type PeerView = 'connected' | 'banned';
type BanDuration = '1h' | '1d' | '1w' | '1y';
type SortColumn = 'address' | 'userAgent' | 'ping' | 'bytesSent' | 'bytesReceived' | 'connTime' | 'height';
type SortDirection = 'asc' | 'desc';

interface ContextMenuState {
  visible: boolean;
  x: number;
  y: number;
  peer: PeerDetail | null;
  bannedPeer: BannedPeerInfo | null;
}

interface ConfirmAction {
  type: 'disconnect' | 'ban';
  peer: PeerDetail;
  banDuration?: BanDuration;
}

// Extract IP from address string (handles IPv4 and IPv6)
const extractIP = (addr: string): string => {
  if (addr.startsWith('[')) {
    return addr.slice(1, addr.indexOf(']'));
  }
  return addr.split(':')[0];
};

// Color-coded ping indicator
const getPingColor = (ms: number): string => {
  if (ms <= 0) return '#666';
  if (ms < 100) return '#00ff00';
  if (ms < 500) return '#ffaa00';
  return '#ff4444';
};

// Color-coded height indicator based on difference from our height
const getHeightColor = (peerHeight: number, ourHeight: number): string => {
  if (peerHeight <= 0) return '#666'; // unknown / pre-70928 peer
  if (ourHeight <= 0) return '#aaa'; // we don't know our own height yet
  const diff = Math.abs(peerHeight - ourHeight);
  if (diff === 0) return '#00ff00'; // green: matches exactly
  if (diff <= 10) return '#ffaa00'; // yellow: within 10 blocks
  return '#ff4444'; // red: divergent
};

// Format duration like C++ Qt wallet: "2d 3h 45m"
const formatDuration = (startUnix: number): string => {
  const seconds = Math.floor(Date.now() / 1000 - startUnix);
  if (seconds < 0) return 'N/A';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
};

const formatTime = (unix: number): string => {
  if (!unix) return 'N/A';
  return new Date(unix * 1000).toLocaleString();
};

const formatPing = (ms: number): string => {
  if (ms <= 0) return 'N/A';
  if (ms < 1000) return `${ms.toFixed(0)} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
};

const banDurationLabels: Record<BanDuration, string> = {
  '1h': '1 hour',
  '1d': '1 day',
  '1w': '1 week',
  '1y': '1 year',
};

export const PeersTab: React.FC = () => {
  const [peers, setPeers] = useState<PeerDetail[]>([]);
  const [bannedPeers, setBannedPeers] = useState<BannedPeerInfo[]>([]);
  const [ourHeight, setOurHeight] = useState<number>(0);
  const [selectedPeerAddr, setSelectedPeerAddr] = useState<string | null>(null);
  const [view, setView] = useState<PeerView>('connected');
  const [actionError, setActionError] = useState<string | null>(null);
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({
    visible: false, x: 0, y: 0, peer: null, bannedPeer: null,
  });

  // Sorting state
  const [sortColumn, setSortColumn] = useState<SortColumn>('address');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');

  // Search/filter
  const [searchFilter, setSearchFilter] = useState('');

  // Add Peer UI
  const [showAddPeer, setShowAddPeer] = useState(false);
  const [addPeerAddr, setAddPeerAddr] = useState('');
  const [addPeerAlias, setAddPeerAlias] = useState('');
  const [addPeerError, setAddPeerError] = useState<string | null>(null);
  const [isMultiLineMode, setIsMultiLineMode] = useState(false);
  const [multiLineInput, setMultiLineInput] = useState('');
  const [multiLineResults, setMultiLineResults] = useState<Array<{ line: string; address: string; alias: string; success: boolean; error: string }> | null>(null);
  const [isAddingPeers, setIsAddingPeers] = useState(false);

  // Alias edit dialog
  const [aliasDialog, setAliasDialog] = useState<{ open: boolean; addr: string; currentAlias: string }>({ open: false, addr: '', currentAlias: '' });
  const [aliasInput, setAliasInput] = useState('');
  const aliasInputRef = useRef<HTMLInputElement>(null);

  // Confirm dialog
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [isActionLoading, setIsActionLoading] = useState(false);

  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const mountedRef = useRef(true);
  const addPeerInputRef = useRef<HTMLInputElement>(null);

  // Fetch peers and our height together to avoid stale ourHeight color flash
  const fetchPeersAndHeight = useCallback(async () => {
    try {
      const [peersData, info] = await Promise.all([
        GetPeerList().catch(() => null),
        GetToolsInfo().catch(() => null),
      ]);
      if (mountedRef.current) {
        setPeers((peersData as PeerDetail[]) || []);
        setOurHeight(info?.blockCount || 0);
      }
    } catch { /* silent */ }
  }, []);

  const fetchBanned = useCallback(async () => {
    try {
      const data = await GetBannedPeers() as BannedPeerInfo[];
      if (mountedRef.current) setBannedPeers(data || []);
    } catch { /* silent */ }
  }, []);

  const fetchAll = useCallback(() => {
    fetchPeersAndHeight();
    fetchBanned();
  }, [fetchPeersAndHeight, fetchBanned]);

  useEffect(() => {
    mountedRef.current = true;
    fetchAll();
    timerRef.current = setInterval(fetchAll, 5000);
    return () => {
      mountedRef.current = false;
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [fetchAll]);

  // Preserve selection across refreshes using address (stable across peer ID changes)
  const selectedPeer = useMemo(() => {
    if (selectedPeerAddr === null) return null;
    return peers.find((p) => p.address === selectedPeerAddr) || null;
  }, [peers, selectedPeerAddr]);

  // Clear selection if peer disconnected
  useEffect(() => {
    if (selectedPeerAddr !== null && !peers.find((p) => p.address === selectedPeerAddr)) {
      setSelectedPeerAddr(null);
    }
  }, [peers, selectedPeerAddr]);

  // Close context menu on outside click
  useEffect(() => {
    const handleMouseDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setContextMenu((prev) => ({ ...prev, visible: false }));
      }
    };
    if (contextMenu.visible) {
      document.addEventListener('mousedown', handleMouseDown);
    }
    return () => document.removeEventListener('mousedown', handleMouseDown);
  }, [contextMenu.visible]);

  // Focus add peer input when shown
  useEffect(() => {
    if (showAddPeer && addPeerInputRef.current) {
      addPeerInputRef.current.focus();
    }
  }, [showAddPeer]);

  // Focus alias dialog input when shown
  useEffect(() => {
    if (aliasDialog.open && aliasInputRef.current) {
      setTimeout(() => aliasInputRef.current?.focus(), 50);
    }
  }, [aliasDialog.open]);

  // Sorted and filtered peers
  const sortedPeers = useMemo(() => {
    let filtered = peers;

    // Apply search filter
    if (searchFilter) {
      const lc = searchFilter.toLowerCase();
      filtered = peers.filter(
        (p) =>
          p.address.toLowerCase().includes(lc) ||
          (p.alias && p.alias.toLowerCase().includes(lc)) ||
          p.userAgent.toLowerCase().includes(lc) ||
          p.services.toLowerCase().includes(lc)
      );
    }

    // Apply sorting
    const sorted = [...filtered].sort((a, b) => {
      let aVal: string | number;
      let bVal: string | number;

      switch (sortColumn) {
        case 'address':
          aVal = a.address.toLowerCase();
          bVal = b.address.toLowerCase();
          break;
        case 'userAgent':
          aVal = a.userAgent.toLowerCase();
          bVal = b.userAgent.toLowerCase();
          break;
        case 'ping':
          aVal = a.pingTime;
          bVal = b.pingTime;
          break;
        case 'bytesSent':
          aVal = a.bytesSent;
          bVal = b.bytesSent;
          break;
        case 'bytesReceived':
          aVal = a.bytesReceived;
          bVal = b.bytesReceived;
          break;
        case 'connTime':
          aVal = a.connTime;
          bVal = b.connTime;
          break;
        case 'height':
          aVal = a.syncedHeight;
          bVal = b.syncedHeight;
          break;
        default:
          return 0;
      }

      if (aVal < bVal) return sortDirection === 'asc' ? -1 : 1;
      if (aVal > bVal) return sortDirection === 'asc' ? 1 : -1;
      return 0;
    });

    return sorted;
  }, [peers, searchFilter, sortColumn, sortDirection]);

  const sortedBannedPeers = useMemo(() => {
    return [...bannedPeers].sort((a, b) => a.address.toLowerCase().localeCompare(b.address.toLowerCase()));
  }, [bannedPeers]);

  const handleSort = (column: SortColumn) => {
    if (sortColumn === column) {
      setSortDirection((prev) => (prev === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortColumn(column);
      setSortDirection('asc');
    }
  };

  const renderSortIndicator = (column: SortColumn) => {
    if (sortColumn !== column) return null;
    return (
      <span style={{ marginLeft: '4px', fontSize: '10px' }}>
        {sortDirection === 'asc' ? '▲' : '▼'}
      </span>
    );
  };

  const handleContextMenu = (e: React.MouseEvent, peer?: PeerDetail, banned?: BannedPeerInfo) => {
    e.preventDefault();
    const menuWidth = 180;
    const menuHeight = banned ? 100 : 280;
    const x = Math.min(e.clientX, window.innerWidth - menuWidth - 10);
    const y = Math.min(e.clientY, window.innerHeight - menuHeight - 10);
    setContextMenu({ visible: true, x, y, peer: peer || null, bannedPeer: banned || null });
    if (peer) setSelectedPeerAddr(peer.address);
  };

  // Confirmation-based ban
  const handleBanRequest = (duration: BanDuration) => {
    if (!contextMenu.peer) return;
    setConfirmAction({ type: 'ban', peer: contextMenu.peer, banDuration: duration });
    setContextMenu((prev) => ({ ...prev, visible: false }));
  };

  // Confirmation-based disconnect
  const handleDisconnectRequest = () => {
    if (!contextMenu.peer) return;
    setConfirmAction({ type: 'disconnect', peer: contextMenu.peer });
    setContextMenu((prev) => ({ ...prev, visible: false }));
  };

  const handleConfirmAction = async () => {
    if (!confirmAction) return;
    setIsActionLoading(true);
    setActionError(null);
    try {
      if (confirmAction.type === 'disconnect') {
        await DisconnectPeer(confirmAction.peer.address);
        setSelectedPeerAddr(null);
      } else if (confirmAction.type === 'ban' && confirmAction.banDuration) {
        const addr = extractIP(confirmAction.peer.address);
        await BanPeer(addr, confirmAction.banDuration);
        setSelectedPeerAddr(null);
      }
      fetchAll();
    } catch (err) {
      setActionError(`Failed to ${confirmAction.type} peer: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
    setIsActionLoading(false);
    setConfirmAction(null);
  };

  const handleUnban = async () => {
    if (!contextMenu.bannedPeer) return;
    setActionError(null);
    try {
      await UnbanPeer(contextMenu.bannedPeer.address);
      fetchAll();
    } catch (err) {
      setActionError(`Failed to unban peer: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
    setContextMenu((prev) => ({ ...prev, visible: false }));
  };

  // Copy to clipboard context menu actions
  const handleCopyAddress = () => {
    if (contextMenu.peer) {
      CopyToClipboard(contextMenu.peer.address);
    }
    setContextMenu((prev) => ({ ...prev, visible: false }));
  };

  const handleCopyUserAgent = () => {
    if (contextMenu.peer) {
      CopyToClipboard(contextMenu.peer.userAgent);
    }
    setContextMenu((prev) => ({ ...prev, visible: false }));
  };

  // Alias actions
  const handleSetAlias = () => {
    const peer = contextMenu.peer;
    const banned = contextMenu.bannedPeer;
    const addr = peer?.address || banned?.address || '';
    const current = peer?.alias || banned?.alias || '';
    setAliasDialog({ open: true, addr, currentAlias: current });
    setAliasInput(current);
    setContextMenu((prev) => ({ ...prev, visible: false }));
  };

  const handleRemoveAlias = async () => {
    const addr = contextMenu.peer?.address || contextMenu.bannedPeer?.address || '';
    if (!addr) return;
    setContextMenu((prev) => ({ ...prev, visible: false }));
    try {
      await RemovePeerAlias(addr);
      fetchAll();
    } catch (err) {
      setActionError(`Failed to remove alias: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const handleAliasSave = async () => {
    const alias = aliasInput.trim();
    if (!aliasDialog.addr) return;
    try {
      if (alias) {
        await SetPeerAlias(aliasDialog.addr, alias);
      } else {
        await RemovePeerAlias(aliasDialog.addr);
      }
      setAliasDialog({ open: false, addr: '', currentAlias: '' });
      fetchAll();
    } catch (err) {
      setActionError(`Failed to save alias: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  // Add Peer (single)
  const handleAddPeer = async () => {
    const addr = addPeerAddr.trim();
    if (!addr) return;
    setAddPeerError(null);
    try {
      await AddPeer(addr, addPeerAlias.trim());
      setAddPeerAddr('');
      setAddPeerAlias('');
      setShowAddPeer(false);
      fetchAll();
    } catch (err) {
      setAddPeerError(err instanceof Error ? err.message : 'Failed to add peer');
    }
  };

  // Add Peers (multi-line)
  const handleAddPeersMulti = async () => {
    const input = multiLineInput.trim();
    if (!input) return;
    setIsAddingPeers(true);
    setMultiLineResults(null);
    try {
      const results = await AddPeers(input);
      setMultiLineResults(results || []);
      const anySuccess = results?.some((r: { success: boolean }) => r.success);
      if (anySuccess) {
        fetchAll();
      }
    } catch (err) {
      setMultiLineResults([{ line: '', address: '', alias: '', success: false, error: err instanceof Error ? err.message : 'Failed to add peers' }]);
    } finally {
      setIsAddingPeers(false);
    }
  };

  // Sanitize CSV field to prevent formula injection (=, +, -, @)
  const sanitizeCSV = (value: string): string => {
    const escaped = value.replace(/"/g, '""');
    if (/^[=+\-@\t\r]/.test(escaped)) {
      return `"'${escaped}"`;
    }
    return `"${escaped}"`;
  };

  // Export CSV
  const handleExportCSV = async () => {
    const headers = ['Address', 'Alias', 'Direction', 'User Agent', 'Services', 'Protocol', 'Start Height', 'Synced Headers', 'Synced Blocks', 'Synced Height', 'Ban Score', 'Connection Time', 'Last Send', 'Last Recv', 'Bytes Sent', 'Bytes Received', 'Ping (ms)', 'Time Offset', 'Whitelisted'];
    const rows = sortedPeers.map((p) => [
      sanitizeCSV(p.address),
      sanitizeCSV(p.alias || ''),
      p.inbound ? 'Inbound' : 'Outbound',
      sanitizeCSV(p.userAgent),
      sanitizeCSV(p.services),
      p.protocolVersion,
      p.startHeight,
      p.syncedHeaders,
      p.syncedBlocks,
      p.syncedHeight > 0 ? p.syncedHeight : '',
      p.banScore,
      formatTime(p.connTime),
      formatTime(p.lastSend),
      formatTime(p.lastRecv),
      p.bytesSent,
      p.bytesReceived,
      p.pingTime > 0 ? p.pingTime.toFixed(0) : '',
      p.timeOffset,
      p.whitelisted ? 'Yes' : 'No',
    ].join(','));

    const csv = [headers.join(','), ...rows].join('\n');
    try {
      await SaveCSVFile(csv, 'peers.csv', 'Export Peer List');
    } catch { /* user cancelled or error */ }
  };

  // Column configuration
  const columns: { key: SortColumn; label: string; width?: string }[] = [
    { key: 'address', label: 'Address' },
    { key: 'userAgent', label: 'User Agent', width: '200px' },
    { key: 'bytesSent', label: 'Sent' },
    { key: 'bytesReceived', label: 'Received' },
    { key: 'connTime', label: 'Connected' },
    { key: 'height', label: 'Height' },
    { key: 'ping', label: 'Ping' },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Toolbar */}
      <div style={{ display: 'flex', gap: '8px', padding: '8px 12px', borderBottom: '1px solid #444', alignItems: 'center', flexWrap: 'wrap' }}>
        {/* View toggle */}
        <button
          onClick={() => setView('connected')}
          style={{
            padding: '4px 12px', fontSize: '12px', border: '1px solid #555', borderRadius: '4px',
            backgroundColor: view === 'connected' ? '#4a9eff' : 'transparent',
            color: view === 'connected' ? '#fff' : '#aaa', cursor: 'pointer',
          }}
        >
          Connected ({peers.length})
        </button>
        <button
          onClick={() => setView('banned')}
          style={{
            padding: '4px 12px', fontSize: '12px', border: '1px solid #555', borderRadius: '4px',
            backgroundColor: view === 'banned' ? '#4a9eff' : 'transparent',
            color: view === 'banned' ? '#fff' : '#aaa', cursor: 'pointer',
          }}
        >
          Banned ({bannedPeers.length})
        </button>

        <div style={{ flex: 1 }} />

        {/* Search filter (connected view only) */}
        {view === 'connected' && (
          <input
            type="text"
            placeholder="Filter peers..."
            value={searchFilter}
            onChange={(e) => setSearchFilter(e.target.value)}
            style={{
              padding: '4px 8px', fontSize: '12px', backgroundColor: '#2a2a2a',
              border: '1px solid #555', borderRadius: '4px', color: '#ddd',
              width: '180px', outline: 'none',
            }}
          />
        )}

        {/* Add Peer button */}
        {view === 'connected' && (
          <button
            onClick={() => { setShowAddPeer(!showAddPeer); setMultiLineResults(null); }}
            style={{
              padding: '4px 10px', fontSize: '12px', border: '1px solid #555', borderRadius: '4px',
              backgroundColor: showAddPeer ? '#4a9eff' : 'transparent',
              color: showAddPeer ? '#fff' : '#aaa', cursor: 'pointer',
            }}
          >
            + Add Peer
          </button>
        )}

        {/* Export CSV */}
        {view === 'connected' && peers.length > 0 && (
          <button
            onClick={handleExportCSV}
            style={{
              padding: '4px 10px', fontSize: '12px', border: '1px solid #555', borderRadius: '4px',
              backgroundColor: 'transparent', color: '#aaa', cursor: 'pointer',
            }}
          >
            Export CSV
          </button>
        )}
      </div>

      {/* Add Peer input bar */}
      {showAddPeer && (
        <div style={{ padding: '8px 12px', borderBottom: '1px solid #444' }}>
          {/* Mode toggle */}
          <div style={{ display: 'flex', gap: '4px', marginBottom: '8px' }}>
            <button
              onClick={() => { setIsMultiLineMode(false); setMultiLineResults(null); }}
              style={{
                padding: '2px 8px', fontSize: '11px', border: '1px solid #555', borderRadius: '3px',
                backgroundColor: !isMultiLineMode ? '#4a9eff' : 'transparent',
                color: !isMultiLineMode ? '#fff' : '#888', cursor: 'pointer',
              }}
            >
              Single
            </button>
            <button
              onClick={() => { setIsMultiLineMode(true); setAddPeerError(null); }}
              style={{
                padding: '2px 8px', fontSize: '11px', border: '1px solid #555', borderRadius: '3px',
                backgroundColor: isMultiLineMode ? '#4a9eff' : 'transparent',
                color: isMultiLineMode ? '#fff' : '#888', cursor: 'pointer',
              }}
            >
              Multiple
            </button>
          </div>

          {!isMultiLineMode ? (
            /* Single peer mode */
            <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
              <span style={{ fontSize: '12px', color: '#aaa' }}>Address:</span>
              <input
                ref={addPeerInputRef}
                type="text"
                placeholder="IP or IP:Port (e.g. 1.2.3.4)"
                value={addPeerAddr}
                onChange={(e) => { setAddPeerAddr(e.target.value); setAddPeerError(null); }}
                onKeyDown={(e) => { if (e.key === 'Enter') handleAddPeer(); if (e.key === 'Escape') setShowAddPeer(false); }}
                style={{
                  flex: 1, padding: '4px 8px', fontSize: '12px', backgroundColor: '#2a2a2a',
                  border: `1px solid ${addPeerError ? '#f44' : '#555'}`, borderRadius: '4px',
                  color: '#ddd', outline: 'none',
                }}
              />
              <span style={{ fontSize: '12px', color: '#aaa' }}>Alias:</span>
              <input
                type="text"
                placeholder="Optional friendly name" maxLength={64}
                value={addPeerAlias}
                onChange={(e) => setAddPeerAlias(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') handleAddPeer(); if (e.key === 'Escape') setShowAddPeer(false); }}
                style={{
                  width: '160px', padding: '4px 8px', fontSize: '12px', backgroundColor: '#2a2a2a',
                  border: '1px solid #555', borderRadius: '4px', color: '#ddd', outline: 'none',
                }}
              />
              <button
                onClick={handleAddPeer}
                disabled={!addPeerAddr.trim()}
                style={{
                  padding: '4px 12px', fontSize: '12px', border: '1px solid #555', borderRadius: '4px',
                  backgroundColor: addPeerAddr.trim() ? '#4a9eff' : '#333',
                  color: addPeerAddr.trim() ? '#fff' : '#666', cursor: addPeerAddr.trim() ? 'pointer' : 'not-allowed',
                }}
              >
                Connect
              </button>
              {addPeerError && <span style={{ fontSize: '11px', color: '#f88' }}>{addPeerError}</span>}
            </div>
          ) : (
            /* Multi-line peer mode */
            <div>
              <textarea
                placeholder={"One peer per line: IP[:port] [alias]\ne.g.\n1.2.3.4\n5.6.7.8:37817 my-node\n9.10.11.12 backup"}
                value={multiLineInput}
                onChange={(e) => { setMultiLineInput(e.target.value); setMultiLineResults(null); }}
                onKeyDown={(e) => { if (e.key === 'Escape') setShowAddPeer(false); }}
                style={{
                  width: '100%', minHeight: '80px', padding: '6px 8px', fontSize: '12px',
                  backgroundColor: '#2a2a2a', border: '1px solid #555', borderRadius: '4px',
                  color: '#ddd', outline: 'none', fontFamily: 'monospace', resize: 'vertical',
                  boxSizing: 'border-box',
                }}
              />
              <div style={{ display: 'flex', gap: '8px', marginTop: '6px', alignItems: 'center' }}>
                <button
                  onClick={handleAddPeersMulti}
                  disabled={!multiLineInput.trim() || isAddingPeers}
                  style={{
                    padding: '4px 12px', fontSize: '12px', border: '1px solid #555', borderRadius: '4px',
                    backgroundColor: multiLineInput.trim() && !isAddingPeers ? '#4a9eff' : '#333',
                    color: multiLineInput.trim() && !isAddingPeers ? '#fff' : '#666',
                    cursor: multiLineInput.trim() && !isAddingPeers ? 'pointer' : 'not-allowed',
                  }}
                >
                  {isAddingPeers ? 'Connecting...' : 'Connect All'}
                </button>
                <span style={{ fontSize: '11px', color: '#666' }}>
                  Format: IP[:port] [alias] — port defaults to network P2P port
                </span>
              </div>

              {/* Multi-line results */}
              {multiLineResults && multiLineResults.length > 0 && (
                <div style={{ marginTop: '6px', maxHeight: '120px', overflowY: 'auto' }}>
                  {multiLineResults.map((r, i) => (
                    <div key={i} style={{ fontSize: '11px', padding: '2px 0', display: 'flex', gap: '6px', alignItems: 'center' }}>
                      <span style={{ color: r.success ? '#4caf50' : '#f44' }}>{r.success ? '\u2713' : '\u2717'}</span>
                      <span style={{ color: '#aaa', fontFamily: 'monospace' }}>{r.address || r.line}</span>
                      {r.alias && <span style={{ color: '#8cb4ff' }}>({r.alias})</span>}
                      {r.error && <span style={{ color: '#f88' }}>{r.error}</span>}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* Action error feedback */}
      {actionError && (
        <div style={{
          padding: '8px 12px', backgroundColor: '#3a1a1a', borderBottom: '1px solid #5a2a2a',
          color: '#f88', fontSize: '12px', display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        }}>
          <span>{actionError}</span>
          <button
            onClick={() => setActionError(null)}
            style={{ background: 'none', border: 'none', color: '#f88', cursor: 'pointer', fontSize: '14px', padding: '0 4px' }}
          >
            &times;
          </button>
        </div>
      )}

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* Peer list */}
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {view === 'connected' ? (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '12px' }}>
              <thead>
                <tr style={{ backgroundColor: '#333', position: 'sticky', top: 0, zIndex: 1 }}>
                  {columns.map((col) => (
                    <th
                      key={col.key}
                      onClick={() => handleSort(col.key)}
                      style={{
                        padding: '6px 8px', textAlign: 'left', color: '#aaa', fontWeight: 500,
                        borderBottom: '1px solid #444', cursor: 'pointer', userSelect: 'none',
                        whiteSpace: 'nowrap', backgroundColor: '#333',
                        maxWidth: col.width,
                      }}
                    >
                      {col.label}
                      {renderSortIndicator(col.key)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {sortedPeers.map((peer) => (
                  <tr
                    key={peer.id}
                    onClick={() => setSelectedPeerAddr(peer.address)}
                    onContextMenu={(e) => handleContextMenu(e, peer)}
                    style={{
                      cursor: 'pointer',
                      backgroundColor: selectedPeerAddr === peer.address ? '#3a3a5a' : 'transparent',
                    }}
                  >
                    <td style={{ padding: '4px 8px', color: '#ddd', whiteSpace: 'nowrap' }}>
                      <span style={{ color: peer.inbound ? '#ffaa00' : '#00ff00', marginRight: '6px', fontSize: '10px' }}>
                        {peer.inbound ? 'IN' : 'OUT'}
                      </span>
                      {peer.alias ? (
                        <><span style={{ color: '#8cb4ff' }}>{peer.alias}</span> <span style={{ color: '#777', fontSize: '11px' }}>({peer.address})</span></>
                      ) : (
                        peer.address
                      )}
                    </td>
                    <td style={{ padding: '4px 8px', color: '#aaa', maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {peer.userAgent}
                    </td>
                    <td style={{ padding: '4px 8px', color: '#aaa', whiteSpace: 'nowrap' }}>
                      {formatBytes(peer.bytesSent)}
                    </td>
                    <td style={{ padding: '4px 8px', color: '#aaa', whiteSpace: 'nowrap' }}>
                      {formatBytes(peer.bytesReceived)}
                    </td>
                    <td style={{ padding: '4px 8px', color: '#aaa', whiteSpace: 'nowrap' }}>
                      {formatDuration(peer.connTime)}
                    </td>
                    <td style={{ padding: '4px 8px', whiteSpace: 'nowrap' }}>
                      <span style={{ color: getHeightColor(peer.syncedHeight, ourHeight) }}>
                        {peer.syncedHeight > 0 ? peer.syncedHeight.toLocaleString() : '-'}
                      </span>
                    </td>
                    <td style={{ padding: '4px 8px', whiteSpace: 'nowrap' }}>
                      <span style={{ color: getPingColor(peer.pingTime) }}>
                        {formatPing(peer.pingTime)}
                      </span>
                    </td>
                  </tr>
                ))}
                {sortedPeers.length === 0 && (
                  <tr>
                    <td colSpan={7} style={{ padding: '20px', textAlign: 'center', color: '#666' }}>
                      {searchFilter ? 'No peers matching filter' : 'No connected peers'}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '12px' }}>
              <thead>
                <tr style={{ backgroundColor: '#333', position: 'sticky', top: 0, zIndex: 1 }}>
                  {['Address', 'Banned Until', 'Reason'].map((col) => (
                    <th key={col} style={{ padding: '6px 8px', textAlign: 'left', color: '#aaa', fontWeight: 500, borderBottom: '1px solid #444', backgroundColor: '#333' }}>
                      {col}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {sortedBannedPeers.map((bp) => (
                  <tr
                    key={bp.address}
                    onContextMenu={(e) => handleContextMenu(e, undefined, bp)}
                    style={{ cursor: 'context-menu' }}
                  >
                    <td style={{ padding: '4px 8px', color: '#ddd' }}>
                      {bp.alias ? (
                        <><span style={{ color: '#8cb4ff' }}>{bp.alias}</span> <span style={{ color: '#777', fontSize: '11px' }}>({bp.address})</span></>
                      ) : (
                        bp.address
                      )}
                    </td>
                    <td style={{ padding: '4px 8px', color: '#aaa' }}>{formatTime(bp.bannedUntil)}</td>
                    <td style={{ padding: '4px 8px', color: '#aaa' }}>{bp.reason}</td>
                  </tr>
                ))}
                {sortedBannedPeers.length === 0 && (
                  <tr><td colSpan={3} style={{ padding: '20px', textAlign: 'center', color: '#666' }}>No banned peers</td></tr>
                )}
              </tbody>
            </table>
          )}
        </div>

        {/* Detail panel - always visible for connected peers view */}
        {view === 'connected' && (
          <div style={{ width: '280px', overflowY: 'auto', padding: '12px', fontSize: '12px', borderLeft: '1px solid #444' }}>
            <h4 style={{ margin: '0 0 10px', color: '#fff', fontSize: '13px' }}>Peer Details</h4>
            {selectedPeer ? (
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <tbody>
                  {([
                    ['Direction', selectedPeer.inbound ? 'Inbound' : 'Outbound'],
                    ...(selectedPeer.alias ? [['Alias', selectedPeer.alias]] : []),
                    ['Address', selectedPeer.address],
                    ['User Agent', selectedPeer.userAgent],
                    ['Services', selectedPeer.services],
                    ['Protocol', String(selectedPeer.protocolVersion)],
                    ['Start Height', String(selectedPeer.startHeight)],
                    ['Synced Headers', String(selectedPeer.syncedHeaders)],
                    ['Synced Blocks', String(selectedPeer.syncedBlocks)],
                    ['Synced Height', selectedPeer.syncedHeight > 0 ? selectedPeer.syncedHeight.toLocaleString() : '-'],
                    ['Ban Score', String(selectedPeer.banScore)],
                    ['Connection Time', formatDuration(selectedPeer.connTime)],
                    ['Last Send', formatTime(selectedPeer.lastSend)],
                    ['Last Recv', formatTime(selectedPeer.lastRecv)],
                    ['Bytes Sent', formatBytes(selectedPeer.bytesSent)],
                    ['Bytes Recv', formatBytes(selectedPeer.bytesReceived)],
                    ['Ping Time', formatPing(selectedPeer.pingTime)],
                    ['Ping Wait', formatPing(selectedPeer.pingWait)],
                    ['Time Offset', `${selectedPeer.timeOffset}s`],
                    ['Whitelisted', selectedPeer.whitelisted ? 'Yes' : 'No'],
                  ] as [string, string][]).map(([label, value]) => (
                    <tr key={label}>
                      <td style={{ padding: '3px 6px 3px 0', color: '#888', whiteSpace: 'nowrap', verticalAlign: 'top' }}>{label}:</td>
                      <td style={{ padding: '3px 0', color: '#ddd', wordBreak: 'break-all' }}>{value}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div style={{ color: '#555', fontSize: '12px', marginTop: '8px' }}>
                Select a peer to view details
              </div>
            )}
          </div>
        )}
      </div>

      {/* Context menu */}
      {contextMenu.visible && (
        <div
          ref={menuRef}
          style={{
            position: 'fixed', left: contextMenu.x, top: contextMenu.y,
            backgroundColor: '#333', border: '1px solid #555', borderRadius: '4px',
            zIndex: 2000, minWidth: '170px', boxShadow: '0 2px 8px rgba(0,0,0,0.5)',
          }}
          role="menu"
        >
          {contextMenu.peer && (
            <>
              {/* Copy actions */}
              <div
                onClick={handleCopyAddress}
                style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
              >
                Copy Address
              </div>
              <div
                onClick={handleCopyUserAgent}
                style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
              >
                Copy User Agent
              </div>
              <div style={{ height: '1px', backgroundColor: '#555', margin: '2px 0' }} />
              {/* Alias actions */}
              <div
                onClick={handleSetAlias}
                style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
              >
                {contextMenu.peer?.alias ? 'Edit Alias' : 'Set Alias'}
              </div>
              {contextMenu.peer?.alias && (
                <div
                  onClick={handleRemoveAlias}
                  style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                  onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                  onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
                >
                  Remove Alias
                </div>
              )}
              <div style={{ height: '1px', backgroundColor: '#555', margin: '2px 0' }} />
              {/* Disconnect */}
              <div
                onClick={handleDisconnectRequest}
                style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
              >
                Disconnect
              </div>
              <div style={{ height: '1px', backgroundColor: '#555', margin: '2px 0' }} />
              {/* Ban durations */}
              {(['1h', '1d', '1w', '1y'] as BanDuration[]).map((dur) => (
                <div
                  key={dur}
                  onClick={() => handleBanRequest(dur)}
                  style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                  onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                  onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
                >
                  Ban for {banDurationLabels[dur]}
                </div>
              ))}
            </>
          )}
          {contextMenu.bannedPeer && (
            <>
              <div
                onClick={handleSetAlias}
                style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
              >
                {contextMenu.bannedPeer?.alias ? 'Edit Alias' : 'Set Alias'}
              </div>
              {contextMenu.bannedPeer?.alias && (
                <div
                  onClick={handleRemoveAlias}
                  style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                  onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                  onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
                >
                  Remove Alias
                </div>
              )}
              <div style={{ height: '1px', backgroundColor: '#555', margin: '2px 0' }} />
              <div
                onClick={handleUnban}
                style={{ padding: '6px 12px', cursor: 'pointer', color: '#ddd', fontSize: '12px' }}
                onMouseEnter={(e) => { (e.target as HTMLElement).style.backgroundColor = '#4a9eff'; }}
                onMouseLeave={(e) => { (e.target as HTMLElement).style.backgroundColor = 'transparent'; }}
              >
                Unban
              </div>
            </>
          )}
        </div>
      )}

      {/* Confirmation dialog for destructive actions */}
      <SimpleConfirmDialog
        isOpen={confirmAction !== null}
        title={confirmAction?.type === 'disconnect' ? 'Disconnect Peer' : 'Ban Peer'}
        message={
          confirmAction?.type === 'disconnect'
            ? `Are you sure you want to disconnect peer ${confirmAction?.peer.address}?`
            : `Are you sure you want to ban peer ${confirmAction?.peer.address} for ${confirmAction?.banDuration ? banDurationLabels[confirmAction.banDuration] : ''}?`
        }
        confirmText={confirmAction?.type === 'disconnect' ? 'Disconnect' : 'Ban'}
        cancelText="Cancel"
        onConfirm={handleConfirmAction}
        onCancel={() => setConfirmAction(null)}
        isDestructive={true}
        isLoading={isActionLoading}
      />

      {/* Alias edit dialog */}
      {aliasDialog.open && (
        <div style={{
          position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.5)',
          display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 3000,
        }}>
          <div style={{
            backgroundColor: '#333', border: '1px solid #555', borderRadius: '8px',
            padding: '20px', minWidth: '340px', boxShadow: '0 4px 16px rgba(0,0,0,0.5)',
          }}>
            <h4 style={{ margin: '0 0 12px', color: '#fff', fontSize: '14px' }}>
              {aliasDialog.currentAlias ? 'Edit Peer Alias' : 'Set Peer Alias'}
            </h4>
            <div style={{ fontSize: '12px', color: '#aaa', marginBottom: '12px', wordBreak: 'break-all' }}>
              {aliasDialog.addr}
            </div>
            <input
              ref={aliasInputRef}
              type="text"
              placeholder="Enter alias (leave empty to remove)" maxLength={64}
              value={aliasInput}
              onChange={(e) => setAliasInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleAliasSave();
                if (e.key === 'Escape') setAliasDialog({ open: false, addr: '', currentAlias: '' });
              }}
              style={{
                width: '100%', padding: '6px 10px', fontSize: '13px', backgroundColor: '#2a2a2a',
                border: '1px solid #555', borderRadius: '4px', color: '#ddd', outline: 'none',
                boxSizing: 'border-box',
              }}
            />
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '16px' }}>
              <button
                onClick={() => setAliasDialog({ open: false, addr: '', currentAlias: '' })}
                style={{
                  padding: '6px 16px', fontSize: '12px', border: '1px solid #555', borderRadius: '4px',
                  backgroundColor: 'transparent', color: '#aaa', cursor: 'pointer',
                }}
              >
                Cancel
              </button>
              <button
                onClick={handleAliasSave}
                style={{
                  padding: '6px 16px', fontSize: '12px', border: '1px solid #555', borderRadius: '4px',
                  backgroundColor: '#4a9eff', color: '#fff', cursor: 'pointer',
                }}
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
