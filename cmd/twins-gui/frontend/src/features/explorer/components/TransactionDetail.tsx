import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import type { ExplorerTransaction, TxInput, TxOutput } from '@/store/slices/explorerSlice';
import { sortOutputs, groupOutputs, type RenderItem } from '../utils/outputDisplay';
import { OpReturnDataDialog } from './OpReturnDataDialog';
import {
  AlertTriangle, ArrowLeft, Check, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Copy, Crown, Eye,
  FileText, Flag, FlaskConical, Hash, Key, Pickaxe, RefreshCw, Send, Server, Shield, Sparkles,
  TrendingUp, Wallet,
} from 'lucide-react';
import { StatusPill } from '@/shared/components/StatusPill';
import { IconButton } from '@/shared/components/IconButton';
import { PillButton } from '@/shared/components/PillButton';
import { RefreshCountdown } from '@/shared/components/RefreshCountdown';
import { UnitBadge } from '@/shared/components/UnitBadge';
import { truncateAddress } from '@/shared/utils/format';
import { writeToClipboard } from '@/shared/utils/clipboard';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';

// Local date formatters with seconds — copied verbatim from BlockDetail.tsx to
// keep the tx detail view's date convention identical to the block detail view
// (long `MMM DD, YYYY, HH:MM:SS GMT+offset` form with UTC variant in tooltip).
// The shared formatTransactionDate / formatTransactionDateUTC in
// transactionIcons.ts produce HH:MM (no seconds) — intentionally kept for
// Transactions / Overview / details dialogs by design. The hero requires
// seconds for precise tx time and a hover tooltip with the UTC equivalent.
function formatTxDateLocal(timestamp: number | string | Date): string {
  return new Date(timestamp).toLocaleString('en-US', {
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZoneName: 'short',
  });
}

function formatTxDateUTC(timestamp: number | string | Date): string {
  return new Date(timestamp).toLocaleString('en-US', {
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZone: 'UTC',
    timeZoneName: 'short',
  });
}

interface TransactionDetailProps {
  transaction: ExplorerTransaction | null;
  isLoading: boolean;
  onAddressClick: (address: string) => void;
  onTxClick: (txid: string) => void;
  onBlockClick: (query: string) => void;
  onBack: () => void;
  // Signature widened to support silent auto-refresh from RefreshCountdown
  // (matches BlockDetail's contract). The 60s tick fires onRefresh({ silent:
  // true }) and awaits the returned promise via .finally() for the in-flight
  // lock.
  onRefresh: (options?: { silent?: boolean }) => Promise<void> | void;
  isAnyLoading: boolean;
  // Ordered list of txids in the parent block. Used by the new Prev/Next
  // pills in the hero header to compute sibling navigation targets. Null
  // when the parent block is unknown (mempool tx) or the background block
  // fetch is still in flight — both Prev/Next pills render disabled in that
  // state. Populated by ExplorerPage's siblingTxids effect.
  siblingTxids?: string[] | null;
  // Called when the user clicks the Prev/Next pills with the resolved
  // sibling txid. The parent (ExplorerPage) wires this to its existing
  // handleTxClick → fetchTransaction path; the same-view check inside
  // fetchTransaction recognizes tx→tx as peer navigation and does NOT push
  // the prior tx onto the parent stack, so back-navigation still walks the
  // pre-sibling-chain ancestry.
  onSiblingTxClick?: (txid: string) => void;
}

// Hero card chrome — bare card (not <DashboardCard>) so we own the internal
// layout. Mirrors BlockDetail's heroCardStyle 1:1.
const heroCardStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  padding: '16px 20px',
  display: 'flex',
  flexDirection: 'column',
  gap: '12px',
};

// Summary column inside hero bottom row. 4 columns: TOTAL OUTPUT / I/O COUNT
// / SIZE / FEE. `flex: '1 1 170px'` gives each column a 170px basis (matches
// BlockDetail's reward columns) and lets columns grow / wrap to 2x2 below
// ~728px viewport (4 * 170 + 3 * 16 gap = 728px).
// `minWidth: 0` deliberately omitted (vs. the typical flex-child idiom): in
// non-TWINS units (µTWINS / mTWINS) the Total Output value can be 14+ digit
// monospace numbers (~280px at 20px font) and we want the column to grow to
// its intrinsic content width rather than clip behind the next column.
// Combined with `whiteSpace: 'nowrap'` on the amount values and parent's
// `flexWrap: 'wrap'`, columns either fit on one row (TWINS / small amounts)
// or fall back to 2x2 / stacked layout (µTWINS / huge amounts) without
// any inter-column collision.
const summaryColumnStyle: React.CSSProperties = {
  flex: '1 1 170px',
  display: 'flex',
  flexDirection: 'column',
  gap: '4px',
};

const summaryLabelStyle: React.CSSProperties = {
  fontSize: '11px',
  fontWeight: 500,
  color: '#888',
};

// TOTAL OUTPUT — large, green, monospace — same `rewardAmountStyle` token
// from BlockDetail. This is the "money figure" emphasis.
// `whiteSpace: 'nowrap'` is load-bearing: in µTWINS mode the value can be
// 14+ digit comma-separated numbers (~280px at 20px font), and without
// nowrap the browser may insert a line break at a comma — and the
// surrounding `summaryColumnStyle` (which lacks `minWidth: 0`) relies on
// the intrinsic content width to grow the column.
const summaryAmountLargeStyle: React.CSSProperties = {
  fontSize: '20px',
  fontWeight: 600,
  color: '#27ae60',
  fontFamily: 'monospace',
  whiteSpace: 'nowrap',
};

// Smaller secondary metrics (I/O count, Size). 14px monospace.
const summaryAmountSmallStyle: React.CSSProperties = {
  fontSize: '14px',
  fontWeight: 600,
  color: '#ddd',
  fontFamily: 'monospace',
  whiteSpace: 'nowrap',
};

// ============================================================================
// Badge system — replaces the prior RoleChip pill primitive.
//
// Each badge collapses to a small dot+glyph (18px circle, glyph 9px) by
// default. Clicking expands inline to a pill with dot+glyph+label, click again
// to collapse. Multi-expand: any badge can be expanded independently; state
// lives per-row as a Set<badgeId>. Hover shows native tooltip on both states.
//
// Top-row badge strip separates "semantic signals" (role + overlay flags) from
// the main row (address + amount + actions) and the optional 3rd tech metadata
// row (vout / script_type / spent / From: txid).
//
// CRITICAL: `textTransform: 'none'` on expanded label — chip text is authored
// in i18n with proper casing (Title Case for labels, M-ИЗ-N Cyrillic for RU
// multisig). CSS uppercase would mis-render Cyrillic (see µ→Μ→M Unicode bug
// documented in `cmd/twins-gui/frontend/CLAUDE.md` round-2 fix for
// TransactionDetail.tsx 2026-05-27 and the UnitBadge docblock).
// ============================================================================
interface BadgeDef {
  color: string;
  icon: React.ReactNode;
  labelKey: string;
  tooltipKey: string;
}

