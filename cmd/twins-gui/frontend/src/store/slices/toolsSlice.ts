import type { SliceCreator } from '../store.types';
import type { ConsoleMessage } from '@/shared/types/tools.types';
import { ToolsTab, type ToolsTabValue } from '@/features/tools/constants';

const MAX_HISTORY = 50;
const MAX_OUTPUT = 500;

export interface RepairResult {
  action: string;
  success: boolean;
  error?: string;
}

export interface ToolsSlice {
  // State
  isToolsDialogOpen: boolean;
  toolsActiveTab: ToolsTabValue;
  consoleHistory: string[];
  consoleOutput: ConsoleMessage[];
  rpcCommands: string[];
  rpcCommandCategories: Record<string, string[]>;
  rpcCommandDescriptions: Record<string, string>;
  rpcCategoryOrder: string[];
  lastRepairResult: RepairResult | null;

  // Actions
  openToolsDialog: (tab?: ToolsTabValue) => void;
  closeToolsDialog: () => void;
  setToolsActiveTab: (tab: ToolsTabValue) => void;
  addConsoleMessage: (msg: ConsoleMessage) => void;
  clearConsole: () => void;
  addToHistory: (command: string) => void;
  setRPCCommands: (commands: string[]) => void;
  setRPCCommandCategories: (categories: Record<string, string[]>) => void;
  setRPCCommandDescriptions: (descriptions: Record<string, string>) => void;
  setRPCCategoryOrder: (order: string[]) => void;
  setLastRepairResult: (result: RepairResult | null) => void;
}

export const createToolsSlice: SliceCreator<ToolsSlice> = (set) => ({
  // Initial state
  isToolsDialogOpen: false,
  toolsActiveTab: ToolsTab.Information,
  consoleHistory: [],
  consoleOutput: [],
  rpcCommands: [],
  rpcCommandCategories: {},
  rpcCommandDescriptions: {},
  rpcCategoryOrder: [],
  lastRepairResult: null,

  // Actions
  openToolsDialog: (tab?: ToolsTabValue) =>
    set((state) => {
      state.isToolsDialogOpen = true;
      if (tab !== undefined) {
        state.toolsActiveTab = tab;
      }
    }),

  closeToolsDialog: () =>
    set((state) => {
      state.isToolsDialogOpen = false;
    }),

  setToolsActiveTab: (tab: ToolsTabValue) =>
    set((state) => {
      state.toolsActiveTab = tab;
    }),

  addConsoleMessage: (msg: ConsoleMessage) =>
    set((state) => {
      state.consoleOutput.push(msg);
      // Trim if over max
      if (state.consoleOutput.length > MAX_OUTPUT) {
        state.consoleOutput = state.consoleOutput.slice(-MAX_OUTPUT);
      }
    }),

  clearConsole: () =>
    set((state) => {
      state.consoleOutput = [];
    }),

  addToHistory: (command: string) =>
    set((state) => {
      // Don't add duplicates of the last command
      if (state.consoleHistory.length > 0 && state.consoleHistory[state.consoleHistory.length - 1] === command) {
        return;
      }
      state.consoleHistory.push(command);
      if (state.consoleHistory.length > MAX_HISTORY) {
        state.consoleHistory = state.consoleHistory.slice(-MAX_HISTORY);
      }
    }),

  setRPCCommands: (commands: string[]) =>
    set((state) => {
      state.rpcCommands = commands;
    }),

  setRPCCommandCategories: (categories: Record<string, string[]>) =>
    set((state) => {
      state.rpcCommandCategories = categories;
    }),

  setRPCCommandDescriptions: (descriptions: Record<string, string>) =>
    set((state) => {
      state.rpcCommandDescriptions = descriptions;
    }),

  setRPCCategoryOrder: (order: string[]) =>
    set((state) => {
      state.rpcCategoryOrder = order;
    }),

  setLastRepairResult: (result: RepairResult | null) =>
    set((state) => {
      state.lastRepairResult = result;
    }),
});
