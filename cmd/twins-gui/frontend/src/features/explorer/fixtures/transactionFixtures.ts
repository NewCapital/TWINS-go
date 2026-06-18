// transactionFixtures.ts — Synthetic ExplorerTransaction fixtures for
// design review of the Inputs/Outputs panels in TransactionDetail.tsx.
//
// DEV-ONLY SCAFFOLDING — preserved in repo for future Tx/Block/Address
// detail-view redesign tasks (see m-tx-details-inputs-outputs-redesign).
// Consumed by `./FixtureOverlay.tsx`. Hidden by default — only renders
// when the `showFixtures` gate in `../pages/ExplorerPage.tsx` evaluates
// true. See that file for activation methods.
//
// Each fixture exercises a subset of the 12 `OutputRole*` classifications,
// overlay flags (is_dust / is_mine / is_change / is_spent / is_coinstake_kernel),
// multisig N-of-M payloads, OP_RETURN ASCII+binary, plus the two `groupOutputs`
// triggers (stake-split when ≥2 stake_returns; dust-collapse when ≥3 dust
// non-stake_return outputs). See `../utils/outputDisplay.ts` for the sort+group
// rules and `../utils/rewardBreakdown.ts` for the coinstake reward card.

import type { ExplorerTransaction, TxInput, TxOutput } from '@/store/slices/explorerSlice';

// 64-char deterministic-looking hex seeded from a string. Not cryptographic.
function makeTxid(seed: string): string {
  // Each ASCII char → 2 hex chars; full seed used so fixture names that share
  // a 10-char prefix (e.g. coinstake-pos vs coinstake-split, op-return-ascii vs
  // op-return-binary) get unique txids. Outer zero-pad fills to 64-char form.
  // Earlier `.slice(0, 10)` truncation caused 4 fixtures to share txids in 2
  // pairs (m-tx-details-io-redesign-v2 Phase 2.1.x fix, 2026-06-03).
  const hexChars = Array.from(seed).map((c) => c.charCodeAt(0).toString(16).padStart(2, '0')).join('');
  return (hexChars + '0'.repeat(64)).slice(0, 64);
}

