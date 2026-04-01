import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { X, Plus, Download, Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useAddressBook, useNotifications } from '@/store/useStore';
import { CopyToClipboard, ExportContactsCSV } from '@wailsjs/go/main/App';
import { SimpleConfirmDialog } from '@/shared/components/SimpleConfirmDialog';
import type { AddressBookContact } from '@/store/slices/addressBookSlice';

// TWINS address regex (W=mainnet P2PKH, a=P2SH, m/n=testnet)
const TWINS_ADDRESS_REGEX = /^[Wamn][a-km-zA-HJ-NP-Z1-9]{33}$/;

interface AddEditDialogProps {
  isOpen: boolean;
  editContact: AddressBookContact | null; // null = add mode
  onSave: (label: string, address: string) => void;
  onCancel: () => void;
  error: string;
}

// Inline add/edit dialog
const AddEditDialog: React.FC<AddEditDialogProps> = ({ isOpen, editContact, onSave, onCancel, error }) => {
  const { t } = useTranslation('wallet');
  const [label, setLabel] = useState('');
  const [address, setAddress] = useState('');
  const labelRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      if (editContact) {
        setLabel(editContact.label);
        setAddress(editContact.address);
      } else {
        setLabel('');
        setAddress('');
      }
      setTimeout(() => labelRef.current?.focus(), 50);
    }
  }, [isOpen, editContact]);

  if (!isOpen) return null;

  const isEditing = editContact !== null;

  return (
    <div style={{
      position: 'fixed', inset: 0, zIndex: 1002,
      backgroundColor: 'rgba(0,0,0,0.5)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }} onClick={onCancel}>
      <div style={{
        backgroundColor: '#2b2b2b', border: '1px solid #555',
        borderRadius: '4px', padding: '16px', width: '420px',
      }} onClick={(e) => e.stopPropagation()}>
        <h3 style={{ color: '#ddd', margin: '0 0 12px 0', fontSize: '14px' }}>
          {isEditing ? t('addressBook.edit') : t('addressBook.add')}
        </h3>

        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <label style={{ color: '#aaa', fontSize: '12px', width: '60px', textAlign: 'right' }}>
              {t('addressBook.label')}:
            </label>
            <input
              ref={labelRef}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              className="qt-input"
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              style={{
                flex: 1, padding: '4px 6px', fontSize: '12px',
                backgroundColor: '#1e1e1e', border: '1px solid #555', borderRadius: '2px', color: '#ddd',
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') onSave(label, address);
                if (e.key === 'Escape') onCancel();
              }}
            />
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <label style={{ color: '#aaa', fontSize: '12px', width: '60px', textAlign: 'right' }}>
              {t('addressBook.address')}:
            </label>
            <input
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              disabled={isEditing}
              className="qt-input"
              autoCapitalize="off"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              style={{
                flex: 1, padding: '4px 6px', fontSize: '12px', fontFamily: 'monospace',
                backgroundColor: isEditing ? '#333' : '#1e1e1e',
                border: '1px solid #555', borderRadius: '2px', color: '#ddd',
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') onSave(label, address);
                if (e.key === 'Escape') onCancel();
              }}
            />
          </div>

          {error && (
            <div style={{ color: '#ff6666', fontSize: '11px', marginLeft: '68px' }}>{error}</div>
          )}
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px', marginTop: '12px' }}>
          <button className="qt-button" onClick={onCancel}
            style={{ padding: '4px 16px', fontSize: '12px', backgroundColor: '#404040', border: '1px solid #555', borderRadius: '2px', color: '#ddd', cursor: 'pointer' }}>
            {t('addressBook.cancel')}
          </button>
          <button className="qt-button" onClick={() => onSave(label, address)}
            style={{ padding: '4px 16px', fontSize: '12px', backgroundColor: '#404040', border: '1px solid #555', borderRadius: '2px', color: '#ddd', cursor: 'pointer' }}>
            {t('addressBook.ok')}
          </button>
        </div>
      </div>
    </div>
  );
};

// Context menu component
interface ContextMenuProps {
  x: number;
  y: number;
  contact: AddressBookContact;
  onCopyAddress: () => void;
  onCopyLabel: () => void;
  onEdit: () => void;
  onDelete: () => void;
}

const ContextMenu: React.FC<ContextMenuProps> = ({ x, y, onCopyAddress, onCopyLabel, onEdit, onDelete }) => {
  const { t } = useTranslation('wallet');
  const menuItems = [
    { label: t('addressBook.contextMenu.copyAddress'), onClick: onCopyAddress },
    { label: t('addressBook.contextMenu.copyLabel'), onClick: onCopyLabel },
    { type: 'separator' as const },
    { label: t('addressBook.contextMenu.edit'), onClick: onEdit },
    { label: t('addressBook.contextMenu.delete'), onClick: onDelete },
  ];

  return (
    <div role="menu" aria-label="Contact context menu" style={{
      position: 'fixed', left: x, top: y, zIndex: 1003,
      backgroundColor: '#3a3a3a', border: '1px solid #555',
      borderRadius: '2px', padding: '4px 0', minWidth: '140px',
      boxShadow: '0 2px 8px rgba(0,0,0,0.5)',
    }}>
      {menuItems.map((item, i) =>
        'type' in item && item.type === 'separator'
          ? <div key={i} style={{ borderTop: '1px solid #555', margin: '4px 0' }} />
          : <div key={i} role="menuitem" onClick={item.onClick} style={{
              padding: '4px 16px', fontSize: '12px', color: '#ddd', cursor: 'pointer',
            }}
            onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = '#505050')}
            onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}>
              {item.label}
            </div>
      )}
    </div>
  );
};