// Role badges — one per OutputRole* constant. external_payment now gets a
// badge (per user decision: every output always has ≥1 badge for consistency).
//
// Palette differentiation rationale:
// - block_marker stays #888 (neutral system marker placeholder).
// - external_payment moved #aaa → #d4d4d4 (lighter neutral) so it's distinct
//   from block_marker on the same tx page.
// - dev_fund moved #ff9966 → #e6a565 (warm honey) so it's distinct from
//   the coinbase overlay (#ff9966 orange).
// - change moved #9e9eff → #bb88dd (lighter purple) so it's distinct from
//   the MINE overlay (#9e9eff violet, unchanged) when both render in the
//   same row.
// - self_send moved #9e9eff → #5fb3a3 (muted teal) so it's distinct from
//   both change purple and MINE violet. Wallet glyph kept.
// - multisig moved #aa88ff → #9c5bd0 (deeper purple) so it's distinct from
//   premine (#aa66ff also purple).
// - mining_reward stays #fbbf24 (gold).
// - premine stays #aa66ff (purple).
// - nonstandard stays #ff6666 (red).
const roleBadgeDefs: Record<string, BadgeDef> = {
  block_marker: { color: '#888', icon: <Flag size={9} />, labelKey: 'explorer.tx.role.blockMarker.label', tooltipKey: 'explorer.tx.role.blockMarker.tooltip' },
  stake_return: { color: '#27ae60', icon: <TrendingUp size={9} />, labelKey: 'explorer.tx.role.stakeReturn.label', tooltipKey: 'explorer.tx.role.stakeReturn.tooltip' },
  masternode_payment: { color: '#6699cc', icon: <Server size={9} />, labelKey: 'explorer.tx.role.masternodePayment.label', tooltipKey: 'explorer.tx.role.masternodePayment.tooltip' },
  dev_fund: { color: '#e6a565', icon: <FlaskConical size={9} />, labelKey: 'explorer.tx.role.devFund.label', tooltipKey: 'explorer.tx.role.devFund.tooltip' },
  external_payment: { color: '#d4d4d4', icon: <Send size={9} />, labelKey: 'explorer.tx.role.externalPayment.label', tooltipKey: 'explorer.tx.role.externalPayment.tooltip' },
  change: { color: '#bb88dd', icon: <RefreshCw size={9} />, labelKey: 'explorer.tx.role.change.label', tooltipKey: 'explorer.tx.role.change.tooltip' },
  self_send: { color: '#5fb3a3', icon: <Wallet size={9} />, labelKey: 'explorer.tx.role.selfSend.label', tooltipKey: 'explorer.tx.role.selfSend.tooltip' },
  data_carrier: { color: '#bbb', icon: <FileText size={9} />, labelKey: 'explorer.tx.role.dataCarrier.label', tooltipKey: 'explorer.tx.role.dataCarrier.tooltip' },
  mining_reward: { color: '#fbbf24', icon: <Pickaxe size={9} />, labelKey: 'explorer.tx.role.miningReward.label', tooltipKey: 'explorer.tx.role.miningReward.tooltip' },
  premine: { color: '#aa66ff', icon: <Crown size={9} />, labelKey: 'explorer.tx.role.premine.label', tooltipKey: 'explorer.tx.role.premine.tooltip' },
  nonstandard: { color: '#ff6666', icon: <AlertTriangle size={9} />, labelKey: 'explorer.tx.role.nonstandard.label', tooltipKey: 'explorer.tx.role.nonstandard.tooltip' },
  multisig: { color: '#9c5bd0', icon: <Shield size={9} />, labelKey: 'explorer.tx.role.multisig.label', tooltipKey: 'explorer.tx.role.multisig.tooltip' },
};

// Overlay flag badges — independent of role. mine appears on outputs when
// is_mine && !is_change && role !== 'self_send' (change and self_send role
// badges already carry the wallet-mine signal — double-badging is visual
// noise). dust appears when is_dust. kernel/coinbase appear on inputs.
const overlayBadgeDefs: Record<'mine' | 'dust' | 'kernel' | 'coinbase', BadgeDef> = {
  mine: { color: '#9e9eff', icon: <Wallet size={9} />, labelKey: 'explorer.tx.input.mineChip', tooltipKey: 'explorer.tx.input.mineTooltip' },
  dust: { color: '#888', icon: <Sparkles size={9} />, labelKey: 'explorer.tx.dust.chip', tooltipKey: 'explorer.tx.dust.tooltip' },
  kernel: { color: '#27ae60', icon: <Key size={9} />, labelKey: 'explorer.tx.input.kernelChip', tooltipKey: 'explorer.tx.input.kernelTooltip' },
  coinbase: { color: '#ff9966', icon: <Sparkles size={9} />, labelKey: 'explorer.tx.input.coinbaseLabel', tooltipKey: 'explorer.tx.input.coinbaseTooltip' },
};

interface BadgeInstance {
  id: string;
  color: string;
  icon: React.ReactNode;
  label: string; // pre-resolved (allows wire-derived overrides like multisig M-OF-N)
  tooltip: string;
  // Phase 2.2: when true, badge starts rendered as the expanded pill (label
  // visible by default). When false/undefined, badge starts as a collapsed dot.
  // Role badges use true (output identity is scan-critical); overlay badges
  // (mine/dust/kernel/coinbase) use false (flags are tertiary, dots suffice).
  // User clicks toggle the state via the existing useExpandedBadges hook.
  defaultExpanded?: boolean;
}

interface BadgeProps {
  badge: BadgeInstance;
  expanded: boolean;
  onToggle: () => void;
}

const Badge: React.FC<BadgeProps> = ({ badge, expanded, onToggle }) => (
  <button
    type="button"
    title={badge.tooltip}
    aria-label={badge.label}
    aria-expanded={expanded}
    onClick={onToggle}
    style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: expanded ? '4px' : 0,
      padding: expanded ? '2px 8px 2px 3px' : '2px',
      borderRadius: '999px',
      backgroundColor: `${badge.color}26`, // 15% alpha — matches StatusPill formula
      border: `1px solid ${badge.color}66`, // 40% alpha
      cursor: 'pointer',
      height: '20px',
      flexShrink: 0,
      lineHeight: 1,
      outline: 'none',
    }}
  >
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '14px',
        height: '14px',
        borderRadius: '50%',
        backgroundColor: badge.color,
        color: '#fff',
        flexShrink: 0,
      }}
    >
      {badge.icon}
    </span>
    {expanded && (
      <span
        style={{
          fontSize: '10px',
          fontWeight: 500,
          color: badge.color,
          letterSpacing: '0.3px',
          whiteSpace: 'nowrap',
          textTransform: 'none',
        }}
      >
        {badge.label}
      </span>
    )}
  </button>
);

// Shared style tokens for the Phase 2.2 footer-based layout.
//
// Row structure (revised in m-tx-details-io-redesign-v2 Phase 2.2):
//   1. mainRowStyle  — address+amount (hero zone, no competing weight above/below)
//   2. rowDividerStyle — `1px #3a3a3a` separator
//   3. footerRowStyle (2-zone flex): footerLeftZoneStyle (badges, flex-wrap) +
//      footerRightZoneStyle (tech text, right-aligned + ellipsis + borderLeft).
//
// Previous 3-row caracass tokens (badgeStripStyle, techRowStyle) removed —
// no remaining callers. Coinbase input + block_marker special cases own
// their layout directly.
const rowDividerStyle: React.CSSProperties = {
  borderBottom: '1px solid #3a3a3a',
};

const mainRowStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  gap: '8px',
  paddingTop: '8px',
  paddingBottom: '4px',
};

const footerRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '12px',
  paddingTop: '6px',
};

const footerLeftZoneStyle: React.CSSProperties = {
  flex: 1,
  display: 'flex',
  flexWrap: 'wrap',
  gap: '4px',
  alignItems: 'center',
  minWidth: 0,
};

const footerRightZoneStyle: React.CSSProperties = {
  flex: 1,
  textAlign: 'right',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
  fontSize: '11px',
  color: '#888',
  fontFamily: 'monospace',
  paddingLeft: '12px',
  borderLeft: '1px solid #3a3a3a',
};

