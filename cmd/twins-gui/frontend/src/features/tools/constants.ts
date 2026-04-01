export const ToolsTab = {
  Information: 0,
  Console: 1,
  NetworkTraffic: 2,
  Peers: 3,
  WalletRepair: 4,
} as const;

export type ToolsTabValue = typeof ToolsTab[keyof typeof ToolsTab];

export const TOOLS_TAB_NAMES = [
  'Information', 'Console', 'Network Traffic', 'Peers', 'Wallet Repair',
] as const;
