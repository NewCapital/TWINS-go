import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Masternode, MasternodeStatus } from '@/shared/types/masternode.types';

// Column configuration from Qt masternodelist.cpp:36-49
// Labels are translation keys - resolved at render time
const COLUMN_CONFIG = {
  alias: { width: 100, labelKey: 'table.alias' },
  address: { width: 200, labelKey: 'table.address' },
  protocol: { width: 60, labelKey: 'table.protocol' },
  status: { width: 80, labelKey: 'table.status' },
  active: { width: 130, labelKey: 'table.activeTime' },
  lastSeen: { width: 130, labelKey: 'table.lastSeen' },
  collateralAddress: { width: 'auto', labelKey: 'table.collateralAddress' }, // Stretches to fill
} as const;

export type SortColumn = keyof typeof COLUMN_CONFIG;
export type SortDirection = 'asc' | 'desc';

// Format active time (seconds) to human-readable format (e.g., "2d 5h 30m")
export const formatActiveTime = (seconds: number): string => {
  if (seconds <= 0) return '0s';

  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;

  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (secs > 0 && days === 0) parts.push(`${secs}s`); // Only show seconds if less than a day

  return parts.join(' ') || '0s';
};

// Parse and validate a date input, returning a valid Date or null
const parseDate = (date: Date | string | number | null | undefined): Date | null => {
  if (date === null || date === undefined) return null;
  if (typeof date === 'string' && date.trim() === '') return null;
  if (typeof date === 'number' && (isNaN(date) || !isFinite(date))) return null;

  const d = date instanceof Date ? date : new Date(date);
  if (!d || isNaN(d.getTime())) return null;

  // Detect zero/epoch dates: Go zero time (year 1) or Unix epoch (year 1970)
  if (d.getUTCFullYear() <= 1970) return null;

  return d;
};

// Format date as relative time (e.g., "5 minutes ago")
export const formatTimeAgo = (date: Date | string | number | null | undefined): string => {
  const d = parseDate(date);
  if (!d) return 'N/A';

  const now = Date.now();
  const diffSec = Math.floor((now - d.getTime()) / 1000);

  if (diffSec < 0) return 'just now';
  if (diffSec < 60) return 'just now';
  if (diffSec < 3600) {
    const mins = Math.floor(diffSec / 60);
    return `${mins} minute${mins !== 1 ? 's' : ''} ago`;
  }
  if (diffSec < 86400) {
    const hours = Math.floor(diffSec / 3600);
    return `${hours} hour${hours !== 1 ? 's' : ''} ago`;
  }
  if (diffSec < 2592000) { // 30 days
    const days = Math.floor(diffSec / 86400);
    return `${days} day${days !== 1 ? 's' : ''} ago`;
  }
  const months = Math.floor(diffSec / 2592000);
  return `${months} month${months !== 1 ? 's' : ''} ago`;
};

// Format date as UTC string for tooltip display
export const formatDateUTC = (date: Date | string | number | null | undefined): string => {
  const d = parseDate(date);
  if (!d) return '';
  return d.toISOString().replace('T', ' ').slice(0, 19) + ' UTC';
};

// Format active time (seconds) as "YYYY-MM-DD HH:MM:SS UTC" for tooltip
export const formatActiveSinceUTC = (seconds: number): string => {
  if (seconds <= 0) return '';
  const activeSince = new Date(Date.now() - seconds * 1000);
  return activeSince.toISOString().replace('T', ' ').slice(0, 19) + ' UTC';
};

// Format last seen/paid date as relative time (backward compatible alias)
export const formatLastSeen = formatTimeAgo;

// Get status color based on masternode status
export const getStatusColor = (status: MasternodeStatus): string => {
  switch (status) {
    case 'ENABLED':
      return '#00ff00'; // Green
    case 'PRE_ENABLED':
      return '#ffff00'; // Yellow
    case 'MISSING':
    case 'EXPIRED':
    case 'VIN_SPENT':
    case 'REMOVE':
      return '#ff6666'; // Red
    case 'NEW_START_REQUIRED':
    case 'UPDATE_REQUIRED':
      return '#ffaa00'; // Orange
    default:
      return '#999999'; // Gray
  }
};

// Validate masternode has required fields for rendering
const isValidMasternode = (mn: Masternode): boolean => {
  return !!(
    mn &&
    typeof mn.id === 'string' &&
    typeof mn.alias === 'string' &&
    typeof mn.address === 'string' &&
    typeof mn.status === 'string'
  );
};

export interface MasternodesTableProps {
  masternodes: Masternode[];
  selectedMasternode: Masternode | null;
  isLoading: boolean;
  sortColumn: SortColumn;
  sortDirection: SortDirection;
  onSort: (column: SortColumn) => void;
  onRowClick: (masternode: Masternode) => void;
  onContextMenu: (e: React.MouseEvent, masternode: Masternode) => void;
}

