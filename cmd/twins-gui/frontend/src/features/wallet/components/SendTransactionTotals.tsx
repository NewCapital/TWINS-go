import React from 'react';
import { formatAmountDisplay } from '@/utils/amountValidation';

interface TransactionTotals {
  recipientsTotal: number;
  estimatedFee: number;
  grandTotal: number;
  remainingBalance: number;
  canSend: boolean;
}

export interface SendTransactionTotalsProps {
  transactionTotals: TransactionTotals | null;
  recipientCount: number;
}

export const SendTransactionTotals: React.FC<SendTransactionTotalsProps> = ({
  transactionTotals,
  recipientCount,
}) => {
  return (
    <>
      {/* Multi-Recipient Info */}
      {recipientCount > 1 && (
        <div className="qt-frame-secondary" style={{
          marginBottom: '8px',
          padding: '8px',
          border: '1px solid #4a7a4a',
          borderRadius: '2px',
          backgroundColor: '#3a4a3a'
        }}>
          <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
            <span className="qt-label" style={{ fontSize: '11px', color: '#88cc88' }}>
              Sending to {recipientCount} recipients in a single transaction.
            </span>
          </div>
        </div>
      )}

      {/* Transaction Totals Display */}
      {transactionTotals && transactionTotals.recipientsTotal > 0 && (
        <div className="qt-frame-secondary" style={{
          marginBottom: '8px',
          padding: '8px',
          border: '1px solid #4a4a4a',
          borderRadius: '2px',
          backgroundColor: transactionTotals.canSend ? '#3a3a3a' : '#4a2a2a'
        }}>
          <div className="qt-vbox" style={{ gap: '4px' }}>
            <div className="qt-hbox" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
              <span className="qt-label" style={{ fontSize: '12px' }}>Recipients Total:</span>
              <span className="qt-label" style={{ fontSize: '12px', fontWeight: 'bold' }}>
                {formatAmountDisplay(transactionTotals.recipientsTotal)}
              </span>
            </div>
            <div className="qt-hbox" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
              <span className="qt-label" style={{ fontSize: '12px' }}>Estimated Fee:</span>
              <span className="qt-label" style={{ fontSize: '12px' }}>
                {formatAmountDisplay(transactionTotals.estimatedFee)}
              </span>
            </div>
            <div style={{ borderTop: '1px solid #555', margin: '4px 0' }} />
            <div className="qt-hbox" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
              <span className="qt-label" style={{ fontSize: '12px', fontWeight: 'bold' }}>Grand Total:</span>
              <span className="qt-label" style={{
                fontSize: '12px',
                fontWeight: 'bold',
                color: transactionTotals.canSend ? '#00ff00' : '#ff6666'
              }}>
                {formatAmountDisplay(transactionTotals.grandTotal)}
              </span>
            </div>
            <div className="qt-hbox" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
              <span className="qt-label" style={{ fontSize: '11px', color: '#999' }}>Remaining Balance:</span>
              <span className="qt-label" style={{ fontSize: '11px', color: '#999' }}>
                {formatAmountDisplay(transactionTotals.remainingBalance)}
              </span>
            </div>
            {!transactionTotals.canSend && (
              <div className="qt-label" style={{
                fontSize: '11px',
                color: '#ff6666',
                textAlign: 'center',
                marginTop: '4px'
              }}>
                Insufficient balance for this transaction
              </div>
            )}
          </div>
        </div>
      )}
    </>
  );
};
