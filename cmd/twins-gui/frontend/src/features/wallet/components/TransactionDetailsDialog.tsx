/**
 * TransactionDetailsDialog Component
 *
 * Displays transaction details using the Receive design language.
 * Structure: Hero (amount + status pill + date), optional Banner
 * (maturity / conflicted), Details ledger, optional Message, Transaction ID.
 * The bottom Close button is removed; the header X is the sole close affordance.
 */

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { X, Copy, ExternalLink } from 'lucide-react';
import { BrowserOpenURL } from '@wailsjs/runtime/runtime';
import { core } from '@/shared/types/wallet.types';
import {
  getTransactionTypeIcon,
  getTransactionTypeLabel,
} from '@/shared/utils/transactionIcons';
import { ConfirmationRing } from '@/shared/components/ConfirmationRing';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { sanitizeText } from '@/shared/utils/sanitize';
import { Banner } from '@/shared/components/Banner';
import { IconButton } from '@/shared/components/IconButton';
import { StatusPill, type StatusPillTone } from '@/shared/components/StatusPill';
import { truncateAddress } from '@/shared/utils/format';
import { useTransactions } from '@/store/useStore';
import { LEGACY_EXPLORER_TX_FALLBACK, buildExplorerURL } from '@/shared/constants/explorer';

interface TransactionDetailsDialogProps {
  isOpen: boolean;
  onClose: () => void;
  transaction: core.Transaction | null;
}

const TXID_REGEX = /^[a-fA-F0-9]{64}$/;

// ---- Receive design tokens (inline constants) ----
const cardStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  padding: '12px 16px',
};

const cardStyleHero: React.CSSProperties = {
  ...cardStyle,
  padding: '16px',
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  gap: '8px',
};

const labelStyle: React.CSSProperties = { fontSize: '11px', color: '#888' };
const valueStyle: React.CSSProperties = { fontSize: '12px', color: '#ddd' };
const monoAddressStyle: React.CSSProperties = {
  fontSize: '12px',
  fontFamily: 'monospace',
  color: '#e0e0e0',
  letterSpacing: '0.3px',
};
const dividerStyle: React.CSSProperties = {
  borderTop: '1px solid #3a3a3a',
  margin: '4px 0',
};
const rowStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  gap: '8px',
  padding: '4px 0',
};
const sectionTitleStyle: React.CSSProperties = {
  fontSize: '13px',
  fontWeight: 600,
  color: '#ccc',
  marginBottom: '8px',
};
const copyToastStyle: React.CSSProperties = {
  fontSize: '10px',
  color: '#27ae60',
  marginTop: '2px',
  textAlign: 'right',
};

// ---- Helpers ----

/**
 * Status text derivation. Mirrors the legacy logic with the
 * Confirming branch collapsed onto the success tone (still on track to confirm).
 * Now takes the i18next `t` function so all branches return localized strings.
 */
function getStatusText(
  t: (key: string, options?: Record<string, unknown>) => string,
  confirmations: number,
  isConflicted: boolean,
  isCoinbase: boolean,
  isCoinstake: boolean,
  maturesIn: number
): string {
  if (isConflicted) return t('transactionDetails.status.conflicted');
  if (confirmations === 0) return t('transactionDetails.status.unconfirmed');
  if ((isCoinbase || isCoinstake) && maturesIn > 0) {
    return t('transactionDetails.status.immature', { blocks: maturesIn });
  }
  if (confirmations < 6) return t('transactionDetails.status.confirming', { count: confirmations });
  // U4: bare "Confirmed" label; full count surfaced via `title` tooltip on the status pill wrapper.
  return t('transactionDetails.status.confirmed');
}

function getStatusTone(
  confirmations: number,
  isConflicted: boolean,
  maturesIn: number
): StatusPillTone {
  if (isConflicted) return 'error';
  if (maturesIn > 0) return 'warning';
  if (confirmations === 0) return 'warning';
  return 'success';
}

// (formatLongDate helper deleted — superseded by the global useDisplayDateTime hook.)

/**
 * Format a signed amount with the active display unit; strips an extra leading
 * minus from the formatter and prepends the proper sign.
 */
function formatAmountWithSign(amount: number, fmtAmount: (n: number) => string): string {
  if (amount < 0) return `-${fmtAmount(Math.abs(amount))}`;
  if (amount > 0) return `+${fmtAmount(amount)}`;
  return fmtAmount(0);
}

