import type { SliceCreator } from '../store.types';
import { GetContacts, AddContact, EditContact, DeleteContact } from '@wailsjs/go/main/App';

export interface AddressBookContact {
  label: string;
  address: string;
  created: string;
}

export type AddressBookMode = 'edit' | 'select';

export interface AddressBookSlice {
  // State
  addressBookContacts: AddressBookContact[];
  isAddressBookLoading: boolean;
  isAddressBookDialogOpen: boolean;
  addressBookMode: AddressBookMode;
  addressBookSearchFilter: string;

  // Actions
  fetchContacts: () => Promise<void>;
  addContact: (label: string, address: string) => Promise<void>;
  editContact: (address: string, newLabel: string) => Promise<void>;
  deleteContact: (address: string) => Promise<void>;
  openAddressBookDialog: (mode?: AddressBookMode) => void;
  closeAddressBookDialog: () => void;
  setAddressBookSearchFilter: (filter: string) => void;
}

export const createAddressBookSlice: SliceCreator<AddressBookSlice> = (set) => ({
  // Initial state
  addressBookContacts: [],
  isAddressBookLoading: false,
  isAddressBookDialogOpen: false,
  addressBookMode: 'edit',
  addressBookSearchFilter: '',

  // Actions
  fetchContacts: async () => {
    set((state) => { state.isAddressBookLoading = true; });
    try {
      const contacts = await GetContacts();
      set((state) => {
        state.addressBookContacts = contacts || [];
        state.isAddressBookLoading = false;
      });
    } catch {
      set((state) => { state.isAddressBookLoading = false; });
    }
  },

  addContact: async (label: string, address: string) => {
    await AddContact(label, address);
    // Refresh the list
    const contacts = await GetContacts();
    set((state) => {
      state.addressBookContacts = contacts || [];
    });
  },

  editContact: async (address: string, newLabel: string) => {
    await EditContact(address, newLabel);
    const contacts = await GetContacts();
    set((state) => {
      state.addressBookContacts = contacts || [];
    });
  },

  deleteContact: async (address: string) => {
    await DeleteContact(address);
    const contacts = await GetContacts();
    set((state) => {
      state.addressBookContacts = contacts || [];
    });
  },

  openAddressBookDialog: (mode: AddressBookMode = 'edit') =>
    set((state) => {
      state.isAddressBookDialogOpen = true;
      state.addressBookMode = mode;
      state.addressBookSearchFilter = '';
    }),

  closeAddressBookDialog: () =>
    set((state) => {
      state.isAddressBookDialogOpen = false;
    }),

  setAddressBookSearchFilter: (filter: string) =>
    set((state) => {
      state.addressBookSearchFilter = filter;
    }),
});
