import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { NetworkMasternode, NetworkMasternodeFilters } from '@/shared/types/masternode.types';
import { formatActiveTime, formatActiveSinceUTC, formatTimeAgo, formatDateUTC, getStatusColor } from './MasternodesTable';

// Detect IPv4 vs IPv6 from addr string (format: "IP:Port" or "[IPv6]:Port")
const getNetworkType = (addr: string): string => {
  if (!addr) return '—';
  return addr.includes('[') ? 'IPv6' : 'IPv4';
};

// Column configuration for network masternodes table
const COLUMN_CONFIG = {
  rank: { width: 80, labelKey: 'table.payRank' },
  network: { width: 80, labelKey: 'table.network' },
  version: { width: 70, labelKey: 'table.protocol' },
  status: { width: 80, labelKey: 'table.status' },
  tier: { width: 80, labelKey: 'table.tier' },
  paymentaddress: { width: 'auto', labelKey: 'table.walletAddress' }, // Stretches to fill
  activetime: { width: 100, labelKey: 'table.activeTime' },
  lastseen: { width: 160, labelKey: 'table.lastSeen' },
  lastpaid: { width: 160, labelKey: 'table.lastPaid' },
} as const;

export type NetworkSortColumn = keyof typeof COLUMN_CONFIG;

// Format tier name with capitalization
const formatTierName = (tier: string): string => {
  if (!tier) return 'Unknown';
  return tier.charAt(0).toUpperCase() + tier.slice(1).toLowerCase();
};

// Validate network masternode has required fields for rendering
const isValidNetworkMasternode = (mn: NetworkMasternode): boolean => {
  return !!(
    mn &&
    typeof mn.rank === 'number' &&
    typeof mn.addr === 'string' &&
    typeof mn.status === 'string'
  );
};

export interface NetworkMasternodesTableProps {
  masternodes: NetworkMasternode[];
  isLoading: boolean;
  hasLoaded: boolean; // True after first successful data load
  filters: NetworkMasternodeFilters;
  onSort: (column: NetworkSortColumn) => void;
}

