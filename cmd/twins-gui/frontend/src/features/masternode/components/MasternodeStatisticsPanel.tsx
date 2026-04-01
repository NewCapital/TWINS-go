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
        backgroundColor: '#2a2a2a',
        border: '1px solid #3a3a3a',
        borderRadius: '4px',
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
      backgroundColor: '#2a2a2a',
      border: '1px solid #3a3a3a',
      borderRadius: '4px',
    }}>
      {/* Header */}
      <div style={{
        fontSize: '13px',
        fontWeight: 'bold',
        color: '#ffffff',
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
          <div style={{
            fontSize: '11px',
            color: '#888',
            marginBottom: '8px',
            textTransform: 'uppercase',
          }}>
            {t('statistics.tierDistribution')}
          </div>

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
                    color: '#ccc',
                  }}>
                    {count.toLocaleString()} ({percentage.toFixed(1)}%)
                  </span>
                </div>

                {/* Progress bar */}
                <div style={{
                  height: '6px',
                  backgroundColor: '#1a1a1a',
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
          <div style={{
            fontSize: '11px',
            color: '#888',
            marginBottom: '8px',
            textTransform: 'uppercase',
          }}>
            {t('statistics.networkSummary')}
          </div>

          {/* Total / Enabled */}
          <div style={{
            display: 'grid',
            gridTemplateColumns: '1fr 1fr',
            gap: '8px',
            marginBottom: '12px',
          }}>
            <div style={{
              backgroundColor: '#1a1a1a',
              padding: '8px',
              borderRadius: '4px',
              textAlign: 'center',
            }}>
              <div style={{
                fontSize: '18px',
                fontWeight: 'bold',
                color: '#4a8af4',
              }}>
                {formatNumber(totalCount)}
              </div>
              <div style={{
                fontSize: '10px',
                color: '#888',
                textTransform: 'uppercase',
              }}>
                {t('statistics.total')}
              </div>
            </div>

            <div style={{
              backgroundColor: '#1a1a1a',
              padding: '8px',
              borderRadius: '4px',
              textAlign: 'center',
            }}>
              <div style={{
                fontSize: '18px',
                fontWeight: 'bold',
                color: '#00cc66',
              }}>
                {formatNumber(enabledCount)}
              </div>
              <div style={{
                fontSize: '10px',
                color: '#888',
                textTransform: 'uppercase',
              }}>
                {t('statistics.enabled')}
              </div>
            </div>
          </div>

          {/* Total Collateral */}
          <div style={{
            backgroundColor: '#1a1a1a',
            padding: '8px',
            borderRadius: '4px',
          }}>
            <div style={{
              fontSize: '10px',
              color: '#888',
              textTransform: 'uppercase',
              marginBottom: '4px',
            }}>
              {t('statistics.totalCollateral')}
            </div>
            <div style={{
              fontSize: '14px',
              fontWeight: 'bold',
              color: '#ffd700',
            }}>
              {formatCollateral(statistics.totalCollateral || 0)} TWINS
            </div>
            <div style={{
              fontSize: '10px',
              color: '#666',
            }}>
              ({formatNumber(statistics.totalCollateral || 0)})
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default MasternodeStatisticsPanel;