// Custom hook for managing per-row badge expansion state (multi-expand).
// Phase 2.2: badges have a `defaultExpanded` field — the hook tracks which
// badges the user has CLICKED, and the final expanded state is the XOR of
// (default ↔ clicked). Role badges default expanded so the user sees the
// output identity at scan-time; overlay badges (mine/dust/kernel/coinbase)
// default collapsed as dots.
function useExpandedBadges() {
  const [toggled, setToggled] = useState<Set<string>>(new Set());
  const toggle = useCallback((id: string) => {
    setToggled((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);
  const isExpanded = useCallback(
    (badge: BadgeInstance) =>
      badge.defaultExpanded ? !toggled.has(badge.id) : toggled.has(badge.id),
    [toggled],
  );
  return { isExpanded, toggle };
}

// Shared hover-border flip used by both <InputRow> and <OutputRow>. Declared
// at module level so both rows share the same zero-re-render convention
// (matches the BlockDetail tx row pattern).
const hoverIn = (e: React.MouseEvent<HTMLDivElement>) => {
  e.currentTarget.style.borderColor = '#444';
};
const hoverOut = (e: React.MouseEvent<HTMLDivElement>) => {
  e.currentTarget.style.borderColor = 'transparent';
};

// Fee — same size as I/O count + Size, but regular weight and full-precision
// 8-decimal display. Fee is informational, not the primary metric — the
// non-bold rendering carries that signal.
const summaryAmountFeeStyle: React.CSSProperties = {
  fontSize: '14px',
  color: '#ddd',
  fontFamily: 'monospace',
  whiteSpace: 'nowrap',
};

const valueStyle: React.CSSProperties = {
  fontSize: '12px',
  color: '#ddd',
};

const monoValueStyle: React.CSSProperties = {
  ...valueStyle,
  fontFamily: 'monospace',
};

const emptyStateStyle: React.CSSProperties = {
  padding: '32px',
  textAlign: 'center',
  color: '#888',
  fontSize: '12px',
};

const sectionTitleStyle: React.CSSProperties = {
  fontSize: '13px',
  fontWeight: 600,
  color: '#ddd',
};

// `minHeight: 0` is load-bearing: each I/O card is a CSS-grid item, and grid
// items default to `min-height: auto` which lets them expand to fit their
// content. Without this, the inner `scrollBodyStyle: flex: 1` cannot
// constrain because its parent (the card) grows unbounded with the row list,
// pushing the totals row off the bottom of the page and suppressing the
// internal scroll bar. With `minHeight: 0` the card respects the grid cell's
// allocated height and the inner scroll body activates correctly.
const ioCardStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  padding: '16px 20px',
  display: 'flex',
  flexDirection: 'column',
  gap: '8px',
  minHeight: 0,
};

// Sticky header inside each I/O card. Matches the BlockDetail Transactions
// card sticky pattern: `position: sticky, top: 0` so the header text + count
// stay visible while the row list scrolls inside the card. `backgroundColor`
// is identical to the card chrome so the sticky strip blends seamlessly.
const stickyHeaderStyle: React.CSSProperties = {
  position: 'sticky',
  top: 0,
  zIndex: 5,
  backgroundColor: '#2f2f2f',
  paddingBottom: '8px',
  borderBottom: '1px solid #3a3a3a',
  ...sectionTitleStyle,
};

// Scroll container inside each I/O card. `flex: 1, minHeight: 0` lets this
// region fill the available card height so the I/O cards stretch to the full
// page height (no more empty band below the row list). The internal scroll
// activates only when the row list overflows the card. `padding: 2px 0`
// gives the 1px row focus border breathing room so the keyboard-focus
// indicator on the first/last visible row is not clipped by the scroll
// viewport edge (matches BlockDetail Transactions card precedent).
const scrollBodyStyle: React.CSSProperties = {
  flex: 1,
  minHeight: 0,
  overflowY: 'auto',
  padding: '2px 0',
};

const itemRowCardStyle: React.CSSProperties = {
  backgroundColor: '#2a2a2a',
  border: '1px solid transparent',
  borderRadius: '6px',
  padding: '8px 12px',
  transition: 'border-color 0.15s',
};

const totalsRowStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  marginTop: '8px',
  paddingTop: '8px',
  borderTop: '1px solid #3a3a3a',
  fontSize: '12px',
  color: '#888',
};

const totalsValueStyle: React.CSSProperties = {
  color: '#27ae60',
  fontFamily: 'monospace',
};

// ============================================================================
// <InputRow> — extracted from inline transaction.inputs.map(). Coinbase inputs
// render as a single muted-warning Sparkles-icon line with the i18n "Newly
// minted coins" label. Non-coinbase inputs render the canonical address row
// with optional KERNEL and MINE chips driven by backend flags.
// ============================================================================
interface InputRowProps {
  input: TxInput;
  onAddressClick: (address: string) => void;
  onTxClick: (txid: string) => void;
  formatAmount: (value: number, withUnit?: boolean) => string;
  t: TFunction;
}

const InputRow: React.FC<InputRowProps> = ({
  input,
  onAddressClick,
  onTxClick,
  formatAmount,
  t,
}) => {
  const hasAddress = !!input.address;
  const isZero = input.amount === 0;
  const { isExpanded, toggle } = useExpandedBadges();

  // Build badges. Inputs do NOT have a "role" — only overlay-flag badges:
  // coinbase (special), kernel (PoS staker), mine (wallet-owned UTXO).
  // All overlays default collapsed (small dots in footer); user clicks toggle
  // to expanded pill.
  const badges: BadgeInstance[] = [];
  if (input.is_coinbase) {
    badges.push({
      id: 'overlay:coinbase',
      color: overlayBadgeDefs.coinbase.color,
      icon: overlayBadgeDefs.coinbase.icon,
      label: t(overlayBadgeDefs.coinbase.labelKey),
      tooltip: t(overlayBadgeDefs.coinbase.tooltipKey),
    });
  } else {
    if (input.is_coinstake_kernel) {
      badges.push({
        id: 'overlay:kernel',
        color: overlayBadgeDefs.kernel.color,
        icon: overlayBadgeDefs.kernel.icon,
        label: t(overlayBadgeDefs.kernel.labelKey),
        tooltip: t(overlayBadgeDefs.kernel.tooltipKey),
      });
    }
    if (input.is_mine) {
      badges.push({
        id: 'overlay:mine',
        color: overlayBadgeDefs.mine.color,
        icon: overlayBadgeDefs.mine.icon,
        label: t(overlayBadgeDefs.mine.labelKey),
        tooltip: t(overlayBadgeDefs.mine.tooltipKey),
      });
    }
  }

  // Coinbase: 1-row compact layout (Phase 2.2). Coinbase badge dot inline at
  // the start of the main row, followed by the muted italic "Newly minted
  // coins" placeholder, then amount. No footer — there's no parent tx to
  // navigate to and the single overlay badge sits inline with the label.
  if (input.is_coinbase) {
    const coinbaseBadge = badges[0]; // exactly one (overlay:coinbase) built above
    return (
      <div style={itemRowCardStyle} onMouseEnter={hoverIn} onMouseLeave={hoverOut}>
        <div style={mainRowStyle}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flex: 1, minWidth: 0 }}>
            {coinbaseBadge && (
              <Badge
                badge={coinbaseBadge}
                expanded={isExpanded(coinbaseBadge)}
                onToggle={() => toggle(coinbaseBadge.id)}
              />
            )}
            <span
              title={t('explorer.tx.input.coinbaseTooltip')}
              style={{
                ...monoValueStyle,
                color: '#888',
                fontStyle: 'italic',
                flex: 1,
                minWidth: 0,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {t('explorer.tx.input.coinbaseLabel')}
            </span>
          </div>
          <span style={{ ...monoValueStyle, color: isZero ? '#888' : '#27ae60', flexShrink: 0 }}>
            {formatAmount(input.amount, false)}
          </span>
        </div>
      </div>
    );
  }

  // Non-coinbase: 2-row layout (Phase 2.2, refined in Phase 2.2.y). Main row
  // order: [address (flex 1)] [amount (right-aligned)] [Eye trailing,
  // conditional on hasAddress]. Copy IconButton removed — browser-native
  // text-select-and-copy still works on the truncated span via the `title`
  // tooltip, and Eye → address details provides indirect access. Footer is
  // 2-zone flex — left zone shows overlay badges (default collapsed dots,
  // click expands), right zone shows clickable `From: <txid>:<vout>`.
  return (
    <div style={itemRowCardStyle} onMouseEnter={hoverIn} onMouseLeave={hoverOut}>
      <div style={mainRowStyle}>
        {hasAddress ? (
          <span
            title={input.address}
            style={{
              ...monoValueStyle,
              color: '#ddd',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              minWidth: 0,
              flex: 1,
            }}
          >
            {truncateAddress(input.address, 10, 10)}
          </span>
        ) : (
          <span style={{ ...monoValueStyle, color: '#888', flex: 1, minWidth: 0 }}>—</span>
        )}
        <span style={{ ...monoValueStyle, color: isZero ? '#888' : '#27ae60', flexShrink: 0 }}>
          {formatAmount(input.amount, false)}
        </span>
        {hasAddress && (
          <IconButton
            size={26}
            icon={<Eye size={12} />}
            title={t('explorer.viewAddressDetails')}
            ariaLabel={t('explorer.viewAddressDetails')}
            onClick={() => onAddressClick(input.address)}
          />
        )}
      </div>
      <div style={rowDividerStyle} />
      <div style={footerRowStyle}>
        <div style={footerLeftZoneStyle}>
          {badges.map((b) => (
            <Badge key={b.id} badge={b} expanded={isExpanded(b)} onToggle={() => toggle(b.id)} />
          ))}
        </div>
        <div
          style={{ ...footerRightZoneStyle, cursor: 'pointer' }}
          role="button"
          tabIndex={0}
          onClick={() => onTxClick(input.txid)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              onTxClick(input.txid);
            }
          }}
          title={`${input.txid}:${input.vout}`}
        >
          {t('explorer.tx.input.fromLabel')}: {truncateAddress(input.txid, 10, 10)}:{input.vout}
        </div>
      </div>
    </div>
  );
};

