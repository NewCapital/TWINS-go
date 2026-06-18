## Changelog

### v4.0.43

**Features & Improvements**

- **GUI Receive Design Language — App-Wide Rollout** — The unified "Receive" design language (shared tokens + primitives) propagated across Overview, Transactions, Explorer, and Masternodes — a coordinated multi-PR restyle of nearly every page shell, table, card, and dialog for visual consistency.
- **Overview Page Redesign** — Rebuilt into a 2×2 card grid (Balance / Sync / Network / Staking) with a horizontal-strip balance hero, a consolidated status card, and a full-width sync-progress row. Adds Money Supply, Chain Difficulty, Mempool Transactions, and Chain-Size-on-Disk to the Sync card.
- **Transactions Page Overhaul** — Chip-based filter bar with smart search, saved views, transaction-type multi-select, min/max amount range, redesigned pagination + export dialog, active-sort column highlighting, and anti-flicker on filter/sort/page changes.
- **Block Explorer Redesign** — Hero-card + 2-column layouts for Block, Transaction, and Address detail views; row-card Block List; real-time search with type detection, history dropdown, and polished not-found UX; auto-refresh with prominent prev/next navigation; and PoS internals (stake modifier + proof hash) surfaced on block detail.
- **Explorer Backend Enrichment** — `TxOutput.IsSpent` now populated from storage, DTO enrichment, P2PK script-type detection, coinstake reward breakdown, an OP_RETURN full-payload modal, and a split fast-basic / slow-stats address fetch with backend UTXO pagination.
- **Masternodes UI Restyle** — Page shell, action buttons, My/Network masternode tables, statistics panel, payment-stats tab, filters, setup wizard, and config/edit dialogs all restyled to the Receive design language. Includes a row-card table conversion, per-row Start actions, and status-gated controls.
- **Receive Page Polish** — Compact hero, custom unit select, server-side pagination for Recent Requests, a QR-card icon column (save image / new address), and style parity with the Transactions table.
- **Global Date/Age Display Format Setting** — Configurable date/age formatting across the GUI, with the timezone suffix moved into column headers and cross-view consistency fixes.
- **Shared UI Primitives Extracted** — Reusable PaginationFooter and RowsPerPageSelect components plus shared design tokens, consumed across Explorer, Transactions, and Masternodes.

**Bug Fixes**

- **maxPeers Not Capping Outbound Connections** — Outbound peer connections now respect the configured `maxPeers` limit.
- **Transactions Date Range Filter** — Uses local timezone with correct week-start and last-month end-bound; stale range inputs no longer stick.
- **Transactions Row Re-render Storm** — Opening a modal no longer triggers a full-list re-render.
- **Transactions Search** — Label-substring and recipient-address matching now work for sent transactions; fixed the inverted amount range.
- **Address Stats Slow on Large Addresses** — Optimized stats computation for high-activity addresses; fixed input/output value overflow and a row key collision.
- **Explorer Navigation Errors** — Fixed Block/Tx detail prev/next navigation and parent-stack handling; removed the BlockList page-switch loading flicker.
- **TransactionDetailsDialog Rules-of-Hooks Violation** — Fixed conditional hook ordering that could crash the dialog.
- **Tx Detail Correctness** — Corrected P2PK script-type detection and coinstake reward breakdown; fixed the misleading "To" address shown on send transactions.
- **Pagination Page-Input Width** — Widened the page-number input to fit large page counts.
- **Balance Card Label Casing** — Fixed inconsistent label casing on the balance card.