function getAmountColor(amount: number, isSelfTransfer: boolean): string {
  if (isSelfTransfer) return '#888';
  if (amount < 0) return '#ff6666';
  if (amount > 0) return '#27ae60';
  return '#ddd';
}

function isReceiveTransaction(type: string): boolean {
  return (
    type.startsWith('receive') ||
    type === 'generated' ||
    type === 'stake' ||
    type === 'masternode'
  );
}

// ---- Sub-component: explorer button (single icon or popover dropdown) ----

interface ExplorerButtonProps {
  /** Pre-formatted value to substitute into each URL's `%s`. */
  value: string;
  /** Each entry: `{ url: string; hostname: string }`. */
  urls: Array<{ url: string; hostname: string }>;
  /** Hidden when no URLs are available (and no legacy fallback applies). */
  ariaLabel: string;
  title: string;
}

const ExplorerButton: React.FC<ExplorerButtonProps> = ({ value, urls, ariaLabel, title }) => {
  const { t } = useTranslation('wallet');
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState<{ x: number; y: number } | null>(null);
  const buttonRef = useRef<HTMLDivElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);

  // Close popover on outside mousedown.
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (
        popoverRef.current &&
        !popoverRef.current.contains(target) &&
        buttonRef.current &&
        !buttonRef.current.contains(target)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  // Close popover on Escape. Capture phase + stopPropagation prevents the
  // parent dialog's Escape handler from also firing and closing the dialog
  // beneath us — without this guard, the user opens the explorer menu to
  // compare options and pressing Escape collapses the whole dialog instead
  // of just dismissing the menu.
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        setOpen(false);
      }
    };
    document.addEventListener('keydown', handler, true); // capture phase: runs before bubble-phase listeners
    return () => document.removeEventListener('keydown', handler, true);
  }, [open]);

  // Close popover on scroll/resize. The popover uses fixed positioning captured
  // at click time; scrolling the dialog body or resizing the window would otherwise
  // visually decouple the popover from its trigger button.
  useEffect(() => {
    if (!open) return;
    const handler = () => setOpen(false);
    window.addEventListener('resize', handler);
    document.addEventListener('scroll', handler, true); // capture phase to catch nested scroll containers
    return () => {
      window.removeEventListener('resize', handler);
      document.removeEventListener('scroll', handler, true);
    };
  }, [open]);

  if (urls.length === 0) return null;

  if (urls.length === 1) {
    return (
      <IconButton
        icon={<ExternalLink size={12} />}
        title={t('transactionDetails.openInExplorerHost', { host: urls[0].hostname })}
        ariaLabel={ariaLabel}
        onClick={() => BrowserOpenURL(buildExplorerURL(urls[0].url, value))}
      />
    );
  }

  const togglePopover = () => {
    if (open) {
      setOpen(false);
      return;
    }
    if (buttonRef.current) {
      const rect = buttonRef.current.getBoundingClientRect();
      setPosition({ x: rect.right, y: rect.top });
    }
    setOpen(true);
  };

  const handlePick = (url: string) => {
    BrowserOpenURL(buildExplorerURL(url, value));
    setOpen(false);
  };

  return (
    <>
      <div ref={buttonRef} style={{ display: 'inline-flex' }}>
        <IconButton
          icon={<ExternalLink size={12} />}
          title={title}
          ariaLabel={ariaLabel}
          onClick={togglePopover}
        />
      </div>
      {open && position && (
        <div
          ref={popoverRef}
          role="menu"
          style={{
            position: 'fixed',
            top: position.y,
            left: position.x,
            transform: 'translate(-100%, calc(-100% - 4px))',
            zIndex: 60,
            backgroundColor: '#2f2f2f',
            border: '1px solid #3a3a3a',
            borderRadius: '6px',
            padding: '4px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.6)',
            minWidth: '180px',
          }}
        >
          {urls.map((u) => (
            <button
              key={u.url}
              type="button"
              onClick={() => handlePick(u.url)}
              role="menuitem"
              style={{
                display: 'block',
                width: '100%',
                textAlign: 'left',
                padding: '6px 12px',
                borderRadius: '4px',
                background: 'none',
                border: 'none',
                color: '#ddd',
                fontSize: '12px',
                cursor: 'pointer',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.backgroundColor = '#383838';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = 'transparent';
              }}
            >
              {u.hostname}
            </button>
          ))}
        </div>
      )}
    </>
  );
};

// ---- Main component ----

