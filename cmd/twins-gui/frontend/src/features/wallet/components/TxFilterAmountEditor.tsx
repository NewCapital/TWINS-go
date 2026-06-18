import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTransactions } from '@/store/useStore';
import { isAmountRangeInverted } from '@/features/wallet/utils/amountRange';

interface TxFilterAmountEditorProps {
  onClose: () => void;
}

// Hide native number-input spinner arrows. The default Chromium/WebKit arrows
// render as light grey UI elements that are almost invisible on the dark
// TWINS theme. We hide them entirely — users type or paste amounts directly,
// which is the realistic crypto-wallet flow (precise satoshi values, not
// step-by-1 increments).
const HIDE_SPINNERS_CSS = `
.tx-amount-input::-webkit-inner-spin-button,
.tx-amount-input::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.tx-amount-input {
  -moz-appearance: textfield;
  appearance: textfield;
}
`;

const inputStyle: React.CSSProperties = {
  backgroundColor: '#252525',
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  padding: '7px 10px',
  fontSize: '12px',
  color: '#ddd',
  width: '100%',
  outline: 'none',
};

const labelStyle: React.CSSProperties = {
  fontSize: '11px',
  color: '#888',
  marginBottom: '4px',
  display: 'block',
};

const primaryButton: React.CSSProperties = {
  backgroundColor: '#4a7c59',
  border: '1px solid #5a8c69',
  borderRadius: '6px',
  padding: '6px 14px',
  fontSize: '12px',
  fontWeight: 500,
  color: '#fff',
  cursor: 'pointer',
};

const secondaryButton: React.CSSProperties = {
  backgroundColor: '#383838',
  border: '1px solid #4a4a4a',
  borderRadius: '6px',
  padding: '6px 14px',
  fontSize: '12px',
  color: '#ccc',
  cursor: 'pointer',
};

/**
 * Amount range editor (Phase 4 max-amount). Two inputs (From / To) committed
 * together on Apply. Empty input = unbounded on that side. Clear empties both
 * and dispatches both setters.
 *
 * Draft-on-Apply commit pattern matches TxFilterTypeEditor: no resync
 * useEffect — all slice mutations that would change min/maxAmount while the
 * editor is open also close the popover (Apply, Clear, chip dismiss, Clear-
 * all). The previous single-input version had a resync useEffect for smart-
 * search dispatch (`>50` routes to setMinAmount), but smart-search only fires
 * when the editor is CLOSED, so it can't drift the draft.
 */
export const TxFilterAmountEditor: React.FC<TxFilterAmountEditorProps> = ({ onClose }) => {
  const { t } = useTranslation('wallet');
  const { minAmount, maxAmount, setAmountRange } = useTransactions();

  const [draftFrom, setDraftFrom] = useState(minAmount);
  const [draftTo, setDraftTo] = useState(maxAmount);

  // Inverted-range guard. Helper is pure (see utils/amountRange.ts) and unit-
  // tested in isolation. Blocks the Apply path when From > To with both bounds
  // set — the prior behavior silently swapped From/To at the backend chip
  // label, masking user input errors. Single-bound inputs (only From or only
  // To) remain valid since the backend treats empty as unbounded.
  const isInverted = isAmountRangeInverted(draftFrom, draftTo);

  // Batched commit via setAmountRange: one state mutation + one fetchPage(1)
  // for both bounds. Calling setMinAmount + setMaxAmount sequentially would
  // schedule two independent 300ms debounce timers and dispatch two identical
  // fetches — see slice docstring for the cancel-pending-then-flush logic.
  const handleApply = () => {
    if (isInverted) return;
    setAmountRange(draftFrom, draftTo);
    onClose();
  };

  const handleClear = () => {
    setDraftFrom('');
    setDraftTo('');
    setAmountRange('', '');
    onClose();
  };

  const handleEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (isInverted) return;
      handleApply();
    }
  };

  const renderInput = (
    key: 'from' | 'to',
    value: string,
    setValue: (v: string) => void,
    autoFocus: boolean,
  ) => (
    <div>
      <span style={labelStyle}>
        {t(
          key === 'from'
            ? 'transactions.filters.chip.amountFromLabel'
            : 'transactions.filters.chip.amountToLabel',
        )}
      </span>
      <div style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
        <input
          type="number"
          className="tx-amount-input"
          min="0"
          step="0.00000001"
          inputMode="decimal"
          style={{
            ...inputStyle,
            paddingRight: '52px',
            borderColor: isInverted ? '#ff6666' : '#3a3a3a',
          }}
          value={value}
          placeholder="0.00"
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleEnter}
          autoFocus={autoFocus}
          aria-label={t(
            key === 'from'
              ? 'transactions.filters.chip.amountFromLabel'
              : 'transactions.filters.chip.amountToLabel',
          )}
        />
        <span
          style={{
            position: 'absolute',
            right: '10px',
            fontSize: '11px',
            color: '#888',
            pointerEvents: 'none',
          }}
        >
          {t('transactions.filters.chip.amountSuffix')}
        </span>
      </div>
    </div>
  );

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', minWidth: '240px' }}>
      <style>{HIDE_SPINNERS_CSS}</style>
      {renderInput('from', draftFrom, setDraftFrom, true)}
      {renderInput('to', draftTo, setDraftTo, false)}

      {isInverted && (
        <div
          role="alert"
          style={{
            fontSize: '11px',
            color: '#ff6666',
            marginTop: '-4px',
          }}
        >
          {t('transactions.filters.chip.invertedRangeError')}
        </div>
      )}

      <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
        <button type="button" style={secondaryButton} onClick={handleClear}>
          {t('transactions.filters.chip.amountClear')}
        </button>
        <button
          type="button"
          style={{
            ...primaryButton,
            opacity: isInverted ? 0.5 : 1,
            cursor: isInverted ? 'not-allowed' : 'pointer',
          }}
          onClick={handleApply}
          disabled={isInverted}
        >
          {t('transactions.filters.chip.amountApply')}
        </button>
      </div>
    </div>
  );
};
