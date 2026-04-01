import type { SliceCreator } from '../store.types';

// Explorer types matching backend core.* types
export interface BlockSummary {
  height: number;
  hash: string;
  time: string;
  tx_count: number;
  size: number;
  is_pos: boolean;
  reward: number;
}

export interface BlockDetail {
  hash: string;
  height: number;
  confirmations: number;
  size: number;
  time: string;
  previousblockhash: string;
  nextblockhash: string;
  txids: string[];
  is_pos: boolean;
  stake_reward: number;
  masternode_reward: number;
  staker_address: string;
  masternode_address: string;
  total_reward: number;
  difficulty: number;
  bits: string;
  nonce: number;
  merkleroot: string;
}

export interface TxInput {
  txid: string;
  vout: number;
  address: string;
  amount: number;
  is_coinbase: boolean;
}

export interface TxOutput {
  index: number;
  address: string;
  amount: number;
  script_type: string;
  is_spent: boolean;
}

export interface ExplorerTransaction {
  txid: string;
  block_hash: string;
  block_height: number;
  confirmations: number;
  time: string;
  size: number;
  fee: number;
  is_coinbase: boolean;
  is_coinstake: boolean;
  inputs: TxInput[];
  outputs: TxOutput[];
  total_input: number;
  total_output: number;
  raw_hex?: string;
}

export interface AddressTx {
  txid: string;
  block_height: number;
  time: string;
  amount: number;
  confirmations: number;
}

export interface AddressUTXO {
  txid: string;
  vout: number;
  amount: number;
  confirmations: number;
  block_height: number;
}

export interface AddressInfo {
  address: string;
  balance: number;
  total_received: number;
  total_sent: number;
  tx_count: number;
  unconfirmed_balance: number;
  transactions: AddressTx[];
  utxos?: AddressUTXO[];
}

export interface AddressTxPage {
  transactions: AddressTx[];
  total: number;
  has_more: boolean;
}

export type SearchResultType = 'block' | 'transaction' | 'address' | 'not_found';

export interface SearchResult {
  type: SearchResultType;
  query: string;
  block?: BlockDetail;
  transaction?: ExplorerTransaction;
  address?: AddressInfo;
  error?: string;
}

// Explorer view types
export type ExplorerView = 'blocks' | 'block' | 'transaction' | 'address' | 'search';

// Parent context for back navigation (one level up)
export interface ParentContext {
  view: ExplorerView;
  blockHash?: string;
}

export interface ExplorerSlice {
  // State
  view: ExplorerView;
  blocks: BlockSummary[];
  currentBlock: BlockDetail | null;
  currentTransaction: ExplorerTransaction | null;
  explorerAddress: AddressInfo | null;
  searchResult: SearchResult | null;
  searchQuery: string;

  // Parent context for back navigation
  parentContext: ParentContext | null;

  // Pagination
  blocksPage: number;
  blocksPerPage: number;
  totalBlocks: number;

  // Loading states
  isLoadingBlocks: boolean;
  isLoadingBlock: boolean;
  isLoadingTransaction: boolean;
  isLoadingAddress: boolean;
  isSearching: boolean;

  // Error states
  error: string | null;

  // Actions
  setView: (view: ExplorerView) => void;
  setBlocks: (blocks: BlockSummary[]) => void;
  setCurrentBlock: (block: BlockDetail | null) => void;
  setCurrentTransaction: (tx: ExplorerTransaction | null) => void;
  setExplorerAddress: (address: AddressInfo | null) => void;
  setSearchResult: (result: SearchResult | null) => void;
  setSearchQuery: (query: string) => void;
  setBlocksPage: (page: number) => void;
  setTotalBlocks: (total: number) => void;
  setLoadingBlocks: (loading: boolean) => void;
  setLoadingBlock: (loading: boolean) => void;
  setLoadingTransaction: (loading: boolean) => void;
  setLoadingAddress: (loading: boolean) => void;
  setSearching: (searching: boolean) => void;
  setError: (error: string | null) => void;
  setParentContext: (ctx: ParentContext | null) => void;
  resetExplorer: () => void;
}

export const createExplorerSlice: SliceCreator<ExplorerSlice> = (set) => ({
  // Initial state
  view: 'blocks',
  blocks: [],
  currentBlock: null,
  currentTransaction: null,
  explorerAddress: null,
  searchResult: null,
  searchQuery: '',

  // Parent context for back navigation
  parentContext: null,

  // Pagination
  blocksPage: 0,
  blocksPerPage: 25,
  totalBlocks: 0,

  // Loading states
  isLoadingBlocks: false,
  isLoadingBlock: false,
  isLoadingTransaction: false,
  isLoadingAddress: false,
  isSearching: false,

  // Error states
  error: null,

  // Actions
  setView: (view) => set({ view }),
  setBlocks: (blocks) => set({ blocks }),
  setCurrentBlock: (block) => set({ currentBlock: block }),
  setCurrentTransaction: (tx) => set({ currentTransaction: tx }),
  setExplorerAddress: (address) => set({ explorerAddress: address }),
  setSearchResult: (result) => set({ searchResult: result }),
  setSearchQuery: (query) => set({ searchQuery: query }),
  setBlocksPage: (page) => set({ blocksPage: page }),
  setTotalBlocks: (total) => set({ totalBlocks: total }),
  setLoadingBlocks: (loading) => set({ isLoadingBlocks: loading }),
  setLoadingBlock: (loading) => set({ isLoadingBlock: loading }),
  setLoadingTransaction: (loading) => set({ isLoadingTransaction: loading }),
  setLoadingAddress: (loading) => set({ isLoadingAddress: loading }),
  setSearching: (searching) => set({ isSearching: searching }),
  setError: (error) => set({ error }),
  setParentContext: (ctx) => set({ parentContext: ctx }),

  resetExplorer: () =>
    set({
      view: 'blocks',
      blocks: [],
      currentBlock: null,
      currentTransaction: null,
      explorerAddress: null,
      searchResult: null,
      searchQuery: '',
      parentContext: null,
      blocksPage: 0,
      isLoadingBlocks: false,
      isLoadingBlock: false,
      isLoadingTransaction: false,
      isLoadingAddress: false,
      isSearching: false,
      error: null,
    }),
});
