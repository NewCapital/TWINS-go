import React from 'react';
import { useTranslation } from 'react-i18next';
import { MasternodeStatistics } from '@/shared/types/masternode.types';

// Tier configuration for display
const TIER_CONFIG = {
  platinum: { name: 'Platinum', color: '#e5e4e2', collateral: 100_000_000 },
  gold: { name: 'Gold', color: '#ffd700', collateral: 20_000_000 },
  silver: { name: 'Silver', color: '#c0c0c0', collateral: 5_000_000 },
  bronze: { name: 'Bronze', color: '#cd7f32', collateral: 1_000_000 },
} as const;

// Order for display (highest to lowest)
const TIER_ORDER = ['platinum', 'gold', 'silver', 'bronze'] as const;

interface MasternodeStatisticsPanelProps {
  statistics: MasternodeStatistics | null;
  isLoading: boolean;
}

// Format large numbers with K/M/B suffixes
const formatNumber = (num: number): string => {
  if (num >= 1_000_000_000) {
    return (num / 1_000_000_000).toFixed(1) + 'B';
  }
  if (num >= 1_000_000) {
    return (num / 1_000_000).toFixed(1) + 'M';
  }
  if (num >= 1_000) {
    return (num / 1_000).toFixed(1) + 'K';
  }
  return num.toString();
};

// Format collateral with commas
const formatCollateral = (num: number): string => {
  return num.toLocaleString();
};

export const MasternodeStatisticsPanel: React.FC<MasternodeStatisticsPanelProps> = ({
  statistics,
  isLoading,
}) => {
  const { t } = useTranslation('masternode');

  // Loading skeleton
  if (isLoading && !statistics) {
    return (
      <div style={{
        padding: '12px',
        marginBottom: '12px',
        backgroundColor: '#2f2f2f',
        border: '1px solid #3a3a3a',
        borderRadius: '8px',
      }}>
        <div style={{ color: '#888', fontSize: '12px' }}>
          {t('statistics.loading')}
        </div>
      </div>
    );
  }

  if (!statistics) {
    return null;
  }

  const totalCount = statistics.totalCount || 0;
  const enabledCount = statistics.enabledCount || 0;

  return (
    <div style={{
      padding: '12px',
      marginBottom: '12px',
      backgroundColor: '#2f2f2f',
      border: '1px solid #3a3a3a',
      borderRadius: '8px',
    }}>
      {/* Header */}
      <div style={{
        fontSize: '13px',
        fontWeight: 600,
        color: '#ccc',
        marginBottom: '12px',
        borderBottom: '1px solid #3a3a3a',
        paddingBottom: '8px',
      }}>
        {t('statistics.title')}
      </div>

      {/* Statistics Grid */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: '1fr 1fr',
        gap: '16px',
      }}>
        {/* Left: Tier Distribution */}
        <div>
          {/* Tier bars */}
          {TIER_ORDER.map((tier) => {
            const count = statistics.tierCounts[tier] || 0;
            const percentage = statistics.tierPercentages[tier] || 0;
            const config = TIER_CONFIG[tier];

            return (
              <div key={tier} style={{ marginBottom: '8px' }}>
                {/* Tier label and count */}
                <div style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  marginBottom: '2px',
                }}>
                  <span style={{
                    fontSize: '11px',
                    color: config.color,
                    fontWeight: '500',
                  }}>
                    {config.name}
                  </span>
                  <span style={{
                    fontSize: '11px',
                    color: '#ddd',
                  }}>
                    {count.toLocaleString()} ({percentage.toFixed(1)}%)
                  </span>
                </div>

                {/* Progress bar */}
                <div style={{
                  height: '6px',
                  backgroundColor: '#3a3a3a',
                  borderRadius: '3px',
                  overflow: 'hidden',
                }}>
                  <div style={{
                    height: '100%',
                    width: `${Math.min(percentage, 100)}%`,
                    backgroundColor: config.color,
                    borderRadius: '3px',
                    transition: 'width 0.3s ease',
                  }} />
                </div>
              </div>
            );
          })}
        </div>

        {/* Right: Summary Stats */}
        <div>
          {/* Total / Enabled */}
          <div style={{
            display: 'grid',
            gridTemplateColumns: '1fr 1fr',
            gap: '8px',
            marginBottom: '12px',
          }}>
            <div style={{
              backgroundColor: '#2a2a2a',
              border: '1px solid #3a3a3a',
              padding: '12px',
              borderRadius: '6px',
              minHeight: '60px',
              display: 'flex',
              flexDirection: 'column',
            }}>
              <div style={{
                fontSize: '10px',
                color: '#888',
              }}>
                {t('statistics.total')}
              </div>
              <div style={{
                flex: 1,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: '18px',
                fontWeight: 600,
                fontFamily: 'monospace',
                color: '#ddd',
              }}>
                {formatNumber(totalCount)}
              </div>
            </div>

            <div style={{
              backgroundColor: '#2a2a2a',
              border: '1px solid #3a3a3a',
              padding: '12px',
              borderRadius: '6px',
              minHeight: '60px',
              display: 'flex',
              flexDirection: 'column',
            }}>
              <div style={{
                fontSize: '10px',
                color: '#888',
              }}>
                {t('statistics.enabled')}
              </div>
              <div style={{
                flex: 1,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: '18px',
                fontWeight: 600,
                fontFamily: 'monospace',
                color: '#27ae60',
              }}>
                {formatNumber(enabledCount)}
              </div>
            </div>
          </div>

          {/* Total Collateral */}
          <div style={{
            backgroundColor: '#2a2a2a',
            border: '1px solid #3a3a3a',
            padding: '12px',
            borderRadius: '6px',
            minHeight: '60px',
            display: 'flex',
            flexDirection: 'column',
          }}>
            <div style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: '8px',
            }}>
              <div style={{
                fontSize: '10px',
                color: '#888',
              }}>
                {t('statistics.totalCollateral')}
              </div>
              <div style={{
                fontSize: '10px',
                color: '#666',
              }}>
                ({formatNumber(statistics.totalCollateral || 0)})
              </div>
            </div>
            <div style={{
              flex: 1,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: '14px',
              fontWeight: 600,
              fontFamily: 'monospace',
              color: '#ffd700',
            }}>
              {formatCollateral(statistics.totalCollateral || 0)} TWINS
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default MasternodeStatisticsPanel;
