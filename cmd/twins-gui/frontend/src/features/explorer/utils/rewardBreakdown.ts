// rewardBreakdown.ts — Pure helper for the Transaction Detail reward card.
//
// Implements R6 of the Tx Explorer redesign series. Derives the coinstake
// reward breakdown from per-output `role` filters (gross output sum semantics)
// instead of reading the top-level backend `transaction.stake_reward` /
// `masternode_reward` / `dev_reward` fields. Those backend fields remain on
// the wire and are still consumed by BlockDetail.tsx — this helper is the
// frontend-only alternative for the TransactionDetail card.
//
// Gross semantics:
//   - Each column total is the SUM of `.amount` over outputs in that role bucket.
//   - For role=='stake_return' this INCLUDES the staker's principal (not just
//     the net reward delta). The mismatch with the prior `transaction.stake_reward`
//     value (= gross − sum(inputs)) is intentional: per-recipient detail rows
//     under the column show gross output values, and the column header total
//     must equal their sum.
//   - For role=='masternode_payment' and role=='dev_fund' there are no inputs
//     flowing through, so gross == net (no semantic change vs. the prior card).
//
// Ordering: outputs are pushed into role buckets in the order they appear in
// the input array. This is a caller responsibility — the helper does NOT sort.
// In practice, callers pass `transaction.outputs` (protocol vout order), which
// preserves the canonical TWINS PoS coinstake layout: block_marker (vout 0),
// stake_return(s), masternode_payment, dev_fund.

import type { TxOutput } from '@/store/slices/explorerSlice';

export interface CoinstakeReward {
  stake: TxOutput[];
  masternode: TxOutput[];
  dev: TxOutput[];
  stakeTotal: number;
  masternodeTotal: number;
  devTotal: number;
  total: number;
}

export function deriveCoinstakeReward(outputs: TxOutput[]): CoinstakeReward {
  const stake: TxOutput[] = [];
  const masternode: TxOutput[] = [];
  const dev: TxOutput[] = [];

  for (const o of outputs) {
    switch (o.role) {
      case 'stake_return':
        stake.push(o);
        break;
      case 'masternode_payment':
        masternode.push(o);
        break;
      case 'dev_fund':
        dev.push(o);
        break;
      default:
        // block_marker, change, external_payment, self_send, data_carrier,
        // mining_reward, premine, nonstandard, multisig — not part of the
        // PoS coinstake reward breakdown. Silently skipped.
        break;
    }
  }

  const stakeTotal = stake.reduce((s, o) => s + o.amount, 0);
  const masternodeTotal = masternode.reduce((s, o) => s + o.amount, 0);
  const devTotal = dev.reduce((s, o) => s + o.amount, 0);

  return {
    stake,
    masternode,
    dev,
    stakeTotal,
    masternodeTotal,
    devTotal,
    total: stakeTotal + masternodeTotal + devTotal,
  };
}