// Module-level callback for select mode (bridges useAddressBookPicker and AddressBookDialog)
let _pendingSelectCallback: ((address: string, label: string) => void) | null = null;

// Sorting
type SortColumn = 'label' | 'address';
type SortDirection = 'asc' | 'desc';

export const AddressBookDialog: React.FC = () => {
  const { t } = useTranslation('wallet');
  const {
    contacts, isLoading, isDialogOpen, mode, searchFilter,
    fetchContacts, addContact, editContact, deleteContact,
    closeAddressBookDialog, setSearchFilter,
  } = useAddressBook();
  const { addNotification } = useNotifications();

  // Local state
  const [selectedAddress, setSelectedAddress] = useState<string | null>(null);
  const [sortColumn, setSortColumn] = useState<SortColumn>('label');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; contact: AddressBookContact } | null>(null);
  const [showAddEdit, setShowAddEdit] = useState(false);
  const [editingContact, setEditingContact] = useState<AddressBookContact | null>(null);
  const [addEditError, setAddEditError] = useState('');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deletingContact, setDeletingContact] = useState<AddressBookContact | null>(null);

  const mountedRef = useRef(true);
  const contextMenuRef = useRef<HTMLDivElement>(null);

  // Load contacts when dialog opens
  useEffect(() => {
    mountedRef.current = true;
    if (isDialogOpen) {
      fetchContacts();
      setSelectedAddress(null);
      setContextMenu(null);
    }
    return () => { mountedRef.current = false; };
  }, [isDialogOpen, fetchContacts]);

  // Close context menu on click outside
  useEffect(() => {
    if (!contextMenu) return;
    const handleMouseDown = (e: MouseEvent) => {
      if (contextMenuRef.current && !contextMenuRef.current.contains(e.target as Node)) {
        setContextMenu(null);
      }
    };
    document.addEventListener('mousedown', handleMouseDown);
    return () => document.removeEventListener('mousedown', handleMouseDown);
  }, [contextMenu]);

  // Wrap close to clear any pending select callback (prevents stale callbacks)
  const handleClose = useCallback(() => {
    _pendingSelectCallback = null;
    closeAddressBookDialog();
  }, [closeAddressBookDialog]);

  // Keyboard handler
  useEffect(() => {
    if (!isDialogOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (showAddEdit) {
          setShowAddEdit(false);
          setAddEditError('');
        } else if (contextMenu) {
          setContextMenu(null);
        } else {
          handleClose();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isDialogOpen, showAddEdit, contextMenu, handleClose]);

  // Filtered and sorted contacts
  const filteredContacts = useMemo(() => {
    let result = contacts;
    if (searchFilter) {
      const lower = searchFilter.toLowerCase();
      result = result.filter(
        (c) => c.label.toLowerCase().includes(lower) || c.address.toLowerCase().includes(lower)
      );
    }
    result = [...result].sort((a, b) => {
      const aVal = sortColumn === 'label' ? a.label : a.address;
      const bVal = sortColumn === 'label' ? b.label : b.address;
      const cmp = aVal.localeCompare(bVal);
      return sortDirection === 'asc' ? cmp : -cmp;
    });
    return result;
  }, [contacts, searchFilter, sortColumn, sortDirection]);

  const handleSort = useCallback((col: SortColumn) => {
    if (sortColumn === col) {
      setSortDirection((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortColumn(col);
      setSortDirection('asc');
    }
  }, [sortColumn]);

  const handleContextMenu = useCallback((e: React.MouseEvent, contact: AddressBookContact) => {
    e.preventDefault();
    setSelectedAddress(contact.address);
    // Viewport boundary checking
    const menuW = 160, menuH = 140;
    let x = e.clientX, y = e.clientY;
    if (x + menuW > window.innerWidth - 10) x = window.innerWidth - menuW - 10;
    if (y + menuH > window.innerHeight - 10) y = window.innerHeight - menuH - 10;
    setContextMenu({ x, y, contact });
  }, []);

  const handleCopyAddress = useCallback(async () => {
    if (!contextMenu) return;
    try {
      await CopyToClipboard(contextMenu.contact.address);
      addNotification({ type: 'success', title: t('addressBook.copied'), message: t('addressBook.addressCopied'), duration: 2000 });
    } catch { /* ignore */ }
    setContextMenu(null);
  }, [contextMenu, addNotification]);

  const handleCopyLabel = useCallback(async () => {
    if (!contextMenu) return;
    try {
      await CopyToClipboard(contextMenu.contact.label);
      addNotification({ type: 'success', title: t('addressBook.copied'), message: t('addressBook.labelCopied'), duration: 2000 });
    } catch { /* ignore */ }
    setContextMenu(null);
  }, [contextMenu, addNotification]);

  const handleEditFromContext = useCallback(() => {
    if (!contextMenu) return;
    setEditingContact(contextMenu.contact);
    setAddEditError('');
    setShowAddEdit(true);
    setContextMenu(null);
  }, [contextMenu]);

  const handleDeleteFromContext = useCallback(() => {
    if (!contextMenu) return;
    setDeletingContact(contextMenu.contact);
    setShowDeleteConfirm(true);
    setContextMenu(null);
  }, [contextMenu]);

  const handleAddNew = useCallback(() => {
    setEditingContact(null);
    setAddEditError('');
    setShowAddEdit(true);
  }, []);

  const handleSaveContact = useCallback(async (label: string, address: string) => {
    const trimLabel = label.trim();
    const trimAddress = address.trim();

    if (!trimLabel) {
      setAddEditError(t('addressBook.validation.labelEmpty'));
      return;
    }

    if (editingContact) {
      // Edit mode - only label changes
      try {
        await editContact(editingContact.address, trimLabel);
        setShowAddEdit(false);
        setAddEditError('');
      } catch (err: any) {
        setAddEditError(err?.message || String(err));
      }
    } else {
      // Add mode
      if (!trimAddress) {
        setAddEditError(t('addressBook.validation.addressEmpty'));
        return;
      }
      if (!TWINS_ADDRESS_REGEX.test(trimAddress)) {
        setAddEditError(t('addressBook.validation.invalidAddress'));
        return;
      }
      try {
        await addContact(trimLabel, trimAddress);
        setShowAddEdit(false);
        setAddEditError('');
      } catch (err: any) {
        setAddEditError(err?.message || String(err));
      }
    }
  }, [editingContact, editContact, addContact, t]);

  const handleConfirmDelete = useCallback(async () => {
    if (!deletingContact) return;
    try {
      await deleteContact(deletingContact.address);
    } catch (err: any) {
      addNotification({ type: 'error', title: t('addressBook.delete'), message: t('addressBook.deleteError', { error: err?.message || err }), duration: 5000 });
    }
    setShowDeleteConfirm(false);
    setDeletingContact(null);
  }, [deletingContact, deleteContact, addNotification]);

  const handleExportCSV = useCallback(async () => {
    try {
      await ExportContactsCSV();
    } catch (err: any) {
      addNotification({ type: 'error', title: t('addressBook.exportFailed'), message: err?.message || String(err), duration: 5000 });
    }
  }, [addNotification]);

  const handleRowDoubleClick = useCallback((contact: AddressBookContact) => {
    if (mode === 'select' && _pendingSelectCallback) {
      _pendingSelectCallback(contact.address, contact.label);
      _pendingSelectCallback = null;
      closeAddressBookDialog();
    } else {
      // In edit mode, double-click opens edit
      setEditingContact(contact);
      setAddEditError('');
      setShowAddEdit(true);
    }
  }, [mode, closeAddressBookDialog]);

  const handleChoose = useCallback(() => {
    const selected = contacts.find((c) => c.address === selectedAddress);
    if (selected && _pendingSelectCallback) {
      _pendingSelectCallback(selected.address, selected.label);
      _pendingSelectCallback = null;
      closeAddressBookDialog();
    }
  }, [contacts, selectedAddress, closeAddressBookDialog]);

  if (!isDialogOpen) return null;

  return (
    <>
      {/* Overlay */}
      <div style={{
        position: 'fixed', inset: 0, zIndex: 1000,
        backgroundColor: 'rgba(0,0,0,0.6)',
      }} onClick={handleClose} />

      {/* Modal */}
      <div style={{
        position: 'fixed', top: '50%', left: '50%',
        transform: 'translate(-50%, -50%)', zIndex: 1001,
        backgroundColor: '#2b2b2b', border: '1px solid #555',
        borderRadius: '4px', width: '620px', maxHeight: '80vh',
        display: 'flex', flexDirection: 'column',
      }}>
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '10px 16px', borderBottom: '1px solid #444',
        }}>
          <h2 style={{ color: '#ddd', fontSize: '14px', margin: 0 }}>
            {t('addressBook.title')}
          </h2>
          <button onClick={handleClose} style={{
            background: 'none', border: 'none', color: '#888', cursor: 'pointer', padding: '2px',
          }}>
            <X size={18} />
          </button>
        </div>

        {/* Toolbar */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: '8px',
          padding: '8px 16px', borderBottom: '1px solid #444',
        }}>
          <button className="qt-button" onClick={handleAddNew} title={t('addressBook.add')}
            style={{ padding: '4px 8px', fontSize: '11px', backgroundColor: '#404040', border: '1px solid #555', borderRadius: '2px', color: '#ddd', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '4px' }}>
            <Plus size={12} /> {t('addressBook.new')}
          </button>
          <button className="qt-button" onClick={handleExportCSV} title={t('addressBook.exportCSV')}
            style={{ padding: '4px 8px', fontSize: '11px', backgroundColor: '#404040', border: '1px solid #555', borderRadius: '2px', color: '#ddd', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '4px' }}>
            <Download size={12} /> {t('addressBook.export')}
          </button>
          <div style={{ flex: 1 }} />
          <div style={{ position: 'relative' }}>
            <Search size={12} style={{ position: 'absolute', left: '6px', top: '50%', transform: 'translateY(-50%)', color: '#888' }} />
            <input
              value={searchFilter}
              onChange={(e) => setSearchFilter(e.target.value)}
              placeholder={t('addressBook.search')}
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              style={{
                padding: '4px 6px 4px 22px', fontSize: '11px', width: '180px',
                backgroundColor: '#1e1e1e', border: '1px solid #555', borderRadius: '2px', color: '#ddd',
              }}
            />
          </div>
        </div>

        {/* Table */}
        <div style={{ flex: 1, overflowY: 'auto', minHeight: '200px', maxHeight: '400px' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead style={{ position: 'sticky', top: 0, zIndex: 1 }}>
              <tr>
                <th onClick={() => handleSort('label')} style={{
                  padding: '6px 12px', textAlign: 'left', fontSize: '11px', color: '#aaa',
                  backgroundColor: '#3a3a3a', borderBottom: '1px solid #555', cursor: 'pointer',
                  userSelect: 'none', width: '40%',
                }}>
                  {t('addressBook.label')} {sortColumn === 'label' && (sortDirection === 'asc' ? '▲' : '▼')}
                </th>
                <th onClick={() => handleSort('address')} style={{
                  padding: '6px 12px', textAlign: 'left', fontSize: '11px', color: '#aaa',
                  backgroundColor: '#3a3a3a', borderBottom: '1px solid #555', cursor: 'pointer',
                  userSelect: 'none', width: '60%',
                }}>
                  {t('addressBook.address')} {sortColumn === 'address' && (sortDirection === 'asc' ? '▲' : '▼')}
                </th>
              </tr>
            </thead>
            <tbody>
              {isLoading && contacts.length === 0 ? (
                <tr><td colSpan={2} style={{ padding: '20px', textAlign: 'center', color: '#888', fontSize: '12px' }}>
                  {t('addressBook.loading')}
                </td></tr>
              ) : filteredContacts.length === 0 ? (
                <tr><td colSpan={2} style={{ padding: '20px', textAlign: 'center', color: '#888', fontSize: '12px' }}>
                  {searchFilter ? t('addressBook.noMatchingContacts') : t('addressBook.noAddresses')}
                </td></tr>
              ) : (
                filteredContacts.map((contact) => (
                  <tr
                    key={contact.address}
                    onClick={() => setSelectedAddress(contact.address)}
                    onDoubleClick={() => handleRowDoubleClick(contact)}
                    onContextMenu={(e) => handleContextMenu(e, contact)}
                    style={{
                      backgroundColor: selectedAddress === contact.address ? '#404060' : 'transparent',
                      cursor: 'pointer',
                    }}
                    onMouseEnter={(e) => {
                      if (selectedAddress !== contact.address) e.currentTarget.style.backgroundColor = '#353535';
                    }}
                    onMouseLeave={(e) => {
                      if (selectedAddress !== contact.address) e.currentTarget.style.backgroundColor = 'transparent';
                    }}
                  >
                    <td style={{ padding: '4px 12px', fontSize: '12px', color: '#ddd', borderBottom: '1px solid #3a3a3a' }}>
                      {contact.label}
                    </td>
                    <td style={{ padding: '4px 12px', fontSize: '12px', color: '#ddd', fontFamily: 'monospace', borderBottom: '1px solid #3a3a3a' }}>
                      {contact.address}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Footer */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '8px 16px', borderTop: '1px solid #444',
        }}>
          <span style={{ color: '#888', fontSize: '11px' }}>
            {t('addressBook.contactCount', { count: filteredContacts.length })}
          </span>
          <div style={{ display: 'flex', gap: '8px' }}>
            {mode === 'select' && (
              <button className="qt-button" onClick={handleChoose}
                disabled={!selectedAddress}
                style={{
                  padding: '4px 16px', fontSize: '12px',
                  backgroundColor: selectedAddress ? '#404040' : '#333',
                  border: '1px solid #555', borderRadius: '2px',
                  color: selectedAddress ? '#ddd' : '#888',
                  cursor: selectedAddress ? 'pointer' : 'default',
                }}>
                {t('addressBook.choose')}
              </button>
            )}
            <button className="qt-button" onClick={handleClose}
              style={{ padding: '4px 16px', fontSize: '12px', backgroundColor: '#404040', border: '1px solid #555', borderRadius: '2px', color: '#ddd', cursor: 'pointer' }}>
              {t('addressBook.close')}
            </button>
          </div>
        </div>
      </div>

      {/* Context Menu */}
      {contextMenu && (
        <div ref={contextMenuRef}>
          <ContextMenu
            x={contextMenu.x}
            y={contextMenu.y}
            contact={contextMenu.contact}
            onCopyAddress={handleCopyAddress}
            onCopyLabel={handleCopyLabel}
            onEdit={handleEditFromContext}
            onDelete={handleDeleteFromContext}
          />
        </div>
      )}

      {/* Add/Edit Dialog */}
      <AddEditDialog
        isOpen={showAddEdit}
        editContact={editingContact}
        onSave={handleSaveContact}
        onCancel={() => { setShowAddEdit(false); setAddEditError(''); }}
        error={addEditError}
      />

      {/* Delete Confirmation */}
      {showDeleteConfirm && deletingContact && (
        <SimpleConfirmDialog
          isOpen={true}
          title={t('addressBook.delete')}
          message={t('addressBook.deleteConfirm', { label: deletingContact.label, address: deletingContact.address })}
          confirmText={t('addressBook.delete')}
          cancelText={t('addressBook.cancel')}
          isDestructive={true}
          zIndex={1004}
          onConfirm={handleConfirmDelete}
          onCancel={() => { setShowDeleteConfirm(false); setDeletingContact(null); }}
        />
      )}
    </>
  );
};

// Hook for external components to open address book in select mode.
// Sets the module-level callback that AddressBookDialog reads on Choose/double-click.
export function useAddressBookPicker() {
  const { openAddressBookDialog } = useAddressBook();

  const openPicker = useCallback((onSelect: (address: string, label: string) => void) => {
    _pendingSelectCallback = onSelect;
    openAddressBookDialog('select');
  }, [openAddressBookDialog]);

  return { openPicker };
}
