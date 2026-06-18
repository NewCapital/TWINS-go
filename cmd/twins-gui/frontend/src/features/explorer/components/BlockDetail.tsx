import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { BlockDetail as BlockDetailType } from '@/store/slices/explorerSlice';
import {
  ArrowLeft,
  Check,
  ChevronLeft,
  ChevronRight,
  Copy,
  Eye,
} from 'lucide-react';
import { DashboardCard } from '@/shared/components/DashboardCard';
import { StatusPill } from '@/shared/components/StatusPill';
import { IconButton } from '@/shared/components/IconButton';
import { PillButton } from '@/shared/components/PillButton';
import { RefreshCountdown } from '@/shared/components/RefreshCountdown';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { UnitBadge } from '@/shared/components/UnitBadge';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { truncateAddress } from '@/shared/utils/format';
import { writeToClipboard } from '@/shared/utils/clipboard';

// Local date formatters with seconds. Intentionally NOT modifying the shared
// formatTransactionDate / formatTransactionDateUTC in transactionIcons.ts —
// those are consumed across Transactions / Overview / detail dialogs and use
// HH:MM (no seconds) by design. Block detail wants seconds for precise block
// time and a hover tooltip with the UTC equivalent.
function formatBlockDateLocal(timestamp: number | string | Date): string {
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

function formatBlockDateUTC(timestamp: number | string | Date): string {
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

// `formatMinedAgo` was removed in m-fix-date-display-inconsistencies
// (2026-06-04). The hero hash+time row now renders only the global
// `formatDateTime` result; Age mode is handled by the hook so duplicating
// the age string in a local helper would produce `<1m ago · <1m ago` on
// the Block Detail page.

// Stake age formatter: seconds → `Xd Yh` / `Xh Ym` / `Xm` tiered display.
// 0 renders as the `—` em-dash placeholder via the caller; this helper is
// only invoked when stake_age > 0.
function formatStakeAge(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days >= 1) return `${days}d ${hours}h`;
  if (hours >= 1) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

interface BlockDetailProps {
  block: BlockDetailType | null;
  isLoading: boolean;
  onTxClick: (txid: string) => void;
  onBlockClick: (query: string) => void;
  onAddressClick: (address: string) => void;
  onBack: () => void;
  onRefresh: (options?: { silent?: boolean }) => void | Promise<void>;
  isAnyLoading: boolean;
}

const labelStyle: React.CSSProperties = {
  fontSize: '11px',
  color: '#888',
  fontWeight: 500,
};

const valueStyle: React.CSSProperties = {
  fontSize: '12px',
  color: '#ddd',
};

const monoValueStyle: React.CSSProperties = {
  fontSize: '12px',
  color: '#ddd',
  fontFamily: 'monospace',
  wordBreak: 'break-all',
};

const ledgerRowStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'flex-start',
  padding: '6px 0',
  gap: '12px',
};

const emptyStateStyle: React.CSSProperties = {
  padding: '32px',
  textAlign: 'center',
  color: '#888',
  fontSize: '12px',
};

const placeholderValueStyle: React.CSSProperties = {
  ...valueStyle,
  color: '#666',
  fontStyle: 'italic',
};

// Tab bar inside Block Information card. Replaces the prior 4-section vertical
// stack so only one section's rows render at a time — keeps card height low
// enough that the Block Detail page fits without vertical scroll on typical
// wallet viewports. Style mirrors the underline-tab convention in
// MasternodesPage.tsx:530-602.
type BlockInfoTab = 'identity' | 'metadata' | 'consensus' | 'pos';

const tabBarStyle: React.CSSProperties = {
  display: 'flex',
  borderBottom: '1px solid #4a4a4a',
  marginBottom: '8px',
};

function tabButtonStyle(active: boolean): React.CSSProperties {
  return {
    padding: '8px 16px',
    fontSize: '12px',
    fontWeight: active ? 600 : 400,
    color: active ? '#ddd' : '#888',
    backgroundColor: active ? '#3a3a3a' : 'transparent',
    border: 'none',
    borderBottom: active ? '2px solid #4a8af4' : '2px solid transparent',
    cursor: 'pointer',
  };
}

// Hero card chrome.
const heroCardStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  padding: '16px 20px',
  display: 'flex',
  flexDirection: 'column',
  gap: '12px',
};

// Tx row-card chrome matching Receive design language.
const txRowCardStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '12px',
  padding: '8px 12px',
  backgroundColor: '#2a2a2a',
  border: '1px solid transparent',
  borderRadius: '6px',
  cursor: 'default',
  transition: 'border-color 0.15s',
  outline: 'none',
};