// ============================================================================
// <OutputRow> — extracted in MR !732, now consumed through the renderItems
// pipeline (sortOutputs + groupOutputs → RenderItem switch in the JSX below).
// Dispatches OP_RETURN and multisig as special-case render paths; falls
// through to the canonical address + role chip + DUST/MINE overlays + amount
// layout for the other 10 roles. (Spent) indicator removed unconditionally —
// backend is_spent is hardcoded false (research §F7 gap #4, deferred via
// l-tx-explorer-spent-flag).
// ============================================================================
interface OutputRowProps {
  output: TxOutput;
  onAddressClick: (address: string) => void;
  formatAmount: (value: number, withUnit?: boolean) => string;
  t: TFunction;
  // R7: opens the OP_RETURN payload modal. Only invoked by the `data_carrier`
  // render branch when at least one of hex/ascii is non-empty. Optional so
  // group wrappers (StakeSplitGroup, DustCollapseGroup) that never wrap a
  // data_carrier output can omit it.
  openDataDialog?: (hex: string, ascii: string) => void;
  // Suppress badges when the row is INSIDE a group wrapper that already
  // labels the badge in question. StakeSplitGroup passes `suppressRoleBadge`
  // because the group header already says "Stake Return (split into N parts)".
  // DustCollapseGroup passes `suppressDustBadge` because the group label says
  // "Dust outputs".
  suppressRoleBadge?: boolean;
  suppressDustBadge?: boolean;
}

