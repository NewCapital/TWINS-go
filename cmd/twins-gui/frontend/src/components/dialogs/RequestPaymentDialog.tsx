import React, { useEffect, useCallback, useRef, useState } from 'react';
import { QRCodeCanvas } from 'qrcode.react';
import { X, Copy, Clipboard, Download } from 'lucide-react';
import { useReceive } from '@/store/useStore';
import { sanitizeText } from '@/shared/utils/sanitize';

// Maximum QR code data length before warning (conservative limit for Level L)
const MAX_QR_DATA_LENGTH = 2000;

// Build twins: URI from payment request data
function buildTwinsURI(address: string, amount?: number, label?: string, message?: string): string {
  let uri = `twins:${address}`;
  const params: string[] = [];

  if (amount && amount > 0) {
    params.push(`amount=${amount}`);
  }
  if (label) {
    params.push(`label=${encodeURIComponent(label)}`);
  }
  if (message) {
    params.push(`message=${encodeURIComponent(message)}`);
  }

  if (params.length > 0) {
    uri += '?' + params.join('&');
  }

  return uri;
}

export const RequestPaymentDialog: React.FC = () => {
  const {
    isRequestDialogOpen,
    selectedRequest,
    closeRequestDialog,
  } = useReceive();

  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);
  const qrRef = useRef<HTMLDivElement>(null);

  // Auto-clear copy feedback
  useEffect(() => {
    if (!copyFeedback) return;
    const timeoutId = setTimeout(() => setCopyFeedback(null), 2000);
    return () => clearTimeout(timeoutId);
  }, [copyFeedback]);

  // Handle keyboard events
  useEffect(() => {
    if (!isRequestDialogOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        closeRequestDialog();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isRequestDialogOpen, closeRequestDialog]);

  // Build URI from selected request
  const uri = selectedRequest
    ? buildTwinsURI(
        selectedRequest.address,
        selectedRequest.amount,
        selectedRequest.label,
        selectedRequest.message
      )
    : '';

  const isURITooLong = uri.length > MAX_QR_DATA_LENGTH;

  // Copy URI to clipboard
  const handleCopyURI = useCallback(async () => {
    if (!uri) return;
    try {
      await navigator.clipboard.writeText(uri);
      setCopyFeedback('URI copied to clipboard');
    } catch {
      // Fallback for older browsers
      try {
        const textArea = document.createElement('textarea');
        textArea.value = uri;
        textArea.style.position = 'fixed';
        textArea.style.opacity = '0';
        document.body.appendChild(textArea);
        textArea.select();
        document.execCommand('copy');
        document.body.removeChild(textArea);
        setCopyFeedback('URI copied to clipboard');
      } catch {
        setCopyFeedback('Failed to copy URI');
      }
    }
  }, [uri]);

  // Copy address to clipboard
  const handleCopyAddress = useCallback(async () => {
    if (!selectedRequest?.address) return;
    try {
      await navigator.clipboard.writeText(selectedRequest.address);
      setCopyFeedback('Address copied to clipboard');
    } catch {
      try {
        const textArea = document.createElement('textarea');
        textArea.value = selectedRequest.address;
        textArea.style.position = 'fixed';
        textArea.style.opacity = '0';
        document.body.appendChild(textArea);
        textArea.select();
        document.execCommand('copy');
        document.body.removeChild(textArea);
        setCopyFeedback('Address copied to clipboard');
      } catch {
        setCopyFeedback('Failed to copy address');
      }
    }
  }, [selectedRequest?.address]);

  // Save QR code as PNG image
  const handleSaveImage = useCallback(() => {
    if (!qrRef.current) return;

    const canvas = qrRef.current.querySelector('canvas');
    if (!canvas) {
      setCopyFeedback('Failed to find QR code canvas');
      return;
    }

    try {
      // Create a link element to trigger download
      const link = document.createElement('a');
      link.download = `twins-payment-${selectedRequest?.address?.slice(0, 8) || 'qr'}.png`;
      link.href = canvas.toDataURL('image/png');
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      setCopyFeedback('QR code image saved');
    } catch {
      setCopyFeedback('Failed to save image');
    }
  }, [selectedRequest?.address]);

  if (!isRequestDialogOpen || !selectedRequest) return null;

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          closeRequestDialog();
        }
      }}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="request-payment-title"
        className="bg-[#2b2b2b] rounded-lg shadow-xl w-[500px] max-h-[90vh] flex flex-col overflow-y-auto"
        style={{ border: '1px solid #555' }}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#555]">
          <h2 id="request-payment-title" className="text-sm font-semibold text-[#ddd] truncate pr-4">
            Request payment to {selectedRequest.address}
          </h2>
          <button
            onClick={closeRequestDialog}
            className="text-[#999] hover:text-[#ddd] transition-colors flex-shrink-0"
            aria-label="Close dialog"
          >
            <X size={20} />
          </button>
        </div>

        {/* QR Code Section */}
        <div className="flex justify-center py-4 bg-[#2b2b2b]">
          <div
            ref={qrRef}
            className="p-3 bg-white rounded"
            style={{ lineHeight: 0 }}
          >
            <QRCodeCanvas
              value={uri}
              size={220}
              level="L"
              includeMargin={false}
              bgColor="#ffffff"
              fgColor="#000000"
            />
          </div>
        </div>

        {/* URI Length Warning */}
        {isURITooLong && (
          <div className="mx-6 mb-2 px-3 py-2 bg-[#4a3a2a] border border-[#ff9966] rounded text-xs text-[#ff9966]">
            Warning: URI is very long and may not scan reliably. Consider shortening the label or message.
          </div>
        )}

        {/* Payment Information Panel */}
        <div
          className="mx-6 mb-4 p-4 rounded"
          style={{
            backgroundColor: '#3a3a3a',
            border: '1px solid #555',
          }}
        >
          <div className="text-sm font-semibold text-[#ddd] mb-3">
            Payment information
          </div>

          {/* URI */}
          <div className="mb-2">
            <span className="text-xs text-[#aaa]">URI: </span>
            <a
              href={uri}
              title="Click to copy URI"
              className="text-xs text-[#6699cc] hover:text-[#88bbee] break-all cursor-pointer"
              onClick={(e) => {
                // Prevent navigation in desktop app - just copy instead
                e.preventDefault();
                handleCopyURI();
              }}
            >
              {uri}
            </a>
          </div>

          {/* Address */}
          <div className="mb-2">
            <span className="text-xs text-[#aaa]">Address: </span>
            <span className="text-xs text-[#ddd] font-mono break-all">
              {sanitizeText(selectedRequest.address)}
            </span>
          </div>

          {/* Amount (if provided) */}
          {selectedRequest.amount > 0 && (
            <div className="mb-2">
              <span className="text-xs text-[#aaa]">Amount: </span>
              <span className="text-xs text-[#ddd]">
                {selectedRequest.amount} TWINS
              </span>
            </div>
          )}

          {/* Label (if provided) */}
          {selectedRequest.label && (
            <div className="mb-2">
              <span className="text-xs text-[#aaa]">Label: </span>
              <span className="text-xs text-[#ddd]">
                {sanitizeText(selectedRequest.label)}
              </span>
            </div>
          )}

          {/* Message (if provided) */}
          {selectedRequest.message && (
            <div>
              <span className="text-xs text-[#aaa]">Message: </span>
              <span className="text-xs text-[#ddd]">
                {sanitizeText(selectedRequest.message)}
              </span>
            </div>
          )}
        </div>

        {/* Action Buttons */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-[#555]">
          <div className="flex items-center gap-2">
            <button
              onClick={handleCopyURI}
              className="px-4 py-2 text-sm bg-[#4a5a6a] text-[#ddd] rounded hover:bg-[#5a6a7a] transition-colors flex items-center gap-2"
              aria-label="Copy URI to clipboard"
            >
              <Copy size={14} />
              Copy URI
            </button>
            <button
              onClick={handleCopyAddress}
              className="px-4 py-2 text-sm bg-[#4a5a6a] text-[#ddd] rounded hover:bg-[#5a6a7a] transition-colors flex items-center gap-2"
              aria-label="Copy address to clipboard"
            >
              <Clipboard size={14} />
              Copy Address
            </button>
            <button
              onClick={handleSaveImage}
              className="px-4 py-2 text-sm bg-[#4a5a6a] text-[#ddd] rounded hover:bg-[#5a6a7a] transition-colors flex items-center gap-2"
              aria-label="Save QR code as image"
            >
              <Download size={14} />
              Save Image...
            </button>
          </div>
          <button
            onClick={closeRequestDialog}
            className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors"
            aria-label="Close dialog"
          >
            Close
          </button>
        </div>

        {/* Copy Feedback Toast */}
        {copyFeedback && (
          <div
            role="status"
            aria-live="polite"
            className="fixed bottom-4 left-1/2 transform -translate-x-1/2 bg-[#333] text-[#ddd] px-4 py-2 rounded shadow-lg text-sm z-50 border border-[#555]"
          >
            {copyFeedback}
          </div>
        )}
      </div>
    </div>
  );
};
