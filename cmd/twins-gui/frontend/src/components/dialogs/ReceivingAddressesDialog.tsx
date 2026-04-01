import React, { useEffect, useState, useMemo, useCallback, useRef } from 'react';
import { X, ChevronUp, ChevronDown, Copy, Download, Plus } from 'lucide-react';
import { useReceive } from '@/store/useStore';
import { SaveCSVFile } from '@wailsjs/go/main/App';

/**
 * Escape a value for CSV format with formula injection protection
 * - Prefixes formula trigger characters (=, +, -, @) with single quote to prevent CSV injection
 * - Always wraps in quotes (matching Qt csvmodelwriter.cpp behavior)
 * - Doubles any quotes within the value
 * @see https://owasp.org/www-community/attacks/CSV_Injection
 */
function escapeCSVValue(value: string | number | boolean): string {
  let str = String(value);

  // Sanitize formula injection (CSV injection attack prevention)
  if (/^[=+\-@]/.test(str)) {
    str = "'" + str;
  }

  // Always escape quotes and wrap in quotes (matching Qt behavior)
  return `"${str.replace(/"/g, '""')}"`;
}

interface ReceivingAddressesDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

type SortDirection = 'asc' | 'desc';

export const ReceivingAddressesDialog: React.FC<ReceivingAddressesDialogProps> = ({
  isOpen,
  onClose,
}) => {
  const {
    receivingAddresses,
    isLoading,
    isGeneratingAddress,
    error,
    fetchReceivingAddresses,
    generateNewAddress,
    clearError,
  } = useReceive();

  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');
  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);
  const [isCopying, setIsCopying] = useState(false);

  // New address label prompt state
  const [showLabelPrompt, setShowLabelPrompt] = useState(false);
  const [newAddressLabel, setNewAddressLabel] = useState('');

  // Track newly created address for selection after state updates
  const pendingSelectAddressRef = useRef<string | null>(null);

  // Auto-clear copy feedback with cleanup to prevent memory leaks
  useEffect(() => {
    if (!copyFeedback) return;
    const timeoutId = setTimeout(() => setCopyFeedback(null), 2000);
    return () => clearTimeout(timeoutId);
  }, [copyFeedback]);

  // Fetch addresses when dialog opens
  useEffect(() => {
    if (isOpen) {
      fetchReceivingAddresses();
      setSelectedIndex(null);
      clearError();
    }
  }, [isOpen, fetchReceivingAddresses, clearError]);

  // Sort addresses by label
  const sortedAddresses = useMemo(() => {
    const sorted = [...receivingAddresses].sort((a, b) => {
      const labelA = a.label || '';
      const labelB = b.label || '';
      const comparison = labelA.localeCompare(labelB);
      return sortDirection === 'asc' ? comparison : -comparison;
    });
    return sorted;
  }, [receivingAddresses, sortDirection]);

  // Select newly created address after sortedAddresses updates
  useEffect(() => {
    if (pendingSelectAddressRef.current) {
      const index = sortedAddresses.findIndex(
        (addr) => addr.address === pendingSelectAddressRef.current
      );
      if (index !== -1) {
        setSelectedIndex(index);
        pendingSelectAddressRef.current = null;
      }
    }
  }, [sortedAddresses]);

  // Toggle sort direction
  const toggleSort = () => {
    setSortDirection((prev) => (prev === 'asc' ? 'desc' : 'asc'));
  };

  // Handle row selection
  const handleRowClick = (index: number) => {
    setSelectedIndex(index === selectedIndex ? null : index);
  };

  // Copy address to clipboard
  const handleCopy = async () => {
    if (selectedIndex === null || isCopying) return;

    const address = sortedAddresses[selectedIndex];
    if (!address) return;

    setIsCopying(true);
    try {
      await navigator.clipboard.writeText(address.address);
      setCopyFeedback('Address copied to clipboard');
    } catch (err) {
      // Fallback for older browsers
      try {
        const textArea = document.createElement('textarea');
        textArea.value = address.address;
        textArea.style.position = 'fixed';
        textArea.style.opacity = '0';
        document.body.appendChild(textArea);
        textArea.select();
        document.execCommand('copy');
        document.body.removeChild(textArea);
        setCopyFeedback('Address copied to clipboard');
      } catch (fallbackErr) {
        setCopyFeedback('Failed to copy');
      }
    } finally {
      setIsCopying(false);
    }
  };

  // Export addresses to CSV
  const handleExport = async () => {
    if (sortedAddresses.length === 0) return;

    // Add UTF-8 BOM for Excel compatibility
    const BOM = '\ufeff';
    const csvContent = BOM + [
      'Label,Address',
      ...sortedAddresses.map((addr) => {
        return [escapeCSVValue(addr.label || ''), escapeCSVValue(addr.address)].join(',');
      }),
    ].join('\n');

    try {
      const saved = await SaveCSVFile(csvContent, 'receiving_addresses.csv', 'Export Addresses');
      if (saved) {
        setCopyFeedback('Addresses exported successfully');
      }
    } catch (err) {
      setCopyFeedback('Failed to export addresses');
    }
  };

  // Handle new address creation
  const handleNewAddress = useCallback(() => {
    setShowLabelPrompt(true);
    setNewAddressLabel('');
  }, []);

  const handleCreateAddress = useCallback(async () => {
    const result = await generateNewAddress(newAddressLabel);
    if (result) {
      setShowLabelPrompt(false);
      setNewAddressLabel('');
      // Store address for selection after state updates
      pendingSelectAddressRef.current = result.address;
    }
  }, [generateNewAddress, newAddressLabel]);

  const handleCancelNewAddress = useCallback(() => {
    setShowLabelPrompt(false);
    setNewAddressLabel('');
  }, []);

  // Handle keyboard events
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (showLabelPrompt) {
          handleCancelNewAddress();
        } else {
          onClose();
        }
      } else if (e.key === 'Enter' && showLabelPrompt) {
        handleCreateAddress();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, showLabelPrompt, onClose, handleCancelNewAddress, handleCreateAddress]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
      onClick={(e) => {
        if (e.target === e.currentTarget && !showLabelPrompt) {
          onClose();
        }
      }}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="receiving-addresses-title"
        aria-describedby="receiving-addresses-description"
        className="bg-[#2b2b2b] rounded-lg shadow-xl w-[750px] max-h-[500px] flex flex-col"
        style={{ border: '1px solid #555' }}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#555]">
          <h2 id="receiving-addresses-title" className="text-lg font-semibold text-[#ddd]">
            Receiving addresses
          </h2>
          <button
            onClick={onClose}
            className="text-[#999] hover:text-[#ddd] transition-colors"
            aria-label="Close dialog"
          >
            <X size={20} />
          </button>
        </div>

        {/* Description */}
        <div className="px-6 py-3 border-b border-[#555]">
          <p id="receiving-addresses-description" className="text-sm text-[#ddd]">
            These are your TWINS addresses for receiving payments. It is recommended to use a new receiving address for each transaction.
          </p>
        </div>

        {/* Error Display */}
        {error && (
          <div className="px-6 py-2 bg-[#4a2a2a] border-b border-[#ff6666]">
            <p className="text-sm text-[#ff6666]">{error}</p>
          </div>
        )}

        {/* Address Table */}
        <div className="flex-1 overflow-auto px-6 py-3">
          {isLoading ? (
            <div className="text-center py-8 text-[#999]">Loading addresses...</div>
          ) : sortedAddresses.length === 0 ? (
            <div className="text-center py-8 text-[#999]">No receiving addresses</div>
          ) : (
            <table className="w-full text-sm">
              <thead className="border-b border-[#555]">
                <tr>
                  <th
                    className="text-left py-2 cursor-pointer hover:bg-[#333] transition-colors select-none"
                    style={{ width: '40%', backgroundColor: '#3a3a3a' }}
                    onClick={toggleSort}
                  >
                    <div className="flex items-center gap-1 px-2 text-[#ddd]">
                      Label
                      {sortDirection === 'asc' ? (
                        <ChevronUp size={14} />
                      ) : (
                        <ChevronDown size={14} />
                      )}
                    </div>
                  </th>
                  <th
                    className="text-left py-2"
                    style={{ width: '60%', backgroundColor: '#3a3a3a' }}
                  >
                    <div className="px-2 text-[#ddd]">Address</div>
                  </th>
                </tr>
              </thead>
              <tbody>
                {sortedAddresses.map((addr, index) => (
                  <tr
                    key={addr.address}
                    onClick={() => handleRowClick(index)}
                    className={`border-b border-[#444] cursor-pointer transition-colors ${
                      selectedIndex === index
                        ? 'bg-[#0066cc] hover:bg-[#0055aa]'
                        : 'hover:bg-[#333]'
                    }`}
                  >
                    <td className="py-2 px-2">
                      <span
                        className={
                          selectedIndex === index
                            ? 'text-white'
                            : addr.label
                            ? 'text-[#ddd]'
                            : 'text-[#999]'
                        }
                      >
                        {addr.label || '(no label)'}
                      </span>
                    </td>
                    <td className="py-2 px-2">
                      <span
                        className={`font-mono ${
                          selectedIndex === index ? 'text-white' : 'text-[#ddd]'
                        }`}
                      >
                        {addr.address}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        {/* Action Buttons */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-[#555]">
          <div className="flex items-center gap-2">
            <button
              onClick={handleNewAddress}
              disabled={isGeneratingAddress}
              aria-label="Create new receiving address"
              className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
            >
              <Plus size={14} />
              New
            </button>
            <button
              onClick={handleCopy}
              disabled={selectedIndex === null || isCopying}
              aria-label={isCopying ? 'Copying address' : 'Copy selected address'}
              className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
            >
              <Copy size={14} />
              {isCopying ? 'Copying...' : 'Copy'}
            </button>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={handleExport}
              disabled={sortedAddresses.length === 0}
              aria-label="Export addresses to CSV"
              className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
            >
              <Download size={14} />
              Export
            </button>
            <button
              onClick={onClose}
              aria-label="Close dialog"
              className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors"
            >
              Close
            </button>
          </div>
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

        {/* New Address Label Prompt Modal */}
        {showLabelPrompt && (
          <div
            className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
            onClick={(e) => {
              if (e.target === e.currentTarget) {
                handleCancelNewAddress();
              }
            }}
            role="presentation"
          >
            <div
              role="dialog"
              aria-modal="true"
              aria-labelledby="new-address-title"
              aria-describedby="new-address-description"
              className="bg-[#2b2b2b] rounded-lg shadow-xl w-[400px] p-6"
              style={{ border: '1px solid #555' }}
            >
              <h3 id="new-address-title" className="text-lg font-semibold text-[#ddd] mb-4">
                New receiving address
              </h3>
              <p id="new-address-description" className="text-sm text-[#999] mb-4">
                Enter an optional label for this address:
              </p>
              <input
                type="text"
                value={newAddressLabel}
                onChange={(e) => setNewAddressLabel(e.target.value)}
                placeholder="Label (optional)"
                maxLength={100}
                autoFocus
                aria-label="Address label"
                className="w-full px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border border-[#555] rounded focus:outline-none focus:border-[#0066cc]"
              />
              <div className="flex items-center justify-end gap-2 mt-4">
                <button
                  onClick={handleCancelNewAddress}
                  aria-label="Cancel creating address"
                  className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={handleCreateAddress}
                  disabled={isGeneratingAddress}
                  aria-label={isGeneratingAddress ? 'Creating address' : 'Create address'}
                  className="px-4 py-2 text-sm bg-[#0066cc] text-white rounded hover:bg-[#0052a3] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {isGeneratingAddress ? 'Creating...' : 'OK'}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
