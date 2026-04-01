import React, { useEffect, useState, useMemo, useCallback, useRef } from 'react';
import { X, ChevronUp, ChevronDown, Plus, Pencil, Trash2, Key, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { sanitizeErrorMessage } from '@/shared/utils/sanitize';
import {
  GetMasternodeConfig,
  AddMasternodeConfig,
  UpdateMasternodeConfig,
  DeleteMasternodeConfig,
  GenerateMasternodeKey,
  GetMasternodeOutputs,
  ReloadMasternodeConfig,
} from '@wailsjs/go/main/App';

// Types matching backend structures
interface MasternodeConfigEntry {
  alias: string;
  ip: string;          // IP:Port format
  privateKey: string;
  txHash: string;
  outputIndex: number;
}

interface MasternodeOutput {
  txHash: string;
  outputIndex: number;
  amount: number;      // In TWINS
  tier: string;        // Bronze/Silver/Gold/Platinum
  confirmations: number;
  isReady: boolean;
}

interface MasternodeConfigDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

type SortDirection = 'asc' | 'desc';

// Form mode: list view, add new, or edit existing
type FormMode = 'list' | 'add' | 'edit';

export const MasternodeConfigDialog: React.FC<MasternodeConfigDialogProps> = ({
  isOpen,
  onClose,
}) => {
  const { t } = useTranslation('masternode');

  // List state
  const [configEntries, setConfigEntries] = useState<MasternodeConfigEntry[]>([]);
  const [availableOutputs, setAvailableOutputs] = useState<MasternodeOutput[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  // Form state
  const [formMode, setFormMode] = useState<FormMode>('list');
  const [editingAlias, setEditingAlias] = useState<string | null>(null);
  const [formData, setFormData] = useState<MasternodeConfigEntry>({
    alias: '',
    ip: '',
    privateKey: '',
    txHash: '',
    outputIndex: 0,
  });
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isGeneratingKey, setIsGeneratingKey] = useState(false);
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  // Delete confirmation state
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  // Mounted ref to prevent state updates after unmount
  const mountedRef = useRef(true);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, []);

  // Auto-clear success message
  useEffect(() => {
    if (!successMessage) return;
    const timeoutId = setTimeout(() => {
      if (mountedRef.current) setSuccessMessage(null);
    }, 3000);
    return () => clearTimeout(timeoutId);
  }, [successMessage]);

  // Fetch config entries when dialog opens
  const fetchConfig = useCallback(async () => {
    if (!mountedRef.current) return;
    setIsLoading(true);
    setError(null);

    try {
      const [entries, outputs] = await Promise.all([
        GetMasternodeConfig(),
        GetMasternodeOutputs(),
      ]);
      if (mountedRef.current) {
        setConfigEntries(entries || []);
        setAvailableOutputs(outputs || []);
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(t('config.fetchError'));
        console.error('Failed to fetch masternode config:', err);
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
      }
    }
  }, [t]);

  useEffect(() => {
    if (isOpen) {
      fetchConfig();
      setSelectedIndex(null);
      setFormMode('list');
      setError(null);
      setSuccessMessage(null);
    }
  }, [isOpen, fetchConfig]);

  // Sort entries by alias
  const sortedEntries = useMemo(() => {
    const sorted = [...configEntries].sort((a, b) => {
      const comparison = a.alias.localeCompare(b.alias);
      return sortDirection === 'asc' ? comparison : -comparison;
    });
    return sorted;
  }, [configEntries, sortDirection]);

  // Filter available outputs to exclude ones already used
  const unusedOutputs = useMemo(() => {
    const usedSet = new Set(
      configEntries.map((entry) => `${entry.txHash}:${entry.outputIndex}`)
    );
    // If editing, include the current entry's output
    if (formMode === 'edit' && editingAlias) {
      const current = configEntries.find((e) => e.alias === editingAlias);
      if (current) {
        usedSet.delete(`${current.txHash}:${current.outputIndex}`);
      }
    }
    return availableOutputs.filter(
      (output) => !usedSet.has(`${output.txHash}:${output.outputIndex}`)
    );
  }, [availableOutputs, configEntries, formMode, editingAlias]);

  const unusedReady = useMemo(() => unusedOutputs.filter((o) => o.isReady), [unusedOutputs]);
  const unusedPending = useMemo(() => unusedOutputs.filter((o) => !o.isReady), [unusedOutputs]);
  const hasPendingUnused = unusedPending.length > 0;

  // Poll for pending collateral confirmations when form is open and UTXOs are pending
  useEffect(() => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
    if (formMode !== 'list' && hasPendingUnused) {
      pollTimerRef.current = setInterval(async () => {
        if (!mountedRef.current) return;
        try {
          const outputs = await GetMasternodeOutputs();
          if (mountedRef.current) setAvailableOutputs(outputs || []);
        } catch { }
      }, 30000);
    }
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [formMode, hasPendingUnused]);

  // Toggle sort direction
  const toggleSort = () => {
    setSortDirection((prev) => (prev === 'asc' ? 'desc' : 'asc'));
  };

  // Handle row selection
  const handleRowClick = (index: number) => {
    setSelectedIndex(index === selectedIndex ? null : index);
  };

  // Reset form to initial state
  const resetForm = useCallback(() => {
    setFormData({
      alias: '',
      ip: '',
      privateKey: '',
      txHash: '',
      outputIndex: 0,
    });
    setFormErrors({});
    setEditingAlias(null);
  }, []);

  // Handle Add button click
  const handleAdd = () => {
    resetForm();
    setFormMode('add');
  };

  // Handle Edit button click
  const handleEdit = () => {
    if (selectedIndex === null) return;
    const entry = sortedEntries[selectedIndex];
    if (!entry) return;

    setFormData({
      alias: entry.alias,
      ip: entry.ip,
      privateKey: entry.privateKey,
      txHash: entry.txHash,
      outputIndex: entry.outputIndex,
    });
    setEditingAlias(entry.alias);
    setFormErrors({});
    setFormMode('edit');
  };

  // Handle Delete button click
  const handleDeleteClick = () => {
    if (selectedIndex === null) return;
    const entry = sortedEntries[selectedIndex];
    if (!entry) return;
    setDeleteConfirm(entry.alias);
  };

  // Confirm delete
  const handleDeleteConfirm = async () => {
    if (!deleteConfirm) return;

    setIsDeleting(true);
    setError(null);

    try {
      await DeleteMasternodeConfig(deleteConfirm);
      if (mountedRef.current) {
        setSuccessMessage(t('config.deleteSuccess', { alias: deleteConfirm }));
        setDeleteConfirm(null);
        setSelectedIndex(null);
        await fetchConfig();
      }
    } catch (err) {
      if (mountedRef.current) {
        const errorMsg = err instanceof Error ? err.message : 'Unknown error';
        setError(t('config.deleteFailed', { error: sanitizeErrorMessage(errorMsg) }));
      }
    } finally {
      if (mountedRef.current) {
        setIsDeleting(false);
      }
    }
  };

  // Generate new private key
  const handleGenerateKey = async () => {
    setIsGeneratingKey(true);
    setError(null);

    try {
      const key = await GenerateMasternodeKey();
      if (mountedRef.current) {
        setFormData((prev) => ({ ...prev, privateKey: key }));
        setFormErrors((prev) => ({ ...prev, privateKey: '' }));
      }
    } catch (err) {
      if (mountedRef.current) {
        const errorMsg = err instanceof Error ? err.message : 'Unknown error';
        setError(t('config.generateKeyFailed', { error: sanitizeErrorMessage(errorMsg) }));
      }
    } finally {
      if (mountedRef.current) {
        setIsGeneratingKey(false);
      }
    }
  };

  // Reload config from file
  const handleReload = async () => {
    setIsLoading(true);
    setError(null);

    try {
      await ReloadMasternodeConfig();
      if (mountedRef.current) {
        setSuccessMessage(t('config.reloadSuccess'));
        await fetchConfig();
      }
    } catch (err) {
      if (mountedRef.current) {
        const errorMsg = err instanceof Error ? err.message : 'Unknown error';
        setError(t('config.reloadFailed', { error: sanitizeErrorMessage(errorMsg) }));
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
      }
    }
  };

  // Validate form
  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    // Alias validation
    if (!formData.alias.trim()) {
      errors.alias = t('config.validation.aliasRequired');
    } else if (!/^[a-zA-Z0-9_]+$/.test(formData.alias)) {
      errors.alias = t('config.validation.aliasFormat');
    } else if (
      formMode === 'add' &&
      configEntries.some((e) => e.alias === formData.alias)
    ) {
      errors.alias = t('config.validation.aliasExists');
    } else if (
      formMode === 'edit' &&
      formData.alias !== editingAlias &&
      configEntries.some((e) => e.alias === formData.alias)
    ) {
      errors.alias = t('config.validation.aliasExists');
    }

    // IP:Port validation
    if (!formData.ip.trim()) {
      errors.ip = t('config.validation.ipRequired');
    } else if (!formData.ip.includes(':')) {
      errors.ip = t('config.validation.ipFormat');
    } else {
      const [ip, port] = formData.ip.split(':');
      if (!ip || !port) {
        errors.ip = t('config.validation.ipFormat');
      } else {
        const portNum = parseInt(port, 10);
        if (isNaN(portNum) || portNum < 1 || portNum > 65535) {
          errors.ip = t('config.validation.portRange');
        } else if (portNum !== 37817) {
          // Mainnet port warning (not an error, but informative)
          // For now, just accept it - backend will validate
        }
      }
    }

    // Private key validation
    if (!formData.privateKey.trim()) {
      errors.privateKey = t('config.validation.privateKeyRequired');
    }

    // Collateral validation
    if (!formData.txHash || formData.txHash === '') {
      errors.collateral = t('config.validation.collateralRequired');
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // Handle form submit
  const handleSubmit = async () => {
    if (!validateForm()) return;

    setIsSubmitting(true);
    setError(null);

    try {
      const entry: MasternodeConfigEntry = {
        alias: formData.alias.trim(),
        ip: formData.ip.trim(),
        privateKey: formData.privateKey.trim(),
        txHash: formData.txHash,
        outputIndex: formData.outputIndex,
      };

      if (formMode === 'add') {
        await AddMasternodeConfig(entry);
        if (mountedRef.current) {
          setSuccessMessage(t('config.addSuccess', { alias: entry.alias }));
        }
      } else if (formMode === 'edit' && editingAlias) {
        await UpdateMasternodeConfig(editingAlias, entry);
        if (mountedRef.current) {
          setSuccessMessage(t('config.updateSuccess', { alias: entry.alias }));
        }
      }

      if (mountedRef.current) {
        setFormMode('list');
        resetForm();
        await fetchConfig();
      }
    } catch (err) {
      if (mountedRef.current) {
        const errorMsg = err instanceof Error ? err.message : 'Unknown error';
        setError(sanitizeErrorMessage(errorMsg));
      }
    } finally {
      if (mountedRef.current) {
        setIsSubmitting(false);
      }
    }
  };

  // Ref so the keyboard handler always calls the latest handleSubmit
  const handleSubmitRef = useRef(handleSubmit);
  handleSubmitRef.current = handleSubmit;

  // Handle cancel form
  const handleCancel = useCallback(() => {
    setFormMode('list');
    resetForm();
    setError(null);
  }, [resetForm]);

  // Handle collateral selection
  const handleCollateralChange = (value: string) => {
    if (value === '') {
      setFormData((prev) => ({ ...prev, txHash: '', outputIndex: 0 }));
    } else {
      const [txHash, outputIndex] = value.split(':');
      setFormData((prev) => ({
        ...prev,
        txHash,
        outputIndex: parseInt(outputIndex, 10),
      }));
    }
    setFormErrors((prev) => ({ ...prev, collateral: '' }));
  };

  // Handle keyboard events
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (deleteConfirm) {
          setDeleteConfirm(null);
        } else if (formMode !== 'list') {
          handleCancel();
        } else {
          onClose();
        }
      } else if (e.key === 'Enter' && formMode !== 'list' && !isSubmitting) {
        handleSubmitRef.current();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, formMode, isSubmitting, deleteConfirm, onClose, handleCancel]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
      onClick={(e) => {
        if (e.target === e.currentTarget && formMode === 'list' && !deleteConfirm) {
          onClose();
        }
      }}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="masternode-config-title"
        className="bg-[#2b2b2b] rounded-lg shadow-xl w-[800px] max-h-[600px] flex flex-col"
        style={{ border: '1px solid #555' }}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#555]">
          <h2 id="masternode-config-title" className="text-lg font-semibold text-[#ddd]">
            {formMode === 'list'
              ? t('config.title')
              : formMode === 'add'
              ? t('config.addTitle')
              : t('config.editTitle')}
          </h2>
          <button
            onClick={formMode === 'list' ? onClose : handleCancel}
            className="text-[#999] hover:text-[#ddd] transition-colors"
            aria-label="Close dialog"
          >
            <X size={20} />
          </button>
        </div>

        {/* Error Display */}
        {error && (
          <div className="px-6 py-2 bg-[#4a2a2a] border-b border-[#ff6666]">
            <p className="text-sm text-[#ff6666]">{error}</p>
          </div>
        )}

        {/* Success Display */}
        {successMessage && (
          <div className="px-6 py-2 bg-[#2a4a2a] border-b border-[#66ff66]">
            <p className="text-sm text-[#66ff66]">{successMessage}</p>
          </div>
        )}

        {/* Content */}
        {formMode === 'list' ? (
          /* List View */
          <>
            <div className="flex-1 overflow-auto px-6 py-3">
              {isLoading ? (
                <div className="text-center py-8 text-[#999]">{t('config.loading')}</div>
              ) : sortedEntries.length === 0 ? (
                <div className="text-center py-8 text-[#999]">{t('config.noEntries')}</div>
              ) : (
                <table className="w-full text-sm">
                  <thead className="border-b border-[#555]">
                    <tr>
                      <th
                        className="text-left py-2 cursor-pointer hover:bg-[#333] transition-colors select-none"
                        style={{ width: '20%', backgroundColor: '#3a3a3a' }}
                        onClick={toggleSort}
                      >
                        <div className="flex items-center gap-1 px-2 text-[#ddd]">
                          {t('config.columns.alias')}
                          {sortDirection === 'asc' ? (
                            <ChevronUp size={14} />
                          ) : (
                            <ChevronDown size={14} />
                          )}
                        </div>
                      </th>
                      <th
                        className="text-left py-2"
                        style={{ width: '25%', backgroundColor: '#3a3a3a' }}
                      >
                        <div className="px-2 text-[#ddd]">{t('config.columns.ip')}</div>
                      </th>
                      <th
                        className="text-left py-2"
                        style={{ width: '55%', backgroundColor: '#3a3a3a' }}
                      >
                        <div className="px-2 text-[#ddd]">{t('config.columns.collateral')}</div>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedEntries.map((entry, index) => (
                      <tr
                        key={entry.alias}
                        onClick={() => handleRowClick(index)}
                        className={`border-b border-[#444] cursor-pointer transition-colors ${
                          selectedIndex === index
                            ? 'bg-[#0066cc] hover:bg-[#0055aa]'
                            : 'hover:bg-[#333]'
                        }`}
                      >
                        <td className="py-2 px-2">
                          <span
                            className={selectedIndex === index ? 'text-white' : 'text-[#ddd]'}
                          >
                            {entry.alias}
                          </span>
                        </td>
                        <td className="py-2 px-2">
                          <span
                            className={`font-mono ${
                              selectedIndex === index ? 'text-white' : 'text-[#ddd]'
                            }`}
                          >
                            {entry.ip}
                          </span>
                        </td>
                        <td className="py-2 px-2">
                          <span
                            className={`font-mono text-xs ${
                              selectedIndex === index ? 'text-white' : 'text-[#999]'
                            }`}
                            title={`${entry.txHash}:${entry.outputIndex}`}
                          >
                            {entry.txHash.substring(0, 12)}...:{entry.outputIndex}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>

            {/* List Action Buttons */}
            <div className="flex items-center justify-between px-6 py-4 border-t border-[#555]">
              <div className="flex items-center gap-2">
                <button
                  onClick={handleAdd}
                  disabled={isLoading}
                  className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
                >
                  <Plus size={14} />
                  {t('config.buttons.add')}
                </button>
                <button
                  onClick={handleEdit}
                  disabled={selectedIndex === null || isLoading}
                  className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
                >
                  <Pencil size={14} />
                  {t('config.buttons.edit')}
                </button>
                <button
                  onClick={handleDeleteClick}
                  disabled={selectedIndex === null || isLoading}
                  className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
                >
                  <Trash2 size={14} />
                  {t('config.buttons.delete')}
                </button>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={handleReload}
                  disabled={isLoading}
                  className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
                >
                  <RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} />
                  {t('config.buttons.reload')}
                </button>
                <button
                  onClick={onClose}
                  className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors"
                >
                  {t('config.buttons.close')}
                </button>
              </div>
            </div>
          </>
        ) : (
          /* Add/Edit Form */
          <div className="flex-1 overflow-auto px-6 py-4">
            <div className="space-y-4">
              {/* Alias Field */}
              <div>
                <label className="block text-sm text-[#ddd] mb-1">
                  {t('config.form.alias')} <span className="text-[#ff6666]">*</span>
                </label>
                <input
                  type="text"
                  value={formData.alias}
                  onChange={(e) => {
                    setFormData((prev) => ({ ...prev, alias: e.target.value }));
                    setFormErrors((prev) => ({ ...prev, alias: '' }));
                  }}
                  placeholder={t('config.form.aliasPlaceholder')}
                  maxLength={50}
                  className={`w-full px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border rounded focus:outline-none focus:border-[#0066cc] ${
                    formErrors.alias ? 'border-[#ff6666]' : 'border-[#555]'
                  }`}
                />
                {formErrors.alias && (
                  <p className="mt-1 text-xs text-[#ff6666]">{formErrors.alias}</p>
                )}
              </div>

              {/* IP:Port Field */}
              <div>
                <label className="block text-sm text-[#ddd] mb-1">
                  {t('config.form.ip')} <span className="text-[#ff6666]">*</span>
                </label>
                <input
                  type="text"
                  value={formData.ip}
                  onChange={(e) => {
                    setFormData((prev) => ({ ...prev, ip: e.target.value }));
                    setFormErrors((prev) => ({ ...prev, ip: '' }));
                  }}
                  placeholder={t('config.form.ipPlaceholder')}
                  className={`w-full px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border rounded focus:outline-none focus:border-[#0066cc] ${
                    formErrors.ip ? 'border-[#ff6666]' : 'border-[#555]'
                  }`}
                />
                {formErrors.ip && (
                  <p className="mt-1 text-xs text-[#ff6666]">{formErrors.ip}</p>
                )}
                <p className="mt-1 text-xs text-[#999]">{t('config.form.ipHint')}</p>
              </div>

              {/* Private Key Field */}
              <div>
                <label className="block text-sm text-[#ddd] mb-1">
                  {t('config.form.privateKey')} <span className="text-[#ff6666]">*</span>
                </label>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={formData.privateKey}
                    onChange={(e) => {
                      setFormData((prev) => ({ ...prev, privateKey: e.target.value }));
                      setFormErrors((prev) => ({ ...prev, privateKey: '' }));
                    }}
                    placeholder={t('config.form.privateKeyPlaceholder')}
                    className={`flex-1 px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border rounded focus:outline-none focus:border-[#0066cc] font-mono ${
                      formErrors.privateKey ? 'border-[#ff6666]' : 'border-[#555]'
                    }`}
                  />
                  <button
                    onClick={handleGenerateKey}
                    disabled={isGeneratingKey}
                    className="px-4 py-2 text-sm bg-[#0066cc] text-white rounded hover:bg-[#0052a3] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
                  >
                    <Key size={14} className={isGeneratingKey ? 'animate-pulse' : ''} />
                    {t('config.form.generateKey')}
                  </button>
                </div>
                {formErrors.privateKey && (
                  <p className="mt-1 text-xs text-[#ff6666]">{formErrors.privateKey}</p>
                )}
                <p className="mt-1 text-xs text-[#999]">{t('config.form.privateKeyHint')}</p>
              </div>

              {/* Collateral UTXO Field */}
              <div>
                <label className="block text-sm text-[#ddd] mb-1">
                  {t('config.form.collateral')} <span className="text-[#ff6666]">*</span>
                </label>
                <select
                  value={formData.txHash ? `${formData.txHash}:${formData.outputIndex}` : ''}
                  onChange={(e) => handleCollateralChange(e.target.value)}
                  className={`w-full px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border rounded focus:outline-none focus:border-[#0066cc] ${
                    formErrors.collateral ? 'border-[#ff6666]' : 'border-[#555]'
                  }`}
                >
                  <option value="">{t('config.form.selectCollateral')}</option>
                  {unusedReady.map((output) => (
                    <option
                      key={`${output.txHash}:${output.outputIndex}`}
                      value={`${output.txHash}:${output.outputIndex}`}
                    >
                      {output.tier} - {output.amount.toLocaleString()} TWINS ({output.txHash.substring(0, 8)}...:{output.outputIndex})
                    </option>
                  ))}
                  {unusedPending.map((output) => (
                    <option
                      key={`pending:${output.txHash}:${output.outputIndex}`}
                      value=""
                      disabled
                    >
                      {output.tier} - {output.amount.toLocaleString()} TWINS ({output.txHash.substring(0, 8)}...:{output.outputIndex}) {t('config.form.collateralNotReady', { count: output.confirmations })}
                    </option>
                  ))}
                </select>
                {formErrors.collateral && (
                  <p className="mt-1 text-xs text-[#ff6666]">{formErrors.collateral}</p>
                )}
                {unusedReady.length === 0 && unusedPending.length === 0 && formMode === 'add' && (
                  <p className="mt-1 text-xs text-[#ffaa00]">{t('config.form.noCollateralAvailable')}</p>
                )}
                {hasPendingUnused && (
                  <p className="mt-1 text-xs text-[#ffaa00]">{t('config.form.pendingCollateralHint')}</p>
                )}
                <p className="mt-1 text-xs text-[#999]">{t('config.form.collateralHint')}</p>
              </div>
            </div>

            {/* Form Action Buttons */}
            <div className="flex items-center justify-end gap-2 mt-6 pt-4 border-t border-[#555]">
              <button
                onClick={handleCancel}
                disabled={isSubmitting}
                className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50"
              >
                {t('config.buttons.cancel')}
              </button>
              <button
                onClick={handleSubmit}
                disabled={isSubmitting}
                className="px-4 py-2 text-sm bg-[#0066cc] text-white rounded hover:bg-[#0052a3] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isSubmitting
                  ? t('config.buttons.saving')
                  : formMode === 'add'
                  ? t('config.buttons.add')
                  : t('config.buttons.save')}
              </button>
            </div>
          </div>
        )}

        {/* Delete Confirmation Modal */}
        {deleteConfirm && (
          <div
            className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
            onClick={(e) => {
              if (e.target === e.currentTarget) {
                setDeleteConfirm(null);
              }
            }}
            role="presentation"
          >
            <div
              role="alertdialog"
              aria-modal="true"
              aria-labelledby="delete-confirm-title"
              className="bg-[#2b2b2b] rounded-lg shadow-xl w-[400px] p-6"
              style={{ border: '1px solid #555' }}
            >
              <h3 id="delete-confirm-title" className="text-lg font-semibold text-[#ddd] mb-4">
                {t('config.deleteConfirm.title')}
              </h3>
              <p className="text-sm text-[#ddd] mb-6">
                {t('config.deleteConfirm.message', { alias: deleteConfirm })}
              </p>
              <div className="flex items-center justify-end gap-2">
                <button
                  onClick={() => setDeleteConfirm(null)}
                  disabled={isDeleting}
                  className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50"
                >
                  {t('config.buttons.cancel')}
                </button>
                <button
                  onClick={handleDeleteConfirm}
                  disabled={isDeleting}
                  className="px-4 py-2 text-sm bg-[#cc3333] text-white rounded hover:bg-[#aa2222] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {isDeleting ? t('config.buttons.deleting') : t('config.buttons.delete')}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