export const NetworkMasternodesTable: React.FC<NetworkMasternodesTableProps> = React.memo(({
  masternodes,
  isLoading,
  hasLoaded,
  filters,
  onSort,
}) => {
  const { t } = useTranslation('masternode');

  // Sort masternodes based on current sort state, filtering out invalid entries
  const sortedMasternodes = useMemo(() => {
    // Filter out invalid masternodes first
    const validMasternodes = masternodes.filter(isValidNetworkMasternode);

    const sorted = [...validMasternodes].sort((a, b) => {
      let aValue: string | number;
      let bValue: string | number;

      const sortColumn = filters.sortColumn as NetworkSortColumn;

      switch (sortColumn) {
        case 'rank':
          aValue = a.rank;
          bValue = b.rank;
          break;
        case 'network':
          aValue = getNetworkType(a.addr);
          bValue = getNetworkType(b.addr);
          break;
        case 'version':
          aValue = a.version;
          bValue = b.version;
          break;
        case 'status':
          aValue = a.status;
          bValue = b.status;
          break;
        case 'tier':
          aValue = a.tier.toLowerCase();
          bValue = b.tier.toLowerCase();
          break;
        case 'paymentaddress':
          aValue = a.paymentaddress.toLowerCase();
          bValue = b.paymentaddress.toLowerCase();
          break;
        case 'activetime':
          aValue = a.activetime;
          bValue = b.activetime;
          break;
        case 'lastseen':
          aValue = a.lastseen ? new Date(a.lastseen).getTime() || 0 : 0;
          bValue = b.lastseen ? new Date(b.lastseen).getTime() || 0 : 0;
          break;
        case 'lastpaid':
          aValue = a.lastpaid ? new Date(a.lastpaid).getTime() || 0 : 0;
          bValue = b.lastpaid ? new Date(b.lastpaid).getTime() || 0 : 0;
          break;
        default:
          return 0;
      }

      if (aValue < bValue) return filters.sortDirection === 'asc' ? -1 : 1;
      if (aValue > bValue) return filters.sortDirection === 'asc' ? 1 : -1;
      return 0;
    });

    return sorted;
  }, [masternodes, filters.sortColumn, filters.sortDirection]);

  // Render sort indicator
  const renderSortIndicator = (column: NetworkSortColumn) => {
    if (filters.sortColumn !== column) return null;
    return (
      <span style={{ marginLeft: '4px', fontSize: '10px' }}>
        {filters.sortDirection === 'asc' ? '▲' : '▼'}
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
      minWidth: '1000px' // Minimum width to ensure columns fit
    }}>
      <table style={{
        width: '100%',
        borderCollapse: 'collapse',
        fontSize: '11px',
        tableLayout: 'fixed'
      }}>
        <colgroup>
          <col style={{ width: `${COLUMN_CONFIG.rank.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.network.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.version.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.status.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.tier.width}px` }} />
          <col /> {/* Wallet Address stretches to fill */}
          <col style={{ width: `${COLUMN_CONFIG.activetime.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.lastseen.width}px` }} />
          <col style={{ width: `${COLUMN_CONFIG.lastpaid.width}px` }} />
        </colgroup>
        <thead style={{
          position: 'sticky',
          top: 0,
          zIndex: 1
        }}>
          <tr style={{ backgroundColor: '#3a3a3a' }}>
            {Object.entries(COLUMN_CONFIG).map(([key, config]) => (
              <th
                key={key}
                onClick={() => onSort(key as NetworkSortColumn)}
                style={{
                  padding: '8px 6px',
                  textAlign: 'left',
                  fontWeight: 'bold',
                  color: '#ffffff',
                  borderBottom: '1px solid #4a4a4a',
                  cursor: 'pointer',
                  userSelect: 'none',
                  whiteSpace: 'nowrap',
                  backgroundColor: '#3a3a3a' // Needed for sticky header to cover content
                }}
              >
                {t(config.labelKey)}
                {renderSortIndicator(key as NetworkSortColumn)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {/* Only show loading on first load - prevents blink during refresh */}
          {isLoading && !hasLoaded ? (
            <tr>
              <td colSpan={9} style={{ padding: '20px', textAlign: 'center', color: '#999' }}>
                {t('common:loading.masternodes')}
              </td>
            </tr>
          ) : sortedMasternodes.length === 0 ? (
            <tr>
              <td colSpan={9} style={{ padding: '20px', textAlign: 'center', color: '#999' }}>
                {t('network.noResults')}
              </td>
            </tr>
          ) : (
            sortedMasternodes.map((mn) => (
              <tr
                key={`${mn.txhash}:${mn.outidx}`}
                style={{
                  backgroundColor: 'transparent',
                  transition: 'background-color 0.15s'
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.backgroundColor = '#3a3a3a';
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.backgroundColor = 'transparent';
                }}
              >
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  textAlign: 'center'
                }}>
                  {mn.rank}
                </td>
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  textAlign: 'center'
                }}>
                  {getNetworkType(mn.addr)}
                </td>
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  textAlign: 'center'
                }}>
                  {mn.version}
                </td>
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  color: getStatusColor(mn.status as any),
                  fontWeight: 'bold'
                }}>
                  {mn.status}
                </td>
                <td style={{
                  padding: '6px',
                  borderBottom: '1px solid #333',
                  fontWeight: '500'
                }}>
                  {formatTierName(mn.tier)}
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
                  {mn.paymentaddress}
                </td>
                <td
                  title={formatActiveSinceUTC(mn.activetime)}
                  style={{
                    padding: '6px',
                    borderBottom: '1px solid #333'
                  }}
                >
                  {formatActiveTime(mn.activetime)}
                </td>
                <td
                  title={formatDateUTC(mn.lastseen)}
                  style={{
                    padding: '6px',
                    borderBottom: '1px solid #333'
                  }}
                >
                  {formatTimeAgo(mn.lastseen)}
                </td>
                <td
                  title={formatDateUTC(mn.lastpaid)}
                  style={{
                    padding: '6px',
                    borderBottom: '1px solid #333'
                  }}
                >
                  {formatTimeAgo(mn.lastpaid)}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
});