const OutputRow: React.FC<OutputRowProps> = ({
  output,
  onAddressClick,
  formatAmount,
  t,
  openDataDialog,
  suppressRoleBadge = false,
  suppressDustBadge = false,
}) => {
  const isZero = output.amount === 0;
  const role = output.role ?? 'nonstandard';
  const roleDef = roleBadgeDefs[role] ?? roleBadgeDefs.nonstandard;
  const { isExpanded, toggle } = useExpandedBadges();

  // Multisig: first address from addresses[] + M-OF-N label override.
  const isMultisig = role === 'multisig' && !!output.addresses && output.addresses.length > 0;
  const renderedAddress = isMultisig ? output.addresses![0] : output.address;

  // Block marker (PoS vout=0 nonstandard zero-value sentinel) — render as a
  // compact single-line row instead of the full 3-row caracass. The marker
  // carries no user-actionable information (no address, value=0, protocol
  // artifact), so reserving ~80px of vertical space for it is wasteful. The
  // badge dot still opens to show "Block Marker" label on click.
  if (role === 'block_marker' && !renderedAddress) {
    const blockMarkerBadge: BadgeInstance = {
      id: 'role:block_marker',
      color: roleDef.color,
      icon: roleDef.icon,
      label: t(roleDef.labelKey),
      tooltip: t(roleDef.tooltipKey),
    };
    return (
      <div
        style={{
          backgroundColor: '#252525',
          border: '1px solid transparent',
          borderRadius: '6px',
          padding: '4px 12px',
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          transition: 'border-color 0.15s',
        }}
        onMouseEnter={hoverIn}
        onMouseLeave={hoverOut}
      >
        <Badge
          badge={blockMarkerBadge}
          expanded={isExpanded(blockMarkerBadge)}
          onToggle={() => toggle(blockMarkerBadge.id)}
        />
        <span
          style={{
            fontSize: '11px',
            color: '#666',
            fontFamily: 'monospace',
            flex: 1,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          vout={output.index} · {output.script_type}
        </span>
        <span style={{ ...monoValueStyle, color: '#666', fontSize: '11px', flexShrink: 0 }}>
          {formatAmount(output.amount, false)}
        </span>
      </div>
    );
  }

  // Build badges. Role badge always present (per user decision: every output
  // has ≥1 badge for consistency) unless suppressed by a group wrapper.
  // Phase 2.2: role badges default to EXPANDED (label visible) — output role
  // is the primary identity signal and must be readable at scan-time.
  const badges: BadgeInstance[] = [];
  if (!suppressRoleBadge) {
    badges.push({
      id: `role:${role}`,
      color: roleDef.color,
      icon: roleDef.icon,
      label: isMultisig
        ? `${output.required_sigs ?? 0}-OF-${output.addresses!.length}`
        : t(roleDef.labelKey),
      tooltip: t(roleDef.tooltipKey),
      defaultExpanded: true,
    });
  }
  // MINE overlay: only when is_mine AND NOT change AND NOT self_send. Both
  // change and self_send role badges already imply wallet ownership (change
  // is auto-generated leftover; self_send is a deliberate send to one of your
  // own addresses) — adding a MINE overlay produces two near-identical violet
  // dots in a row, which read as a visual duplicate to the user.
  if (output.is_mine && !output.is_change && role !== 'self_send') {
    badges.push({
      id: 'overlay:mine',
      color: overlayBadgeDefs.mine.color,
      icon: overlayBadgeDefs.mine.icon,
      label: t(overlayBadgeDefs.mine.labelKey),
      tooltip: t(overlayBadgeDefs.mine.tooltipKey),
    });
  }
  if (output.is_dust && !suppressDustBadge) {
    badges.push({
      id: 'overlay:dust',
      color: overlayBadgeDefs.dust.color,
      icon: overlayBadgeDefs.dust.icon,
      label: t(overlayBadgeDefs.dust.labelKey),
      tooltip: t(overlayBadgeDefs.dust.tooltipKey),
    });
  }

  // Build main row content. Three branches:
  //   1. data_carrier — preview text (clickable to open modal) instead of address
  //   2. has address (incl. multisig first key) — address + copy/eye actions
  //   3. no address (block_marker, nonstandard without extractable addr) — em-dash
  let mainContent: React.ReactNode;
  if (role === 'data_carrier') {
    const hex = output.data_hex ?? '';
    const ascii = output.data_ascii ?? '';
    const preview = ascii.length > 0 ? ascii : hex.length > 40 ? `${hex.slice(0, 40)}…` : hex;
    const hasPayload = hex.length > 0 || ascii.length > 0;
    const handlePreviewActivate = () => {
      if (hasPayload && openDataDialog) {
        openDataDialog(hex, ascii);
      }
    };
    mainContent = (
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0, flex: 1 }}>
        {hasPayload && openDataDialog ? (
          <span
            role="button"
            tabIndex={0}
            title={t('explorer.tx.dataModal.viewFullTooltip')}
            aria-label={t('explorer.tx.dataModal.viewFullAriaLabel')}
            onClick={handlePreviewActivate}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                handlePreviewActivate();
              }
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.color = '#ddd';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = '#aaa';
            }}
            style={{
              ...monoValueStyle,
              color: '#aaa',
              cursor: 'pointer',
              textDecoration: 'underline dotted',
              textUnderlineOffset: '2px',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              minWidth: 0,
              flex: 1,
            }}
          >
            {preview || '—'}
          </span>
        ) : (
          <span
            title={hex || ascii}
            style={{
              ...monoValueStyle,
              color: '#888',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              minWidth: 0,
              flex: 1,
            }}
          >
            {preview || '—'}
          </span>
        )}
      </div>
    );
  } else if (renderedAddress) {
    // Phase 2.2.y: mainContent is just the truncated address span — Copy
    // IconButton removed; Eye IconButton moved to trailing edge after amount
    // (see the main row JSX below).
    mainContent = (
      <span
        title={renderedAddress}
        style={{
          ...monoValueStyle,
          color: '#ddd',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          minWidth: 0,
          flex: 1,
        }}
      >
        {truncateAddress(renderedAddress, 10, 10)}
      </span>
    );
  } else {
    // No-address branch: render a descriptive muted-italic placeholder. For
    // nonstandard outputs we explicitly say so (the role badge above already
    // signals the role, but the italic copy makes the absence intentional).
    // Other no-address cases (block_marker fell through, etc.) get the bare
    // em-dash since they're handled by special-case paths above or are
    // genuinely uninformative.
    const placeholder =
      role === 'nonstandard'
        ? `(${t('explorer.tx.role.nonstandard.label')} · no address)`
        : '—';
    mainContent = (
      <span
        style={{
          ...monoValueStyle,
          color: '#888',
          flex: 1,
          minWidth: 0,
          fontStyle: role === 'nonstandard' ? 'italic' : 'normal',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {placeholder}
      </span>
    );
  }

  // Tech metadata row: vout=N · script_type · spent (if applicable). For
  // OP_RETURN we replace script_type with the friendlier "Data" + nulldata
  // so the tech row stays consistent with the canonical convention.
  const techParts: string[] = [`vout=${output.index}`];
  if (role === 'data_carrier') {
    techParts.push(`${t('explorer.tx.role.dataCarrier.label')} (${output.script_type})`);
  } else {
    techParts.push(output.script_type);
  }
  if (output.is_spent) {
    techParts.push(t('explorer.tx.spent.label'));
  }
  const techText = techParts.join(' · ');
  const techTooltip = output.is_spent ? t('explorer.tx.spent.tooltip') : techText;

  // Phase 2.2.y: trailing Eye renders only when there's a navigable address —
  // data_carrier preview has no address (OP_RETURN), and no-address placeholder
  // branches (block_marker fell-through, nonstandard without address) have
  // nothing to drill into.
  const showTrailingEye = !!renderedAddress && role !== 'data_carrier';

  return (
    <div style={itemRowCardStyle} onMouseEnter={hoverIn} onMouseLeave={hoverOut}>
      <div style={mainRowStyle}>
        {mainContent}
        <span style={{ ...monoValueStyle, color: isZero ? '#888' : '#27ae60', flexShrink: 0 }}>
          {formatAmount(output.amount, false)}
        </span>
        {showTrailingEye && (
          <IconButton
            size={26}
            icon={<Eye size={12} />}
            title={t('explorer.viewAddressDetails')}
            ariaLabel={t('explorer.viewAddressDetails')}
            onClick={() => onAddressClick(renderedAddress)}
          />
        )}
      </div>
      <div style={rowDividerStyle} />
      <div style={footerRowStyle}>
        <div style={footerLeftZoneStyle}>
          {badges.map((b) => (
            <Badge key={b.id} badge={b} expanded={isExpanded(b)} onToggle={() => toggle(b.id)} />
          ))}
        </div>
        <div style={footerRightZoneStyle} title={techTooltip}>
          {techText}
        </div>
      </div>
    </div>
  );
};

// ============================================================================
// <StakeSplitGroup> — R4 §1: wrap 2+ stake_return outputs into one subtle
// outlined container with a shared header label, so the user reads them as
// "parts of one staking reward" rather than as independent payments. The
// canonical mainnet case is exactly 2 stake_returns (split); 1 stake_return
// renders as a single (no wrapper); >2 is a rare edge case and per design
// decision the wrapper still groups them all (with templated count).
// ============================================================================
interface StakeSplitGroupProps {
  outputs: TxOutput[];
  onAddressClick: (address: string) => void;
  formatAmount: (value: number, withUnit?: boolean) => string;
  t: TFunction;
}

const StakeSplitGroup: React.FC<StakeSplitGroupProps> = ({
  outputs,
  onAddressClick,
  formatAmount,
  t,
}) => {
  // For N=2 (canonical mainnet split) use the existing i18n key with its
  // baked-in "split into 2 parts" copy. For N>2 (rare edge case), fall back to
  // a template literal — the i18n catalog has no plural-count variant for this
  // since N>2 is non-canonical. Hardcoded English is acceptable per project
  // convention; non-EN locales fall back via i18next anyway.
  const label =
    outputs.length === 2
      ? t('explorer.tx.role.stakeReturn.splitGroup')
      : `Stake Return (split into ${outputs.length} parts)`;
  return (
    <div
      style={{
        border: '1px dashed #3a3a3a',
        borderRadius: '6px',
        padding: '8px',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
      }}
    >
      <div
        style={{
          fontSize: '11px',
          fontWeight: 500,
          color: '#27ae60',
          marginBottom: '2px',
          paddingLeft: '4px',
        }}
      >
        {label}
      </div>
      {/* suppressRoleBadge: the group header already says "Stake Return" — the
          per-row badge would be duplicate noise. MINE/DUST overlays still
          render normally. */}
      {outputs.map((output) => (
        <OutputRow
          key={output.index}
          output={output}
          onAddressClick={onAddressClick}
          formatAmount={formatAmount}
          t={t}
          suppressRoleBadge
        />
      ))}
    </div>
  );
};

// ============================================================================
// <DustCollapseGroup> — R4 §3: when there are 3+ dust outputs, show the first
// 2 as full rows + collapse the rest behind an expandable footer button. The
// collapsed pool is rendered on demand when `isExpanded` flips true. Footer
// is keyboard-accessible (role="button", Enter/Space handler).
//
// Hardcoded English ("+ N more dust outputs" / "− Show less") is acceptable
// per project convention; i18n keys can be added in a follow-up if needed.
// ============================================================================
interface DustCollapseGroupProps {
  visible: TxOutput[];
  collapsed: TxOutput[];
  isExpanded: boolean;
  onToggle: () => void;
  onAddressClick: (address: string) => void;
  formatAmount: (value: number, withUnit?: boolean) => string;
  t: TFunction;
}

const DustCollapseGroup: React.FC<DustCollapseGroupProps> = ({
  visible,
  collapsed,
  isExpanded,
  onToggle,
  onAddressClick,
  formatAmount,
  t,
}) => {
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onToggle();
    }
  };
  const totalCount = visible.length + collapsed.length;
  return (
    <div
      style={{
        border: '1px dashed #3a3a3a',
        borderRadius: '6px',
        padding: '8px',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
      }}
    >
      <div
        style={{
          fontSize: '11px',
          fontWeight: 500,
          color: '#888',
          marginBottom: '2px',
          paddingLeft: '4px',
        }}
      >
        Dust outputs ({totalCount})
      </div>
      {/* suppressDustBadge: group header already says "Dust outputs" — the
          per-row DUST badge would be duplicate noise. Role badge (CHANGE/
          SELF/EXTERNAL) and MINE overlay still render normally. */}
      {visible.map((output) => (
        <OutputRow
          key={output.index}
          output={output}
          onAddressClick={onAddressClick}
          formatAmount={formatAmount}
          t={t}
          suppressDustBadge
        />
      ))}
      {isExpanded &&
        collapsed.map((output) => (
          <OutputRow
            key={output.index}
            output={output}
            onAddressClick={onAddressClick}
            formatAmount={formatAmount}
            t={t}
            suppressDustBadge
          />
        ))}
      <div
        role="button"
        tabIndex={0}
        onClick={onToggle}
        onKeyDown={handleKeyDown}
        style={{
          fontSize: '12px',
          color: '#888',
          padding: '6px 12px',
          cursor: 'pointer',
          textAlign: 'center',
          backgroundColor: '#2a2a2a',
          border: '1px dashed #3a3a3a',
          borderRadius: '6px',
          userSelect: 'none',
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: '6px',
          alignSelf: 'center',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.color = '#ddd';
          e.currentTarget.style.borderColor = '#444';
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.color = '#888';
          e.currentTarget.style.borderColor = '#3a3a3a';
        }}
      >
        {isExpanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
        {isExpanded ? 'Show less' : `${collapsed.length} more dust outputs`}
      </div>
    </div>
  );
};

