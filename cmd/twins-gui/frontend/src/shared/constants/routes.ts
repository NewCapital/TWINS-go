export const ROUTES = {

  // Main routes
  DASHBOARD: '/',
  WALLET: '/wallet',
  SEND: '/wallet/send',
  RECEIVE: '/wallet/receive',
  TRANSACTIONS: '/wallet/transactions',

  // Masternode routes
  MASTERNODES: '/masternodes',
  MASTERNODE_DETAIL: '/masternodes/:id',

  // Explorer routes
  EXPLORER: '/explorer',
  EXPLORER_BLOCK: '/explorer/block/:query',
  EXPLORER_TX: '/explorer/tx/:txid',
  EXPLORER_ADDRESS: '/explorer/address/:address',

  // Staking routes
  STAKING: '/staking',
} as const;

export type RoutePath = typeof ROUTES[keyof typeof ROUTES];