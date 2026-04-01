import React from 'react';
import { useTranslation } from 'react-i18next';
import type { ExplorerTransaction } from '@/store/slices/explorerSlice';
import { ArrowLeft, ArrowRight } from 'lucide-react';

interface TransactionDetailProps {
  transaction: ExplorerTransaction | null;
  isLoading: boolean;
  onAddressClick: (address: string) => void;
  onTxClick: (txid: string) => void;
  onBlockClick: (query: string) => void;
  onBack: () => void;
}

export const TransactionDetail: React.FC<TransactionDetailProps> = ({
  transaction,
  isLoading,
  onAddressClick,
  onTxClick,
  onBlockClick,
  onBack,
}) => {
  const { t } = useTranslation('common');

  if (isLoading) {
    return (
      <div style={{ padding: '24px', textAlign: 'center', color: '#888888' }}>
        {t('explorer.loadingTransaction')}
      </div>
    );
  }

  if (!transaction) {
    return (
      <div style={{ padding: '24px', textAlign: 'center', color: '#888888' }}>
        {t('explorer.transactionNotFound')}
      </div>
    );
  }

  const formatTime = (timeStr: string) => {
    const date = new Date(timeStr);
    return date.toLocaleString();
  };

  const formatAmount = (amount: number) => {
    return amount.toFixed(8) + ' TWINS';
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
      <div style={{ width: '140px', color: '#888888', flexShrink: 0 }}>{label}</div>
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
        <span style={{ fontSize: '14px', fontWeight: 'bold', color: '#dddddd' }}>
          {t('explorer.transactionDetails')}
        </span>
        {transaction.is_coinbase && (
          <span style={{
            padding: '2px 8px',
            fontSize: '10px',
            backgroundColor: '#f39c12',
            color: '#ffffff',
            borderRadius: '2px',
          }}>
            Coinbase
          </span>
        )}
        {transaction.is_coinstake && (
          <span style={{
            padding: '2px 8px',
            fontSize: '10px',
            backgroundColor: '#27ae60',
            color: '#ffffff',
            borderRadius: '2px',
          }}>
            Coinstake
          </span>
        )}
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {/* Transaction Info */}
        <div style={{
          backgroundColor: '#1a1a1a',
          border: '1px solid #3a3a3a',
          borderRadius: '2px',
          padding: '12px',
          marginBottom: '12px',
        }}>
          <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#dddddd' }}>
            {t('explorer.txInfo')}
          </div>
          <InfoRow label={t('explorer.txid')} value={transaction.txid} mono />
          <InfoRow
            label={t('explorer.block')}
            value={`#${transaction.block_height}`}
            link
            onClick={() => onBlockClick(transaction.block_hash)}
          />
          <InfoRow label={t('explorer.confirmations')} value={transaction.confirmations} />
          <InfoRow label={t('explorer.time')} value={formatTime(transaction.time)} />
          <InfoRow label={t('explorer.size')} value={`${transaction.size} bytes`} />
          <InfoRow label={t('explorer.fee')} value={formatAmount(transaction.fee)} />
        </div>

        {/* Inputs and Outputs */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr auto 1fr', gap: '12px' }}>
          {/* Inputs */}
          <div style={{
            backgroundColor: '#1a1a1a',
            border: '1px solid #3a3a3a',
            borderRadius: '2px',
            padding: '12px',
          }}>
            <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#dddddd' }}>
              {t('explorer.inputs')} ({transaction.inputs?.length || 0})
            </div>
            {transaction.inputs && transaction.inputs.length > 0 ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {transaction.inputs.map((input) => (
                  <div
                    key={`${input.txid}-${input.vout}`}
                    style={{
                      padding: '8px',
                      backgroundColor: '#252525',
                      borderRadius: '2px',
                      fontSize: '11px',
                    }}
                  >
                    {input.is_coinbase ? (
                      <div style={{ color: '#f39c12' }}>Coinbase (New Coins)</div>
                    ) : (
                      <>
                        <div
                          style={{
                            fontFamily: 'monospace',
                            color: input.address ? '#4a9eff' : '#888888',
                            cursor: input.address ? 'pointer' : 'default',
                            marginBottom: '4px',
                            wordBreak: 'break-all',
                          }}
                          onClick={() => input.address && onAddressClick(input.address)}
                        >
                          {input.address || 'Unknown'}
                        </div>
                        <div style={{ color: '#27ae60' }}>
                          {formatAmount(input.amount)}
                        </div>
                        <div
                          style={{
                            fontSize: '10px',
                            color: '#666666',
                            marginTop: '2px',
                            cursor: 'pointer',
                          }}
                          onClick={() => onTxClick(input.txid)}
                        >
                          From: {input.txid.substring(0, 16)}...:{input.vout}
                        </div>
                      </>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <div style={{ color: '#888888', fontSize: '12px' }}>{t('explorer.noInputs')}</div>
            )}
            <div style={{
              marginTop: '8px',
              paddingTop: '8px',
              borderTop: '1px solid #3a3a3a',
              fontSize: '12px',
              color: '#888888',
            }}>
              {t('explorer.totalInput')}: <span style={{ color: '#27ae60' }}>{formatAmount(transaction.total_input)}</span>
            </div>
          </div>

          {/* Arrow */}
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <ArrowRight size={24} style={{ color: '#4a9eff' }} />
          </div>

          {/* Outputs */}
          <div style={{
            backgroundColor: '#1a1a1a',
            border: '1px solid #3a3a3a',
            borderRadius: '2px',
            padding: '12px',
          }}>
            <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#dddddd' }}>
              {t('explorer.outputs')} ({transaction.outputs?.length || 0})
            </div>
            {transaction.outputs && transaction.outputs.length > 0 ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {transaction.outputs.map((output) => (
                  <div
                    key={`${output.index}`}
                    style={{
                      padding: '8px',
                      backgroundColor: '#252525',
                      borderRadius: '2px',
                      fontSize: '11px',
                    }}
                  >
                    <div
                      style={{
                        fontFamily: 'monospace',
                        color: output.address ? '#4a9eff' : '#888888',
                        cursor: output.address ? 'pointer' : 'default',
                        marginBottom: '4px',
                        wordBreak: 'break-all',
                      }}
                      onClick={() => output.address && onAddressClick(output.address)}
                    >
                      {output.address || 'OP_RETURN'}
                    </div>
                    <div style={{ color: '#27ae60' }}>
                      {formatAmount(output.amount)}
                    </div>
                    <div style={{ fontSize: '10px', color: '#666666', marginTop: '2px' }}>
                      {output.script_type}
                      {output.is_spent && <span style={{ color: '#ff6666' }}> (Spent)</span>}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div style={{ color: '#888888', fontSize: '12px' }}>{t('explorer.noOutputs')}</div>
            )}
            <div style={{
              marginTop: '8px',
              paddingTop: '8px',
              borderTop: '1px solid #3a3a3a',
              fontSize: '12px',
              color: '#888888',
            }}>
              {t('explorer.totalOutput')}: <span style={{ color: '#27ae60' }}>{formatAmount(transaction.total_output)}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
