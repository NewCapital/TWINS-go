import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { AddressInfo } from '@/store/slices/explorerSlice';
import { ArrowLeft, Copy, ChevronDown, ChevronRight, Loader } from 'lucide-react';

interface AddressViewProps {
  addressInfo: AddressInfo | null;
  isLoading: boolean;
  isLoadingTx?: boolean;
  txLoadedCount?: number;
  onTxClick: (txid: string) => void;
  onBack: () => void;
}

const TRANSACTIONS_PER_PAGE = 10;

export const AddressView: React.FC<AddressViewProps> = ({
  addressInfo,
  isLoading,
  isLoadingTx = false,
  txLoadedCount = 0,
  onTxClick,
  onBack,
}) => {
  const { t } = useTranslation('common');
  const [txExpanded, setTxExpanded] = useState(false);
  const [utxoExpanded, setUtxoExpanded] = useState(false);
  const [txPage, setTxPage] = useState(0);

  if (isLoading) {
    return (
      <div style={{ padding: '24px', textAlign: 'center', color: '#888888' }}>
        {t('explorer.loadingAddress')}
      </div>
    );
  }

  if (!addressInfo) {
    return (
      <div style={{ padding: '24px', textAlign: 'center', color: '#888888' }}>
        {t('explorer.addressNotFound')}
      </div>
    );
  }

  // Format number with space as thousands separator (using Intl.NumberFormat for safety)
  const formatNumber = (amount: number) => {
    return amount.toLocaleString('en-US', {
      minimumFractionDigits: 8,
      maximumFractionDigits: 8,
      useGrouping: true,
    }).replace(/,/g, ' ');
  };

  const formatAmount = (amount: number) => {
    return formatNumber(amount) + ' TWINS';
  };

  const formatTime = (timeStr: string) => {
    const date = new Date(timeStr);
    return date.toLocaleString();
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  const InfoRow = ({ label, value }: { label: string; value: string | number }) => (
    <div style={{
      display: 'flex',
      padding: '6px 0',
      borderBottom: '1px solid #2a2a2a',
      fontSize: '12px',
    }}>
      <div style={{ width: '160px', color: '#888888', flexShrink: 0 }}>{label}</div>
      <div style={{ flex: 1, color: '#dddddd' }}>{value}</div>
    </div>
  );

  // Collapsible section header
  const SectionHeader = ({
    title,
    count,
    expanded,
    onToggle,
    isLoading: sectionLoading,
    loadedCount,
    totalCount,
  }: {
    title: string;
    count: number;
    expanded: boolean;
    onToggle: () => void;
    isLoading?: boolean;
    loadedCount?: number;
    totalCount?: number;
  }) => (
    <div
      onClick={(e) => {
        e.stopPropagation();
        e.preventDefault();
        onToggle();
      }}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        cursor: 'pointer',
        userSelect: 'none',
      }}
    >
      {expanded ? <ChevronDown size={16} color="#888888" /> : <ChevronRight size={16} color="#888888" />}
      <span style={{ fontSize: '13px', fontWeight: 'bold', color: '#dddddd' }}>
        {title} ({count})
      </span>
      {sectionLoading && (
        <span style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '10px', color: '#888888' }}>
          <Loader size={12} className="animate-spin" style={{ animation: 'spin 1s linear infinite' }} />
          Loading... {loadedCount !== undefined && totalCount !== undefined && `${loadedCount}/${totalCount}`}
        </span>
      )}
    </div>
  );

  // Sort and paginate transactions (fewer confirmations = newer = top)
  const transactions = [...(addressInfo.transactions || [])].sort(
    (a, b) => a.confirmations - b.confirmations
  );
  const totalTxPages = Math.ceil(transactions.length / TRANSACTIONS_PER_PAGE);
  const paginatedTransactions = transactions.slice(
    txPage * TRANSACTIONS_PER_PAGE,
    (txPage + 1) * TRANSACTIONS_PER_PAGE
  );

  // Sort UTXOs (fewer confirmations = newer = top)
  const sortedUtxos = [...(addressInfo.utxos || [])].sort(
    (a, b) => a.confirmations - b.confirmations
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
          {t('explorer.addressDetails')}
        </span>
      </div>

      {/* Address with copy button */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '8px 12px',
        backgroundColor: '#1a1a1a',
        border: '1px solid #3a3a3a',
        borderRadius: '2px',
        marginBottom: '12px',
      }}>
        <span style={{
          flex: 1,
          fontFamily: 'monospace',
          fontSize: '13px',
          color: '#dddddd',
          wordBreak: 'break-all',
        }}>
          {addressInfo.address}
        </span>
        <button
          className="qt-button"
          onClick={() => copyToClipboard(addressInfo.address)}
          style={{ padding: '4px 8px', display: 'flex', alignItems: 'center', gap: '4px' }}
          title="Copy address"
        >
          <Copy size={14} />
        </button>
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {/* Balance Info */}
        <div style={{
          backgroundColor: '#1a1a1a',
          border: '1px solid #3a3a3a',
          borderRadius: '2px',
          padding: '12px',
          marginBottom: '12px',
        }}>
          <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#dddddd' }}>
            {t('explorer.balanceInfo')}
          </div>
          <InfoRow label={t('explorer.balance')} value={formatAmount(addressInfo.balance)} />
          <InfoRow label={t('explorer.totalReceived')} value={formatAmount(addressInfo.total_received)} />
          <InfoRow label={t('explorer.totalSent')} value={formatAmount(addressInfo.total_sent)} />
          {addressInfo.unconfirmed_balance !== 0 && (
            <InfoRow label={t('explorer.unconfirmed')} value={formatAmount(addressInfo.unconfirmed_balance)} />
          )}
          <InfoRow label={t('explorer.transactionCount')} value={addressInfo.tx_count} />
        </div>

        {/* Transactions - Collapsible */}
        <div style={{
          backgroundColor: '#1a1a1a',
          border: '1px solid #3a3a3a',
          borderRadius: '2px',
          padding: '12px',
          marginBottom: '12px',
        }}>
          <SectionHeader
            title={t('explorer.transactions')}
            count={isLoadingTx ? txLoadedCount : transactions.length}
            expanded={txExpanded}
            onToggle={() => setTxExpanded(prev => !prev)}
            isLoading={isLoadingTx}
            loadedCount={txLoadedCount}
            totalCount={addressInfo.tx_count}
          />

          {txExpanded && transactions.length > 0 && (
            <div style={{ marginTop: '8px' }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                {/* Table Header */}
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 128px 128px 80px',
                  gap: '8px',
                  padding: '6px 8px',
                  backgroundColor: '#252525',
                  borderRadius: '2px',
                  fontSize: '10px',
                  fontWeight: 'bold',
                  color: '#888888',
                }}>
                  <div>{t('explorer.txid')}</div>
                  <div>{t('explorer.time')}</div>
                  <div style={{ textAlign: 'right' }}>{t('explorer.amount')}</div>
                  <div style={{ textAlign: 'right' }}>{t('explorer.confirmations')}</div>
                </div>

                {/* Transaction Rows */}
                {paginatedTransactions.map((tx, idx) => (
                  <div
                    key={`${tx.txid}:${idx}`}
                    onClick={() => onTxClick(tx.txid)}
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '1fr 128px 128px 80px',
                      gap: '8px',
                      padding: '6px 8px',
                      backgroundColor: '#202020',
                      borderRadius: '2px',
                      fontSize: '11px',
                      cursor: 'pointer',
                      transition: 'background-color 0.15s',
                    }}
                    onMouseEnter={(e) => {
                      e.currentTarget.style.backgroundColor = '#282828';
                    }}
                    onMouseLeave={(e) => {
                      e.currentTarget.style.backgroundColor = '#202020';
                    }}
                  >
                    <div style={{
                      fontFamily: 'monospace',
                      color: '#4a9eff',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}>
                      {tx.txid}
                    </div>
                    <div style={{ color: '#888888', fontSize: '10px' }}>
                      {formatTime(tx.time)}
                    </div>
                    <div style={{
                      textAlign: 'right',
                      color: tx.amount >= 0 ? '#27ae60' : '#e74c3c',
                    }}>
                      {tx.amount >= 0 ? '+' : ''}{formatNumber(Math.abs(tx.amount))}
                    </div>
                    <div style={{ textAlign: 'right', color: '#888888' }}>
                      {tx.confirmations}
                    </div>
                  </div>
                ))}
              </div>

              {/* Pagination */}
              {totalTxPages > 1 && (
                <div style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  marginTop: '12px',
                  paddingTop: '8px',
                  borderTop: '1px solid #2a2a2a',
                }}>
                  <button
                    className="qt-button"
                    onClick={() => setTxPage(Math.max(0, txPage - 1))}
                    disabled={txPage === 0}
                    style={{ padding: '4px 8px', fontSize: '11px' }}
                  >
                    {t('explorer.prev')}
                  </button>
                  <span style={{ fontSize: '11px', color: '#888888' }}>
                    {t('explorer.page', { current: txPage + 1, total: totalTxPages })}
                  </span>
                  <button
                    className="qt-button"
                    onClick={() => setTxPage(Math.min(totalTxPages - 1, txPage + 1))}
                    disabled={txPage >= totalTxPages - 1}
                    style={{ padding: '4px 8px', fontSize: '11px' }}
                  >
                    {t('explorer.next')}
                  </button>
                </div>
              )}

              {/* Loading indicator at bottom */}
              {isLoadingTx && (
                <div style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  marginTop: '8px',
                  padding: '8px',
                  color: '#888888',
                  fontSize: '11px',
                }}>
                  <Loader size={14} style={{ animation: 'spin 1s linear infinite' }} />
                  {t('explorer.loadingMore')}
                </div>
              )}
            </div>
          )}

          {txExpanded && transactions.length === 0 && !isLoadingTx && (
            <div style={{ color: '#888888', fontSize: '12px', marginTop: '8px' }}>
              {t('explorer.noTransactions')}
            </div>
          )}

          {txExpanded && transactions.length === 0 && isLoadingTx && (
            <div style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              marginTop: '8px',
              color: '#888888',
              fontSize: '12px',
            }}>
              <Loader size={14} style={{ animation: 'spin 1s linear infinite' }} />
              Loading transactions...
            </div>
          )}
        </div>

        {/* UTXOs - Collapsible */}
        {sortedUtxos.length > 0 && (
          <div style={{
            backgroundColor: '#1a1a1a',
            border: '1px solid #3a3a3a',
            borderRadius: '2px',
            padding: '12px',
          }}>
            <SectionHeader
              title={t('explorer.unspentOutputs')}
              count={sortedUtxos.length}
              expanded={utxoExpanded}
              onToggle={() => setUtxoExpanded(prev => !prev)}
            />

            {utxoExpanded && (
              <div style={{ marginTop: '8px' }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                  {/* Table Header */}
                  <div style={{
                    display: 'grid',
                    gridTemplateColumns: '1fr 40px 128px 80px',
                    gap: '8px',
                    padding: '6px 8px',
                    backgroundColor: '#252525',
                    borderRadius: '2px',
                    fontSize: '10px',
                    fontWeight: 'bold',
                    color: '#888888',
                  }}>
                    <div>{t('explorer.txid')}</div>
                    <div>{t('explorer.vout')}</div>
                    <div style={{ textAlign: 'right' }}>{t('explorer.amount')}</div>
                    <div style={{ textAlign: 'right' }}>{t('explorer.confirmations')}</div>
                  </div>

                  {/* UTXO Rows */}
                  {sortedUtxos.map((utxo) => (
                    <div
                      key={`${utxo.txid}-${utxo.vout}`}
                      onClick={() => onTxClick(utxo.txid)}
                      style={{
                        display: 'grid',
                        gridTemplateColumns: '1fr 40px 128px 80px',
                        gap: '8px',
                        padding: '6px 8px',
                        backgroundColor: '#202020',
                        borderRadius: '2px',
                        fontSize: '11px',
                        cursor: 'pointer',
                        transition: 'background-color 0.15s',
                      }}
                      onMouseEnter={(e) => {
                        e.currentTarget.style.backgroundColor = '#282828';
                      }}
                      onMouseLeave={(e) => {
                        e.currentTarget.style.backgroundColor = '#202020';
                      }}
                    >
                      <div style={{
                        fontFamily: 'monospace',
                        color: '#4a9eff',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}>
                        {utxo.txid}
                      </div>
                      <div style={{ color: '#888888' }}>
                        {utxo.vout}
                      </div>
                      <div style={{ textAlign: 'right', color: '#27ae60' }}>
                        {formatNumber(utxo.amount)}
                      </div>
                      <div style={{ textAlign: 'right', color: '#888888' }}>
                        {utxo.confirmations}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* CSS for spinner animation */}
      <style>{`
        @keyframes spin {
          from { transform: rotate(0deg); }
          to { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  );
};