export const TransactionDetail: React.FC<TransactionDetailProps> = ({
  transaction,
  isLoading,
  onAddressClick,
  onTxClick,
  onBlockClick,
  onBack,
  onRefresh,
  isAnyLoading,
  siblingTxids,
  onSiblingTxClick,
}) => {
  const { t } = useTranslation('common');
  const { formatAmount } = useDisplayUnits();
  const [copiedField, setCopiedField] = useState<string | null>(null);
  // Dust-collapse expansion state for the outputs panel. Local because expand
  // state should not persist across tx navigation — reset effect below clears
  // it on `transaction?.txid` change so navigating to a new tx always lands
  // with the dust group collapsed.
  const [isDustExpanded, setIsDustExpanded] = useState<boolean>(false);
  // R7: when non-null, <OpReturnDataDialog> is open and shows the full hex +
  // best-effort ASCII for the given OP_RETURN output. Reset to null on tx
  // navigation in the reset useEffect below alongside copiedField /
  // isDustExpanded so navigating away closes the dialog cleanly.
  const [opReturnDialogData, setOpReturnDialogData] = useState<{ hex: string; ascii: string } | null>(null);
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Auto-refresh: 60-second countdown driving onRefresh(). Full pattern copied
  // verbatim from BlockDetail.tsx (countdown ref + onRefresh ref + silent
  // in-flight lock with monotonic token to defend against stale `.finally()`
  // from a prior tx unlocking the new tx's fresh fetch).
  const REFRESH_INTERVAL_SECONDS = 60;
  const [countdown, setCountdown] = useState(REFRESH_INTERVAL_SECONDS);
  const countdownRef = useRef(countdown);
  useEffect(() => {
    countdownRef.current = countdown;
  }, [countdown]);
  const onRefreshRef = useRef(onRefresh);
  useEffect(() => {
    onRefreshRef.current = onRefresh;
  }, [onRefresh]);

  const silentInFlightRef = useRef(false);
  const silentTokenRef = useRef(0);

  useEffect(() => {
    setCountdown(REFRESH_INTERVAL_SECONDS);
    // Tx navigation: clear the lock AND bump the token so a still-pending
    // `.finally()` for the prior tx's silent fetch cannot clear the lock
    // for the new tx's fresh fetch. Stale response data is already
    // discarded by `fetchTransactionSeqRef` + `currentTxidRef` guards on
    // the ExplorerPage side, so clearing the lock here is safe.
    silentInFlightRef.current = false;
    silentTokenRef.current += 1;
  }, [transaction?.txid]);

  // Read latest countdown via a ref + decide-then-update side-effect outside
  // the `setCountdown` updater. React may invoke setState updaters twice in
  // StrictMode/dev or during concurrent rendering; calling `onRefresh` from
  // inside the updater would double-fire. Reading the ref here keeps the
  // side effect single-shot per tick.
  useEffect(() => {
    if (!transaction?.txid || isAnyLoading) return;
    const id = setInterval(() => {
      if (countdownRef.current <= 1) {
        if (silentInFlightRef.current) {
          // Prior silent refresh still in flight — skip firing a new one,
          // hold the countdown at 1 until it resolves so we don't pile up
          // concurrent fetches on a slow daemon. The next tick re-checks.
          return;
        }
        silentInFlightRef.current = true;
        const myToken = silentTokenRef.current;
        Promise.resolve(onRefreshRef.current({ silent: true })).finally(() => {
          // Only clear the lock if this is still the active token — the
          // tx-change reset bumps the counter, so a stale `.finally()` for
          // a prior tx must not unlock the new tx's in-flight fetch.
          if (silentTokenRef.current === myToken) {
            silentInFlightRef.current = false;
          }
        });
        setCountdown(REFRESH_INTERVAL_SECONDS);
      } else {
        setCountdown((prev) => prev - 1);
      }
    }, 1000);
    return () => clearInterval(id);
  }, [transaction?.txid, isAnyLoading]);

  // Manual click on the RefreshCountdown ring fires onRefresh AND resets the
  // countdown so the auto-refresh interval doesn't fire again moments later
  // (without reset, clicking the ring at countdown=1 would dispatch a manual
  // fetch plus the imminent automatic one back-to-back).
  const handleManualRefresh = useCallback(() => {
    setCountdown(REFRESH_INTERVAL_SECONDS);
    onRefresh();
  }, [onRefresh]);

  useEffect(() => {
    return () => {
      if (copyTimerRef.current !== null) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    };
  }, []);

  // Reset copy toast AND dust-collapse expansion when navigating between txs.
  // Without this, a stale copy-success Check icon would render on the next tx,
  // and a previously-expanded dust group on tx A would render expanded on tx B
  // even if its dust count is different.
  useEffect(() => {
    if (copyTimerRef.current !== null) {
      clearTimeout(copyTimerRef.current);
      copyTimerRef.current = null;
    }
    setCopiedField(null);
    setIsDustExpanded(false);
    setOpReturnDialogData(null);
  }, [transaction?.txid]);

  const handleCopy = useCallback(async (text: string, field: string) => {
    const ok = await writeToClipboard(text);
    if (!ok) return;
    if (copyTimerRef.current !== null) {
      clearTimeout(copyTimerRef.current);
    }
    setCopiedField(field);
    copyTimerRef.current = setTimeout(() => {
      setCopiedField(null);
      copyTimerRef.current = null;
    }, 2000);
  }, []);

  // R3 (sorting) + R4 (grouping) — see features/explorer/utils/outputDisplay.ts.
  // Memoized so the transform only re-runs when the outputs slice ref changes
  // (60s auto-refresh, Prev/Next navigation, etc.) — not on copy/expand state ticks.
  //
  // CRITICAL: this useMemo MUST stay above the `if (isLoading)` / `if
  // (!transaction)` early-return guards below. Rules of Hooks requires the
  // same hook call order on every render. When the user clicks the new tx
  // Prev/Next sibling pills (m-fix-explorer-navigation-parent-stack-and-tx-
  // prev-next, 2026-06-01), `fetchTransaction` flips `isLoading` to true,
  // re-renders the SAME TransactionDetail instance, then resolves and flips
  // back to false. If the useMemo were below the early return, the render
  // with `isLoading=true` would call N hooks (returning before the useMemo)
  // and the render with `isLoading=false` would call N+1 hooks — React
  // throws "Rendered more hooks than during the previous render", and the
  // React Router error boundary catches it with "Something went wrong
  // loading this page." Prior to the Prev/Next pills, all tx-to-tx
  // navigation went through Back→click which unmounts TransactionDetail
  // and remounts a fresh instance, so the hook-count mismatch never fired.
  // Now that same-instance tx-to-tx navigation is possible, the useMemo
  // must run unconditionally. The `transaction?.outputs ?? []` guard makes
  // the input safe when transaction is null.
  const renderItems = useMemo<RenderItem[]>(
    () => groupOutputs(sortOutputs(transaction?.outputs ?? [])),
    [transaction?.outputs],
  );

  // CRITICAL: same Rules-of-Hooks placement as the `renderItems` useMemo
  // above. `useDisplayDateTime()` MUST stay above the `if (isLoading)` /
  // `if (!transaction)` early-return guards below — Prev/Next Tx sibling
  // navigation does same-instance fetch and toggles `isLoading` across the
  // guard, so the hook count must be stable across renders. Added in
  // m-fix-explorer-blockdetail-txdetail-navigation-error (2026-06-04);
  // mirrors the Round-3 precedent from
  // m-fix-explorer-navigation-parent-stack-and-tx-prev-next (2026-06-01)
  // that already locked the `renderItems` useMemo above. The hook was
  // originally added below the guards by m-fix-date-display-inconsistencies
  // (2026-06-04) when the file migrated from local date helpers (regular
  // function calls — safe below guards) to the global hook (unsafe).
  // Do NOT relocate this back below the guards.
  const { formatDateTime, formatTooltip } = useDisplayDateTime();

  if (isLoading) {
    return <div style={emptyStateStyle}>{t('explorer.loadingTransaction')}</div>;
  }

  if (!transaction) {
    return <div style={emptyStateStyle}>{t('explorer.transactionNotFound')}</div>;
  }

  const inputsCount = transaction.inputs?.length || 0;
  const outputsCount = transaction.outputs?.length || 0;
  const formattedTimeLocal = formatDateTime(transaction.time);
  const formattedTimeUTC = formatTooltip(transaction.time);
  void formatTxDateLocal;
  void formatTxDateUTC;

  // Prev/Next sibling-tx navigation. siblingTxids comes from ExplorerPage's
  // useEffect that loads the parent block in the background when needed.
  // `currentIndex === -1` covers both "siblingTxids is null" (parent block
  // not yet loaded / mempool tx) and "current txid not found in the list"
  // (a corruption guard — should not happen in practice but the explicit
  // bounds checks defend the indexOf return value either way). hasNext gates
  // on `siblingTxids != null` as defense-in-depth — if siblingTxids is null
  // currentIndex is -1, but a future engineer who inverts the gate logic
  // shouldn't accidentally enable Next when the txid list is unknown.
  const currentIndex = siblingTxids ? siblingTxids.indexOf(transaction.txid) : -1;
  const hasPrev = siblingTxids != null && currentIndex > 0;
  const hasNext = siblingTxids != null && currentIndex >= 0 && currentIndex < siblingTxids.length - 1;
  const prevTxid = hasPrev ? siblingTxids![currentIndex - 1] : null;
  const nextTxid = hasNext ? siblingTxids![currentIndex + 1] : null;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column', gap: '12px', minHeight: 0 }}>
        {/* Hero card: top row (Back + title + status pills + RefreshCountdown
            + Block PillButton) + TXID/date row + divider + 4-column summary
            (Total Output / I/O Count / Size / Fee). Replaces the legacy flat
            header + Tx Info DashboardCard. */}
        <div style={heroCardStyle}>
          {/* Top row: Back IconButton + title + optional Coinbase/Coinstake
              pills + Confirmations pill + RefreshCountdown + spacer + Block
              PillButton. flexWrap + rowGap so the right-side cluster drops
              to a second line on narrow widths instead of clipping. */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap', rowGap: '8px' }}>
            <IconButton
              icon={<ArrowLeft size={14} />}
              title={t('buttons.back')}
              ariaLabel={t('buttons.back')}
              onClick={onBack}
            />
            {/* lineHeight: 1 collapses the line-box to the glyph height so the
                title's optical center aligns with the pills / IconButton /
                RefreshCountdown sibling axes. Without it the default ~1.2
                line-height adds vertical slack above and below the visible
                glyphs and the row reads as if pressed to the bottom of an
                invisible content box (m-tx-details-io-redesign-v2 Phase 2). */}
            <span style={{ fontSize: '18px', fontWeight: 600, color: '#ddd', lineHeight: 1 }}>
              {t('explorer.transactionDetails')}
            </span>
            {transaction.is_coinbase && (
              <StatusPill tone="warning" label={t('explorer.tx.pill.coinbase')} />
            )}
            {transaction.is_coinstake && (
              <StatusPill tone="success" label={t('explorer.tx.pill.coinstake')} />
            )}
            {!transaction.is_coinbase && !transaction.is_coinstake && (
              <StatusPill tone="neutral" label={t('explorer.tx.pill.regular')} />
            )}
            <StatusPill
              tone={transaction.confirmations < 6 ? 'warning' : 'success'}
              label={`${transaction.confirmations} ${t('explorer.conf')}`}
            />
            <UnitBadge />
            <RefreshCountdown
              countdown={countdown}
              total={REFRESH_INTERVAL_SECONDS}
              mode="interactive"
              onRefresh={handleManualRefresh}
              isLoading={isAnyLoading}
            />
            {/* Right-side navigation cluster: Prev / Next sibling-tx pills
                first, then the Block #N pill that drills up to the parent
                block. Pills render disabled when siblingTxids is null (parent
                block fetch in flight or mempool tx) or when the current tx is
                the first/last in its block (e.g. a coinbase-only block with
                only one tx renders both disabled). Click handlers gate on
                `onSiblingTxClick` truthy AND the resolved sibling txid; the
                `&&` chain is defense-in-depth — the `disabled` prop already
                blocks pointer/keyboard activation, but if a parent forgets to
                wire onSiblingTxClick, the click would otherwise no-op
                silently with no developer signal. */}
            <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <PillButton
                icon={<ChevronLeft size={12} />}
                label={t('explorer.previousTx', { defaultValue: 'Previous Tx' })}
                title={t('explorer.previousTx', { defaultValue: 'Previous Tx' })}
                ariaLabel={t('explorer.previousTx', { defaultValue: 'Previous Tx' })}
                onClick={() => onSiblingTxClick && prevTxid && onSiblingTxClick(prevTxid)}
                disabled={!hasPrev || !onSiblingTxClick}
              />
              <PillButton
                icon={<ChevronRight size={12} />}
                label={t('explorer.nextTx', { defaultValue: 'Next Tx' })}
                title={t('explorer.nextTx', { defaultValue: 'Next Tx' })}
                ariaLabel={t('explorer.nextTx', { defaultValue: 'Next Tx' })}
                onClick={() => onSiblingTxClick && nextTxid && onSiblingTxClick(nextTxid)}
                disabled={!hasNext || !onSiblingTxClick}
              />
              <PillButton
                icon={<Hash size={12} />}
                label={`${t('explorer.block')} #${transaction.block_height}`}
                title={t('explorer.viewBlockDetails')}
                ariaLabel={t('explorer.viewBlockDetails')}
                onClick={() => onBlockClick(transaction.block_hash)}
              />
            </div>
          </div>

          {/* TXID + date row: truncated TXID (full in title tooltip) + Copy
              IconButton with 2s green-Check icon-swap feedback on the left;
              long local date right-aligned with UTC variant in title.
              Both spans unified at 13px / #ddd so the row reads as a single
              continuous identity strip rather than two mismatched tones
              (TXID was #aaa monospace 13px, date was valueStyle 12px #ddd).
              TXID stays monospace because it's a hash; the date keeps the
              default sans body face since it's natural language
              (m-tx-details-io-redesign-v2 Phase 2). */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span
                title={transaction.txid}
                style={{ fontSize: '13px', color: '#ddd', fontFamily: 'monospace', minWidth: 0 }}
              >
                {truncateAddress(transaction.txid, 16, 16)}
              </span>
              <IconButton
                icon={
                  copiedField === 'txid' ? (
                    <Check size={12} color="#27ae60" />
                  ) : (
                    <Copy size={12} />
                  )
                }
                title={t('explorer.copyTxId')}
                ariaLabel={t('explorer.copyTxId')}
                onClick={() => handleCopy(transaction.txid, 'txid')}
              />
            </div>
            <span
              title={formattedTimeUTC}
              style={{
                ...valueStyle,
                fontSize: '13px',
                textAlign: 'right',
                whiteSpace: 'nowrap',
                marginLeft: 'auto',
              }}
            >
              {formattedTimeLocal}
            </span>
          </div>

          {/* Divider above summary breakdown. The prior "Amounts in X" text
              subtitle that lived here was hoisted into the hero top-row as a
              <UnitBadge /> pill (canonical Receive design-language pattern
              shared with BlockDetail / AddressView). The pill uses no
              `textTransform: 'uppercase'`, so the µ→Μ→M Unicode bug that
              forced the subtitle workaround in the 2026-05-27 round-2 fix
              is eliminated at the root. */}
          <div style={{ borderTop: '1px solid #3a3a3a' }} />

          {/* Summary row: 4 columns. `flexWrap: wrap, rowGap: 12px` lets the
              columns drop to a 2x2 layout below ~728px viewport (with
              `flex: 1 1 170px` per column). */}
          <div style={{ display: 'flex', gap: '16px', alignItems: 'flex-start', flexWrap: 'wrap', rowGap: '12px' }}>
            <div style={summaryColumnStyle}>
              <div style={summaryLabelStyle}>{t('explorer.totalOutput')}</div>
              <div
                style={{
                  ...summaryAmountLargeStyle,
                  color: transaction.total_output === 0 ? '#888' : '#27ae60',
                }}
              >
                {formatAmount(transaction.total_output, false)}
              </div>
            </div>
            <div style={summaryColumnStyle}>
              <div style={summaryLabelStyle}>{t('explorer.fee')}</div>
              <div style={summaryAmountFeeStyle}>{formatAmount(transaction.fee, false)}</div>
            </div>
            <div style={summaryColumnStyle}>
              <div style={summaryLabelStyle}>{t('explorer.inputsOutputs')}</div>
              <div style={summaryAmountSmallStyle}>
                {inputsCount} → {outputsCount}
              </div>
            </div>
            <div style={summaryColumnStyle}>
              <div style={summaryLabelStyle}>{t('explorer.size')}</div>
              <div style={summaryAmountSmallStyle}>{transaction.size} bytes</div>
            </div>
          </div>
        </div>

        {/* Inputs and Outputs cards: each has an internal sticky header (so
            the section title + count stay visible while rows scroll inside
            the card) and a scrollable row body with `maxHeight: 400px`. The
            Totals row sits OUTSIDE the scroll body, always visible at the
            card's bottom edge. */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', flex: 1, minHeight: 0 }}>
          {/* Inputs card */}
          <div style={ioCardStyle}>
            <div style={stickyHeaderStyle}>
              {t('explorer.inputs')} ({inputsCount})
            </div>
            <div style={scrollBodyStyle}>
              {inputsCount > 0 ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                  {transaction.inputs.map((input) => (
                    <InputRow
                      key={`${input.txid}-${input.vout}`}
                      input={input}
                      onAddressClick={onAddressClick}
                      onTxClick={onTxClick}
                      formatAmount={formatAmount}
                      t={t}
                    />
                  ))}
                </div>
              ) : (
                <div style={{ color: '#888', fontSize: '12px' }}>{t('explorer.tx.empty.noInputs')}</div>
              )}
            </div>
            <div style={totalsRowStyle}>
              <span>{t('explorer.totalInput')}</span>
              <span style={{ ...totalsValueStyle, color: transaction.total_input === 0 ? '#888' : '#27ae60' }}>
                {formatAmount(transaction.total_input, false)}
              </span>
            </div>
          </div>

          {/* Outputs card */}
          <div style={ioCardStyle}>
            <div style={stickyHeaderStyle}>
              {t('explorer.outputs')} ({outputsCount})
            </div>
            <div style={scrollBodyStyle}>
              {outputsCount > 0 ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                  {renderItems.map((item) => {
                    // Discriminated union switch. Single = plain OutputRow.
                    // stake_split = StakeSplitGroup (subtle outline + shared label).
                    // dust_collapse = DustCollapseGroup (2 visible + expandable footer).
                    //
                    // Keys are derived from output indices (stable across renders)
                    // rather than array position, so React reconciliation matches the
                    // right element if the grouping shape changes (e.g. a future
                    // extension allowing multiple stake_splits per tx). This keeps
                    // `isDustExpanded` state correctly associated with its group
                    // across re-renders.
                    if (item.kind === 'single') {
                      return (
                        <OutputRow
                          key={`out-${item.output.index}`}
                          output={item.output}
                          onAddressClick={onAddressClick}
                          formatAmount={formatAmount}
                          t={t}
                          openDataDialog={(hex, ascii) =>
                            setOpReturnDialogData({ hex, ascii })
                          }
                        />
                      );
                    }
                    if (item.kind === 'stake_split') {
                      return (
                        <StakeSplitGroup
                          key={`split-${item.outputs.map((o) => o.index).join('-')}`}
                          outputs={item.outputs}
                          onAddressClick={onAddressClick}
                          formatAmount={formatAmount}
                          t={t}
                        />
                      );
                    }
                    // item.kind === 'dust_collapse'
                    return (
                      <DustCollapseGroup
                        key={`dust-${item.visible[0]?.index ?? 'empty'}`}
                        visible={item.visible}
                        collapsed={item.collapsed}
                        isExpanded={isDustExpanded}
                        onToggle={() => setIsDustExpanded((v) => !v)}
                        onAddressClick={onAddressClick}
                        formatAmount={formatAmount}
                        t={t}
                      />
                    );
                  })}
                </div>
              ) : (
                <div style={{ color: '#888', fontSize: '12px' }}>{t('explorer.tx.empty.noOutputs')}</div>
              )}
            </div>
            <div style={totalsRowStyle}>
              <span>{t('explorer.totalOutput')}</span>
              <span style={{ ...totalsValueStyle, color: transaction.total_output === 0 ? '#888' : '#27ae60' }}>
                {formatAmount(transaction.total_output, false)}
              </span>
            </div>
          </div>
        </div>
      </div>
      <OpReturnDataDialog
        isOpen={opReturnDialogData !== null}
        dataHex={opReturnDialogData?.hex ?? ''}
        dataAscii={opReturnDialogData?.ascii ?? ''}
        onClose={() => setOpReturnDialogData(null)}
      />
    </div>
  );
};
