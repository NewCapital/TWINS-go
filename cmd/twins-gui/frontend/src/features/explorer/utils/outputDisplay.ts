// outputDisplay.ts — Pure helpers for the Transaction Detail outputs panel.
//
// Implements the R3 (sorting) + R4 (grouping) rules from research
// `?-research-tx-inputs-outputs-display-system` (archived).
//
// Two pure functions:
//   sortOutputs(outputs):  TxOutput[]    — apply R3 priority sort
//   groupOutputs(sorted):  RenderItem[]  — apply R4 grouping (stake-split + dust-collapse)
//
// Consumer (TransactionDetail.tsx) pipes outputs through both:
//   const renderItems = useMemo(
//     () => groupOutputs(sortOutputs(transaction.outputs ?? [])),
//     [transaction.outputs],
//   );
//
// Caller-side responsibility: groupOutputs assumes its input is already sorted
// per sortOutputs. Passing unsorted input produces undefined (but harmless) row
// ordering inside groups.

import type { TxOutput } from '@/store/slices/explorerSlice';

// ---------------------------------------------------------------------------
// R3: Sorting priority table
// ---------------------------------------------------------------------------
//
// Maps each role string (the 12 backend OutputRole* constants from
// internal/gui/core/types.go) to its visual priority in the outputs panel.
// LOWER number = renders FIRST (top of panel).
//
// Priority 10 is intentionally absent from the role-to-priority map: it is
// reserved for the `is_dust` overlay, which sits between data_carrier (9) and
// nonstandard (11) regardless of the underlying role. The dust overlay is
// applied inline in `getPriority` below.
//
// Unknown/undefined roles fall through to nonstandard (priority 11) as
// defense-in-depth against a future backend role enum extension that the
// frontend dispatch table doesn't yet know about.

const ROLE_PRIORITY: Record<string, number> = {
  block_marker: 1,
  stake_return: 2,
  masternode_payment: 3,
  dev_fund: 4,
  external_payment: 5,
  multisig: 6,
  self_send: 7,
  change: 8,
  data_carrier: 9,
  // priority 10 = dust overlay (applied inline)
  nonstandard: 11,
  // PoW-rare additional roles fall back to nonstandard via getPriority().
  mining_reward: 5, // treat like external_payment visually (PoW coinbase reward)
  premine: 5,      // treat like external_payment visually (genesis-block premine)
};

function getPriority(output: TxOutput): number {
  // is_dust overlay overrides role priority (R3 §10).
  if (output.is_dust === true) return 10;
  const role = output.role ?? 'nonstandard';
  return ROLE_PRIORITY[role] ?? 11;
}

// ---------------------------------------------------------------------------
// sortOutputs
// ---------------------------------------------------------------------------
//
// Returns a NEW array (does not mutate input). Uses `.sort()` on a shallow
// copy. Array.prototype.sort is specified as stable since ES2019 (per ECMA-262
// §22.1.3.30) so non-tie-break buckets preserve the input order — which, when
// the caller passes wire-order outputs, equals protocol vout order.
//
// Tie-break rule (R3 §5): external_payment outputs within the bucket sorted by
// amount descending (largest first). Other priority buckets rely on stable sort.

export function sortOutputs(outputs: TxOutput[]): TxOutput[] {
  return [...outputs].sort((a, b) => {
    const pa = getPriority(a);
    const pb = getPriority(b);
    if (pa !== pb) return pa - pb;
    // Same priority: apply tie-break only inside the external_payment bucket.
    // Both must be external_payment AND non-dust (dust outputs share priority
    // 10 with other dust regardless of role) for the amount-desc tie-break
    // to apply. Without the dust guard, a dust external_payment and a dust
    // change would compare amounts even though they belong to the dust bucket.
    if (
      pa === 5 &&
      a.role === 'external_payment' &&
      b.role === 'external_payment' &&
      !a.is_dust &&
      !b.is_dust
    ) {
      return b.amount - a.amount; // desc
    }
    // All other ties: return 0 so stable sort preserves input order.
    return 0;
  });
}

