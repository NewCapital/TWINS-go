import React from 'react';
import { useTranslation } from 'react-i18next';
import type { BlockDetail as BlockDetailType } from '@/store/slices/explorerSlice';
import { ArrowLeft, ChevronLeft, ChevronRight } from 'lucide-react';

interface BlockDetailProps {
  block: BlockDetailType | null;
  isLoading: boolean;
  onTxClick: (txid: string) => void;
  onBlockClick: (query: string) => void;
  onAddressClick: (address: string) => void;
  onBack: () => void;
}

export const BlockDetail: React.FC<BlockDetailProps> = ({
  block,
  isLoading,
  onTxClick,
  onBlockClick,
  onAddressClick,
  onBack,
}) => {
  const { t } = useTranslation('common');

  if (isLoading) {
    return (
      <div style={{ padding: '24px', textAlign: 'center', color: '#888888' }}>
        {t('explorer.loadingBlock')}
      </div>
    );
  }

  if (!block) {
    return (
      <div style={{ padding: '24px', textAlign: 'center', color: '#888888' }}>
        {t('explorer.blockNotFound')}
      </div>
    );
  }

  const formatTime = (timeStr: string) => {
    const date = new Date(timeStr);
    return date.toLocaleString();
  };

  const InfoRow = ({ label, value, mono = false, link = false, onClick }: {
    label: string;
    value: string | number;
    mono?: boolean;
    link?: boolean;
    onClick?: () => void;
  }) => (
    <div style={{
      display: 'flex',
      padding: '6px 0',
      borderBottom: '1px solid #2a2a2a',
      fontSize: '12px',
    }}>
      <div style={{ width: '160px', color: '#888888', flexShrink: 0 }}>{label}</div>
      <div
        style={{
          flex: 1,
          color: link ? '#4a9eff' : '#dddddd',
          fontFamily: mono ? 'monospace' : 'inherit',
          cursor: link ? 'pointer' : 'default',
          wordBreak: 'break-all',
        }}
        onClick={onClick}
      >
        {value}
      </div>
    </div>
  );

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '12px' }}>
        <button
          className="qt-button"
          onClick={onBack}
          style={{ padding: '4px 8px', display: 'flex', alignItems: 'center', gap: '4px' }}
        >
          <ArrowLeft size={14} />
          {t('buttons.back')}
        </button>
        <span style={{ fontSize: '14px', fontWeight: 'bold', color: '#dddddd', fontFamily: 'monospace'}}>
          Block #{block.height}
        </span>
        -
        <span style={{
          fontSize: '14px',
          color: '#dddddd',
          fontWeight: 'bold',
          fontFamily: 'monospace',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          flex: 1,
        }}>
          {block.hash}
        </span>
        {block.is_pos && (
          <span style={{
            padding: '2px 8px',
            fontSize: '10px',
            backgroundColor: '#27ae60',
            color: '#ffffff',
            borderRadius: '2px',
          }}>
            PoS
          </span>
        )}
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {/* Block Info - Horizontal Table */}
        <div style={{
          backgroundColor: '#1a1a1a',
          border: '1px solid #3a3a3a',
          borderRadius: '2px',
          padding: '12px',
          marginBottom: '12px',
        }}>
          <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#dddddd' }}>
            {t('explorer.blockInfo')}
          </div>

          {/* Horizontal stats row */}
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(5, 1fr)',
            gap: '16px',
            padding: '8px 0',
            borderBottom: '1px solid #2a2a2a',
            marginBottom: '8px',
          }}>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: '10px', color: '#888888', marginBottom: '4px' }}>{t('explorer.confirmations')}</div>
              <div style={{ fontSize: '13px', color: '#dddddd', fontWeight: 'bold' }}>{block.confirmations}</div>
            </div>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: '10px', color: '#888888', marginBottom: '4px' }}>{t('explorer.size')}</div>
              <div style={{ fontSize: '13px', color: '#dddddd', fontWeight: 'bold' }}>{block.size} B</div>
            </div>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: '10px', color: '#888888', marginBottom: '4px' }}>{t('explorer.difficulty')}</div>
              <div style={{ fontSize: '13px', color: '#dddddd', fontWeight: 'bold' }}>{parseFloat(block.difficulty.toFixed(8)).toString()}</div>
            </div>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: '10px', color: '#888888', marginBottom: '4px' }}>{t('explorer.bits')}</div>
              <div style={{ fontSize: '13px', color: '#dddddd', fontFamily: 'monospace' }}>{block.bits}</div>
            </div>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: '10px', color: '#888888', marginBottom: '4px' }}>{t('explorer.nonce')}</div>
              <div style={{ fontSize: '13px', color: '#dddddd' }}>{block.nonce}</div>
            </div>
          </div>

          {/* Time and Merkle Root */}
          <InfoRow label={t('explorer.time')} value={formatTime(block.time)} />
          <InfoRow label={t('explorer.merkleRoot')} value={block.merkleroot} mono />
        </div>

        {/* Navigation */}
        <div style={{ display: 'flex', gap: '8px', marginBottom: '12px' }}>
          {block.previousblockhash && (
            <button
              className="qt-button"
              onClick={() => onBlockClick(block.previousblockhash)}
              style={{ padding: '4px 12px', fontSize: '11px', display: 'flex', alignItems: 'center', gap: '4px' }}
            >
              <ChevronLeft size={12} />
              {t('explorer.previousBlock')}
            </button>
          )}
          {block.nextblockhash && (
            <button
              className="qt-button"
              onClick={() => onBlockClick(block.nextblockhash)}
              style={{ padding: '4px 12px', fontSize: '11px', display: 'flex', alignItems: 'center', gap: '4px' }}
            >
              {t('explorer.nextBlock')}
              <ChevronRight size={12} />
            </button>
          )}
        </div>

        {/* PoS Rewards (if applicable) */}
        {block.is_pos && (
          <div style={{
            backgroundColor: '#1a1a1a',
            border: '1px solid #3a3a3a',
            borderRadius: '2px',
            padding: '12px',
            marginBottom: '12px',
          }}>
            <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#27ae60' }}>
              {t('explorer.stakingRewards')}
            </div>
            <InfoRow label={t('explorer.totalReward')} value={`${block.total_reward.toFixed(2)} TWINS`} />
            <InfoRow label={t('explorer.stakeReward')} value={`${block.stake_reward.toFixed(2)} TWINS`} />
            <InfoRow label={t('explorer.masternodeReward')} value={`${block.masternode_reward.toFixed(2)} TWINS`} />
            {block.staker_address && (
              <InfoRow label={t('explorer.stakerAddress')} value={block.staker_address} mono link onClick={() => onAddressClick(block.staker_address)} />
            )}
            {block.masternode_address && (
              <InfoRow label={t('explorer.masternodeAddress')} value={block.masternode_address} mono link onClick={() => onAddressClick(block.masternode_address)} />
            )}
          </div>
        )}

        {/* Transactions */}
        <div style={{
          backgroundColor: '#1a1a1a',
          border: '1px solid #3a3a3a',
          borderRadius: '2px',
          padding: '12px',
        }}>
          <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#dddddd' }}>
            {t('explorer.transactions')} ({block.txids?.length || 0})
          </div>
          {block.txids && block.txids.length > 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
              {block.txids.map((txid, index) => (
                <div
                  key={txid}
                  onClick={() => onTxClick(txid)}
                  style={{
                    padding: '6px 8px',
                    backgroundColor: '#252525',
                    borderRadius: '2px',
                    fontSize: '11px',
                    fontFamily: 'monospace',
                    color: '#4a9eff',
                    cursor: 'pointer',
                    wordBreak: 'break-all',
                    transition: 'background-color 0.15s',
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.backgroundColor = '#303030';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.backgroundColor = '#252525';
                  }}
                >
                  {index + 1}. {txid}
                </div>
              ))}
            </div>
          ) : (
            <div style={{ color: '#888888', fontSize: '12px' }}>{t('explorer.noTransactions')}</div>
          )}
        </div>
      </div>
    </div>
  );
};
