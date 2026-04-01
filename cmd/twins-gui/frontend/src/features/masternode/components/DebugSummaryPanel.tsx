import React, { useState } from 'react';

// Category colors (shared with MasternodeDebugPanel)
const CATEGORY_COLORS: Record<string, string> = {
  sync: '#4a8af4',
  broadcast: '#4caf50',
  ping: '#f0c040',
  status: '#b070d0',
  winner: '#ff9800',
  active: '#00bcd4',
  network: '#e0e0e0',
};

interface ReasonCount {
  label: string;
  count: number;
}

interface SourceCount {
  source: string;
  count: number;
}

interface StatusTransition {
  timestamp: string;
  from: string;
  to: string;
}

interface PeerDetailEntry {
  address: string;
  eventCount: number;
}

interface MasternodeDetailEntry {
  outpoint: string;
  address: string;
  tier: string;
  eventCount: number;
}

export interface DebugSummary {
  firstEvent: string;
  lastEvent: string;
  totalEvents: number;
  fileSize: number;
  sessionCount: number;
  broadcastReceived: number;
  broadcastAccepted: number;
  broadcastRejected: number;
  broadcastDedup: number;
  acceptRate: number;
  rejectReasons: ReasonCount[];
  uniqueMasternodes: number;
  tierBreakdown: Record<string, number>;
  topSources: SourceCount[];
  pingReceived: number;
  pingAccepted: number;
  pingFailed: number;
  pingAcceptRate: number;
  activePingsSent: number;
  activePingsSuccess: number;
  activePingsFailed: number;
  dsegRequests: number;
  dsegResponses: number;
  avgMNsServed: number;
  networkMNBCount: number;
  networkMNPCount: number;
  uniquePeers: number;
  syncTransitions: StatusTransition[];
  statusChanges: ReasonCount[];
  activeMNChanges: StatusTransition[];
  peerDetails: PeerDetailEntry[];
  masternodeDetails: MasternodeDetailEntry[];
}

const formatFileSize = (bytes: number): string => {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
};

