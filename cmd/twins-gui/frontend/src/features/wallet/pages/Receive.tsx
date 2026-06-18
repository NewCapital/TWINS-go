import React, { useEffect, useState, useCallback, useRef, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { QRCodeCanvas } from 'qrcode.react';
import { useReceive, usePaymentRequests } from '@/store/useStore';
import { Copy, RefreshCw, ChevronDown, ChevronUp, Eye, Trash2, List, Download } from 'lucide-react';
import { PaginationFooter } from '@/shared/components/PaginationFooter';
import {
  PAYMENT_REQUESTS_PAGE_SIZES,
  type PaymentRequestsSortColumn,
} from '@/store/slices/paymentRequestsSlice';
import { sanitizeText } from '@/shared/utils/sanitize';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { buildTwinsURI, MAX_QR_DATA_LENGTH } from '@/shared/utils/twinsUri';
import { writeToClipboard } from '@/shared/utils/clipboard';
import { truncateAddress } from '@/shared/utils/format';
import { SaveQRImage } from '@wailsjs/go/main/App';
import { createCircularLogoDataURL } from '@/shared/utils/qrLogo';
import { buildQRFilename } from '@/shared/utils/qrFilename';
import { SimpleConfirmDialog } from '@/shared/components/SimpleConfirmDialog';
import { IconButton } from '@/shared/components/IconButton';
import { RowsPerPageSelect } from '@/shared/components/RowsPerPageSelect';
import { Banner } from '@/shared/components/Banner';
import { ReceivingAddressesDialog, RequestPaymentDialog } from '@/components/dialogs';

// Card padding tokens — see frontend/CLAUDE.md "Design Tokens".
const CARD_PADDING_DENSE = '12px 16px';
const CARD_PADDING_STANDARD = '20px';

// Amount unit options
const UNIT_OPTIONS = ['TWINS', 'mTWINS', 'uTWINS'] as const;
type AmountUnit = typeof UNIT_OPTIONS[number];

// Helper to generate unique key for payment request (ID is per-address, not global)
const getRequestKey = (request: { address: string; id: number }): string =>
  `${request.address}_${request.id}`;

// Sortable column header for the Recent Requests table. Renders the label +
// (when active) a TWINS-green chevron pointing up/down depending on direction.
// Inactive columns render in muted #888; active column flips to #27ae60.
// Clicking the header dispatches `onSort(column)` which the slice
// translates into either "same column → toggle direction" or "new column →
// pick a sensible default direction" (date/amount desc, label asc).
interface SortableHeaderCellProps {
  label: string;
  column: PaymentRequestsSortColumn;
  width?: string;
  flex?: number;
  align?: 'left' | 'right';
  sortColumn: PaymentRequestsSortColumn;
  sortDirection: 'asc' | 'desc';
  onSort: (column: PaymentRequestsSortColumn) => void;
}
const SortableHeaderCell: React.FC<SortableHeaderCellProps> = ({
  label,
  column,
  width,
  flex,
  align = 'left',
  sortColumn,
  sortDirection,
  onSort,
}) => {
  const isActive = sortColumn === column;
  const color = isActive ? '#27ae60' : '#888';
  return (
    <button
      type="button"
      onClick={() => onSort(column)}
      title={`Sort by ${label}`}
      style={{
        width,
        flex,
        display: 'flex',
        alignItems: 'center',
        justifyContent: align === 'right' ? 'flex-end' : 'flex-start',
        gap: '4px',
        cursor: 'pointer',
        background: 'none',
        border: 'none',
        fontSize: '11px',
        fontWeight: 500,
        color,
        padding: 0,
      }}
    >
      <span>{label}</span>
      {isActive && (sortDirection === 'asc' ? <ChevronUp size={12} /> : <ChevronDown size={12} />)}
    </button>
  );
};

export const Receive: React.FC = () => {
  const { t } = useTranslation('wallet');
  const { formatAmount, unitLabel } = useDisplayUnits();
  const { formatDateValue, formatTooltip, formatDateHeader } = useDisplayDateTime();
  const {
    currentAddress,
    reuseAddress,
    formState,
    isLoading,
    isCreatingRequest,
    isGeneratingAddress,
    error,
    setReuseAddress,
    updateFormField,
    clearForm,
    fetchCurrentAddress,
    createPaymentRequest,
    deletePaymentRequest,
    generateNewAddress,
    isAddressesDialogOpen,
    openAddressesDialog,
    closeAddressesDialog,
    openRequestDialog,
    clearError,
    addressJustSelected,
    clearAddressJustSelected,
  } = useReceive();

  // Recent Requests table state lives in the paymentRequestsSlice (server-side
  // pagination + sort, mirrors receivingAddressesSlice). The mount effect
  // below calls `fetchRequestsPage(1)` instead of the legacy
  // `fetchPaymentRequests`; create/delete flows also call it to refresh.
  const {
    requests,
    total: requestsTotal,
    totalPages: requestsTotalPages,
    currentPage: requestsCurrentPage,
    pageSize: requestsPageSize,
    sortColumn,
    sortDirection,
    isLoading: isLoadingRequests,
    fetchPage: fetchRequestsPage,
    setPage: setRequestsPage,
    setPageSize: setRequestsPageSize,
    setSortColumn: setRequestsSortColumn,
  } = usePaymentRequests();

  // Local state
  const [selectedUnit, setSelectedUnit] = useState<AmountUnit>('TWINS');
  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);
  const [confirmRemoveKey, setConfirmRemoveKey] = useState<string | null>(null);

  // Brief highlight on the address row after picker selection
  const [addressHighlight, setAddressHighlight] = useState(false);

  const qrRef = useRef<HTMLDivElement>(null);
  const [qrLogoSrc, setQrLogoSrc] = useState<string | undefined>();

  // Three-layer Enter-spam guard for the request-payment form:
  //  1. `submittingRef` blocks PARALLEL submissions inside one in-flight
  //     cycle (synchronous, set before async, cleared in `.finally`).
  //  2. `e.repeat` block in `onKeyDown` suppresses OS auto-repeat from a
  //     held key (held Enter no longer fans out across cycle boundaries).
  //  3. `lastSubmitAtRef` + `SUBMIT_COOLDOWN_MS` debounces rapid taps —
  //     each cycle can finish in ~100ms with a fast backend, so without
  //     the cooldown five rapid Enter presses produce five requests with
  //     five rotated addresses. The cooldown collapses tap-spam into a
  //     single submission while leaving deliberately-spaced submissions
  //     (>800ms apart) unaffected. To avoid blocking deliberate
  //     fix-and-retry after a validation error, the cooldown is reset
  //     whenever the user types non-empty content into any field — see
  //     the dedicated useEffect below.
  const submittingRef = useRef(false);
  const lastSubmitAtRef = useRef(0);
  const SUBMIT_COOLDOWN_MS = 800;

  // Show toast + highlight when an address is picked from the dialog.
  // Split into two effects: one to consume the flag, one to manage the
  // highlight timer. Combining them caused clearAddressJustSelected() to
  // change the dependency, triggering cleanup which cancelled the timer.
  useEffect(() => {
    if (!addressJustSelected) return;
    setCopyFeedback(t('receive.addressSelectedFeedback'));
    setAddressHighlight(true);
    clearAddressJustSelected();
  }, [addressJustSelected, clearAddressJustSelected, t]);

  // Auto-clear address highlight after 1.5s
  useEffect(() => {
    if (!addressHighlight) return;
    const timer = setTimeout(() => setAddressHighlight(false), 1500);
    return () => clearTimeout(timer);
  }, [addressHighlight]);

  // Generate circular-bordered logo for QR code
  useEffect(() => {
    createCircularLogoDataURL('/icons/twins-logo.png', 64, 4, '#27ae60')
      .then(setQrLogoSrc)
      .catch(() => {}); // Falls back to no logo if image fails to load
  }, []);

  // Fetch data on mount only
  useEffect(() => {
    fetchCurrentAddress();
    fetchRequestsPage(1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Auto-clear copy feedback
  useEffect(() => {
    if (!copyFeedback) return;
    const timeoutId = setTimeout(() => setCopyFeedback(null), 2000);
    return () => clearTimeout(timeoutId);
  }, [copyFeedback]);

  // Reset the submit-cooldown timestamp when the user types non-empty
  // content into any form field. This unblocks deliberate fix-and-retry
  // after a validation error: the user edits the bad field, and the next
  // Enter is allowed through immediately. The auto-clear that runs on
  // successful submit empties all three fields, which fails the
  // `hasContent` check so the cooldown is preserved against rapid taps.
  useEffect(() => {
    const hasContent = !!(formState.label || formState.amount || formState.message);
    if (hasContent) lastSubmitAtRef.current = 0;
  }, [formState.label, formState.amount, formState.message]);

  // Converted amount in TWINS — shared by liveURI and handleSaveQR
  const convertedAmount = useMemo((): number | undefined => {
    if (!formState.amount) return undefined;
    const parsed = parseFloat(formState.amount);
    if (isNaN(parsed) || parsed <= 0) return undefined;
    switch (selectedUnit) {
      case 'mTWINS': return parsed / 1000;
      case 'uTWINS': return parsed / 1000000;
      default: return parsed;
    }
  }, [formState.amount, selectedUnit]);

  // Live QR code URI — updates as form fields change
  const liveURI = useMemo(() => {
    if (!currentAddress) return '';
    return buildTwinsURI(
      currentAddress,
      convertedAmount,
      formState.label || undefined,
      formState.message || undefined,
    );
  }, [currentAddress, convertedAmount, formState.label, formState.message]);

  const isURITooLong = liveURI.length > MAX_QR_DATA_LENGTH;

  // Copy address to clipboard
  const handleCopyAddress = useCallback(async () => {
    if (!currentAddress) return;
    const ok = await writeToClipboard(currentAddress);
    setCopyFeedback(ok ? t('receive.copied') : t('receive.copyFailed'));
  }, [currentAddress, t]);

  // Copy URI to clipboard. Falls back to a bare `twins:<address>` URI when
  // `liveURI` is not yet built so the handler always copies whatever the UI
  // is currently displaying in the URI row.
  const handleCopyURI = useCallback(async () => {
    const uri = liveURI || (currentAddress ? `twins:${currentAddress}` : '');
    if (!uri) return;
    const ok = await writeToClipboard(uri);
    setCopyFeedback(ok ? t('receive.uriCopied') : t('receive.copyFailed'));
  }, [liveURI, currentAddress, t]);

  // Generate new address
  const handleNewAddress = useCallback(async () => {
    await generateNewAddress('');
  }, [generateNewAddress]);

  // Handle form submission. After the request is created server-side, refresh
  // the current page so the new row is visible immediately. Without the
  // refresh the slice cache would stay stale until the next page navigation.
  const handleCreateRequest = useCallback(async () => {
    await createPaymentRequest(selectedUnit);
    await fetchRequestsPage(requestsCurrentPage);
  }, [createPaymentRequest, selectedUnit, fetchRequestsPage, requestsCurrentPage]);

  // Handle clear button
  const handleClear = useCallback(() => {
    clearForm();
    clearError();
    setSelectedUnit('TWINS');
  }, [clearForm, clearError]);

  // Date rendering for Recent Requests rows is delegated to the shared
  // `useDisplayDateTime` hook (`formatDateTime` + `formatTooltip`) so the
  // global Date/Age Display Format setting (Local / UTC / Age) controls
  // every Receive page row. Per m-fix-date-display-inconsistencies
  // (2026-06-04).

  // Handle View button on history row
  const handleViewRequest = useCallback((key: string) => {
    const request = requests.find(r => getRequestKey(r) === key);
    if (request) openRequestDialog(request);
  }, [requests, openRequestDialog]);

  // Handle Remove button - show confirmation first
  const handleRemoveClick = useCallback((key: string) => {
    setConfirmRemoveKey(key);
  }, []);

  const handleConfirmRemove = useCallback(async () => {
    if (confirmRemoveKey !== null) {
      const request = requests.find(r => getRequestKey(r) === confirmRemoveKey);
      if (request) {
        await deletePaymentRequest(request.address, request.id);
        await fetchRequestsPage(requestsCurrentPage);
      }
    }
    setConfirmRemoveKey(null);
  }, [confirmRemoveKey, requests, deletePaymentRequest, fetchRequestsPage, requestsCurrentPage]);

  // Save QR code as image via native save dialog
  const handleSaveQR = useCallback(async () => {
    if (!qrRef.current) return;
    try {
      const canvas = qrRef.current.querySelector('canvas');
      if (!canvas) throw new Error('canvas not found');
      const pngBase64 = canvas.toDataURL('image/png');
      const defaultFilename = buildQRFilename(currentAddress, formState.label, convertedAmount);
      const saved = await SaveQRImage(pngBase64, defaultFilename);
      if (saved) {
        setCopyFeedback(t('receive.qrSaved'));
      }
      // If saved === false the user cancelled — show no feedback
    } catch {
      setCopyFeedback(t('receive.copyFailed'));
    }
  }, [currentAddress, formState.label, convertedAmount, t]);

  return (
    <div className="qt-frame" style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ padding: '12px 16px', display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0, gap: '12px' }}>

        {/* Two-column hero section */}
        <div style={{ display: 'flex', gap: '16px', flexShrink: 0 }}>

          {/* LEFT COLUMN — QR Code Hero */}
          {/*
            minWidth: 0 + maxWidth: 340px pin the column at exactly its
            flex-basis (340px) regardless of inner content. Without these,
            `min-width: auto` on the flex item lets the URI row's intrinsic
            content width override the basis, causing the entire column to
            grow when the URI gets long. With the column pinned, the URI
            text inside the URI row truncates correctly via its existing
            overflow/textOverflow/whiteSpace styles.
          */}
          <div style={{
            flex: '0 0 340px',
            minWidth: 0,
            maxWidth: '340px',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            padding: CARD_PADDING_STANDARD,
            backgroundColor: '#2f2f2f',
            borderRadius: '8px',
            border: '1px solid #3a3a3a',
            position: 'relative',
          }}>
            {/* Save image icon — absolute-positioned in the top-right corner
                of the QR card. The companion "New address" IconButton
                previously stacked here (task l-receive-action-buttons-icon-column,
                2026-06-02) moved into the address row alongside Copy in
                task l-receive-form-select-new-address-and-header-padding
                (2026-06-03) so the two address-related affordances (copy,
                regenerate) sit together. Container kept absolute-positioned
                for the single Save image icon. */}
            <div style={{
              position: 'absolute',
              top: '12px',
              right: '12px',
              display: 'flex',
              flexDirection: 'column',
              gap: '6px',
              zIndex: 1,
            }}>
              <IconButton
                onClick={handleSaveQR}
                disabled={!currentAddress || isGeneratingAddress}
                title={t('receive.saveImage')}
                ariaLabel={t('receive.saveImage')}
                icon={<Download size={12} />}
              />
            </div>

            {/* QR Code */}
            <div
              ref={qrRef}
              style={{
                padding: '12px',
                backgroundColor: '#ffffff',
                borderRadius: '8px',
                lineHeight: 0,
                cursor: 'pointer',
              }}
              onClick={handleSaveQR}
              title={t('receive.clickToSaveQR')}
            >
              {currentAddress ? (
                <QRCodeCanvas
                  value={liveURI || `twins:${currentAddress}`}
                  size={200}
                  level="H"
                  includeMargin={false}
                  bgColor="#ffffff"
                  fgColor="#000000"
                  imageSettings={qrLogoSrc ? {
                    src: qrLogoSrc,
                    height: 76,
                    width: 76,
                    excavate: true,
                  } : undefined}
                />
              ) : (
                <div style={{ width: 200, height: 200, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#999', fontSize: '12px' }}>
                  {t('common:status.loading')}
                </div>
              )}
            </div>

            {/* URI too long warning */}
            {isURITooLong && (
              <div style={{ marginTop: '12px', width: '100%' }}>
                <Banner variant="warning" message={t('receive.uriTooLong')} />
              </div>
            )}

            {/* Address row with inline copy icon */}
            <div style={{
              marginTop: '12px',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '8px 12px',
              backgroundColor: '#252525',
              border: addressHighlight ? '1px solid #27ae60' : '1px solid #3a3a3a',
              borderRadius: '6px',
              width: '100%',
              transition: 'border-color 0.3s ease-in-out',
            }}>
              <span
                title={currentAddress}
                style={{
                  flex: 1,
                  fontFamily: 'monospace',
                  fontSize: '13px',
                  color: '#e0e0e0',
                  letterSpacing: '0.3px',
                  textAlign: 'center',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  minWidth: 0,
                }}
              >
                {currentAddress ? truncateAddress(currentAddress, 12, 10) : '...'}
              </span>
              <IconButton
                onClick={handleNewAddress}
                disabled={isGeneratingAddress || isLoading}
                title={t('receive.newAddress')}
                ariaLabel={t('receive.newAddress')}
                icon={<RefreshCw size={12} style={isGeneratingAddress ? { animation: 'spin 1s linear infinite' } : undefined} />}
              />
              <IconButton
                onClick={handleCopyAddress}
                disabled={!currentAddress}
                title={t('receive.copyAddress')}
                ariaLabel={t('receive.copyAddress')}
                icon={<Copy size={12} />}
              />
            </div>

            {/* URI row with inline copy icon — always visible */}
            <div style={{
              marginTop: '8px',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '8px 12px',
              backgroundColor: '#252525',
              border: '1px solid #3a3a3a',
              borderRadius: '6px',
              width: '100%',
            }}>
              <span
                title={liveURI || (currentAddress ? `twins:${currentAddress}` : '')}
                style={{
                  flex: 1,
                  fontSize: '11px',
                  color: '#6699cc',
                  fontFamily: 'monospace',
                  textAlign: 'center',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  minWidth: 0,
                }}
              >
                {liveURI || (currentAddress ? `twins:${currentAddress}` : '...')}
              </span>
              <IconButton
                onClick={handleCopyURI}
                disabled={!liveURI && !currentAddress}
                title={t('receive.copyUri')}
                ariaLabel={t('receive.copyUri')}
                icon={<Copy size={12} />}
              />
            </div>
          </div>

          {/* RIGHT COLUMN — Request Payment Form */}
          <div style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            padding: CARD_PADDING_STANDARD,
            backgroundColor: '#2f2f2f',
            borderRadius: '8px',
            border: '1px solid #3a3a3a',
          }}>
            <div style={{ fontSize: '13px', fontWeight: 600, color: '#ccc', marginBottom: '12px' }}>
              {t('receive.requestPaymentTitle')}
            </div>

            <form
              onKeyDown={(e) => {
                // Suppress Enter auto-repeat from a held key. Without this,
                // a key-hold spans multiple full request cycles (each cycle
                // generates a fresh address when "new address per request"
                // is enabled), so holding Enter creates A, B, C, D...
                // `event.repeat === true` only for keystrokes synthesized
                // by the OS auto-repeat — a deliberate single keypress is
                // unaffected.
                if (e.key === 'Enter' && e.repeat) {
                  e.preventDefault();
                }
              }}
              onSubmit={(e) => {
                e.preventDefault();
                // Three-layer guard: parallel-submission block + cooldown
                // for rapid taps. See `submittingRef` declaration above
                // for the full rationale.
                if (submittingRef.current || isCreatingRequest || isLoading) return;
                if (Date.now() - lastSubmitAtRef.current < SUBMIT_COOLDOWN_MS) return;
                lastSubmitAtRef.current = Date.now();
                submittingRef.current = true;
                Promise.resolve(handleCreateRequest()).finally(() => {
                  submittingRef.current = false;
                });
              }}
              style={{ display: 'flex', flexDirection: 'column', gap: '8px', flex: 1 }}
            >
              {/* Label */}
              <div>
                <label htmlFor="receive-label" style={{ display: 'block', fontSize: '11px', color: '#888', marginBottom: '4px' }}>
                  {t('receive.label')}
                </label>
                <input
                  id="receive-label"
                  type="text"
                  value={formState.label}
                  onChange={(e) => updateFormField('label', e.target.value)}
                  maxLength={100}
                  autoCapitalize="off"
                  autoCorrect="off"
                  spellCheck={false}
                  placeholder={t('receive.labelPlaceholder')}
                  style={{
                    width: '100%',
                    padding: '7px 10px',
                    fontSize: '12px',
                    backgroundColor: '#252525',
                    border: '1px solid #3a3a3a',
                    borderRadius: '4px',
                    color: '#ddd',
                    outline: 'none',
                    boxSizing: 'border-box',
                  }}
                />
              </div>

              {/* Amount + unit */}
              <div>
                <label htmlFor="receive-amount" style={{ display: 'block', fontSize: '11px', color: '#888', marginBottom: '4px' }}>
                  {t('receive.amount')}
                </label>
                <div style={{ display: 'flex', gap: '6px' }}>
                  <input
                    id="receive-amount"
                    type="text"
                    value={formState.amount}
                    onChange={(e) => {
                      const value = e.target.value;
                      if (value === '' || /^\d*$/.test(value) || /^\d*\.\d*$/.test(value)) {
                        updateFormField('amount', value);
                      }
                    }}
                    placeholder="0.00"
                    style={{
                      flex: 1,
                      padding: '7px 10px',
                      fontSize: '12px',
                      backgroundColor: '#252525',
                      border: '1px solid #3a3a3a',
                      borderRadius: '4px',
                      color: '#ddd',
                      outline: 'none',
                    }}
                  />
                  <RowsPerPageSelect<AmountUnit>
                    value={selectedUnit}
                    options={UNIT_OPTIONS}
                    onChange={setSelectedUnit}
                    ariaLabel={`Amount unit: ${selectedUnit}`}
                    align="left"
                    triggerStyle={{
                      minWidth: '85px',
                      padding: '7px 10px',
                    }}
                  />
                </div>
              </div>

              {/* Message */}
              <div>
                <label htmlFor="receive-message" style={{ display: 'block', fontSize: '11px', color: '#888', marginBottom: '4px' }}>
                  {t('receive.message')}
                </label>
                <input
                  id="receive-message"
                  type="text"
                  value={formState.message}
                  onChange={(e) => updateFormField('message', e.target.value)}
                  maxLength={120}
                  autoCapitalize="off"
                  autoCorrect="off"
                  spellCheck={false}
                  placeholder={t('receive.messagePlaceholder')}
                  style={{
                    width: '100%',
                    padding: '7px 10px',
                    fontSize: '12px',
                    backgroundColor: '#252525',
                    border: '1px solid #3a3a3a',
                    borderRadius: '4px',
                    color: '#ddd',
                    outline: 'none',
                  }}
                />
              </div>

              {/* Checkbox + Receiving Addresses link row
                  (m-receive-form-compact-height, 2026-06-02 round 2 after
                  live testing): the prior layout had the Receiving Addresses
                  link inlined into the action row via marginLeft:auto, which
                  left a large dead band between the checkbox and the buttons
                  when the form was shorter than the QR column. New layout:
                  checkbox left + Receiving Addresses link right via
                  justify-content:space-between. Logically these two affordances
                  both manage address selection, so they pair semantically. */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '8px' }}>
                <label style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  cursor: 'pointer',
                  fontSize: '11px',
                  color: '#888',
                }}>
                  <input
                    type="checkbox"
                    checked={!reuseAddress}
                    onChange={(e) => setReuseAddress(!e.target.checked)}
                    className="qt-checkbox"
                    style={{ width: '13px', height: '13px' }}
                  />
                  {t('receive.newAddressPerRequest')}
                </label>
                <button
                  type="button"
                  onClick={openAddressesDialog}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: '#6699cc',
                    fontSize: '11px',
                    cursor: 'pointer',
                    padding: '0 4px',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                    flexShrink: 0,
                  }}
                >
                  <List size={11} />
                  {t('receive.receivingAddresses')}
                </button>
              </div>

              {/* Error display */}
              {error && (
                <div style={{
                  padding: '6px 10px',
                  backgroundColor: '#4a2a2a',
                  border: '1px solid #ff6666',
                  borderRadius: '4px',
                  color: '#ff6666',
                  fontSize: '11px',
                }}>
                  {sanitizeText(error)}
                </div>
              )}

              {/* Spacer — pins action row to bottom of form when QR
                  column is slightly taller (form has flex:1). */}
              <div style={{ flex: 1 }} />

              {/* Action buttons: Create Request takes available width,
                  Clear sits at the right edge. Receiving Addresses link
                  moved up to the checkbox row (see above). */}
              <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                <button
                  type="submit"
                  disabled={isCreatingRequest || isLoading}
                  style={{
                    flex: 1,
                    padding: '8px 16px',
                    fontSize: '12px',
                    fontWeight: 500,
                    backgroundColor: '#4a7c59',
                    border: '1px solid #5a8c69',
                    borderRadius: '6px',
                    color: '#fff',
                    cursor: isCreatingRequest ? 'wait' : 'pointer',
                    opacity: isCreatingRequest ? 0.7 : 1,
                    transition: 'background-color 0.15s',
                  }}
                >
                  {isCreatingRequest ? t('receive.requestingPayment') : t('receive.createRequest')}
                </button>
                <button
                  type="button"
                  onClick={handleClear}
                  style={{
                    padding: '8px 16px',
                    fontSize: '12px',
                    backgroundColor: '#383838',
                    border: '1px solid #4a4a4a',
                    borderRadius: '6px',
                    color: '#ccc',
                    cursor: 'pointer',
                    transition: 'background-color 0.15s',
                  }}
                >
                  {t('receive.clear')}
                </button>
              </div>
            </form>
          </div>
        </div>

        {/* BOTTOM — Recent Requests History
            Stretches to fill remaining viewport height below the hero zone
            (mirrors the 2026-06-01 Overview Recent transactions flex-stretch
            pattern). The sticky column header sits above the scrollable row
            list inside this card; the pagination footer is pinned outside
            the scroll container so it stays visible regardless of how many
            rows the user is paging through.
         */}
        <div style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          minHeight: 0,
          backgroundColor: '#2f2f2f',
          borderRadius: '8px',
          border: '1px solid #3a3a3a',
          padding: CARD_PADDING_DENSE,
        }}>
          {/* Section title row removed in m-receive-table-headers-message-
              column-pagination (2026-06-02) along with the trailing TWINS
              unit label. The unit indicator is hoisted into the Amount
              column header below (Amount (TWINS)) so it stays semantically
              attached to its values while reclaiming vertical space.
              The pagination footer's row-count line carries the total. */}

          {/* Sticky column header — Date | Label | Message | Amount (UNIT) | actions.
              Geometry mirrors the row cells below 1:1 so headers sit exactly
              above values. Active column renders in TWINS green with a
              chevron indicator. Message column is display-only (not in the
              paymentRequestsSlice SortableColumn union: 'date'|'label'|'amount').
              Padding `4px 12px 8px` (top-right-bottom-left form) bumps the bottom
              edge by 4px over the horizontal 12px so the header is visually
              separated from the first row beneath it; the prior uniform `4px 12px`
              made headers look pressed against row content (task
              l-receive-form-select-new-address-and-header-padding, 2026-06-03).
              Horizontal `12px` is preserved so column alignment with row cells
              (which use `padding: 4px 10px`) stays intact. */}
          <div
            style={{
              display: 'flex',
              gap: '12px',
              alignItems: 'center',
              position: 'sticky',
              top: 0,
              zIndex: 10,
              backgroundColor: '#2f2f2f',
              borderBottom: '1px solid #3a3a3a',
              padding: '4px 12px 8px',
              flexShrink: 0,
            }}
          >
            <SortableHeaderCell
              label={formatDateHeader()}
              column="date"
              width="220px"
              sortColumn={sortColumn}
              sortDirection={sortDirection}
              onSort={setRequestsSortColumn}
            />
            <SortableHeaderCell
              label={t('receive.table.label')}
              column="label"
              flex={1}
              sortColumn={sortColumn}
              sortDirection={sortDirection}
              onSort={setRequestsSortColumn}
            />
            {/* Message column header — display-only (no sort) because
                paymentRequestsSlice does not expose 'message' on its
                SortableColumn union. Plain span styled to match the
                muted form-label tokens used by SortableHeaderCell's
                non-active state (11px 500 #888). */}
            <span style={{ flex: 1, minWidth: 0, fontSize: '11px', fontWeight: 500, color: '#888' }}>
              {t('receive.table.message')}
            </span>
            <SortableHeaderCell
              label={`${t('receive.table.amount')} (${unitLabel})`}
              column="amount"
              width="160px"
              align="right"
              sortColumn={sortColumn}
              sortDirection={sortDirection}
              onSort={setRequestsSortColumn}
            />
            {/* Spacer aligned with the two trailing 26px action icons + their
                4px inter-button gap (= 56px) plus the 12px row gap headroom. */}
            <div style={{ width: '60px', flexShrink: 0 }} />
          </div>

          {/* Scrollable card list (stretches; pagination pinned below) */}
          <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}>
            {requests.length === 0 ? (
              <div style={{ textAlign: 'center', color: '#555', padding: '24px', fontSize: '12px' }}>
                {isLoadingRequests || isLoading ? t('common:status.loading') : t('receive.noRequests')}
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1px', padding: '2px 0' }}>
                {requests.map((request) => {
                  const rowKey = getRequestKey(request);
                  return (
                    <div
                      key={rowKey}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: '12px',
                        padding: '6px 10px',
                        backgroundColor: '#2a2a2a',
                        borderRadius: '6px',
                        border: '1px solid transparent',
                        cursor: 'default',
                        transition: 'border-color 0.15s',
                      }}
                      onMouseEnter={(e) => { (e.currentTarget as HTMLDivElement).style.borderColor = '#444'; }}
                      onMouseLeave={(e) => { (e.currentTarget as HTMLDivElement).style.borderColor = 'transparent'; }}
                    >
                      {/* Date — formatDateValue strips the TZ suffix
                          (Local/UTC mode → YYYY-MM-DD HH:MM:SS; Age mode →
                          "Nm ago"); the column header carries the TZ via
                          formatDateHeader() above. Tooltip via formatTooltip
                          shows the OPPOSITE representation WITH the TZ suffix
                          intact so users can hover-disambiguate. Width pinned
                          at 220px to match Transactions COL.date. See
                          l-date-display-suffix-cleanup (2026-06-04). */}
                      <span
                        style={{ fontSize: '14px', color: '#ddd', width: '220px', flexShrink: 0, whiteSpace: 'nowrap' }}
                        title={formatTooltip(request.date)}
                      >
                        {formatDateValue(request.date)}
                      </span>

                      {/* Label (single-line; message moved to its own column).
                          Empty label renders as muted em-dash (mirrors the
                          Message + Amount empty placeholders). */}
                      <div
                        style={{ flex: 1, minWidth: 0, fontSize: '14px', color: '#ddd', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        title={request.label || ''}
                      >
                        {request.label ? sanitizeText(request.label) : <span style={{ color: '#666' }}>—</span>}
                      </div>

                      {/* Message column — single-line, ellipsis-truncated,
                          full text exposed via title tooltip. Empty message
                          renders as muted em-dash for symmetry with the
                          Amount cell's '-' empty placeholder. */}
                      <div
                        style={{
                          flex: 1,
                          minWidth: 0,
                          fontSize: '14px',
                          color: request.message ? '#aaa' : '#666',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}
                        title={request.message || ''}
                      >
                        {request.message ? sanitizeText(request.message) : '—'}
                      </div>

                      {/* Amount (160px, right-aligned monospace 14px 600
                          #27ae60) — matches Transactions row Amount tokens. */}
                      <span style={{ fontSize: '14px', color: '#27ae60', fontWeight: 600, fontFamily: 'monospace', width: '160px', flexShrink: 0, textAlign: 'right' }}>
                        {request.amount ? formatAmount(request.amount, false) : '-'}
                      </span>

                      {/* Action buttons (2x 26px + 4px gap = 56px; container
                          width matches the 60px header spacer with a 4px
                          right-edge headroom). */}
                      <div style={{ display: 'flex', gap: '4px', flexShrink: 0, width: '60px', justifyContent: 'flex-end' }}>
                        <button
                          type="button"
                          onClick={() => handleViewRequest(rowKey)}
                          title={t('receive.show')}
                          style={{
                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                            width: '24px', height: '24px',
                            background: 'none', border: '1px solid #3a3a3a', borderRadius: '4px',
                            color: '#888', cursor: 'pointer', transition: 'color 0.15s, border-color 0.15s',
                          }}
                          onMouseEnter={(e) => { const el = e.currentTarget; el.style.color = '#ddd'; el.style.borderColor = '#555'; }}
                          onMouseLeave={(e) => { const el = e.currentTarget; el.style.color = '#888'; el.style.borderColor = '#3a3a3a'; }}
                        >
                          <Eye size={13} />
                        </button>
                        <button
                          type="button"
                          onClick={() => handleRemoveClick(rowKey)}
                          title={t('receive.remove')}
                          style={{
                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                            width: '24px', height: '24px',
                            background: 'none', border: '1px solid #3a3a3a', borderRadius: '4px',
                            color: '#888', cursor: 'pointer', transition: 'color 0.15s, border-color 0.15s',
                          }}
                          onMouseEnter={(e) => { const el = e.currentTarget; el.style.color = '#ff6666'; el.style.borderColor = '#555'; }}
                          onMouseLeave={(e) => { const el = e.currentTarget; el.style.color = '#888'; el.style.borderColor = '#3a3a3a'; }}
                        >
                          <Trash2 size={13} />
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* Pagination footer — outside the scroll container so it stays
              visible at the bottom of the card regardless of row count.
              No rightSlot (no Export on Receive). */}
          {requestsTotal > 0 && (
            <div style={{ flexShrink: 0, marginTop: '4px' }}>
              <PaginationFooter
                rangeStart={
                  requestsTotal === 0 ? 0 : (requestsCurrentPage - 1) * requestsPageSize + 1
                }
                rangeEnd={Math.min(requestsCurrentPage * requestsPageSize, requestsTotal)}
                total={requestsTotal}
                currentPage={requestsCurrentPage}
                totalPages={requestsTotalPages}
                onPageChange={setRequestsPage}
                pageSize={requestsPageSize}
                pageSizeOptions={PAYMENT_REQUESTS_PAGE_SIZES}
                onPageSizeChange={setRequestsPageSize}
                isLoading={isLoadingRequests}
                dense
              />
            </div>
          )}
        </div>
      </div>

      {/* Receiving Addresses Dialog */}
      <ReceivingAddressesDialog
        isOpen={isAddressesDialogOpen}
        onClose={closeAddressesDialog}
      />

      {/* Request Payment Dialog (for viewing saved requests) */}
      <RequestPaymentDialog />

      {/* Remove Confirmation Dialog */}
      <SimpleConfirmDialog
        isOpen={confirmRemoveKey !== null}
        title={t('receive.removeConfirmTitle')}
        message={t('receive.removeConfirmMessage')}
        confirmText={t('receive.remove')}
        onConfirm={handleConfirmRemove}
        onCancel={() => setConfirmRemoveKey(null)}
        isDestructive
      />

      {/* Copy Feedback Toast */}
      {copyFeedback && (
        <div
          role="status"
          aria-live="polite"
          style={{
            position: 'fixed',
            bottom: 'calc(var(--qt-statusbar-height) + 12px)',
            left: '50%',
            transform: 'translateX(-50%)',
            backgroundColor: '#333',
            color: '#ddd',
            padding: '8px 16px',
            borderRadius: '6px',
            boxShadow: '0 4px 12px rgba(0, 0, 0, 0.3)',
            fontSize: '12px',
            zIndex: 50,
            border: '1px solid #555',
          }}
        >
          {copyFeedback}
        </div>
      )}

    </div>
  );
};