export const TransactionDetailsDialog: React.FC<TransactionDetailsDialogProps> = ({
  isOpen,
  onClose,
  transaction,
}) => {
  const { t } = useTranslation('wallet');
  const [copiedField, setCopiedField] = useState<string | null>(null);
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);
  const { formatAmount } = useDisplayUnits();
  const { blockExplorerUrls, syncBlockExplorerUrls } = useTransactions();

  // Track whether we've resolved the block-explorer URL setting at least once.
  // While unresolved, the TXID explorer button is suppressed so a user can't
  // click it during the brief window between dialog open and the
  // `syncBlockExplorerUrls()` call resolving — without this gate, an open from
  // a context that hasn't pre-synced (rare but possible) could route the user
  // to the legacy `explorer.win.win` fallback even when they have a custom
  // explorer configured. Initialized true if the store already has URLs (the
  // normal case — Overview / Transactions pages pre-sync on mount).
  const [urlsResolved, setUrlsResolved] = useState(blockExplorerUrls.length > 0);

  // Defense-in-depth: sync block explorer URLs on dialog open if store is empty.
  // We mark `urlsResolved` once the sync settles (success or failure) so the
  // TXID explorer button can render with the correct URLs (or fall back to
  // the legacy `explorer.win.win` only if the sync genuinely returned empty).
  //
  // The component stays mounted while the dialog is closed (returns null) so
  // useState persists across reopens. We re-evaluate `urlsResolved` at every
  // (re)open: if the store currently has URLs, mark resolved immediately; if
  // the store is empty, reset to false and re-run the sync — this prevents a
  // stale `urlsResolved=true` from a prior mount cycle from briefly exposing
  // the legacy fallback when the user has cleared their custom explorer URL
  // setting between opens.
  useEffect(() => {
    if (!isOpen) return;
    if (blockExplorerUrls.length > 0) {
      setUrlsResolved(true);
      return;
    }
    setUrlsResolved(false);
    let cancelled = false;
    Promise.resolve(syncBlockExplorerUrls()).finally(() => {
      if (!cancelled) setUrlsResolved(true);
    });
    return () => {
      cancelled = true;
    };
  }, [isOpen, blockExplorerUrls.length, syncBlockExplorerUrls]);

  // A3: dialog focus management — auto-focus on open, Tab-cycling trap, restore focus on close.
  // Escape continues to close the dialog (no regression).
  //
  // The effect is keyed ONLY on `isOpen` so that the cleanup runs exactly once per
  // open→close transition. An earlier draft included `onClose` in the dep array;
  // parents (e.g. OverviewPage) commonly pass an inline arrow `() => setSelected(null)`
  // which produces a new function identity on every render, causing the cleanup to
  // fire on every parent re-render mid-open and yank focus back to the opener,
  // defeating the trap. The handler reads the latest `onClose` via a ref so we
  // still call the up-to-date callback on Escape.
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);
  useEffect(() => {
    if (!isOpen) return;

    // Snapshot the previously-focused element so we can restore on close.
    previouslyFocusedRef.current = (document.activeElement as HTMLElement) ?? null;

    const getFocusable = (): HTMLElement[] => {
      const root = dialogRef.current;
      if (!root) return [];
      const selector =
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]),' +
        ' textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
      return Array.from(root.querySelectorAll<HTMLElement>(selector)).filter(
        (el) => !el.hasAttribute('disabled') && el.offsetParent !== null
      );
    };

    // Place initial focus on the first focusable child (typically the X close button)
    // instead of the dialog root. This gives keyboard users an immediately visible
    // focus indicator on an expected interactive element (small ring on a 24×24 button),
    // avoids the visually heavy browser-default ring around the entire dialog frame,
    // and keeps the root reserved as the no-children fallback anchor for the Tab trap.
    const initialFocusables = getFocusable();
    if (initialFocusables.length > 0) {
      initialFocusables[0].focus();
    } else {
      // No focusable children — fall back to programmatic focus on the dialog root
      // (tabIndex={-1}) so Tab keystrokes still land in the dialog scope. The root's
      // outline is suppressed visually; this branch is structurally unreachable in
      // the current dialog (X button is always present) but kept for defensive depth.
      dialogRef.current?.focus();
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onCloseRef.current();
        return;
      }
      if (e.key !== 'Tab') return;
      const focusable = getFocusable();
      if (focusable.length === 0) {
        // No focusable children — keep focus on the dialog root.
        e.preventDefault();
        dialogRef.current?.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement as HTMLElement | null;
      if (e.shiftKey && (active === first || active === dialogRef.current)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      // Restore focus to the element that opened the dialog. Guard against the
      // element having been removed from the DOM in the meantime.
      const prev = previouslyFocusedRef.current;
      if (prev && document.body.contains(prev)) {
        prev.focus();
      }
      previouslyFocusedRef.current = null;
    };
  }, [isOpen]);

  // Clear any pending copy-feedback timer on unmount so it doesn't fire
  // setCopiedField on an unmounted component.
  useEffect(() => {
    return () => {
      if (copyTimerRef.current !== null) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    };
  }, []);

  const handleCopy = useCallback(async (text: string, field: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedField(field);
      // Cancel any in-flight prior timer so the new copy gets the full 2s window.
      if (copyTimerRef.current !== null) clearTimeout(copyTimerRef.current);
      copyTimerRef.current = setTimeout(() => {
        setCopiedField(null);
        copyTimerRef.current = null;
      }, 2000);
    } catch {
      // Clipboard copy failed - UI won't show success state.
    }
  }, []);

  // CRITICAL: `useDisplayDateTime()` MUST be called BEFORE the early-return
  // guard below. The dialog stays mounted with `isOpen=false` while the parent
  // tree is alive (e.g. Transactions page or Overview Recent Transactions
  // keeps `<TransactionDetailsDialog isOpen={...}>` rendered across both
  // states). Initial render with `isOpen=false` runs N hooks then short-circuits
  // at the early-return; clicking the Eye flips `isOpen` to true on the same
  // instance (no unmount), re-renders past the guard, and if the hook is
  // called AFTER the guard the second render runs N+1 hooks. React throws
  // "Rendered more hooks than during the previous render", which the React
  // Router error boundary at app/router.tsx:22-62 catches and renders as
  // "Something went wrong loading this page". Fixed in
  // h-fix-tx-details-dialog-rules-of-hooks (2026-06-11); mirrors the
  // Explorer-side precedent
  // m-fix-explorer-blockdetail-txdetail-navigation-error (2026-06-04) on
  // BlockDetail.tsx + TransactionDetail.tsx. The same-day regression origin
  // for this file is m-fix-date-display-inconsistencies (2026-06-04), which
  // migrated this dialog onto the global `useDisplayDateTime` hook without
  // accounting for the early-return placement. Do NOT relocate this back
  // below the guard.
  const { formatDateTime, formatTooltip } = useDisplayDateTime();

  if (!isOpen || !transaction) return null;

  const typeIcon = getTransactionTypeIcon(transaction.type);
  const typeLabel = getTransactionTypeLabel(transaction.type);

  const isConflicted = transaction.is_conflicted || false;
  const isCoinbase = transaction.is_coinbase || false;
  const isCoinstake = transaction.is_coinstake || false;
  const maturesIn = transaction.matures_in || 0;
  const isWatchOnly = transaction.is_watch_only || false;
  const confirmations = transaction.confirmations || 0;

  const statusText = getStatusText(t, confirmations, isConflicted, isCoinbase, isCoinstake, maturesIn);
  const statusTone = getStatusTone(confirmations, isConflicted, maturesIn);

  // `useDisplayDateTime()` hook was hoisted above the early-return guard — see
  // the CRITICAL comment block above. These are plain function calls on the
  // destructured outputs and stay here so they can read `transaction.time`
  // after the non-null guard has passed.
  const formattedDate = formatDateTime(transaction.time);
  const formattedDateUTC = formatTooltip(transaction.time);

  const isSelfTransfer =
    transaction.type === 'send_to_self' || transaction.type === 'consolidation';
  const amountColor = getAmountColor(transaction.amount, isSelfTransfer);

  const fee = transaction.fee || 0;
  const isReceive = isReceiveTransaction(transaction.type);
  const isSend = !isReceive;

  // Maturity message body (preserved verbatim from the legacy implementation, now i18n'd).
  const maturityMessage = isCoinbase
    ? t('transactionDetails.banner.maturityCoinbase')
    : transaction.type === 'masternode'
      ? t('transactionDetails.banner.maturityMasternode')
      : t('transactionDetails.banner.maturityStaking');

  const conflictedMessage = t('transactionDetails.banner.conflicted');

  // Recipient addresses for send transactions (from backend extractRecipientAddressesFromTx).
  // Populated only for TxCategorySend; empty for receive / stake / masternode / etc.
  // When non-empty, we render one row per recipient under a "To" label.
  // When empty for a send (cache-loaded entry with storage miss), we fall back to
  // displaying transaction.address under a "Sent from" label — wtx.Address is the
  // wallet's funding address, NOT a recipient.
  const recipientAddresses = transaction.recipient_addresses ?? [];
  const isSendWithRecipients = isSend && recipientAddresses.length > 0;
  const isSendFallback = isSend && recipientAddresses.length === 0;

  // Address row label for the non-send cases handled by the single-address fallthrough.
  // Send transactions are handled by the dedicated Recipients block below (or the
  // "Sent from" fallback when recipient_addresses is empty).
  const addressLabel =
    transaction.type === 'consolidation'
      ? t('transactionDetails.addressLabel.consolidatedTo')
      : transaction.type === 'send_to_self'
        ? t('transactionDetails.addressLabel.toYourself')
        : isSendFallback
          ? t('transactionDetails.addressLabel.sentFrom')
          : isReceive
            ? t('transactionDetails.addressLabel.receivedWith')
            : t('transactionDetails.addressLabel.address');

  // For TXID legacy fallback: only the txid gets the legacy hardcoded URL.
  // Gated on `urlsResolved` so we never render the button (and never expose the
  // legacy fallback) while a sync from an empty store is still in flight —
  // see the `urlsResolved` state declaration above for the rationale.
  const isTxidValid = TXID_REGEX.test(transaction.txid);
  const txidExplorerUrls = !urlsResolved
    ? []
    : blockExplorerUrls.length > 0
      ? blockExplorerUrls
      : isTxidValid
        ? [{ url: LEGACY_EXPLORER_TX_FALLBACK, hostname: 'explorer.win.win' }]
        : [];

  return (
    <>
      {/* Overlay */}
      <div className="fixed inset-0 bg-black/60 z-50" onClick={onClose} />

      {/* Modal */}
      <div className="fixed inset-0 flex items-center justify-center z-50 pointer-events-none">
        <div
          ref={dialogRef}
          className="pointer-events-auto"
          role="dialog"
          aria-modal="true"
          aria-labelledby="tx-details-title"
          tabIndex={-1}
          style={{
            width: '560px',
            maxWidth: '90vw',
            maxHeight: '90vh',
            overflow: 'auto',
            backgroundColor: '#2f2f2f',
            border: '1px solid #3a3a3a',
            borderRadius: '8px',
            boxShadow: '0 8px 32px rgba(0, 0, 0, 0.8)',
            // Suppress the browser default focus ring on the dialog root. The root
            // carries tabIndex={-1} solely so the A3 focus-trap can place focus on
            // it programmatically (open + empty-children fallback) — the user never
            // interacts with the root directly, and all real interactive descendants
            // (X, Copy, Open-in-explorer) keep their own visible focus indicators.
            outline: 'none',
          }}
          onClick={(e) => e.stopPropagation()}
        >
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: '12px',
              padding: '16px 20px',
            }}
          >
            {/* Header */}
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                gap: '12px',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <ConfirmationRing
                  typeIcon={typeIcon}
                  confirmations={confirmations}
                  isConflicted={isConflicted}
                  isCoinstake={isCoinstake}
                  maturesIn={maturesIn}
                  size={40}
                />
                <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                  <span id="tx-details-title" style={{ fontSize: '14px', fontWeight: 600, color: '#ddd' }}>
                    {t('transactionDetails.title')}
                  </span>
                  {/* U3: type subtitle removed — body Type row is the single source of type information. */}
                </div>
              </div>
              <IconButton
                icon={<X size={14} />}
                title={t('transactionDetails.closeButton')}
                ariaLabel={t('transactionDetails.closeAriaLabel')}
                onClick={onClose}
              />
            </div>

            {/* Hero card */}
            <div style={cardStyleHero}>
              <span
                style={{
                  fontSize: '24px',
                  fontWeight: 600,
                  fontFamily: 'monospace',
                  color: amountColor,
                  letterSpacing: '0.5px',
                }}
              >
                {formatAmountWithSign(transaction.amount, formatAmount)}
              </span>
              {/* U4: bare status label; full confirmation count is surfaced via the title tooltip. */}
              <span title={confirmations > 0 ? t('transactionDetails.confirmationsCount', { count: confirmations }) : undefined}>
                <StatusPill tone={statusTone} label={statusText} />
              </span>
              <span
                style={{ fontSize: '11px', color: '#888', cursor: 'default' }}
                title={formattedDateUTC}
              >
                {formattedDate}
              </span>
            </div>

            {/* Maturity / Conflicted banners */}
            {(isCoinbase || isCoinstake) && maturesIn > 0 && (
              <Banner variant="warning" message={maturityMessage} />
            )}
            {isConflicted && <Banner variant="error" message={conflictedMessage} />}

            {/* Details ledger card */}
            <div style={cardStyle}>
              <div style={{ display: 'flex', flexDirection: 'column' }}>
                {/* Type */}
                <div style={rowStyle}>
                  <span style={labelStyle}>{t('transactionDetails.row.type')}</span>
                  <span style={valueStyle}>{typeLabel}</span>
                </div>

                {/* Source row dropped — was tautological with Type (Mined / Staking Reward / Masternode Reward). */}

                {/* From (receive transactions, when sender known) */}
                {isReceive && !isCoinbase && !isCoinstake && (
                  <>
                    <div style={rowStyle}>
                      <span style={labelStyle}>{t('transactionDetails.row.from')}</span>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <span
                          style={{
                            ...monoAddressStyle,
                            color: transaction.from_address ? '#e0e0e0' : '#888',
                            maxWidth: '320px',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                          title={
                            transaction.from_address
                              ? sanitizeText(transaction.from_address)
                              : t('transactionDetails.senderAddressUnknown')
                          }
                        >
                          {transaction.from_address
                            ? truncateAddress(sanitizeText(transaction.from_address), 12, 10)
                            : t('transactionDetails.unknown')}
                        </span>
                        {transaction.from_address && (
                          <IconButton
                            icon={<Copy size={12} />}
                            title={t('transactionDetails.copy.senderAddress')}
                            ariaLabel={t('transactionDetails.copy.senderAddress')}
                            onClick={() =>
                              handleCopy(transaction.from_address || '', 'fromAddress')
                            }
                          />
                        )}
                      </div>
                    </div>
                    {copiedField === 'fromAddress' && (
                      <div style={copyToastStyle}>{t('transactionDetails.copyToast')}</div>
                    )}
                  </>
                )}

                {/* Sent from — wallet's own funding-source address for SEND
                    transactions with populated recipient_addresses. Pairs
                    symmetrically with the per-recipient "To" rows below: user
                    sees both who they sent from and who they sent to. The
                    `isSendFallback` path (cache miss with no recipients) is
                    handled by the single-address row further below, so we
                    only render this here when recipients ARE present. */}
                {isSendWithRecipients && transaction.address && (
                  <>
                    <div style={rowStyle}>
                      <span style={labelStyle}>{t('transactionDetails.row.sentFrom')}</span>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <span
                          style={{
                            ...monoAddressStyle,
                            maxWidth: '320px',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                          title={sanitizeText(transaction.address)}
                        >
                          {truncateAddress(sanitizeText(transaction.address), 12, 10)}
                        </span>
                        <IconButton
                          icon={<Copy size={12} />}
                          title={t('transactionDetails.copy.senderAddress')}
                          ariaLabel={t('transactionDetails.copy.senderAddress')}
                          onClick={() => handleCopy(transaction.address, 'sentFromAddress')}
                        />
                      </div>
                    </div>
                    {copiedField === 'sentFromAddress' && (
                      <div style={copyToastStyle}>{t('transactionDetails.copyToast')}</div>
                    )}
                  </>
                )}

                {/* Recipients (send transactions with non-empty recipient_addresses).
                    Renders one row per external recipient extracted from the raw tx
                    outputs. Replaces the legacy single "To" row that misleadingly
                    showed wtx.Address (the wallet's funding address, not the recipient). */}
                {isSendWithRecipients && (
                  <>
                    {recipientAddresses.map((addr, idx) => {
                      const safeAddr = sanitizeText(addr);
                      const fieldKey = `recipient-${idx}`;
                      return (
                        <React.Fragment key={fieldKey}>
                          <div style={rowStyle}>
                            <span style={labelStyle}>
                              {recipientAddresses.length === 1
                                ? t('transactionDetails.row.to')
                                : t('transactionDetails.row.toNumbered', { index: idx + 1 })}
                            </span>
                            <div
                              style={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: '6px',
                              }}
                            >
                              <span
                                style={{
                                  ...monoAddressStyle,
                                  maxWidth: '320px',
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                  whiteSpace: 'nowrap',
                                }}
                                title={safeAddr}
                              >
                                {truncateAddress(safeAddr, 12, 10)}
                              </span>
                              <IconButton
                                icon={<Copy size={12} />}
                                title={t('transactionDetails.copy.recipientAddress')}
                                ariaLabel={t('transactionDetails.copy.recipientAddress')}
                                onClick={() => handleCopy(addr, fieldKey)}
                              />
                            </div>
                          </div>
                          {copiedField === fieldKey && (
                            <div style={copyToastStyle}>{t('transactionDetails.copyToast')}</div>
                          )}
                        </React.Fragment>
                      );
                    })}
                  </>
                )}

                {/* Single-address row (receive / stake / masternode / send-to-self / consolidation / send fallback).
                    Skipped for sends with populated recipient_addresses — the Recipients block above
                    handles those. Send fallback (cache-loaded entry, no raw tx) renders here under
                    a "Sent from" label so the funding address is at least visible. */}
                {transaction.address && !isSendWithRecipients && (
                  <>
                    <div style={rowStyle}>
                      <span style={labelStyle}>{addressLabel}</span>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <span
                          style={{
                            ...monoAddressStyle,
                            maxWidth: '320px',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                          title={sanitizeText(transaction.address)}
                        >
                          {truncateAddress(sanitizeText(transaction.address), 12, 10)}
                          {isReceive && isWatchOnly && (
                            <span style={{ color: '#888', fontFamily: 'inherit' }}>
                              {' '}
                              {t('transactionDetails.watchOnly')}
                            </span>
                          )}
                        </span>
                        <IconButton
                          icon={<Copy size={12} />}
                          title={t('transactionDetails.copy.address')}
                          ariaLabel={t('transactionDetails.copy.address')}
                          onClick={() => handleCopy(transaction.address, 'address')}
                        />
                      </div>
                    </div>
                    {copiedField === 'address' && (
                      <div style={copyToastStyle}>{t('transactionDetails.copyToast')}</div>
                    )}
                  </>
                )}

                {/* Label */}
                {/* Label row suppressed when external recipients are shown.
                    transaction.label is derived from wtx.Address (the wallet's
                    funding address for sends), so it does not correspond to the
                    recipients listed in the Recipients block above — showing
                    both side-by-side would be misleading. Contacts-aware
                    recipient labels are deferred to a follow-up task. */}
                {transaction.label && !isSendWithRecipients && (
                  <div style={rowStyle}>
                    <span style={labelStyle}>{t('transactionDetails.row.label')}</span>
                    <span style={valueStyle}>{sanitizeText(transaction.label)}</span>
                  </div>
                )}

                {/* Money rows separator — gated to mirror the union of conditions that actually
                    render a money row (Debit / Credit / Fee). Mirrors the Fee gate exactly so
                    we never render a lone divider above an empty money block. */}
                {(transaction.debit !== undefined && transaction.debit !== 0 && !isSelfTransfer) ||
                (transaction.credit !== undefined && transaction.credit !== 0) ||
                (isSend && fee > 0) ? (
                  <div style={dividerStyle} />
                ) : null}

                {/* Debit (send_to_self/consolidation suppressed; debit equals fee, shown below) */}
                {transaction.debit !== undefined &&
                  transaction.debit !== 0 &&
                  !isSelfTransfer && (
                    <div style={rowStyle}>
                      <span style={labelStyle}>{t('transactionDetails.row.debit')}</span>
                      <span
                        style={{ ...valueStyle, fontFamily: 'monospace', color: '#ff6666' }}
                      >
                        -{formatAmount(Math.abs(transaction.debit))}
                      </span>
                    </div>
                  )}

                {/* Credit */}
                {transaction.credit !== undefined && transaction.credit !== 0 && (
                  <div style={rowStyle}>
                    <span style={labelStyle}>{t('transactionDetails.row.credit')}</span>
                    <span style={{ ...valueStyle, fontFamily: 'monospace', color: '#27ae60' }}>
                      +{formatAmount(transaction.credit)}
                    </span>
                  </div>
                )}

                {/* B3: Fee row — rendered for sent transactions when transaction.fee is populated.
                    Per internal/wallet/CLAUDE.md, `transaction.fee` is currently only populated for
                    self-transfer/consolidation paths; ordinary outgoing sends leave it at 0, so the
                    row stays hidden for those until backend wiring is extended. Receive/reward/mined
                    types continue to omit the Fee row entirely (they structurally have no fee).
                    Rendering a fabricated "Fee: 0.00" when backend has no fee data would be
                    actively misleading, so the `fee > 0` gate is preserved from the original
                    implementation; only the visible label changes ("Network Fee" → "Fee"). */}
                {isSend && fee > 0 && (
                  <div style={rowStyle}>
                    <span style={labelStyle}>{t('transactionDetails.row.fee')}</span>
                    <span style={{ ...valueStyle, fontFamily: 'monospace', color: '#888' }}>
                      -{formatAmount(fee)}
                    </span>
                  </div>
                )}

                {/* Final divider + Net Amount */}
                <div style={dividerStyle} />
                <div style={rowStyle}>
                  <span style={{ fontSize: '13px', fontWeight: 600, color: '#ddd' }}>
                    {t('transactionDetails.row.netAmount')}
                  </span>
                  <span
                    style={{
                      fontSize: '14px',
                      fontWeight: 600,
                      fontFamily: 'monospace',
                      color: amountColor,
                    }}
                  >
                    {formatAmountWithSign(transaction.amount, formatAmount)}
                  </span>
                </div>
              </div>
            </div>

            {/* Optional Message card */}
            {transaction.comment && (
              <div style={cardStyle}>
                <div style={sectionTitleStyle}>{t('transactionDetails.section.message')}</div>
                <span
                  style={{
                    fontSize: '12px',
                    color: '#ddd',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                  }}
                >
                  {sanitizeText(transaction.comment)}
                </span>
              </div>
            )}

            {/* Transaction ID card */}
            <div style={cardStyle}>
              <div style={sectionTitleStyle}>{t('transactionDetails.section.transactionId')}</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                {/* TXID row */}
                <div style={rowStyle}>
                  <span style={labelStyle}>{t('transactionDetails.row.txid')}</span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                    <span style={monoAddressStyle} title={transaction.txid}>
                      {truncateAddress(transaction.txid, 12, 10)}
                    </span>
                    <IconButton
                      icon={<Copy size={12} />}
                      title={t('transactionDetails.copy.transactionId')}
                      ariaLabel={t('transactionDetails.copy.transactionId')}
                      onClick={() => handleCopy(transaction.txid, 'txid')}
                    />
                    {isTxidValid && (
                      <ExplorerButton
                        value={transaction.txid}
                        urls={txidExplorerUrls}
                        title={t('transactionDetails.viewInExplorer')}
                        ariaLabel={t('transactionDetails.viewInExplorer')}
                      />
                    )}
                  </div>
                </div>
                {copiedField === 'txid' && <div style={copyToastStyle}>{t('transactionDetails.copyToast')}</div>}

                {/* Block Height row */}
                {/*
                  Block Height + Block Hash rows do NOT render explorer buttons.
                  `blockExplorerUrls` comes from `strThirdPartyTxUrls`, which is a
                  list of TRANSACTION URL templates (the `%s` placeholder is the
                  txid). Reusing those templates with a block height or block hash
                  would produce broken links (e.g. `/tx/<height>`). Block-specific
                  explorer URLs require a separate `strThirdPartyBlockUrls` setting
                  which does not yet exist. Copy buttons are still rendered so the
                  user can manually paste the value into a block explorer.
                */}
                {transaction.block_height !== undefined && transaction.block_height > 0 && (
                  <div style={rowStyle}>
                    <span style={labelStyle}>{t('transactionDetails.row.blockHeight')}</span>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                      <span style={{ ...valueStyle, fontFamily: 'monospace' }}>
                        {transaction.block_height.toLocaleString()}
                      </span>
                    </div>
                  </div>
                )}

                {/* Block Hash row */}
                {transaction.block_hash && (
                  <>
                    <div style={rowStyle}>
                      <span style={labelStyle}>{t('transactionDetails.row.blockHash')}</span>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <span style={monoAddressStyle} title={transaction.block_hash}>
                          {truncateAddress(transaction.block_hash, 12, 10)}
                        </span>
                        <IconButton
                          icon={<Copy size={12} />}
                          title={t('transactionDetails.copy.blockHash')}
                          ariaLabel={t('transactionDetails.copy.blockHash')}
                          onClick={() =>
                            handleCopy(transaction.block_hash || '', 'blockhash')
                          }
                        />
                      </div>
                    </div>
                    {copiedField === 'blockhash' && (
                      <div style={copyToastStyle}>{t('transactionDetails.copyToast')}</div>
                    )}
                  </>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};
