import React, { useEffect, useState, useCallback, useRef } from 'react';
import { useTranslation, Trans } from 'react-i18next';
import { useReceive } from '@/store/useStore';
import { Copy, Clipboard, ChevronDown } from 'lucide-react';
import { sanitizeText } from '@/shared/utils/sanitize';
import { ReceivingAddressesDialog, RequestPaymentDialog } from '@/components/dialogs';

// CSS for dark theme scrollbar and table styling
const scrollbarStyles = `
  .history-scroll-container::-webkit-scrollbar {
    width: 8px;
  }
  .history-scroll-container::-webkit-scrollbar-track {
    background: #2b2b2b;
    border-radius: 4px;
  }
  .history-scroll-container::-webkit-scrollbar-thumb {
    background: #555;
    border-radius: 4px;
  }
  .history-scroll-container::-webkit-scrollbar-thumb:hover {
    background: #666;
  }

  .history-table {
    width: 100%;
    border-collapse: collapse;
  }

  .history-table thead {
    position: sticky;
    top: 0;
    z-index: 1;
  }

  .history-table th {
    background: #3a3a3a;
    padding: 6px 8px;
    text-align: left;
    font-weight: normal;
    font-size: 12px;
    color: #ccc;
    border-bottom: 1px solid #555;
    cursor: pointer;
    user-select: none;
  }

  .history-table th:hover {
    background: #444;
  }

  .history-table td {
    padding: 6px 8px;
    font-size: 11px;
    color: #ddd;
    border-bottom: 1px solid #3a3a3a;
    max-width: 150px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    background: #2b2b2b;
  }

  .history-table tbody tr:hover td {
    background: #3a3a3a;
  }

  .history-table tbody tr.selected td {
    background: #4a5568;
  }

  .history-table tbody tr.selected:hover td {
    background: #5a6578;
  }

  .context-menu {
    position: fixed;
    background: #3a3a3a;
    border: 1px solid #555;
    border-radius: 4px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    z-index: 1000;
    min-width: 160px;
    padding: 4px 0;
  }

  .context-menu-item {
    padding: 6px 12px;
    font-size: 12px;
    color: #ddd;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .context-menu-item:hover {
    background: #4a5568;
  }
`;

// Amount unit options
const UNIT_OPTIONS = ['TWINS', 'mTWINS', 'uTWINS'] as const;
type AmountUnit = typeof UNIT_OPTIONS[number];

interface ContextMenuState {
  visible: boolean;
  x: number;
  y: number;
  requestKey: string | null;
}

// Helper to generate unique key for payment request (ID is per-address, not global)
const getRequestKey = (request: { address: string; id: number }): string =>
  `${request.address}_${request.id}`;