const txRowIndexStyle: React.CSSProperties = {
  width: '24px',
  flexShrink: 0,
  fontSize: '12px',
  color: '#888',
  textAlign: 'right',
  fontVariantNumeric: 'tabular-nums',
};

const txRowHashGroupStyle: React.CSSProperties = {
  flexShrink: 0,
  display: 'flex',
  alignItems: 'center',
  gap: '6px',
};

const txRowTxidStyle: React.CSSProperties = {
  fontSize: '12px',
  fontFamily: 'monospace',
  color: '#ddd',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
};

// Type column: plain colored text (no background pill). Stretches to fill
// the gap between hash group and Eye via flex:1 + textAlign center, so the
// type label visually centers in the leftover row space; `color` applied
// per-row at the call site.
const txRowTypeBadgeStyle: React.CSSProperties = {
  flex: 1,
  textAlign: 'center',
  fontSize: '11px',
  fontWeight: 500,
  textTransform: 'lowercase',
  letterSpacing: '0.3px',
};

// HERO: Total Reward + breakdown layout (Variant B).
// Two-column grid: left = TOTAL BLOCK REWARD label + hero value; right =
// 3-row Stake / Masternode / Dev Fund breakdown with inline progress bars.
// Below: 3 narrow recipient cards (full address + copy IconButton, no amount).
const heroRewardGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: '24px',
  alignItems: 'center',
};

const heroTotalLabelStyle: React.CSSProperties = {
  fontSize: '11px',
  fontWeight: 500,
  color: '#888',
  marginBottom: '8px',
};

const heroTotalValueStyle: React.CSSProperties = {
  fontSize: '36px',
  fontWeight: 600,
  color: '#27ae60',
  fontFamily: 'monospace',
  fontVariantNumeric: 'tabular-nums',
  lineHeight: 1.1,
};

const breakdownRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '10px',
  padding: '4px 0',
  fontSize: '12px',
  color: '#ddd',
};

const breakdownLabelStyle: React.CSSProperties = {
  fontSize: '11px',
  fontWeight: 500,
  color: '#888',
  minWidth: '90px',
  flexShrink: 0,
};

const breakdownAmountStyle: React.CSSProperties = {
  fontSize: '12px',
  fontFamily: 'monospace',
  color: '#ddd',
  fontVariantNumeric: 'tabular-nums',
  minWidth: '80px',
  textAlign: 'right',
};

const breakdownBarTrackStyle: React.CSSProperties = {
  flex: 1,
  minWidth: '40px',
  height: '6px',
  backgroundColor: '#3a3a3a',
  borderRadius: '3px',
  overflow: 'hidden',
};

const breakdownPctStyle: React.CSSProperties = {
  fontSize: '11px',
  color: '#888',
  fontFamily: 'monospace',
  minWidth: '36px',
  textAlign: 'right',
  flexShrink: 0,
};

// Recipient cards row beneath the reward grid. Each card shows the full
// 34-character TWINS address + a copy IconButton + an Eye IconButton (for
// navigating to the address detail view via onAddressClick). The amount
// values were dropped from these cards — they live in the hero breakdown.
const recipientCardsRowStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
  gap: '12px',
};

const recipientCardStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '4px',
  padding: '10px 12px',
  backgroundColor: '#2a2a2a',
  border: '1px solid #3a3a3a',
  borderRadius: '6px',
  minWidth: 0,
};

const recipientLabelStyle: React.CSSProperties = {
  fontSize: '11px',
  fontWeight: 500,
  color: '#888',
};

const recipientHeaderRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'flex-start',
  justifyContent: 'space-between',
  gap: '8px',
};

const recipientAddressTextStyle: React.CSSProperties = {
  fontSize: '12px',
  fontFamily: 'monospace',
  color: '#ddd',
  wordBreak: 'break-all',
  whiteSpace: 'normal',
  marginTop: '2px',
};

const recipientIconsStyle: React.CSSProperties = {
  display: 'flex',
  gap: '4px',
  flexShrink: 0,
};