// Visually plausible mainnet TWINS address (W-prefix + 33 chars). Not a real
// address — base58 alphabet is approximated.
function fixtureAddress(seed: string): string {
  const padded = (seed + 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa').slice(0, 33);
  return 'W' + padded;
}

function makeInput(overrides: Partial<TxInput> = {}): TxInput {
  return {
    txid: makeTxid('prev'),
    vout: 0,
    address: fixtureAddress('input'),
    amount: 0,
    is_coinbase: false,
    ...overrides,
  };
}

function makeOutput(overrides: Partial<TxOutput> = {}): TxOutput {
  return {
    index: 0,
    address: fixtureAddress('out'),
    amount: 0,
    script_type: 'pubkeyhash',
    is_spent: false,
    role: 'nonstandard',
    ...overrides,
  };
}

// Base ExplorerTransaction skeleton — every fixture overrides the lists +
// is_coinbase/is_coinstake + reward fields as needed. `time` is a fixed
// ISO string so screenshots are reproducible.
function makeTx(name: string, overrides: Partial<ExplorerTransaction>): ExplorerTransaction {
  const inputs = overrides.inputs ?? [];
  const outputs = overrides.outputs ?? [];
  const total_input = overrides.total_input ?? inputs.reduce((sum, i) => sum + i.amount, 0);
  const total_output = overrides.total_output ?? outputs.reduce((sum, o) => sum + o.amount, 0);
  return {
    txid: makeTxid(name),
    block_hash: makeTxid(name + '-block'),
    block_height: 1740000,
    confirmations: 100,
    time: '2026-05-30T12:00:00Z',
    size: 250,
    fee: overrides.fee ?? 0,
    is_coinbase: overrides.is_coinbase ?? false,
    is_coinstake: overrides.is_coinstake ?? false,
    stake_reward: overrides.stake_reward ?? 0,
    masternode_reward: overrides.masternode_reward ?? 0,
    dev_reward: overrides.dev_reward ?? 0,
    inputs,
    outputs,
    total_input,
    total_output,
    raw_hex: overrides.raw_hex,
  };
}

// ============================================================================
// 11 Fixtures
// ============================================================================

// 1. PoW coinbase — single mining_reward output.
const coinbasePow: ExplorerTransaction = makeTx('coinbase-pow', {
  is_coinbase: true,
  inputs: [makeInput({ txid: '0'.repeat(64), vout: 0xffffffff, address: '', amount: 0, is_coinbase: true })],
  outputs: [
    makeOutput({ index: 0, address: fixtureAddress('miner'), amount: 250, role: 'mining_reward', script_type: 'pubkeyhash' }),
  ],
});

// 2. Canonical PoS coinstake — empty marker + stake_return + MN + dev (4 outputs).
const coinstakePos: ExplorerTransaction = makeTx('coinstake-pos', {
  is_coinstake: true,
  stake_reward: 10,
  masternode_reward: 80,
  dev_reward: 10,
  inputs: [
    makeInput({ txid: makeTxid('kernel-funding'), vout: 1, address: fixtureAddress('staker'), amount: 1000, is_coinstake_kernel: true, is_mine: true }),
  ],
  outputs: [
    makeOutput({ index: 0, address: '', amount: 0, role: 'block_marker', script_type: 'nonstandard' }),
    makeOutput({ index: 1, address: fixtureAddress('staker'), amount: 1010, role: 'stake_return', script_type: 'pubkey', is_mine: true }),
    makeOutput({ index: 2, address: fixtureAddress('masternode01'), amount: 80, role: 'masternode_payment', script_type: 'pubkeyhash' }),
    makeOutput({ index: 3, address: fixtureAddress('devfund'), amount: 10, role: 'dev_fund', script_type: 'pubkeyhash' }),
  ],
});

// 3. Stake-split coinstake — 2x stake_return (triggers stake_split grouping).
const coinstakeSplit: ExplorerTransaction = makeTx('coinstake-split', {
  is_coinstake: true,
  stake_reward: 10,
  masternode_reward: 80,
  dev_reward: 10,
  inputs: [
    makeInput({ txid: makeTxid('kernel-funding'), vout: 1, address: fixtureAddress('staker'), amount: 1000, is_coinstake_kernel: true, is_mine: true }),
  ],
  outputs: [
    makeOutput({ index: 0, address: '', amount: 0, role: 'block_marker', script_type: 'nonstandard' }),
    makeOutput({ index: 1, address: fixtureAddress('staker'), amount: 505, role: 'stake_return', script_type: 'pubkey', is_mine: true }),
    makeOutput({ index: 2, address: fixtureAddress('staker'), amount: 505, role: 'stake_return', script_type: 'pubkey', is_mine: true }),
    makeOutput({ index: 3, address: fixtureAddress('masternode01'), amount: 80, role: 'masternode_payment', script_type: 'pubkeyhash' }),
    makeOutput({ index: 4, address: fixtureAddress('devfund'), amount: 10, role: 'dev_fund', script_type: 'pubkeyhash' }),
  ],
});

// 4. Regular send — 2 inputs (both is_mine), external_payment + change.
const regularSend: ExplorerTransaction = makeTx('regular-send', {
  fee: 0.0001,
  inputs: [
    makeInput({ txid: makeTxid('utxo-a'), vout: 0, address: fixtureAddress('wallet'), amount: 100, is_mine: true }),
    makeInput({ txid: makeTxid('utxo-b'), vout: 1, address: fixtureAddress('wallet'), amount: 50, is_mine: true }),
  ],
  outputs: [
    makeOutput({ index: 0, address: fixtureAddress('recipient'), amount: 99.9999, role: 'external_payment', script_type: 'pubkeyhash' }),
    makeOutput({ index: 1, address: fixtureAddress('changeaddr'), amount: 50, role: 'change', script_type: 'pubkeyhash', is_mine: true, is_change: true }),
  ],
});

// 5. Self-send — wallet → wallet (different address) + change.
const selfSend: ExplorerTransaction = makeTx('self-send', {
  fee: 0.0001,
  inputs: [
    makeInput({ txid: makeTxid('utxo-c'), vout: 0, address: fixtureAddress('walletA'), amount: 100, is_mine: true }),
  ],
  outputs: [
    makeOutput({ index: 0, address: fixtureAddress('walletB'), amount: 99, role: 'self_send', script_type: 'pubkeyhash', is_mine: true }),
    makeOutput({ index: 1, address: fixtureAddress('change'), amount: 0.9999, role: 'change', script_type: 'pubkeyhash', is_mine: true, is_change: true }),
  ],
});

// 6. Multisig output — 2-of-3 (M-OF-N chip).
const multisigOut: ExplorerTransaction = makeTx('multisig-out', {
  fee: 0.0001,
  inputs: [
    makeInput({ txid: makeTxid('utxo-d'), vout: 0, address: fixtureAddress('sender'), amount: 100 }),
  ],
  outputs: [
    makeOutput({
      index: 0,
      address: fixtureAddress('multisig01'),
      amount: 50,
      role: 'multisig',
      script_type: 'multisig',
      addresses: [
        fixtureAddress('key1'),
        fixtureAddress('key2'),
        fixtureAddress('key3'),
      ],
      required_sigs: 2,
    }),
    makeOutput({ index: 1, address: fixtureAddress('change'), amount: 49.9999, role: 'change', script_type: 'pubkeyhash', is_mine: true, is_change: true }),
  ],
});

// 7. OP_RETURN with printable ASCII payload.
const opReturnAscii: ExplorerTransaction = makeTx('op-return-ascii', {
  fee: 0.0001,
  inputs: [
    makeInput({ txid: makeTxid('utxo-e'), vout: 0, address: fixtureAddress('sender'), amount: 10 }),
  ],
  outputs: [
    makeOutput({
      index: 0,
      address: '',
      amount: 0,
      role: 'data_carrier',
      script_type: 'nulldata',
      data_hex: '48656c6c6f20776f726c6420636f6e74656e7421',
      data_ascii: 'Hello world content!',
    }),
    makeOutput({ index: 1, address: fixtureAddress('recipient'), amount: 9.9999, role: 'external_payment', script_type: 'pubkeyhash' }),
  ],
});

// 8. OP_RETURN with binary payload (no printable ASCII → data_ascii empty).
const opReturnBinary: ExplorerTransaction = makeTx('op-return-binary', {
  fee: 0.0001,
  inputs: [
    makeInput({ txid: makeTxid('utxo-f'), vout: 0, address: fixtureAddress('sender'), amount: 10 }),
  ],
  outputs: [
    makeOutput({
      index: 0,
      address: '',
      amount: 0,
      role: 'data_carrier',
      script_type: 'nulldata',
      data_hex: 'deadbeef0001020304ff0a1b2c3d4e5f607080',
      data_ascii: '',
    }),
    makeOutput({ index: 1, address: fixtureAddress('recipient'), amount: 9.9999, role: 'external_payment', script_type: 'pubkeyhash' }),
  ],
});

// 9. Dust collapse — 5 dust outputs of mixed non-stake_return roles + 2 normal
//    outputs. groupOutputs collapses when count(is_dust && role !== 'stake_return') ≥ 3.
const dustCollapse: ExplorerTransaction = makeTx('dust-collapse', {
  fee: 0.0001,
  inputs: [
    makeInput({ txid: makeTxid('utxo-g'), vout: 0, address: fixtureAddress('sender'), amount: 100, is_mine: true }),
  ],
  outputs: [
    makeOutput({ index: 0, address: fixtureAddress('recipientA'), amount: 30, role: 'external_payment', script_type: 'pubkeyhash' }),
    makeOutput({ index: 1, address: fixtureAddress('recipientB'), amount: 40, role: 'external_payment', script_type: 'pubkeyhash' }),
    makeOutput({ index: 2, address: fixtureAddress('dustA'), amount: 0.00001, role: 'external_payment', script_type: 'pubkeyhash', is_dust: true }),
    makeOutput({ index: 3, address: fixtureAddress('dustB'), amount: 0.00001, role: 'change', script_type: 'pubkeyhash', is_dust: true, is_mine: true, is_change: true }),
    makeOutput({ index: 4, address: fixtureAddress('dustC'), amount: 0.00001, role: 'external_payment', script_type: 'pubkeyhash', is_dust: true }),
    makeOutput({ index: 5, address: fixtureAddress('dustD'), amount: 0.00001, role: 'self_send', script_type: 'pubkeyhash', is_dust: true, is_mine: true }),
    makeOutput({ index: 6, address: fixtureAddress('dustE'), amount: 0.00001, role: 'change', script_type: 'pubkeyhash', is_dust: true, is_mine: true, is_change: true }),
  ],
});

// 10. Nonstandard output — unrecognized script.
const nonstandard: ExplorerTransaction = makeTx('nonstandard', {
  fee: 0.0001,
  inputs: [
    makeInput({ txid: makeTxid('utxo-h'), vout: 0, address: fixtureAddress('sender'), amount: 10 }),
  ],
  outputs: [
    makeOutput({ index: 0, address: '', amount: 5, role: 'nonstandard', script_type: 'nonstandard' }),
    makeOutput({ index: 1, address: fixtureAddress('recipient'), amount: 4.9999, role: 'external_payment', script_type: 'pubkeyhash' }),
  ],
});

// 11. Spent-mixed — same tx with is_spent toggled on some outputs.
const spentMixed: ExplorerTransaction = makeTx('spent-mixed', {
  fee: 0.0001,
  inputs: [
    makeInput({ txid: makeTxid('utxo-i'), vout: 0, address: fixtureAddress('wallet'), amount: 100, is_mine: true }),
  ],
  outputs: [
    makeOutput({ index: 0, address: fixtureAddress('recipientSpent'), amount: 50, role: 'external_payment', script_type: 'pubkeyhash', is_spent: true }),
    makeOutput({ index: 1, address: fixtureAddress('recipientUnspent'), amount: 30, role: 'external_payment', script_type: 'pubkeyhash', is_spent: false }),
    makeOutput({ index: 2, address: fixtureAddress('changeSpent'), amount: 19.9999, role: 'change', script_type: 'pubkeyhash', is_mine: true, is_change: true, is_spent: true }),
  ],
});

export const transactionFixtures: Record<string, ExplorerTransaction> = {
  'coinbase-pow': coinbasePow,
  'coinstake-pos': coinstakePos,
  'coinstake-split': coinstakeSplit,
  'regular-send': regularSend,
  'self-send': selfSend,
  'multisig-out': multisigOut,
  'op-return-ascii': opReturnAscii,
  'op-return-binary': opReturnBinary,
  'dust-collapse': dustCollapse,
  nonstandard,
  'spent-mixed': spentMixed,
};

export const fixtureNames = Object.keys(transactionFixtures);
