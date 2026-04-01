import React from 'react';
import { UseFormRegister } from 'react-hook-form';
import { useCoinControl } from '@/store/useStore';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { Coins } from 'lucide-react';

export interface SendCoinControlProps {
  register: UseFormRegister<any>;
  watchedCustomChangeAddress: boolean;
  watchedSplitUTXO: boolean;
  calculateUTXOSize: () => string;
  onOpenCoinControl: () => void;
}

export const SendCoinControl: React.FC<SendCoinControlProps> = ({
  register,
  watchedCustomChangeAddress,
  watchedSplitUTXO,
  calculateUTXOSize,
  onOpenCoinControl,
}) => {
  const { coinControl, utxos } = useCoinControl();
  const { formatAmount } = useDisplayUnits();

  // Get selected coins count and total amount
  const selectedCount = coinControl.selectedCoins.size;
  const hasManualSelection = selectedCount > 0;

  // Calculate total selected amount from UTXOs
  const selectedAmount = hasManualSelection
    ? utxos
        .filter(utxo => coinControl.selectedCoins.has(`${utxo.txid}:${utxo.vout}`))
        .reduce((sum, utxo) => sum + utxo.amount, 0)
    : 0;

  return (
    <div className="qt-frame-secondary" style={{
      marginBottom: '8px',
      padding: '6px',
      border: '1px solid #4a4a4a',
      borderRadius: '2px',
      backgroundColor: '#3a3a3a'
    }}>
      <div className="qt-vbox" style={{ gap: '6px' }}>
        <div className="qt-label" style={{ fontWeight: 'bold', marginBottom: '2px', fontSize: '12px' }}>
          Coin Control Features
        </div>

        <div className="qt-hbox" style={{ gap: '10px', alignItems: 'center' }}>
          <button
            type="button"
            onClick={onOpenCoinControl}
            className="qt-button"
            style={{ padding: '3px 10px', fontSize: '12px' }}
          >
            Open Coin Control...
          </button>

          {/* Dynamic coin selection status */}
          {hasManualSelection ? (
            <div className="qt-hbox" style={{ gap: '6px', alignItems: 'center' }}>
              <Coins size={14} style={{ color: '#ffaa00' }} />
              <span className="qt-label" style={{ fontSize: '11px', color: '#ffaa00' }}>
                {selectedCount} coin{selectedCount !== 1 ? 's' : ''} manually selected ({formatAmount(selectedAmount)})
              </span>
            </div>
          ) : (
            <span className="qt-label" style={{ fontSize: '11px', color: '#999' }}>
              Coins automatically selected
            </span>
          )}
        </div>

        <div className="qt-hbox" style={{ gap: '15px', marginTop: '6px', alignItems: 'center' }}>
          <label className="qt-hbox" style={{ alignItems: 'center', gap: '4px' }}>
            <input
              type="checkbox"
              {...register('customChangeAddress')}
              className="qt-checkbox"
              style={{ width: '13px', height: '13px' }}
            />
            <span className="qt-label" style={{ fontSize: '12px' }}>Custom change address</span>
          </label>

          <input
            type="text"
            {...register('changeAddress')}
            placeholder="Enter a TWINS address (e.g. WJmGqDGiE5sGJxHwvW4sSodfnGMgQ9XFb)"
            className="qt-input"
            disabled={!watchedCustomChangeAddress}
            style={{
              flex: 1,
              padding: '2px 4px',
              fontSize: '11px',
              backgroundColor: watchedCustomChangeAddress ? '#2b2b2b' : '#232323',
              border: '1px solid #1a1a1a',
              opacity: watchedCustomChangeAddress ? 1 : 0.5,
              cursor: watchedCustomChangeAddress ? 'text' : 'not-allowed'
            }}
          />
        </div>

        <div className="qt-hbox" style={{ gap: '15px', alignItems: 'center' }}>
          <label className="qt-hbox" style={{ alignItems: 'center', gap: '4px' }}>
            <input
              type="checkbox"
              {...register('splitUTXO')}
              className="qt-checkbox"
              style={{ width: '13px', height: '13px' }}
            />
            <span className="qt-label" style={{ fontSize: '12px' }}>Split UTXO</span>
          </label>

          <div className="qt-hbox" style={{ alignItems: 'center', gap: '6px' }}>
            <span className="qt-label" style={{
              fontSize: '12px',
              opacity: watchedSplitUTXO ? 1 : 0.5
            }}>
              # of outputs
            </span>
            <input
              type="text"
              {...register('splitOutputs')}
              className="qt-input"
              disabled={!watchedSplitUTXO}
              style={{
                width: '60px',
                padding: '2px 4px',
                fontSize: '11px',
                backgroundColor: watchedSplitUTXO ? '#2b2b2b' : '#232323',
                border: '1px solid #1a1a1a',
                opacity: watchedSplitUTXO ? 1 : 0.5,
                cursor: watchedSplitUTXO ? 'text' : 'not-allowed'
              }}
            />
          </div>

          <div className="qt-hbox" style={{ alignItems: 'center', gap: '6px' }}>
            <span className="qt-label" style={{
              fontSize: '12px',
              opacity: watchedSplitUTXO ? 1 : 0.5
            }}>
              UTXO Size:
            </span>
            <span className="qt-label" style={{
              fontSize: '12px',
              opacity: watchedSplitUTXO ? 1 : 0.5
            }}>
              {watchedSplitUTXO ? calculateUTXOSize() : '0'} TWINS
            </span>
          </div>
        </div>
      </div>
    </div>
  );
};