export const BlockDetail: React.FC<BlockDetailProps> = ({
  block,
  isLoading,
  onTxClick,
  onBlockClick,
  onAddressClick,
  onBack,
  onRefresh,
  isAnyLoading,
}) => {
  const { t } = useTranslation('common');
  const { formatAmount } = useDisplayUnits();

  // Hash copy feedback: 2s `✓ <field> copied` icon swap. Per-field key gates
  // which inline IconButton renders the green Check.
  const [copiedField, setCopiedField] = useState<string | null>(null);
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Active tab inside Block Information card. Resets to 'identity' on every
  // block change (Prev/Next navigation) so the user always lands on the most
  // common section. Separately, if the user is sitting on the 'pos' tab and
  // navigates to a PoW block, the second effect drops them back to identity.
  const [activeTab, setActiveTab] = useState<BlockInfoTab>('identity');
  useEffect(() => {
    setActiveTab('identity');
  }, [block?.hash]);
  useEffect(() => {
    if (!block?.is_pos && activeTab === 'pos') setActiveTab('identity');
  }, [block?.is_pos, activeTab]);

  // Auto-refresh countdown. 60s tick fires onRefresh({silent: true}).
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
    silentInFlightRef.current = false;
    silentTokenRef.current += 1;
  }, [block?.hash]);

  useEffect(() => {
    if (!block?.hash || isAnyLoading) return;
    const id = setInterval(() => {
      if (countdownRef.current <= 1) {
        if (silentInFlightRef.current) {
          return;
        }
        silentInFlightRef.current = true;
        const myToken = silentTokenRef.current;
        Promise.resolve(onRefreshRef.current({ silent: true })).finally(() => {
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
  }, [block?.hash, isAnyLoading]);

  const handleManualRefresh = useCallback(() => {
    setCountdown(REFRESH_INTERVAL_SECONDS);
    onRefresh();
  }, [onRefresh]);

  useEffect(() => {
    return () => {
      if (copyTimerRef.current) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    };
  }, []);

  // Reset copy toast when navigating between blocks.
  useEffect(() => {
    if (copyTimerRef.current) {
      clearTimeout(copyTimerRef.current);
      copyTimerRef.current = null;
    }
    setCopiedField(null);
  }, [block?.hash]);

  const handleCopy = useCallback(async (field: string, value: string) => {
    const ok = await writeToClipboard(value);
    if (!ok) return;
    if (copyTimerRef.current) {
      clearTimeout(copyTimerRef.current);
    }
    setCopiedField(field);
    copyTimerRef.current = setTimeout(() => {
      setCopiedField(null);
      copyTimerRef.current = null;
    }, 2000);
  }, []);

  // The prior per-second `setRelTimeTick` live tick was removed in
  // m-fix-date-display-inconsistencies (2026-06-04) along with the local
  // `formatMinedAgo` helper. The hero row now renders only the global
  // `formatDateTime` result, which already participates in the user's
  // selected Local/UTC/Age mode and re-renders via the Zustand
  // `dateFormat` subscription. Per-second ticking is no longer needed.

  // Coinstake reward percentages. Compute against total_reward to keep the
  // bars proportional. Guard div-by-zero on PoW / empty blocks.
  const breakdownPercents = useMemo(() => {
    if (!block || block.total_reward <= 0) return { stake: 0, mn: 0, dev: 0 };
    return {
      stake: (block.stake_reward / block.total_reward) * 100,
      mn: (block.masternode_reward / block.total_reward) * 100,
      dev: (block.dev_reward / block.total_reward) * 100,
    };
  }, [block]);

  // CRITICAL: `useDisplayDateTime()` MUST be called BEFORE the early-return
  // guards below. Prev/Next Block navigation uses same-instance fetch
  // (setLoadingBlock(true) → re-render with isLoading=true → early-return at
  // line ~493 → setLoadingBlock(false) → re-render past guard → reach this
  // line). If the hook is called AFTER the guards, React sees N hooks in the
  // loading render and N+1 hooks in the loaded render and throws "Rendered
  // more hooks than during the previous render", which the React Router
  // error boundary catches as "Something went wrong loading this page".
  // Fixed in m-fix-explorer-blockdetail-txdetail-navigation-error (2026-06-04);
  // mirrors the Round-3 precedent from
  // m-fix-explorer-navigation-parent-stack-and-tx-prev-next (2026-06-01) that
  // resolved the same class of bug on TransactionDetail.tsx. Do NOT relocate
  // this back below the guards.
  const { formatDateTime, formatTooltip } = useDisplayDateTime();

  if (isLoading) {
    return <div style={emptyStateStyle}>{t('explorer.loadingBlock')}</div>;
  }

  if (!block) {
    return <div style={emptyStateStyle}>{t('explorer.blockNotFound')}</div>;
  }

  const formattedHeight = block.height.toLocaleString();
  const formattedTime = formatDateTime(block.time);
  const formattedTimeUTC = formatTooltip(block.time);
  // Local `formatMinedAgo` / `formatBlockDateLocal` / `formatBlockDateUTC`
  // helpers were dropped in m-fix-date-display-inconsistencies (2026-06-04).
  // The hero row now renders only `formattedTime` from the global hook so
  // the cell does not duplicate the Age representation when the user has
  // selected the Age display mode (`{minedAgo} · {formattedTime}` would
  // render `<1m ago · <1m ago` in Age mode).
  void formatBlockDateLocal;
  void formatBlockDateUTC;

  // Stake age: pre-formatted for the PoS section row. 0 → `—` placeholder.
  const stakeAgeText =
    block.stake_age && block.stake_age > 0 ? formatStakeAge(block.stake_age) : '—';
  const stakeAmountText =
    block.stake_amount && block.stake_amount > 0
      ? `${formatAmount(block.stake_amount, false)} TWINS`
      : '—';

  // Version display: hex form matches the legacy Qt wallet convention. The
  // BlockDetail TS interface doesn't expose a `version` field — Wails has it
  // on the class but the slice's narrower interface omits it; cast through
  // unknown to read it without widening the interface.
  const versionRaw = (block as unknown as { version?: number }).version;
  const versionHex =
    typeof versionRaw === 'number'
      ? `0x${versionRaw.toString(16).padStart(8, '0')}`
      : '—';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column', gap: '12px' }}>
        {/* Hero card: top row + hash row + time row, then reward grid
            (Total Reward hero left + breakdown right), then 3 recipient
            cards below with full addresses. */}
        <div style={heroCardStyle}>
          {/* Top row: Back + title + pills + nav cluster. The
              RefreshCountdown groups with the confirmations pill on the left
              identity cluster; Prev/Next cluster floats right. */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap', rowGap: '8px' }}>
            <PillButton
              icon={<ArrowLeft size={12} />}
              label={t('buttons.back')}
              title={t('buttons.back')}
              ariaLabel={t('buttons.back')}
              onClick={onBack}
            />
            <span style={{ fontSize: '18px', fontWeight: 600, color: '#ddd' }}>
              Block #{formattedHeight}
            </span>
            {block.is_pos && <StatusPill tone="success" label="PoS" />}
            <StatusPill
              tone={block.confirmations < 6 ? 'warning' : 'success'}
              label={`${block.confirmations} ${t('explorer.conf')}`}
            />
            <UnitBadge />
            <RefreshCountdown
              countdown={countdown}
              total={REFRESH_INTERVAL_SECONDS}
              mode="interactive"
              onRefresh={handleManualRefresh}
              isLoading={isAnyLoading}
            />
            <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <PillButton
                icon={<ChevronLeft size={12} />}
                label={t('explorer.previousBlock')}
                title={t('explorer.previousBlock')}
                ariaLabel={t('explorer.previousBlock')}
                onClick={() => block.previousblockhash && onBlockClick(block.previousblockhash)}
                disabled={!block.previousblockhash}
              />
              <PillButton
                icon={<ChevronRight size={12} />}
                label={t('explorer.nextBlock')}
                title={t('explorer.nextBlock')}
                ariaLabel={t('explorer.nextBlock')}
                onClick={() => block.nextblockhash && onBlockClick(block.nextblockhash)}
                disabled={!block.nextblockhash}
              />
            </div>
          </div>

          {/* Hash + Time row — truncated hash + copy IconButton on left,
              formatted timestamp with relative-time prefix right-aligned. */}
          <div style={ledgerRowStyle}>
            <div style={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span
                title={block.hash}
                style={{ fontSize: '13px', color: '#aaa', fontFamily: 'monospace', minWidth: 0 }}
              >
                {truncateAddress(block.hash, 16, 16)}
              </span>
              <IconButton
                icon={
                  copiedField === 'hash' ? (
                    <Check size={12} color="#27ae60" />
                  ) : (
                    <Copy size={12} />
                  )
                }
                title={t('explorer.copyHash')}
                ariaLabel={t('explorer.copyHash')}
                onClick={() => handleCopy('hash', block.hash)}
              />
            </div>
            <span
              title={formattedTimeUTC}
              style={{ ...valueStyle, textAlign: 'right', whiteSpace: 'nowrap', marginLeft: 'auto' }}
            >
              {formattedTime}
            </span>
          </div>

          {/* Reward section: divider + Variant B grid (hero + breakdown) +
              recipient cards. Gated so a data-less PoW block does not render
              a lonely divider with nothing below it. */}
          {(block.is_pos || block.total_reward > 0 || block.staker_address) && (
            <div style={{ borderTop: '1px solid #3a3a3a' }} />
          )}

          {block.is_pos ? (
            <>
              {/* 2-col grid: hero Total Reward left, 3-row breakdown right. */}
              <div style={heroRewardGridStyle}>
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                  <div style={heroTotalLabelStyle}>{t('explorer.totalReward')}</div>
                  <div style={{ display: 'flex', alignItems: 'baseline' }}>
                    <span style={heroTotalValueStyle}>
                      {formatAmount(block.total_reward, false)}
                    </span>
                  </div>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                  {/* Stake breakdown row. */}
                  <div style={breakdownRowStyle}>
                    <span style={breakdownLabelStyle}>{t('explorer.stake')}</span>
                    <span style={breakdownAmountStyle}>
                      {formatAmount(block.stake_reward, false)}
                    </span>
                    <div style={breakdownBarTrackStyle}>
                      <div
                        style={{
                          width: `${Math.min(100, Math.max(0, breakdownPercents.stake))}%`,
                          height: '100%',
                          backgroundColor: '#27ae60',
                        }}
                      />
                    </div>
                    <span style={breakdownPctStyle}>
                      {breakdownPercents.stake.toFixed(0)}%
                    </span>
                  </div>
                  {/* Masternode breakdown row. */}
                  <div style={breakdownRowStyle}>
                    <span style={breakdownLabelStyle}>{t('explorer.masternode')}</span>
                    <span style={breakdownAmountStyle}>
                      {formatAmount(block.masternode_reward, false)}
                    </span>
                    <div style={breakdownBarTrackStyle}>
                      <div
                        style={{
                          width: `${Math.min(100, Math.max(0, breakdownPercents.mn))}%`,
                          height: '100%',
                          backgroundColor: '#6699cc',
                        }}
                      />
                    </div>
                    <span style={breakdownPctStyle}>
                      {breakdownPercents.mn.toFixed(0)}%
                    </span>
                  </div>
                  {/* Dev Fund breakdown row. */}
                  <div style={breakdownRowStyle}>
                    <span style={breakdownLabelStyle}>{t('explorer.devFund')}</span>
                    <span style={breakdownAmountStyle}>
                      {formatAmount(block.dev_reward, false)}
                    </span>
                    <div style={breakdownBarTrackStyle}>
                      <div
                        style={{
                          width: `${Math.min(100, Math.max(0, breakdownPercents.dev))}%`,
                          height: '100%',
                          backgroundColor: '#ff9966',
                        }}
                      />
                    </div>
                    <span style={breakdownPctStyle}>
                      {breakdownPercents.dev.toFixed(0)}%
                    </span>
                  </div>
                </div>
              </div>

              {/* Recipient cards row: 3 narrow cards with full address +
                  copy + Eye IconButtons. Amount values dropped — they're in
                  the breakdown above. */}
              <div style={recipientCardsRowStyle}>
                {/* Staker recipient. */}
                <div style={recipientCardStyle}>
                  <div style={recipientHeaderRowStyle}>
                    <div style={recipientLabelStyle}>{t('explorer.stake')}</div>
                    {block.staker_address && (
                      <div style={recipientIconsStyle}>
                        <IconButton
                          icon={
                            copiedField === 'staker' ? (
                              <Check size={12} color="#27ae60" />
                            ) : (
                              <Copy size={12} />
                            )
                          }
                          title={t('explorer.copyAddress')}
                          ariaLabel={t('explorer.copyAddress')}
                          onClick={() => handleCopy('staker', block.staker_address)}
                        />
                        <IconButton
                          icon={<Eye size={12} />}
                          title={t('explorer.viewAddressDetails')}
                          ariaLabel={t('explorer.viewAddressDetails')}
                          onClick={() => onAddressClick(block.staker_address)}
                        />
                      </div>
                    )}
                  </div>
                  {block.staker_address ? (
                    <div title={block.staker_address} style={recipientAddressTextStyle}>
                      {block.staker_address}
                    </div>
                  ) : (
                    <span style={placeholderValueStyle}>—</span>
                  )}
                </div>
                {/* Masternode recipient. */}
                <div style={recipientCardStyle}>
                  <div style={recipientHeaderRowStyle}>
                    <div style={recipientLabelStyle}>{t('explorer.masternode')}</div>
                    {block.masternode_address && (
                      <div style={recipientIconsStyle}>
                        <IconButton
                          icon={
                            copiedField === 'masternode' ? (
                              <Check size={12} color="#27ae60" />
                            ) : (
                              <Copy size={12} />
                            )
                          }
                          title={t('explorer.copyAddress')}
                          ariaLabel={t('explorer.copyAddress')}
                          onClick={() => handleCopy('masternode', block.masternode_address)}
                        />
                        <IconButton
                          icon={<Eye size={12} />}
                          title={t('explorer.viewAddressDetails')}
                          ariaLabel={t('explorer.viewAddressDetails')}
                          onClick={() => onAddressClick(block.masternode_address)}
                        />
                      </div>
                    )}
                  </div>
                  {block.masternode_address ? (
                    <div title={block.masternode_address} style={recipientAddressTextStyle}>
                      {block.masternode_address}
                    </div>
                  ) : (
                    <span style={placeholderValueStyle}>—</span>
                  )}
                </div>
                {/* Dev Fund recipient. */}
                <div style={recipientCardStyle}>
                  <div style={recipientHeaderRowStyle}>
                    <div style={recipientLabelStyle}>{t('explorer.devFund')}</div>
                    {block.dev_address && (
                      <div style={recipientIconsStyle}>
                        <IconButton
                          icon={
                            copiedField === 'dev' ? (
                              <Check size={12} color="#27ae60" />
                            ) : (
                              <Copy size={12} />
                            )
                          }
                          title={t('explorer.copyAddress')}
                          ariaLabel={t('explorer.copyAddress')}
                          onClick={() => handleCopy('dev', block.dev_address)}
                        />
                        <IconButton
                          icon={<Eye size={12} />}
                          title={t('explorer.viewAddressDetails')}
                          ariaLabel={t('explorer.viewAddressDetails')}
                          onClick={() => onAddressClick(block.dev_address)}
                        />
                      </div>
                    )}
                  </div>
                  {block.dev_address ? (
                    <div title={block.dev_address} style={recipientAddressTextStyle}>
                      {block.dev_address}
                    </div>
                  ) : (
                    <span style={placeholderValueStyle}>—</span>
                  )}
                </div>
              </div>
            </>
          ) : block.total_reward > 0 || block.staker_address ? (
            /* PoW fallback: single Coinbase Reward column at hero width,
               single Miner Address recipient card below. */
            <>
              <div style={{ display: 'flex', flexDirection: 'column' }}>
                <div style={heroTotalLabelStyle}>{t('explorer.coinbaseReward')}</div>
                <div style={{ display: 'flex', alignItems: 'baseline' }}>
                  <span style={heroTotalValueStyle}>
                    {formatAmount(block.total_reward, false)}
                  </span>
                </div>
              </div>
              {block.staker_address && (
                <div style={{ ...recipientCardStyle, maxWidth: '500px' }}>
                  <div style={recipientHeaderRowStyle}>
                    <div style={recipientLabelStyle}>{t('explorer.minerAddress')}</div>
                    <div style={recipientIconsStyle}>
                      <IconButton
                        icon={
                          copiedField === 'miner' ? (
                            <Check size={12} color="#27ae60" />
                          ) : (
                            <Copy size={12} />
                          )
                        }
                        title={t('explorer.copyAddress')}
                        ariaLabel={t('explorer.copyAddress')}
                        onClick={() => handleCopy('miner', block.staker_address)}
                      />
                      <IconButton
                        icon={<Eye size={12} />}
                        title={t('explorer.viewAddressDetails')}
                        ariaLabel={t('explorer.viewAddressDetails')}
                        onClick={() => onAddressClick(block.staker_address)}
                      />
                    </div>
                  </div>
                  <div title={block.staker_address} style={recipientAddressTextStyle}>
                    {block.staker_address}
                  </div>
                </div>
              )}
            </>
          ) : null}
        </div>

        {/* 2-column grid: Block Info (left, 4 sections) + Transactions (right). */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))', gridAutoRows: 'minmax(0, 1fr)', gap: '12px', flex: 1, minHeight: 0 }}>
          <DashboardCard title={t('explorer.blockInfo')}>
            {/* Tab bar: one section visible at a time so the card body stays
                short enough for the whole page to fit in viewport without
                vertical scroll. PoS tab is omitted entirely on PoW blocks. */}
            <div style={tabBarStyle}>
              {(
                [
                  { id: 'identity' as const, label: t('explorer.identity') },
                  { id: 'metadata' as const, label: t('explorer.metadata') },
                  { id: 'consensus' as const, label: t('explorer.consensus') },
                  ...(block.is_pos
                    ? [{ id: 'pos' as const, label: t('explorer.proofOfStake') }]
                    : []),
                ]
              ).map((tab) => (
                <button
                  key={tab.id}
                  type="button"
                  onClick={() => setActiveTab(tab.id)}
                  style={tabButtonStyle(activeTab === tab.id)}
                >
                  {tab.label}
                </button>
              ))}
            </div>

            {activeTab === 'identity' && (<>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.hash')}</span>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                <span title={block.hash} style={{ ...monoValueStyle, fontSize: '12px' }}>
                  {truncateAddress(block.hash, 16, 16)}
                </span>
                <IconButton
                  icon={
                    copiedField === 'hash-info' ? (
                      <Check size={12} color="#27ae60" />
                    ) : (
                      <Copy size={12} />
                    )
                  }
                  title={t('explorer.copyHash')}
                  ariaLabel={t('explorer.copyHash')}
                  onClick={() => handleCopy('hash-info', block.hash)}
                />
              </div>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.merkleRoot')}</span>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                <span title={block.merkleroot} style={{ ...monoValueStyle, fontSize: '12px' }}>
                  {truncateAddress(block.merkleroot, 16, 16)}
                </span>
                <IconButton
                  icon={
                    copiedField === 'merkleroot' ? (
                      <Check size={12} color="#27ae60" />
                    ) : (
                      <Copy size={12} />
                    )
                  }
                  title={t('explorer.copyMerkleRoot')}
                  ariaLabel={t('explorer.copyMerkleRoot')}
                  onClick={() => handleCopy('merkleroot', block.merkleroot)}
                />
              </div>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.prevBlock')}</span>
              {block.previousblockhash ? (
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                  <span title={block.previousblockhash} style={{ ...monoValueStyle, fontSize: '12px' }}>
                    {truncateAddress(block.previousblockhash, 16, 16)}
                  </span>
                  <IconButton
                    icon={
                      copiedField === 'prev' ? (
                        <Check size={12} color="#27ae60" />
                      ) : (
                        <Copy size={12} />
                      )
                    }
                    title={t('explorer.copyHash')}
                    ariaLabel={t('explorer.copyHash')}
                    onClick={() => handleCopy('prev', block.previousblockhash)}
                  />
                </div>
              ) : (
                <span style={placeholderValueStyle}>—</span>
              )}
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.nextBlock')}</span>
              {block.nextblockhash ? (
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                  <span title={block.nextblockhash} style={{ ...monoValueStyle, fontSize: '12px' }}>
                    {truncateAddress(block.nextblockhash, 16, 16)}
                  </span>
                  <IconButton
                    icon={
                      copiedField === 'next' ? (
                        <Check size={12} color="#27ae60" />
                      ) : (
                        <Copy size={12} />
                      )
                    }
                    title={t('explorer.copyHash')}
                    ariaLabel={t('explorer.copyHash')}
                    onClick={() => handleCopy('next', block.nextblockhash)}
                  />
                </div>
              ) : (
                <span style={placeholderValueStyle}>({t('explorer.tipNoNext')})</span>
              )}
            </div>

            </>)}
            {activeTab === 'metadata' && (<>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.height')}</span>
              <span style={valueStyle}>{formattedHeight}</span>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.version')}</span>
              <span style={{ ...monoValueStyle, fontSize: '12px' }}>{versionHex}</span>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.timestamp')}</span>
              <span title={formattedTimeUTC} style={valueStyle}>
                {formattedTime}
              </span>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.size')}</span>
              <span style={valueStyle}>{block.size} B</span>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.confirmationsLabel')}</span>
              <span style={valueStyle}>{block.confirmations}</span>
            </div>

            </>)}
            {activeTab === 'consensus' && (<>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.difficulty')}</span>
              <span style={valueStyle}>
                {parseFloat(block.difficulty.toFixed(8)).toString()}
              </span>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.bits')}</span>
              <span style={{ ...monoValueStyle, fontSize: '12px' }}>{block.bits}</span>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.nonce')}</span>
              <span style={valueStyle}>{block.nonce}</span>
            </div>
            <div style={ledgerRowStyle}>
              <span style={labelStyle}>{t('explorer.chainwork')}</span>
              <span style={placeholderValueStyle}>{t('explorer.comingSoon')}</span>
            </div>

            </>)}
            {activeTab === 'pos' && block.is_pos && (
              <>
                <div style={ledgerRowStyle}>
                  <span style={labelStyle}>{t('explorer.staker')}</span>
                  {block.staker_address ? (
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                      <span title={block.staker_address} style={{ ...monoValueStyle, fontSize: '12px', whiteSpace: 'nowrap' }}>
                        {block.staker_address}
                      </span>
                      <IconButton
                        icon={
                          copiedField === 'staker-info' ? (
                            <Check size={12} color="#27ae60" />
                          ) : (
                            <Copy size={12} />
                          )
                        }
                        title={t('explorer.copyAddress')}
                        ariaLabel={t('explorer.copyAddress')}
                        onClick={() => handleCopy('staker-info', block.staker_address)}
                      />
                    </div>
                  ) : (
                    <span style={placeholderValueStyle}>—</span>
                  )}
                </div>
                {/* Kernel Hash row dropped — in TWINS / legacy PIVX,
                    hashProofOfStake IS the kernel hash (same 32-byte value,
                    two historical names from PIVX's PoS lineage). The single
                    Proof Hash row below covers both. */}
                <div style={ledgerRowStyle}>
                  <span style={labelStyle}>{t('explorer.proofHash')}</span>
                  {block.proof_hash ? (
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                      <span title={block.proof_hash} style={{ ...monoValueStyle, fontSize: '12px' }}>
                        {truncateAddress(block.proof_hash, 16, 16)}
                      </span>
                      <IconButton
                        icon={
                          copiedField === 'proof-hash' ? (
                            <Check size={12} color="#27ae60" />
                          ) : (
                            <Copy size={12} />
                          )
                        }
                        title={t('explorer.copyHash')}
                        ariaLabel={t('explorer.copyHash')}
                        onClick={() => handleCopy('proof-hash', block.proof_hash!)}
                      />
                    </div>
                  ) : (
                    <span style={placeholderValueStyle}>—</span>
                  )}
                </div>
                <div style={ledgerRowStyle}>
                  <span style={labelStyle}>{t('explorer.stakeAmount')}</span>
                  <span style={valueStyle}>{stakeAmountText}</span>
                </div>
                <div style={ledgerRowStyle}>
                  <span style={labelStyle}>{t('explorer.stakeAge')}</span>
                  <span style={valueStyle}>{stakeAgeText}</span>
                </div>
                <div style={ledgerRowStyle}>
                  <span style={labelStyle}>{t('explorer.stakeModifier')}</span>
                  {block.stake_modifier ? (
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                      <span title={block.stake_modifier} style={{ ...monoValueStyle, fontSize: '12px' }}>
                        {block.stake_modifier}
                      </span>
                      <IconButton
                        icon={
                          copiedField === 'stake-modifier' ? (
                            <Check size={12} color="#27ae60" />
                          ) : (
                            <Copy size={12} />
                          )
                        }
                        title={t('explorer.copyHash')}
                        ariaLabel={t('explorer.copyHash')}
                        onClick={() => handleCopy('stake-modifier', block.stake_modifier!)}
                      />
                    </div>
                  ) : (
                    <span style={placeholderValueStyle}>—</span>
                  )}
                </div>
              </>
            )}
          </DashboardCard>

          {/* Transactions card with per-row layout: [index] [hash 10...10 + Copy]
              [type colored text] [Eye]. Type rendered as plain colored text
              (no background pill): coinbase #ff9966, coinstake #27ae60,
              regular #888. coinstake at index 1 on PoS, coinbase at index 0
              on PoW, all others = regular. In/out counts and per-tx total
              value are not available on the BlockDetail payload (txids is
              just string[]) — those enrichments are deferred to a follow-up
              that adds richer tx metadata to the backend BlockDetail. */}
          <DashboardCard
            title={`${t('explorer.transactions')} (${block.txids?.length || 0})`}
          >
            {block.txids && block.txids.length > 0 ? (
              <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '2px 0' }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                  {block.txids.map((txid, index) => {
                    const copyKey = `tx-${index}`;
                    // Row column order: [index] [hash 6...4 + Copy] [type] [Eye].
                    // Type rendered as plain colored text (no background pill).
                    // coinbase at index 0, coinstake at index 1 on PoS, regular otherwise.
                    let badgeLabel: string;
                    let badgeColor: string;
                    if (index === 0) {
                      badgeLabel = t('explorer.txTypeCoinbase');
                      badgeColor = '#ff9966';
                    } else if (index === 1 && block.is_pos) {
                      badgeLabel = t('explorer.txTypeCoinstake');
                      badgeColor = '#27ae60';
                    } else {
                      badgeLabel = t('explorer.txTypeRegular');
                      badgeColor = '#888';
                    }
                    return (
                      <div
                        key={txid}
                        title={txid}
                        style={txRowCardStyle}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.borderColor = '#444';
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.borderColor = 'transparent';
                        }}
                      >
                        <span style={txRowIndexStyle}>{index}</span>
                        <div style={txRowHashGroupStyle}>
                          <span style={txRowTxidStyle}>
                            {truncateAddress(txid, 10, 10)}
                          </span>
                          <span
                            onClick={(e) => e.stopPropagation()}
                            onKeyDown={(e) => e.stopPropagation()}
                            style={{ display: 'inline-flex' }}
                          >
                            <IconButton
                              icon={
                                copiedField === copyKey ? (
                                  <Check size={12} color="#27ae60" />
                                ) : (
                                  <Copy size={12} />
                                )
                              }
                              title={t('explorer.copyTxId')}
                              ariaLabel={t('explorer.copyTxId')}
                              onClick={() => handleCopy(copyKey, txid)}
                            />
                          </span>
                        </div>
                        <span
                          style={{
                            ...txRowTypeBadgeStyle,
                            color: badgeColor,
                          }}
                        >
                          {badgeLabel}
                        </span>
                        <span
                          onClick={(e) => e.stopPropagation()}
                          onKeyDown={(e) => e.stopPropagation()}
                          style={{ display: 'inline-flex' }}
                        >
                          <IconButton
                            icon={<Eye size={12} />}
                            title={t('explorer.viewTxDetails')}
                            ariaLabel={t('explorer.viewTxDetails')}
                            onClick={() => onTxClick(txid)}
                          />
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : (
              <div style={{ color: '#888', fontSize: '12px' }}>{t('explorer.noTransactions')}</div>
            )}
          </DashboardCard>
        </div>
      </div>
    </div>
  );
};