export const MasternodesTable: React.FC<MasternodesTableProps> = React.memo(({
  masternodes,
  selectedMasternode,
  isLoading,
  sortColumn,
  sortDirection,
  onSort,
  onRowClick,
  onContextMenu,
}) => {
  const { t } = useTranslation('masternode');
  // Sort masternodes based on current sort state, filtering out invalid entries
  const sortedMasternodes = useMemo(() => {
    // Filter out invalid masternodes first
    const validMasternodes = masternodes.filter(isValidMasternode);

    const sorted = [...validMasternodes].sort((a, b) => {
      let aValue: string | number | Date;
      let bValue: string | number | Date;

      switch (sortColumn) {
        case 'alias':
          aValue = a.alias.toLowerCase();
          bValue = b.alias.toLowerCase();
          break;
        case 'address':
          aValue = a.address.toLowerCase();
          bValue = b.address.toLowerCase();
          break;
        case 'protocol':
          aValue = a.protocol;
          bValue = b.protocol;
          break;
        case 'status':
          aValue = a.status;
          bValue = b.status;
          break;
        case 'active':
          aValue = a.activeTime;
          bValue = b.activeTime;
          break;
        case 'lastSeen':
          aValue = new Date(a.lastSeen).getTime();
          bValue = new Date(b.lastSeen).getTime();
          break;
        case 'collateralAddress':
          aValue = a.collateralAddress.toLowerCase();
          bValue = b.collateralAddress.toLowerCase();
          break;
        default:
          return 0;
      }

      if (aValue < bValue) return sortDirection === 'asc' ? -1 : 1;
      if (aValue > bValue) return sortDirection === 'asc' ? 1 : -1;
      return 0;
    });

    return sorted;
  }, [masternodes, sortColumn, sortDirection]);

  // Render sort indicator
  const renderSortIndicator = (column: SortColumn) => {
    if (sortColumn !== column) return null;
    return (
      <span style={{ marginLeft: '4px', fontSize: '10px' }}>
        {sortDirection === 'asc' ? '▲' : '▼'}
      </span>
    );
  };

  return (
    <div style={{
      flex: 1,
      overflow: 'auto',
      border: '1px solid #4a4a4a',
      borderRadius: '2px',
      backgroundColor: '#2b2b2b',
      minWidth: '700px' // Minimum width from Qt (695px) to ensure columns fit
    }}>
      <table style={{
        width: '100%',
        borderCollapse: 'collapse',
        fontSize: '11px',
        tableLayout: 'fixed'
      }}>
        <colgroup>
          <col style={{ width: `${COLUMN_CONFIG.alias.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.address.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.protocol.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.status.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.active.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.lastSeen.width}px` }} />
          <col /> {/* Collateral Address stretches */}
        </colgroup>
        <thead>
          <tr style={{ backgroundColor: '#3a3a3a' }}>
            {Object.entries(COLUMN_CONFIG).map(([key, config]) => (
              <th
                key={key}
                onClick={() => onSort(key as SortColumn)}
                style={{
                  padding: '8px 6px',
                  textAlign: 'left',
                  fontWeight: 'bold',
                  color: '#ffffff',
                  borderBottom: '1px solid #4a4a4a',
                  cursor: 'pointer',
                  userSelect: 'none',
                  whiteSpace: 'nowrap'
                }}
              >
                {t(config.labelKey)}
                {renderSortIndicator(key as SortColumn)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {isLoading ? (
            <tr>
              <td colSpan={7} style={{ padding: '20px', textAlign: 'center', color: '#999' }}>
                {t('common:loading.masternodes')}
              </td>
            </tr>
          ) : sortedMasternodes.length === 0 ? (
            <tr>
              <td colSpan={7} style={{ padding: '20px', textAlign: 'center', color: '#999' }}>
                {t('noMasternodes')}
              </td>
            </tr>
          ) : (
            sortedMasternodes.map((mn) => (
              <tr
                key={mn.id}
                onClick={() => onRowClick(mn)}
                onContextMenu={(e) => onContextMenu(e, mn)}
                style={{
                  backgroundColor: selectedMasternode?.id === mn.id ? '#4a6a8a' : 'transparent',
                  cursor: 'pointer',
                  transition: 'background-color 0.15s'
                }}
                onMouseEnter={(e) => {
                  if (selectedMasternode?.id !== mn.id) {
                    e.currentTarget.style.backgroundColor = '#3a3a3a';
                  }
                }}
                onMouseLeave={(e) => {
                  if (selectedMasternode?.id !== mn.id) {
                    e.currentTarget.style.backgroundColor = 'transparent';
                  }
                }}
              >
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap'
                }}>
                  {mn.alias}
                </td>
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap'
                }}>
                  {mn.address}
                </td>
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  textAlign: 'center'
                }}>
                  {mn.protocol}
                </td>
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  color: getStatusColor(mn.status),
                  fontWeight: 'bold'
                }}>
                  {mn.status}
                </td>
                <td
                  title={formatActiveSinceUTC(mn.activeTime)}
                  style={{
                    padding: '6px',
                    borderBottom: '1px solid #333'
                  }}
                >
                  {formatActiveTime(mn.activeTime)}
                </td>
                <td
                  title={formatDateUTC(mn.lastSeen)}
                  style={{
                    padding: '6px',
                    borderBottom: '1px solid #333'
                  }}
                >
                  {formatTimeAgo(mn.lastSeen)}
                </td>
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  fontFamily: 'monospace',
                  fontSize: '10px'
                }}>
                  {mn.collateralAddress}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
});