const formatDate = (iso: string): string => {
  if (!iso) return '-';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${yyyy}-${mm}-${dd}`;
};

const formatTimeOnly = (iso: string): string => {
  if (!iso) return '-';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
};

const formatRate = (rate: number): string => {
  if (isNaN(rate) || rate === 0) return '0%';
  return `${rate.toFixed(1)}%`;
};

const TIER_COLORS: Record<string, string> = {
  Bronze: '#cd7f32',
  Silver: '#c0c0c0',
  Gold: '#ffd700',
  Platinum: '#e5e4e2',
};

interface DebugOverviewProps {
  summary: DebugSummary | null;
}

export const DebugOverviewPanel: React.FC<DebugOverviewProps> = ({ summary }) => {
  const [showPeersDialog, setShowPeersDialog] = useState(false);
  const [showMNsDialog, setShowMNsDialog] = useState(false);

  if (!summary) return null;

  const hasData = summary.totalEvents > 0;

  if (!hasData) {
    return (
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666', fontSize: '12px' }}>
        No events to summarize.
      </div>
    );
  }

  const hasBroadcast = summary.broadcastReceived > 0 || summary.broadcastAccepted > 0;
  const hasPing = summary.pingReceived > 0 || summary.pingAccepted > 0 || summary.activePingsSent > 0;
  const hasNetwork = summary.dsegRequests > 0 || summary.networkMNBCount > 0 || summary.uniquePeers > 0;
  const hasStatus = summary.syncTransitions.length > 0 || summary.statusChanges.length > 0 || summary.activeMNChanges.length > 0;

  return (
    <div style={{ flex: 1, overflow: 'auto', padding: '8px' }}>
      {/* Overview bar */}
      <div style={{
        display: 'flex',
        gap: '12px',
        flexWrap: 'wrap',
        marginBottom: '12px',
        padding: '8px 12px',
        backgroundColor: '#2b2b2b',
        borderRadius: '4px',
        border: '1px solid #3a3a3a',
      }}>
        <OverviewStatDateTime label="Time range" firstEvent={summary.firstEvent} lastEvent={summary.lastEvent} />
        <OverviewStat label="Total events" value={summary.totalEvents.toLocaleString()} />
        <OverviewStat label="Log size" value={formatFileSize(summary.fileSize)} />
        <OverviewStat label="Sessions" value={String(summary.sessionCount)} />
        {hasNetwork && (
          <OverviewStat
            label="Unique peers"
            value={summary.uniquePeers.toLocaleString()}
            onClick={summary.peerDetails?.length > 0 ? () => setShowPeersDialog(true) : undefined}
          />
        )}
        {hasBroadcast && (
          <OverviewStat
            label="Unique MNs"
            value={summary.uniqueMasternodes.toLocaleString()}
            onClick={summary.masternodeDetails?.length > 0 ? () => setShowMNsDialog(true) : undefined}
          />
        )}
      </div>

      {/* Dashboard cards grid */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
        gap: '10px',
      }}>
        {/* Broadcast Health card */}
        {hasBroadcast && (
          <DashboardCard title="Broadcast Health" color={CATEGORY_COLORS.broadcast}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 16px' }}>
              <StatRow label="Received" value={summary.broadcastReceived.toLocaleString()} />
              <StatRow label="Accepted" value={summary.broadcastAccepted.toLocaleString()} valueColor="#4caf50" />
              <StatRow label="Rejected" value={summary.broadcastRejected.toLocaleString()} valueColor={summary.broadcastRejected > 0 ? '#ff6666' : undefined} />
              <StatRow label="Dedup" value={summary.broadcastDedup.toLocaleString()} />
            </div>
            <div style={{ marginTop: '6px', borderTop: '1px solid #333', paddingTop: '6px' }}>
              <StatRow label="Accept rate" value={formatRate(summary.acceptRate)} valueColor={summary.acceptRate >= 90 ? '#4caf50' : summary.acceptRate >= 70 ? '#f0c040' : '#ff6666'} />
            </div>

            {Object.keys(summary.tierBreakdown).length > 0 && (
              <div style={{ marginTop: '6px', borderTop: '1px solid #333', paddingTop: '4px' }}>
                <SubLabel>Tier breakdown</SubLabel>
                <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap', marginTop: '2px' }}>
                  {Object.entries(summary.tierBreakdown)
                    .sort(([, a], [, b]) => b - a)
                    .map(([tier, count]) => (
                      <span key={tier} style={{ fontSize: '10px' }}>
                        <span style={{ color: TIER_COLORS[tier] || '#aaa' }}>{tier}</span>
                        <span style={{ color: '#ccc', fontFamily: 'monospace', marginLeft: '4px' }}>{count}</span>
                      </span>
                    ))}
                </div>
              </div>
            )}

            {summary.rejectReasons.length > 0 && (
              <div style={{ marginTop: '6px', borderTop: '1px solid #333', paddingTop: '4px' }}>
                <SubLabel>Top rejection reasons</SubLabel>
                {summary.rejectReasons.slice(0, 5).map((r, i) => (
                  <StatRow key={i} label={truncate(r.label, 40)} value={String(r.count)} valueColor="#ff6666" />
                ))}
              </div>
            )}

            {summary.topSources.length > 0 && (
              <div style={{ marginTop: '6px', borderTop: '1px solid #333', paddingTop: '4px' }}>
                <SubLabel>Top sources</SubLabel>
                {summary.topSources.map((s, i) => (
                  <StatRow key={i} label={s.source} value={String(s.count)} />
                ))}
              </div>
            )}
          </DashboardCard>
        )}

        {/* Ping Health card */}
        {hasPing && (
          <DashboardCard title="Ping Health" color={CATEGORY_COLORS.ping}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 16px' }}>
              <StatRow label="Received" value={summary.pingReceived.toLocaleString()} />
              <StatRow label="Accepted" value={summary.pingAccepted.toLocaleString()} valueColor="#4caf50" />
              <StatRow label="Failed" value={summary.pingFailed.toLocaleString()} valueColor={summary.pingFailed > 0 ? '#ff6666' : undefined} />
              <StatRow label="Accept rate" value={formatRate(summary.pingAcceptRate)} />
            </div>
            {summary.activePingsSent > 0 && (
              <div style={{ marginTop: '6px', borderTop: '1px solid #333', paddingTop: '4px' }}>
                <SubLabel>Active MN pings</SubLabel>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 16px' }}>
                  <StatRow label="Sent" value={String(summary.activePingsSent)} />
                  <StatRow label="OK" value={String(summary.activePingsSuccess)} valueColor="#4caf50" />
                  <StatRow label="Failed" value={String(summary.activePingsFailed)} valueColor={summary.activePingsFailed > 0 ? '#ff6666' : undefined} />
                </div>
              </div>
            )}
          </DashboardCard>
        )}

        {/* Network Activity card */}
        {hasNetwork && (
          <DashboardCard title="Network Activity" color={CATEGORY_COLORS.network}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 16px' }}>
              <StatRow label="MNB from network" value={summary.networkMNBCount.toLocaleString()} />
              <StatRow label="MNP from network" value={summary.networkMNPCount.toLocaleString()} />
              <StatRow label="DSEG requests" value={summary.dsegRequests.toLocaleString()} />
              <StatRow label="DSEG responses" value={summary.dsegResponses.toLocaleString()} />
            </div>
            {summary.avgMNsServed > 0 && (
              <div style={{ marginTop: '4px' }}>
                <StatRow label="Avg MNs served per DSEG" value={summary.avgMNsServed.toFixed(0)} />
              </div>
            )}
          </DashboardCard>
        )}

        {/* Status & Sync card */}
        {hasStatus && (
          <DashboardCard title="Status & Sync" color={CATEGORY_COLORS.sync}>
            {summary.syncTransitions.length > 0 && (
              <div>
                <SubLabel>Sync transitions</SubLabel>
                <div style={{ maxHeight: '150px', overflow: 'auto' }}>
                  {summary.syncTransitions.map((t, i) => (
                    <div key={i} style={{ fontSize: '10px', color: '#aaa', padding: '1px 0' }}>
                      <span style={{ color: '#666', fontFamily: 'monospace' }}>{formatTimeOnly(t.timestamp)}</span>{' '}
                      <span style={{ color: '#888' }}>{t.from}</span>
                      <span style={{ color: '#555' }}>{' → '}</span>
                      <span style={{ color: '#4a8af4' }}>{t.to}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {summary.statusChanges.length > 0 && (
              <div style={{ marginTop: summary.syncTransitions.length > 0 ? '6px' : 0, borderTop: summary.syncTransitions.length > 0 ? '1px solid #333' : 'none', paddingTop: summary.syncTransitions.length > 0 ? '4px' : 0 }}>
                <SubLabel>Status changes</SubLabel>
                {summary.statusChanges.map((s, i) => (
                  <StatRow key={i} label={truncate(s.label, 35)} value={String(s.count)} valueColor="#b070d0" />
                ))}
              </div>
            )}

            {summary.activeMNChanges.length > 0 && (
              <div style={{ marginTop: '6px', borderTop: '1px solid #333', paddingTop: '4px' }}>
                <SubLabel>Active MN changes</SubLabel>
                {summary.activeMNChanges.map((t, i) => (
                  <div key={i} style={{ fontSize: '10px', color: '#aaa', padding: '1px 0' }}>
                    <span style={{ color: '#666', fontFamily: 'monospace' }}>{formatTimeOnly(t.timestamp)}</span>{' '}
                    <span style={{ color: '#888' }}>{t.from}</span>
                    <span style={{ color: '#555' }}>{' → '}</span>
                    <span style={{ color: '#00bcd4' }}>{t.to}</span>
                  </div>
                ))}
              </div>
            )}
          </DashboardCard>
        )}
      </div>

      {/* Peer Details Dialog */}
      {showPeersDialog && (
        <DetailDialog
          title={`Unique Peers (${summary.peerDetails?.length || 0})`}
          onClose={() => setShowPeersDialog(false)}
        >
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '11px' }}>
            <thead>
              <tr style={{ backgroundColor: '#3a3a3a' }}>
                <th style={{ ...dialogThStyle, width: '40px', textAlign: 'right' }}>#</th>
                <th style={dialogThStyle}>Address</th>
                <th style={{ ...dialogThStyle, width: '100px', textAlign: 'right' }}>Events</th>
              </tr>
            </thead>
            <tbody>
              {(summary.peerDetails || []).map((p, i) => (
                <tr key={i} style={{ backgroundColor: i % 2 === 0 ? '#1e1e1e' : '#232323' }}>
                  <td style={{ ...dialogTdStyle, textAlign: 'right', color: '#666' }}>{i + 1}</td>
                  <td style={{ ...dialogTdStyle, fontFamily: 'monospace', color: '#ccc' }}>{p.address}</td>
                  <td style={{ ...dialogTdStyle, textAlign: 'right', fontFamily: 'monospace', color: '#4a8af4' }}>{p.eventCount.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </DetailDialog>
      )}

      {/* Masternode Details Dialog */}
      {showMNsDialog && (
        <DetailDialog
          title={`Unique Masternodes (${summary.masternodeDetails?.length || 0})`}
          onClose={() => setShowMNsDialog(false)}
        >
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '11px' }}>
            <thead>
              <tr style={{ backgroundColor: '#3a3a3a' }}>
                <th style={{ ...dialogThStyle, width: '40px', textAlign: 'right' }}>#</th>
                <th style={dialogThStyle}>Address</th>
                <th style={{ ...dialogThStyle, width: '80px' }}>Tier</th>
                <th style={{ ...dialogThStyle, width: '100px', textAlign: 'right' }}>Events</th>
              </tr>
            </thead>
            <tbody>
              {(summary.masternodeDetails || []).map((m, i) => (
                <tr key={i} style={{ backgroundColor: i % 2 === 0 ? '#1e1e1e' : '#232323' }}>
                  <td style={{ ...dialogTdStyle, textAlign: 'right', color: '#666' }}>{i + 1}</td>
                  <td style={{ ...dialogTdStyle, fontFamily: 'monospace', color: '#ccc' }} title={m.outpoint}>{m.address || m.outpoint}</td>
                  <td style={{ ...dialogTdStyle, color: TIER_COLORS[m.tier] || '#aaa' }}>{m.tier || '-'}</td>
                  <td style={{ ...dialogTdStyle, textAlign: 'right', fontFamily: 'monospace', color: '#4a8af4' }}>{m.eventCount.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </DetailDialog>
      )}
    </div>
  );
};

// --- Sub-components ---

const OverviewStatDateTime: React.FC<{ label: string; firstEvent: string; lastEvent: string }> = ({ label, firstEvent, lastEvent }) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: '1px' }}>
    <span style={{ fontSize: '9px', color: '#777', textTransform: 'uppercase', letterSpacing: '0.5px' }}>{label}</span>
    <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
        <span style={{ fontSize: '12px', color: '#ddd', fontFamily: 'monospace' }}>{formatTimeOnly(firstEvent)}</span>
        <span style={{ fontSize: '10px', color: '#ddd', fontFamily: 'monospace' }}>{formatDate(firstEvent)}</span>
      </div>
      <span style={{ fontSize: '12px', color: '#666' }}>—</span>
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
        <span style={{ fontSize: '12px', color: '#ddd', fontFamily: 'monospace' }}>{formatTimeOnly(lastEvent)}</span>
        <span style={{ fontSize: '10px', color: '#ddd', fontFamily: 'monospace' }}>{formatDate(lastEvent)}</span>
      </div>
    </div>
  </div>
);

const OverviewStat: React.FC<{ label: string; value: string; onClick?: () => void }> = ({ label, value, onClick }) => (
  <div
    style={{ display: 'flex', flexDirection: 'column', gap: '1px', cursor: onClick ? 'pointer' : 'default' }}
    onClick={onClick}
  >
    <span style={{ fontSize: '9px', color: '#777', textTransform: 'uppercase', letterSpacing: '0.5px' }}>{label}</span>
    <span style={{
      fontSize: '12px',
      color: onClick ? '#4a8af4' : '#ddd',
      fontFamily: 'monospace',
      textDecoration: onClick ? 'underline' : 'none',
    }}>{value}</span>
  </div>
);

const DashboardCard: React.FC<{ title: string; color: string; children: React.ReactNode }> = ({ title, color, children }) => (
  <div style={{
    backgroundColor: '#252525',
    border: '1px solid #3a3a3a',
    borderRadius: '4px',
    borderTop: `2px solid ${color}`,
    padding: '10px 12px',
  }}>
    <div style={{
      fontSize: '11px',
      fontWeight: 'bold',
      color: color,
      marginBottom: '6px',
    }}>
      {title}
    </div>
    {children}
  </div>
);

const SubLabel: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div style={{ fontSize: '10px', color: '#777', marginBottom: '2px' }}>{children}</div>
);

const StatRow: React.FC<{ label: string; value: string; valueColor?: string }> = ({ label, value, valueColor }) => (
  <div style={{
    display: 'flex',
    justifyContent: 'space-between',
    fontSize: '11px',
    padding: '1px 0',
  }}>
    <span style={{ color: '#888' }}>{label}</span>
    <span style={{ color: valueColor || '#ccc', fontFamily: 'monospace', fontSize: '10px' }}>{value}</span>
  </div>
);

const DetailDialog: React.FC<{ title: string; onClose: () => void; children: React.ReactNode }> = ({ title, onClose, children }) => {
  React.useEffect(() => {
    const handleKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [onClose]);

  return (
  <div
    style={{
      position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
      backgroundColor: 'rgba(0,0,0,0.6)', zIndex: 1000,
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }}
    onClick={onClose}
  >
    <div
      style={{
        backgroundColor: '#2b2b2b', border: '1px solid #555', borderRadius: '4px',
        width: '560px', maxHeight: '70vh', display: 'flex', flexDirection: 'column',
      }}
      onClick={(e) => e.stopPropagation()}
    >
      <div style={{
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        padding: '8px 12px', borderBottom: '1px solid #444',
      }}>
        <span style={{ fontSize: '12px', fontWeight: 'bold', color: '#ddd' }}>{title}</span>
        <button
          onClick={onClose}
          style={{
            background: 'none', border: 'none', color: '#888', fontSize: '16px',
            cursor: 'pointer', padding: '0 4px', lineHeight: 1,
          }}
        >
          ×
        </button>
      </div>
      <div style={{ overflow: 'auto', flex: 1 }}>
        {children}
      </div>
    </div>
  </div>
  );
};

const dialogThStyle: React.CSSProperties = {
  padding: '6px 8px',
  textAlign: 'left',
  color: '#aaa',
  fontWeight: 'bold',
  borderBottom: '1px solid #444',
  position: 'sticky',
  top: 0,
  backgroundColor: '#3a3a3a',
};

const dialogTdStyle: React.CSSProperties = {
  padding: '4px 8px',
  verticalAlign: 'top',
};

const truncate = (s: string, max: number): string => {
  if (s.length <= max) return s;
  return s.slice(0, max - 1) + '…';
};
