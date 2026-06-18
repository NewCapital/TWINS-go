import React from 'react';
import { useTransactions } from '@/store/useStore';
import type { TypeFilter } from '@/store/slices/transactionsSlice';
import { getTransactionTypeIcon } from '@/shared/utils/transactionIcons';

interface TxFilterTypeEditorProps {
  onClose: () => void;
}

// Filter-key -> human label. Distinct from getTransactionTypeLabel which keys
// on raw transaction.type values (filter keys are derived but not 1:1).
//
// Phase 3 multi-select: legacy 'all' and 'mostCommon' rows are gone. "All
// categories" is the implicit state when no checkbox is ticked.
const TYPE_OPTIONS: { value: TypeFilter; label: string }[] = [
  { value: 'received', label: 'Received' },
  { value: 'sent', label: 'Sent' },
  { value: 'toYourself', label: 'To yourself' },
  { value: 'mined', label: 'Mined' },
  { value: 'minted', label: 'Minted' },
  { value: 'masternode', label: 'Masternode Reward' },
  { value: 'consolidation', label: 'UTXO Consolidation' },
  { value: 'other', label: 'Other' },
];

// Filter-key -> icon for visual preview in the editor. Maps each filter to
// the closest representative transaction.type.
function iconForFilter(value: TypeFilter): string | null {
  switch (value) {
    case 'received':
      return getTransactionTypeIcon('receive');
    case 'sent':
      return getTransactionTypeIcon('send');
    case 'toYourself':
      return getTransactionTypeIcon('send_to_self');
    case 'mined':
      return getTransactionTypeIcon('generated');
    case 'minted':
      return getTransactionTypeIcon('stake');
    case 'masternode':
      return getTransactionTypeIcon('masternode');
    case 'consolidation':
      return getTransactionTypeIcon('consolidation');
    case 'other':
      return getTransactionTypeIcon('other');
  }
}

const rowBase: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '10px',
  padding: '6px 8px',
  borderRadius: '4px',
  cursor: 'pointer',
  fontSize: '12px',
  color: '#ddd',
  border: '1px solid transparent',
  transition: 'background-color 0.15s, border-color 0.15s',
};

const checkboxStyle: React.CSSProperties = {
  width: '14px',
  height: '14px',
  flexShrink: 0,
  cursor: 'pointer',
  accentColor: '#27ae60',
};

const footerStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: '8px',
  paddingTop: '8px',
  marginTop: '4px',
  borderTop: '1px solid #3a3a3a',
};

const applyButtonStyle: React.CSSProperties = {
  backgroundColor: '#4a7c59',
  border: '1px solid #5a8c69',
  borderRadius: '6px',
  padding: '6px 14px',
  fontSize: '11px',
  fontWeight: 500,
  color: '#fff',
  cursor: 'pointer',
};

const clearButtonStyle: React.CSSProperties = {
  backgroundColor: '#383838',
  border: '1px solid #4a4a4a',
  borderRadius: '6px',
  padding: '6px 14px',
  fontSize: '11px',
  color: '#ccc',
  cursor: 'pointer',
};

/**
 * Multi-select checkbox editor for the Type filter chip.
 *
 * Local draft pattern: changes accumulate in `draft` state until the user clicks
 * Apply. Clear empties the draft AND commits immediately (per-row checkboxes
 * stay editable for free re-selection without re-opening the popover). Cancel
 * — via Esc / click-outside — is handled by the parent TxFilterPopover, which
 * just closes the popover; the draft is discarded on unmount.
 */
export const TxFilterTypeEditor: React.FC<TxFilterTypeEditorProps> = ({ onClose }) => {
  const { typeFilter, setTypeFilter } = useTransactions();
  const [draft, setDraft] = React.useState<TypeFilter[]>(typeFilter);
  const [hoverIdx, setHoverIdx] = React.useState<number | null>(null);

  // Draft state intentionally seeds ONCE from typeFilter on mount via useState
  // initializer above — no resync useEffect. All slice mutations that change
  // typeFilter while this editor is open are popover-closing actions (Apply,
  // Clear, chip dismiss, Clear-all on the parent bar) which unmount this
  // component. A resync useEffect would clobber user-in-progress checkbox
  // edits against a phantom external-change case that cannot occur.

  const handleToggle = (value: TypeFilter) => {
    setDraft((cur) =>
      cur.includes(value) ? cur.filter((v) => v !== value) : [...cur, value],
    );
  };

  const handleApply = () => {
    setTypeFilter(draft);
    onClose();
  };

  const handleClear = () => {
    setDraft([]);
    setTypeFilter([]);
    onClose();
  };

  return (
    <div
      style={{ display: 'flex', flexDirection: 'column', gap: '2px', minWidth: '240px', maxHeight: '380px' }}
      role="group"
      aria-label="Transaction type filter"
    >
      {/*
        WAI-ARIA filter UI pattern: role="group" with native <input
        type="checkbox"> per row. Each row's <label> wraps both the checkbox
        and the visible label/icon so a click anywhere on the row toggles the
        underlying checkbox. Native checkboxes handle Space/Enter activation
        and report state to screen readers via the implicit aria-checked.
      */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '2px', overflowY: 'auto', flex: 1 }}>
        {TYPE_OPTIONS.map((opt, idx) => {
          const icon = iconForFilter(opt.value);
          const isChecked = draft.includes(opt.value);
          const isHover = hoverIdx === idx;
          const style: React.CSSProperties = isHover
            ? { ...rowBase, backgroundColor: '#383838' }
            : rowBase;
          return (
            <label
              key={opt.value}
              style={style}
              onMouseEnter={() => setHoverIdx(idx)}
              onMouseLeave={() => setHoverIdx((cur) => (cur === idx ? null : cur))}
            >
              <input
                type="checkbox"
                checked={isChecked}
                onChange={() => handleToggle(opt.value)}
                style={checkboxStyle}
              />
              {icon ? (
                <img src={icon} alt="" width={20} height={20} style={{ flexShrink: 0 }} />
              ) : (
                <div style={{ width: 20, height: 20, flexShrink: 0 }} />
              )}
              <span style={{ flex: 1, textAlign: 'left' }}>{opt.label}</span>
            </label>
          );
        })}
      </div>
      <div style={footerStyle}>
        <button
          type="button"
          onClick={handleClear}
          style={clearButtonStyle}
        >
          Clear
        </button>
        <button
          type="button"
          onClick={handleApply}
          style={applyButtonStyle}
        >
          Apply
        </button>
      </div>
    </div>
  );
};
