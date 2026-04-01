import type { SliceCreator } from '../store.types';

export type SignVerifyTab = 'sign' | 'verify';

export interface SignVerifySlice {
  // State
  isSignVerifyDialogOpen: boolean;
  signVerifyActiveTab: SignVerifyTab;

  // Actions
  openSignVerifyDialog: (tab?: SignVerifyTab) => void;
  closeSignVerifyDialog: () => void;
  setSignVerifyActiveTab: (tab: SignVerifyTab) => void;
}

export const createSignVerifySlice: SliceCreator<SignVerifySlice> = (set) => ({
  // Initial state
  isSignVerifyDialogOpen: false,
  signVerifyActiveTab: 'sign',

  // Actions
  openSignVerifyDialog: (tab?: SignVerifyTab) =>
    set((state) => {
      state.isSignVerifyDialogOpen = true;
      if (tab !== undefined) {
        state.signVerifyActiveTab = tab;
      }
    }),

  closeSignVerifyDialog: () =>
    set((state) => {
      state.isSignVerifyDialogOpen = false;
    }),

  setSignVerifyActiveTab: (tab: SignVerifyTab) =>
    set((state) => {
      state.signVerifyActiveTab = tab;
    }),
});
