// FixtureOverlay.tsx — Debug navigation surface for the synthetic
// ExplorerTransaction fixtures defined in `./transactionFixtures.ts`.
//
// DEV-ONLY SCAFFOLDING — preserved in repo for future Tx/Block/Address
// detail-view redesign tasks (see m-tx-details-inputs-outputs-redesign).
// Hidden by default — only mounts in `../pages/ExplorerPage.tsx` when the
// `showFixtures` gate evaluates true. Activation methods documented at the
// `FixtureOverlay` import site in `ExplorerPage.tsx`.
//
// Renders a horizontal scrollable strip of fixture-name buttons. Clicking
// a button loads the fixture into the explorer slice via setCurrentTransaction
// + setView('transaction'), so TransactionDetail renders the synthetic data
// without any Wails round-trip.

import React from 'react';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { fixtureNames, transactionFixtures } from './transactionFixtures';

const overlayStyle: React.CSSProperties = {
  backgroundColor: '#3a2a4a',
  border: '1px solid #6a4a8a',
  borderRadius: '6px',
  padding: '8px 12px',
  display: 'flex',
  flexDirection: 'column',
  gap: '6px',
};

const headerStyle: React.CSSProperties = {
  fontSize: '11px',
  fontWeight: 600,
  color: '#ddd',
  textTransform: 'uppercase',
  letterSpacing: '0.5px',
};

const buttonRowStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: '4px',
};

const buttonStyle: React.CSSProperties = {
  padding: '4px 10px',
  fontSize: '11px',
  fontFamily: 'monospace',
  color: '#ddd',
  backgroundColor: '#4a3a5a',
  border: '1px solid #5a4a7a',
  borderRadius: '4px',
  cursor: 'pointer',
};

// Active variant — spread on top of buttonStyle when the row's fixture txid
// matches the currently-loaded `currentTransaction.txid`. Uses the canonical
// project TWINS green tokens (matches Send button / Total balance / Stake
// Return badge — established affirmative-action chrome). Active state clears
// automatically when the user navigates to a real (non-fixture) tx because
// `currentTransaction.txid` no longer matches any entry in `transactionFixtures`.
const activeButtonStyle: React.CSSProperties = {
  backgroundColor: '#27ae60',
  border: '1px solid #5a8c69',
  color: '#fff',
  fontWeight: 600,
};

export const FixtureOverlay: React.FC = () => {
  const { currentTransaction, setCurrentTransaction, setView, clearParentStack } = useStore(
    useShallow((state) => ({
      currentTransaction: state.currentTransaction,
      setCurrentTransaction: state.setCurrentTransaction,
      setView: state.setView,
      clearParentStack: state.clearParentStack,
    }))
  );

  // Active fixture detection — derived from the currently-loaded transaction's
  // txid. No new state added (the fixture txids are deterministic via
  // makeTxid(seed), so the lookup is exact and cheap at O(11)). Clears
  // automatically when user navigates to a real tx via search/Prev/Next.
  const selectedName = currentTransaction
    ? fixtureNames.find((n) => transactionFixtures[n].txid === currentTransaction.txid) ?? null
    : null;

  const handleClick = (name: string) => {
    const fixture = transactionFixtures[name];
    if (!fixture) return;
    // Clear parent stack so the back button on TransactionDetail returns to
    // the blocks list (and re-shows the overlay), not to a stale ancestor.
    clearParentStack();
    setCurrentTransaction(fixture);
    setView('transaction');
  };

  return (
    <div style={overlayStyle}>
      <span style={headerStyle}>Tx Detail Fixtures (dev-only — enable via ?fixtures=1)</span>
      <div style={buttonRowStyle}>
        {fixtureNames.map((name) => (
          <button
            key={name}
            type="button"
            style={name === selectedName ? { ...buttonStyle, ...activeButtonStyle } : buttonStyle}
            onClick={() => handleClick(name)}
          >
            {name}
          </button>
        ))}
      </div>
    </div>
  );
};