export const Receive: React.FC = () => {
  const { t } = useTranslation('wallet');
  const {
    currentAddress,
    paymentRequests,
    reuseAddress,
    formState,
    isLoading,
    isCreatingRequest,
    error,
    setReuseAddress,
    updateFormField,
    clearForm,
    fetchCurrentAddress,
    fetchPaymentRequests,
    createPaymentRequest,
    deletePaymentRequest,
    isAddressesDialogOpen,
    openAddressesDialog,
    closeAddressesDialog,
    openRequestDialog,
    clearError,
  } = useReceive();

  // Local state
  const [selectedUnit, setSelectedUnit] = useState<AmountUnit>('TWINS');
  const [selectedRowKey, setSelectedRowKey] = useState<string | null>(null);
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({
    visible: false,
    x: 0,
    y: 0,
    requestKey: null,
  });
  const [sortColumn, setSortColumn] = useState<'date' | 'label' | 'address' | 'message' | 'amount'>('date');
  const [sortAscending, setSortAscending] = useState(false);

  const contextMenuRef = useRef<HTMLDivElement>(null);

  // Fetch data on mount only
  useEffect(() => {
    fetchCurrentAddress();
    fetchPaymentRequests();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Close context menu on click outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (contextMenuRef.current && !contextMenuRef.current.contains(e.target as Node)) {
        setContextMenu(prev => ({ ...prev, visible: false }));
      }
    };

    if (contextMenu.visible) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [contextMenu.visible]);

  // Handle form submission
  const handleRequestPayment = useCallback(async () => {
    await createPaymentRequest(selectedUnit);
  }, [createPaymentRequest, selectedUnit]);

  // Handle clear button
  const handleClear = useCallback(() => {
    clearForm();
    clearError();
    setSelectedUnit('TWINS');
  }, [clearForm, clearError]);

  // Handle row selection
  const handleRowClick = useCallback((key: string) => {
    setSelectedRowKey(key === selectedRowKey ? null : key);
  }, [selectedRowKey]);

  // Handle row double-click (show request)
  const handleRowDoubleClick = useCallback((key: string) => {
    const request = paymentRequests.find(r => getRequestKey(r) === key);
    if (request) {
      openRequestDialog(request);
    }
  }, [paymentRequests, openRequestDialog]);

  // Handle context menu
  const handleContextMenu = useCallback((e: React.MouseEvent, key: string) => {
    e.preventDefault();
    setContextMenu({
      visible: true,
      x: e.clientX,
      y: e.clientY,
      requestKey: key,
    });
    setSelectedRowKey(key);
  }, []);

  // Copy to clipboard helper
  const copyToClipboard = useCallback(async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // Clipboard API may fail in some contexts - silently fail
      // User feedback would require notification system integration
    }
    setContextMenu(prev => ({ ...prev, visible: false }));
  }, []);

  // Context menu actions
  const handleCopyLabel = useCallback(() => {
    const request = paymentRequests.find(r => getRequestKey(r) === contextMenu.requestKey);
    if (request) copyToClipboard(request.label || '');
  }, [paymentRequests, contextMenu.requestKey, copyToClipboard]);

  const handleCopyAddress = useCallback(() => {
    const request = paymentRequests.find(r => getRequestKey(r) === contextMenu.requestKey);
    if (request) copyToClipboard(request.address);
  }, [paymentRequests, contextMenu.requestKey, copyToClipboard]);

  const handleCopyMessage = useCallback(() => {
    const request = paymentRequests.find(r => getRequestKey(r) === contextMenu.requestKey);
    if (request) copyToClipboard(request.message || '');
  }, [paymentRequests, contextMenu.requestKey, copyToClipboard]);

  const handleCopyAmount = useCallback(() => {
    const request = paymentRequests.find(r => getRequestKey(r) === contextMenu.requestKey);
    if (request) copyToClipboard(request.amount?.toString() || '0');
  }, [paymentRequests, contextMenu.requestKey, copyToClipboard]);

  // Handle Show button
  const handleShow = useCallback(() => {
    if (selectedRowKey !== null) {
      const request = paymentRequests.find(r => getRequestKey(r) === selectedRowKey);
      if (request) {
        openRequestDialog(request);
      }
    }
  }, [selectedRowKey, paymentRequests, openRequestDialog]);

  // Handle Remove button
  const handleRemove = useCallback(async () => {
    if (selectedRowKey !== null) {
      const request = paymentRequests.find(r => getRequestKey(r) === selectedRowKey);
      if (request) {
        await deletePaymentRequest(request.address, request.id);
        setSelectedRowKey(null);
      }
    }
  }, [selectedRowKey, paymentRequests, deletePaymentRequest]);

  // Handle column header click for sorting
  const handleSort = useCallback((column: typeof sortColumn) => {
    if (sortColumn === column) {
      setSortAscending(!sortAscending);
    } else {
      setSortColumn(column);
      setSortAscending(column === 'date' ? false : true); // Date defaults to descending
    }
  }, [sortColumn, sortAscending]);

  // Sort payment requests
  const sortedRequests = [...paymentRequests].sort((a, b) => {
    let comparison = 0;
    switch (sortColumn) {
      case 'date':
        comparison = new Date(a.date).getTime() - new Date(b.date).getTime();
        break;
      case 'label':
        comparison = (a.label || '').localeCompare(b.label || '');
        break;
      case 'address':
        comparison = a.address.localeCompare(b.address);
        break;
      case 'message':
        comparison = (a.message || '').localeCompare(b.message || '');
        break;
      case 'amount':
        comparison = (a.amount || 0) - (b.amount || 0);
        break;
    }
    return sortAscending ? comparison : -comparison;
  });

  // Format date for display
  const formatDate = (dateStr: string): string => {
    const date = new Date(dateStr);
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  // Render sort indicator
  const renderSortIndicator = (column: typeof sortColumn) => {
    if (sortColumn !== column) return null;
    return (
      <ChevronDown
        size={12}
        style={{
          display: 'inline-block',
          marginLeft: '4px',
          transform: sortAscending ? 'rotate(180deg)' : 'rotate(0deg)',
          transition: 'transform 0.2s',
        }}
      />
    );
  };

  return (
    <div className="qt-frame" style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <style>{scrollbarStyles}</style>
      <div className="qt-vbox" style={{ padding: '8px', display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
        {/* Page Header */}
        <div className="qt-header-label" style={{ marginBottom: '8px', fontSize: '18px', flexShrink: 0 }}>
          {t('receive.title').toUpperCase()}
        </div>

        {/* Request Payment Form Section */}
        <div className="qt-frame-secondary" style={{
          marginBottom: '8px',
          padding: '10px',
          border: '1px solid #4a4a4a',
          borderRadius: '2px',
          backgroundColor: '#3a3a3a',
          flexShrink: 0,
        }}>
          <div className="qt-vbox" style={{ gap: '10px' }}>
            {/* Info text */}
            <div className="qt-label" style={{ fontSize: '12px', color: '#aaa' }}>
              <Trans i18nKey="receive.formInfo" ns="wallet">
                Use this form to request payments. All fields are <strong>optional</strong>.
              </Trans>
            </div>

            {/* Label field */}
            <div className="qt-hbox" style={{ alignItems: 'center', gap: '10px' }}>
              <label htmlFor="receive-label" className="qt-label" style={{ width: '70px', fontSize: '12px', textAlign: 'right' }}>
                {t('receive.label')}:
              </label>
              <input
                id="receive-label"
                type="text"
                value={formState.label}
                onChange={(e) => updateFormField('label', e.target.value)}
                className="qt-input"
                aria-label="Payment request label"
                style={{
                  flex: 1,
                  padding: '4px 8px',
                  fontSize: '12px',
                  backgroundColor: '#2b2b2b',
                  border: '1px solid #1a1a1a',
                  color: '#ddd',
                }}
                placeholder=""
              />
            </div>

            {/* Address field (read-only) */}
            <div className="qt-hbox" style={{ alignItems: 'center', gap: '10px' }}>
              <label htmlFor="receive-address" className="qt-label" style={{ width: '70px', fontSize: '12px', textAlign: 'right' }}>
                {t('receive.address')}:
              </label>
              <input
                id="receive-address"
                type="text"
                value={currentAddress}
                readOnly
                className="qt-input"
                aria-label="Current receiving address"
                aria-readonly="true"
                style={{
                  flex: 1,
                  padding: '4px 8px',
                  fontSize: '12px',
                  backgroundColor: '#2b2b2b',
                  border: '1px solid #1a1a1a',
                  color: '#ddd',
                  cursor: 'default',
                }}
              />
            </div>

            {/* Amount field with unit selector */}
            <div className="qt-hbox" style={{ alignItems: 'center', gap: '10px' }}>
              <label htmlFor="receive-amount" className="qt-label" style={{ width: '70px', fontSize: '12px', textAlign: 'right' }}>
                {t('receive.amount')}:
              </label>
              <input
                id="receive-amount"
                type="text"
                value={formState.amount}
                onChange={(e) => {
                  // Only allow numbers and single decimal point
                  const value = e.target.value;
                  if (value === '' || /^\d*$/.test(value) || /^\d*\.\d*$/.test(value)) {
                    updateFormField('amount', value);
                  }
                }}
                className="qt-input"
                aria-label="Payment request amount"
                style={{
                  width: '150px',
                  padding: '4px 8px',
                  fontSize: '12px',
                  backgroundColor: '#2b2b2b',
                  border: '1px solid #1a1a1a',
                  color: '#ddd',
                }}
                placeholder=""
              />
              <select
                id="receive-unit"
                value={selectedUnit}
                onChange={(e) => setSelectedUnit(e.target.value as AmountUnit)}
                className="qt-select"
                aria-label="Amount unit"
                style={{
                  minWidth: '90px',
                  padding: '4px 20px 4px 8px',
                  fontSize: '12px',
                  backgroundColor: '#2b2b2b',
                  border: '1px solid #1a1a1a',
                  color: '#ddd',
                  cursor: 'pointer',
                }}
              >
                {UNIT_OPTIONS.map(unit => (
                  <option key={unit} value={unit}>{unit}</option>
                ))}
              </select>
            </div>

            {/* Message field */}
            <div className="qt-hbox" style={{ alignItems: 'center', gap: '10px' }}>
              <label htmlFor="receive-message" className="qt-label" style={{ width: '70px', fontSize: '12px', textAlign: 'right' }}>
                {t('receive.message')}:
              </label>
              <input
                id="receive-message"
                type="text"
                value={formState.message}
                onChange={(e) => updateFormField('message', e.target.value)}
                className="qt-input"
                aria-label="Payment request message"
                style={{
                  flex: 1,
                  padding: '4px 8px',
                  fontSize: '12px',
                  backgroundColor: '#2b2b2b',
                  border: '1px solid #1a1a1a',
                  color: '#ddd',
                }}
                placeholder=""
              />
            </div>

            {/* Reuse address checkbox */}
            <div className="qt-hbox" style={{ alignItems: 'center', gap: '10px', marginLeft: '80px' }}>
              <label className="qt-hbox" style={{ alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={reuseAddress}
                  onChange={(e) => setReuseAddress(e.target.checked)}
                  className="qt-checkbox"
                  style={{ width: '14px', height: '14px' }}
                />
                <span className="qt-label" style={{ fontSize: '12px' }}>
                  {t('receive.reusePrevious')}
                </span>
              </label>
            </div>

            {/* Error display */}
            {error && (
              <div style={{
                marginLeft: '80px',
                padding: '6px 10px',
                backgroundColor: '#4a2a2a',
                border: '1px solid #ff6666',
                borderRadius: '2px',
                color: '#ff6666',
                fontSize: '11px',
              }}>
                {sanitizeText(error)}
              </div>
            )}

            {/* Action buttons */}
            <div className="qt-hbox" style={{ justifyContent: 'space-between', alignItems: 'center', marginTop: '4px' }}>
              <div className="qt-hbox" style={{ gap: '8px', marginLeft: '80px' }}>
                <button
                  type="button"
                  onClick={handleRequestPayment}
                  disabled={isCreatingRequest || isLoading}
                  className="qt-button-primary"
                  style={{
                    padding: '6px 16px',
                    fontSize: '12px',
                    backgroundColor: '#4a7c59',
                    border: '1px solid #5a8c69',
                    borderRadius: '3px',
                    color: '#fff',
                    cursor: isCreatingRequest ? 'wait' : 'pointer',
                    opacity: isCreatingRequest ? 0.7 : 1,
                  }}
                >
                  {isCreatingRequest ? t('receive.requestingPayment') : t('receive.requestPayment')}
                </button>
                <button
                  type="button"
                  onClick={handleClear}
                  className="qt-button"
                  style={{
                    padding: '6px 16px',
                    fontSize: '12px',
                    backgroundColor: '#404040',
                    border: '1px solid #555',
                    borderRadius: '3px',
                    color: '#ddd',
                    cursor: 'pointer',
                  }}
                >
                  {t('receive.clear')}
                </button>
              </div>

              <button
                type="button"
                onClick={openAddressesDialog}
                className="qt-button"
                style={{
                  padding: '6px 16px',
                  fontSize: '12px',
                  backgroundColor: '#4a7c59',
                  border: '1px solid #5a8c69',
                  borderRadius: '3px',
                  color: '#fff',
                  cursor: 'pointer',
                }}
              >
                {t('receive.receivingAddresses')}
              </button>
            </div>
          </div>
        </div>

        {/* Requested Payments History Section */}
        <div className="qt-frame-secondary" style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          padding: '10px',
          border: '1px solid #4a4a4a',
          borderRadius: '2px',
          backgroundColor: '#3a3a3a',
          minHeight: 0,
          overflow: 'hidden',
        }}>
          <div className="qt-label" style={{ fontWeight: 'bold', marginBottom: '8px', fontSize: '12px' }}>
            {t('receive.requestedHistory')}
          </div>

          {/* Table container */}
          <div
            className="history-scroll-container"
            style={{
              flex: 1,
              minHeight: 0,
              overflow: 'auto',
              border: '1px solid #2b2b2b',
              borderRadius: '2px',
              backgroundColor: '#2b2b2b',
            }}
          >
            <table className="history-table">
              <thead>
                <tr>
                  <th onClick={() => handleSort('date')} style={{ width: '140px' }}>
                    {t('receive.table.date')} {renderSortIndicator('date')}
                  </th>
                  <th onClick={() => handleSort('label')} style={{ width: '120px' }}>
                    {t('receive.table.label')} {renderSortIndicator('label')}
                  </th>
                  <th onClick={() => handleSort('address')} style={{ width: '150px' }}>
                    {t('receive.table.address')} {renderSortIndicator('address')}
                  </th>
                  <th onClick={() => handleSort('message')}>
                    {t('receive.table.message')} {renderSortIndicator('message')}
                  </th>
                  <th onClick={() => handleSort('amount')} style={{ width: '120px', textAlign: 'right' }}>
                    {t('receive.table.amountTwins')} {renderSortIndicator('amount')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {sortedRequests.length === 0 ? (
                  <tr>
                    <td colSpan={5} style={{ textAlign: 'center', color: '#888', padding: '20px' }}>
                      {isLoading ? t('common:status.loading') : t('receive.noRequests')}
                    </td>
                  </tr>
                ) : (
                  sortedRequests.map((request, index) => {
                    const rowKey = getRequestKey(request);
                    return (
                    <tr
                      key={rowKey}
                      className={selectedRowKey === rowKey ? 'selected' : ''}
                      tabIndex={0}
                      role="row"
                      aria-selected={selectedRowKey === rowKey}
                      onClick={() => handleRowClick(rowKey)}
                      onDoubleClick={() => handleRowDoubleClick(rowKey)}
                      onContextMenu={(e) => handleContextMenu(e, rowKey)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          if (e.shiftKey) {
                            handleRowDoubleClick(rowKey);
                          } else {
                            handleRowClick(rowKey);
                          }
                        } else if (e.key === ' ') {
                          e.preventDefault();
                          handleRowClick(rowKey);
                        } else if (e.key === 'ArrowDown' && index < sortedRequests.length - 1) {
                          e.preventDefault();
                          const nextRow = e.currentTarget.nextElementSibling as HTMLElement;
                          nextRow?.focus();
                        } else if (e.key === 'ArrowUp' && index > 0) {
                          e.preventDefault();
                          const prevRow = e.currentTarget.previousElementSibling as HTMLElement;
                          prevRow?.focus();
                        }
                      }}
                      style={{ cursor: 'pointer' }}
                    >
                      <td>{formatDate(request.date)}</td>
                      <td title={sanitizeText(request.label || '')}>{sanitizeText(request.label || '')}</td>
                      <td title={sanitizeText(request.address)}>{sanitizeText(request.address)}</td>
                      <td title={sanitizeText(request.message || '')}>{sanitizeText(request.message || '')}</td>
                      <td style={{ textAlign: 'right' }}>
                        {request.amount ? request.amount.toFixed(8) : '-'}
                      </td>
                    </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>

          {/* Table action buttons */}
          <div className="qt-hbox" style={{ gap: '8px', marginTop: '8px' }}>
            <button
              type="button"
              onClick={handleShow}
              disabled={selectedRowKey === null}
              className="qt-button"
              style={{
                padding: '4px 12px',
                fontSize: '11px',
                backgroundColor: '#404040',
                border: '1px solid #555',
                borderRadius: '3px',
                color: '#ddd',
                cursor: selectedRowKey === null ? 'not-allowed' : 'pointer',
                opacity: selectedRowKey === null ? 0.5 : 1,
              }}
            >
              {t('receive.show')}
            </button>
            <button
              type="button"
              onClick={handleRemove}
              disabled={selectedRowKey === null}
              className="qt-button"
              style={{
                padding: '4px 12px',
                fontSize: '11px',
                backgroundColor: '#404040',
                border: '1px solid #555',
                borderRadius: '3px',
                color: '#ddd',
                cursor: selectedRowKey === null ? 'not-allowed' : 'pointer',
                opacity: selectedRowKey === null ? 0.5 : 1,
              }}
            >
              {t('receive.remove')}
            </button>
          </div>
        </div>
      </div>

      {/* Receiving Addresses Dialog */}
      <ReceivingAddressesDialog
        isOpen={isAddressesDialogOpen}
        onClose={closeAddressesDialog}
      />

      {/* Request Payment Dialog */}
      <RequestPaymentDialog />

      {/* Context Menu */}
      {contextMenu.visible && (
        <div
          ref={contextMenuRef}
          className="context-menu"
          role="menu"
          aria-label="Payment request actions"
          style={{
            left: contextMenu.x,
            top: contextMenu.y,
          }}
        >
          <div
            className="context-menu-item"
            role="menuitem"
            tabIndex={0}
            onClick={handleCopyLabel}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                handleCopyLabel();
              }
            }}
          >
            <Copy size={14} />
            {t('receive.contextMenu.copyLabel')}
          </div>
          <div
            className="context-menu-item"
            role="menuitem"
            tabIndex={0}
            onClick={handleCopyAddress}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                handleCopyAddress();
              }
            }}
          >
            <Clipboard size={14} />
            {t('receive.contextMenu.copyAddress')}
          </div>
          <div
            className="context-menu-item"
            role="menuitem"
            tabIndex={0}
            onClick={handleCopyMessage}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                handleCopyMessage();
              }
            }}
          >
            <Copy size={14} />
            {t('receive.contextMenu.copyMessage')}
          </div>
          <div
            className="context-menu-item"
            role="menuitem"
            tabIndex={0}
            onClick={handleCopyAmount}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                handleCopyAmount();
              }
            }}
          >
            <Copy size={14} />
            {t('receive.contextMenu.copyAmount')}
          </div>
        </div>
      )}
    </div>
  );
};