// ---------------------------------------------------------------------------
// R4: Grouping rules — RenderItem discriminated union
// ---------------------------------------------------------------------------

export type RenderItem =
  | { kind: 'single'; output: TxOutput }
  | { kind: 'stake_split'; outputs: TxOutput[] }
  | { kind: 'dust_collapse'; visible: TxOutput[]; collapsed: TxOutput[] };

// ---------------------------------------------------------------------------
// groupOutputs
// ---------------------------------------------------------------------------
//
// Walks the (already-sorted) input list once and produces a list of render
// items. Three grouping rules per R4:
//
//   1. Stake split: count(role === 'stake_return') >= 2 → wrap ALL stake_returns
//      into one stake_split item. (Design choice over "first 2 only": semantic
//      consistency for the rare N > 2 edge case; aligned with user decision.)
//
//   2. Dust collapse: count(is_dust === true) >= 3 → first 2 dust in sorted
//      order are `visible`, rest are `collapsed`. count < 3 → render dust as
//      singles.
//
//   3. Precedence (when an output is both stake_return AND dust): stake_split
//      wins. The output is consumed by the stake_split group and does NOT
//      contribute to the dust_collapse threshold count. Rationale: stake_split
//      carries semantic meaning ("these are paired parts of one staking
//      reward"), while is_dust is just a "tiny" overlay flag; double-grouping
//      makes no sense.
//
// Pre-pass counts both totals so the single walk can dispatch correctly.
//
// Multisig grouping (R4 §4: all N keys in one row with expand affordance) is
// already handled inside <OutputRow> per MR !732 — no group wrapper needed.
//
// Multiple payments to same address (R4 §2): NOT deduplicated per spec;
// optional "× N" badge deferred to a follow-up task.

export function groupOutputs(sorted: TxOutput[]): RenderItem[] {
  // Pre-pass: count stake_returns and dust outputs. Note that stake_return AND
  // dust outputs are counted in `stakeReturnCount` (precedence rule 3 above),
  // but only NON-stake_return dust contributes to `dustCount` for the collapse
  // threshold decision.
  let stakeReturnCount = 0;
  let dustNonStakeReturnCount = 0;
  for (const o of sorted) {
    if (o.role === 'stake_return') {
      stakeReturnCount++;
    } else if (o.is_dust === true) {
      dustNonStakeReturnCount++;
    }
  }

  const groupStakeReturns = stakeReturnCount >= 2;
  const collapseDust = dustNonStakeReturnCount >= 3;

  const items: RenderItem[] = [];
  let stakeSplitAccumulator: TxOutput[] | null = groupStakeReturns ? [] : null;
  let stakeSplitInserted = false;

  // Dust collapse accumulators (built as we walk; emitted at the end).
  let dustVisible: TxOutput[] = [];
  let dustCollapsed: TxOutput[] = [];

  for (const o of sorted) {
    // Stake split grouping has precedence over dust collapse.
    if (o.role === 'stake_return' && groupStakeReturns) {
      stakeSplitAccumulator!.push(o);
      if (!stakeSplitInserted) {
        // Insert a placeholder slot for the stake_split group at the position
        // of the first stake_return. The accumulator reference lives in
        // `stakeSplitAccumulator` and will be filled by subsequent iterations.
        items.push({ kind: 'stake_split', outputs: stakeSplitAccumulator! });
        stakeSplitInserted = true;
      }
      continue;
    }

    // Dust collapse: non-stake_return dust outputs split between visible (first 2) and collapsed.
    if (o.is_dust === true && collapseDust) {
      if (dustVisible.length < 2) {
        dustVisible.push(o);
      } else {
        dustCollapsed.push(o);
      }
      continue;
    }

    // Default: single render item.
    items.push({ kind: 'single', output: o });
  }

  // Emit dust_collapse item at the end of the list. Per R3, dust sorts at
  // priority 10 → already at the bottom of the sorted input → appending here
  // preserves the correct visual order.
  if (collapseDust && dustVisible.length > 0) {
    items.push({
      kind: 'dust_collapse',
      visible: dustVisible,
      collapsed: dustCollapsed,
    });
  }

  return items;
}
