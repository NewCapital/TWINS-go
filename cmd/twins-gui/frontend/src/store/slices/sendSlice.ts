import type { SliceCreator } from '../store.types';

export interface Recipient {
  id: string;
  address: string;
  amount: string;
  label: string;
}

export interface FeeLevel {
  id: 'fast' | 'normal' | 'slow' | 'custom';
  label: string;
  rate: number; // TWINS per KB
  estimatedBlocks: number;
  estimatedTime: string;
}

export interface FeeOption {
  level: 'slow' | 'normal' | 'fast' | 'custom';
  amount: number;
  confirmationTime: string;
}

export interface SendCoinControlConfig {
  enabled: boolean;
  selectedUTXOs: string[];
  totalSelected: number;
  changeAddress?: string;
  splitUTXO: boolean;
  splitOutputs: number;
}

export interface SendState {
  // Form state
  recipients: Recipient[];

  // Coin Control config (basic settings for Send page)
  sendCoinControlConfig: SendCoinControlConfig;

  // UI state
  isValidating: boolean;
  validationErrors: Record<string, string>;
  lastSentTxId?: string;
}

export interface SendActions {
  // Recipient management
  addRecipient: () => void;
  removeRecipient: (index: number) => void;
  updateRecipient: (index: number, recipient: Partial<Recipient>) => void;
  clearRecipients: () => void;

  // Coin Control
  toggleCoinControl: (enabled: boolean) => void;
  setSelectedUTXOs: (utxos: string[]) => void;
  setSendChangeAddress: (address: string) => void;
  toggleSplitUTXO: (enabled: boolean) => void;
  setSplitOutputs: (outputs: number) => void;

  // Validation
  setValidating: (validating: boolean) => void;
  setValidationError: (field: string, error: string) => void;
  clearValidationErrors: () => void;

  // Transaction
  setLastSentTxId: (txId: string) => void;
  resetSendForm: () => void;
}

export type SendSlice = SendState & SendActions;

const initialState: SendState = {
  recipients: [
    {
      id: crypto.randomUUID(),
      address: '',
      amount: '',
      label: '',
    },
  ],
  sendCoinControlConfig: {
    enabled: false,
    selectedUTXOs: [],
    totalSelected: 0,
    splitUTXO: false,
    splitOutputs: 2,
  },
  isValidating: false,
  validationErrors: {},
};

export const createSendSlice: SliceCreator<SendSlice> = (set) => ({
  ...initialState,

  // Recipient management
  addRecipient: () =>
    set((state) => ({
      recipients: [
        ...state.recipients,
        {
          id: crypto.randomUUID(),
          address: '',
          amount: '',
          label: '',
        },
      ],
    })),

  removeRecipient: (index) =>
    set((state) => ({
      recipients: state.recipients.filter((_, i) => i !== index),
    })),

  updateRecipient: (index, recipient) =>
    set((state) => ({
      recipients: state.recipients.map((r, i) =>
        i === index ? { ...r, ...recipient } : r
      ),
    })),

  clearRecipients: () =>
    set(() => ({
      recipients: [
        {
          id: crypto.randomUUID(),
          address: '',
          amount: '',
          label: '',
        },
      ],
    })),

  // Coin Control
  toggleCoinControl: (enabled) =>
    set((state) => ({
      sendCoinControlConfig: {
        ...state.sendCoinControlConfig,
        enabled,
      },
    })),

  setSelectedUTXOs: (utxos) =>
    set((state) => ({
      sendCoinControlConfig: {
        ...state.sendCoinControlConfig,
        selectedUTXOs: utxos,
      },
    })),

  setSendChangeAddress: (address) =>
    set((state) => ({
      sendCoinControlConfig: {
        ...state.sendCoinControlConfig,
        changeAddress: address,
      },
    })),

  toggleSplitUTXO: (enabled) =>
    set((state) => ({
      sendCoinControlConfig: {
        ...state.sendCoinControlConfig,
        splitUTXO: enabled,
      },
    })),

  setSplitOutputs: (outputs) =>
    set((state) => ({
      sendCoinControlConfig: {
        ...state.sendCoinControlConfig,
        splitOutputs: outputs,
      },
    })),

  // Validation
  setValidating: (validating) =>
    set(() => ({
      isValidating: validating,
    })),

  setValidationError: (field, error) =>
    set((state) => ({
      validationErrors: {
        ...state.validationErrors,
        [field]: error,
      },
    })),

  clearValidationErrors: () =>
    set(() => ({
      validationErrors: {},
    })),

  // Transaction
  setLastSentTxId: (txId) =>
    set(() => ({
      lastSentTxId: txId,
    })),

  resetSendForm: () =>
    set(() => ({
      ...initialState,
      recipients: [
        {
          id: crypto.randomUUID(),
          address: '',
          amount: '',
          label: '',
        },
      ],
    })),
});