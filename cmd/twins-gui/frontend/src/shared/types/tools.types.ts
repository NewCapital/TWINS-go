// Tools window types matching Go backend structs

export interface ToolsInfo {
  clientName: string;
  clientVersion: string;
  goVersion: string;
  platform: string;
  buildDate: string;
  databaseVersion: string;
  startupTime: number;
  dataDir: string;
  networkName: string;
  connections: number;
  inPeers: number;
  outPeers: number;
  blockCount: number;
  lastBlockTime: number;
  masternodeCount: number;
}

export interface RPCCommandResult {
  result?: unknown;
  error?: string;
  time: string;
}

export interface PeerDetail {
  id: number;
  address: string;
  alias: string;
  services: string;
  lastSend: number;
  lastRecv: number;
  bytesSent: number;
  bytesReceived: number;
  connTime: number;
  timeOffset: number;
  pingTime: number;
  pingWait: number;
  protocolVersion: number;
  userAgent: string;
  inbound: boolean;
  startHeight: number;
  banScore: number;
  syncedHeaders: number;
  syncedBlocks: number;
  syncedHeight: number;
  whitelisted: boolean;
}

export interface BannedPeerInfo {
  address: string;
  alias: string;
  bannedUntil: number;
  banCreated: number;
  reason: string;
}

export interface TrafficInfo {
  totalBytesRecv: number;
  totalBytesSent: number;
  peerCount: number;
}

export type ConsoleMessageType = 'command' | 'reply' | 'error' | 'info' | 'warning';

export interface ConsoleMessage {
  type: ConsoleMessageType;
  text: string;
  time: string;
}

export interface TrafficSample {
  timestamp: number;
  bytesIn: number;
  bytesOut: number;
  rateIn: number;   // KB/s received
  rateOut: number;   // KB/s sent
}

export type TrafficTimeRange = '5m' | '15m' | '30m' | '1h' | '6h' | '24h';
