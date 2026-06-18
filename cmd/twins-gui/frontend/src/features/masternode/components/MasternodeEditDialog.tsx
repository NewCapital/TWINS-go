import React, { useEffect, useState, useMemo, useCallback, useRef } from 'react';
import { X, Key, Copy, Check, Eye, EyeOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { sanitizeErrorMessage } from '@/shared/utils/sanitize';
import { Banner } from '@/shared/components/Banner';
import { IconButton } from '@/shared/components/IconButton';
import { RowsPerPageSelect } from '@/shared/components/RowsPerPageSelect';
import { SimpleConfirmDialog } from '@/shared/components/SimpleConfirmDialog';
import { writeToClipboard } from '@/shared/utils/clipboard';
import { useStore } from '@/store/useStore';
import {
  GetMasternodeConfig,
  UpdateMasternodeConfig,
  GenerateMasternodeKey,
  GetMasternodeOutputs,
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

/**
 * Initial form data passed when the dialog is opened via the per-row Pencil
 * IconButton in MasternodesTable. Lets the dialog open IMMEDIATELY in edit
 * mode with form fields populated from the row, instead of flashing through
 * an empty form while `fetchConfig()` resolves. `privateKey` defaults to empty
 * — it is hot-populated from `configEntries` once the fetch lands AND the
 * matching real entry is found in `masternode.conf`.
 *
 * Migrated from the old MasternodeConfigDialog by m-masternodes-actions-
 * restructure (2026-06-11) — this dialog now ONLY renders the edit form.
 * Add is covered by MasternodeSetupWizard; Delete is covered by the per-row
 * Trash IconButton in MasternodesTable; Reload was dropped (rare operation,
 * backend re-reads on GUI mutations).
 */
export interface MasternodeEditEntry {
  alias: string;
  ip: string;
  txHash: string;
  outputIndex: number;
}

interface MasternodeEditDialogProps {
  isOpen: boolean;
  onClose: () => void;
  /**
   * When `isOpen` is true the dialog opens directly in edit mode with the form
   * pre-populated from this row data. Once `configEntries` loads, if a real
   * entry exists in masternode.conf with the matching alias, `privateKey` is
   * hot-merged. For dummy / unknown aliases the form still opens — `privateKey`
   * stays empty and saving will surface a backend "entry not found" error.
   * Migrated from the old MasternodeConfigDialog's `initialEntry` prop.
   */
  initialEntry: MasternodeEditEntry | null;
}

// ---------------------------------------------------------------------------
// Receive design tokens. See the canonical reference at
// `cmd/twins-gui/frontend/src/features/wallet/pages/Receive.tsx` and the
// "Design Tokens" section of `cmd/twins-gui/frontend/CLAUDE.md`. The previous
// legacy Qt chrome (#2b2b2b / 1px #555 / 2px radius) was migrated by
// m-redesign-masternode-edit-dialog (2026-06-11).
// ---------------------------------------------------------------------------
const cardChromeStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
};

const inputBaseStyle: React.CSSProperties = {
  width: '100%',
  backgroundColor: '#252525',
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  padding: '7px 10px',
  fontSize: '12px',
  color: '#ddd',
  outline: 'none',
};

const labelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: '12px',
  color: '#ddd',
  marginBottom: '4px',
};

const requiredMarkStyle: React.CSSProperties = {
  color: '#ff6666',
};

const errorTextStyle: React.CSSProperties = {
  marginTop: '4px',
  fontSize: '11px',
  color: '#ff6666',
};

const hintTextStyle: React.CSSProperties = {
  marginTop: '4px',
  fontSize: '11px',
  color: '#888',
};

const pendingHintTextStyle: React.CSSProperties = {
  marginTop: '4px',
  fontSize: '11px',
  color: '#ffaa00',
};

const statusIndicatorStyle: React.CSSProperties = {
  position: 'absolute',
  right: '8px',
  top: '50%',
  transform: 'translateY(-50%)',
  pointerEvents: 'none',
  fontSize: '14px',
  fontWeight: 'bold',
};

// Pad the right edge of inputs that render an inline validation indicator so
// the user's text never collides with the ✓/✗ glyph.
const inputWithIndicatorPaddingRight = '28px';

export const MasternodeEditDialog: React.FC<MasternodeEditDialogProps> = ({
  isOpen,
  onClose,
  initialEntry,
}) => {
  const { t } = useTranslation('masternode');
  const blockchainInfo = useStore((state) => state.blockchainInfo);

  // Backing config entries (used to hot-merge privateKey for the editing alias)
  const [configEntries, setConfigEntries] = useState<MasternodeConfigEntry[]>([]);
  const [availableOutputs, setAvailableOutputs] = useState<MasternodeOutput[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  // Form state
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

  // NEW (m-redesign-masternode-edit-dialog 2026-06-11):
  //   showPrivateKey   — masks the private key by default; toggled via Eye/EyeOff IconButton.
  //   copiedField      — drives the 2-second green Check icon swap on the Copy IconButton.
  //   pendingChangeKey — opens the destructive SimpleConfirmDialog when the user
  //                      attempts to save a changed private key in edit mode.
  const [showPrivateKey, setShowPrivateKey] = useState(false);
  const [copiedField, setCopiedField] = useState<'privateKey' | null>(null);
  const [pendingChangeKey, setPendingChangeKey] = useState(false);

  // Mounted ref to prevent state updates after unmount
  const mountedRef = useRef(true);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // One-shot guard for the initialEntry auto-populate effect.
  // Reset to false whenever the dialog closes so the next open cycle can
  // re-apply.
  const autoEditAppliedRef = useRef(false);

  // NEW (m-redesign-masternode-edit-dialog 2026-06-11):
  //   originalPrivateKeyRef — captures the real masternode.conf privateKey at
  //     Phase 2 hot-merge time. Drives the "Change Key?" confirmation gate so
  //     the user is only prompted when the key actually changes (not on every
  //     edit save). Stays empty for dummy/unknown aliases — confirmation
  //     never fires for them, save fails at the backend with "entry not found"
  //     instead.
  //   copyTimerRef — 2-second auto-revert timer for the copy success Check icon.
  const originalPrivateKeyRef = useRef<string>('');
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
      if (copyTimerRef.current) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
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

  // Fetch config entries + available outputs when dialog opens
  const fetchConfig = useCallback(async () => {
    if (!mountedRef.current) return;
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
    }
  }, [t]);

  // Reset all transient UI state on dialog close. Extended by
  // m-redesign-masternode-edit-dialog (2026-06-11) to also clear:
  //   showPrivateKey   — next open starts masked again
  //   copiedField      — kills any in-flight Check icon swap
  //   pendingChangeKey — closes a stuck Change Key confirmation overlay
  //   originalPrivateKeyRef — next Phase 2 hot-merge re-captures fresh
  //   copyTimerRef     — invalidates any pending 2s revert
  useEffect(() => {
    if (isOpen) {
      fetchConfig();
      setError(null);
      setSuccessMessage(null);
      return;
    }
    setShowPrivateKey(false);
    setCopiedField(null);
    setPendingChangeKey(false);
    originalPrivateKeyRef.current = '';
    if (copyTimerRef.current) {
      clearTimeout(copyTimerRef.current);
      copyTimerRef.current = null;
    }
  }, [isOpen, fetchConfig]);

  // Phase 1 auto-populate: when isOpen flips with initialEntry, seed the form
  // from the row data. privateKey defaults to empty until Phase 2 merges it
  // from configEntries (if a real entry with matching alias exists).
  //
  // The autoEditAppliedRef guard ensures Phase 1 fires at most once per dialog
  // open cycle — resets on isOpen=false so the next reopen re-applies.
  useEffect(() => {
    if (!isOpen) {
      autoEditAppliedRef.current = false;
      return;
    }
    if (autoEditAppliedRef.current) return;
    if (!initialEntry) return;
    setFormData({
      alias: initialEntry.alias,
      ip: initialEntry.ip,
      privateKey: '',
      txHash: initialEntry.txHash,
      outputIndex: initialEntry.outputIndex,
    });
    setEditingAlias(initialEntry.alias);
    setFormErrors({});
    autoEditAppliedRef.current = true;
  }, [isOpen, initialEntry]);

  // Phase 2 — merge privateKey from masternode.conf into the form once
  // configEntries loads, IF a real entry with the matching alias exists. For
  // dummies / unknown aliases this is a no-op (privateKey stays empty; save
  // will fail with backend "entry not found").
  useEffect(() => {
    if (!isOpen) return;
    if (!initialEntry) return;
    if (configEntries.length === 0) return;
    const real = configEntries.find((e) => e.alias === initialEntry.alias);
    if (!real) return;
    // Stomp guard — bail if the user has already edited ANY field that Phase 1
    // populated (alias, ip, txHash, outputIndex) against initialEntry, OR if
    // privateKey is non-empty (only happens after user typing or a previous
    // Phase 2 merge — in both cases, don't stomp).
    setFormData((prev) => {
      if (
        prev.alias !== initialEntry.alias ||
        prev.ip !== initialEntry.ip ||
        prev.txHash !== initialEntry.txHash ||
        prev.outputIndex !== initialEntry.outputIndex ||
        prev.privateKey !== ''
      ) {
        return prev;
      }
      // NEW (m-redesign-masternode-edit-dialog 2026-06-11): snapshot the real
      // privateKey into a ref so handleSaveClick can detect when the user has
      // changed it. originalPrivateKeyRef stays empty for dummy/unknown aliases
      // (the `real` lookup short-circuits above), so the confirmation gate
      // remains gated correctly for those.
      originalPrivateKeyRef.current = real.privateKey;
      return {
        alias: real.alias,
        ip: real.ip,
        privateKey: real.privateKey,
        txHash: real.txHash,
        outputIndex: real.outputIndex,
      };
    });
  }, [isOpen, initialEntry, configEntries]);

  // Filter available outputs to exclude ones already used by other entries.
  // The current editing entry's output stays selectable so the user can leave
  // collateral unchanged.
  const unusedOutputs = useMemo(() => {
    const usedSet = new Set(
      configEntries.map((entry) => `${entry.txHash}:${entry.outputIndex}`)
    );
    if (editingAlias) {
      const current = configEntries.find((e) => e.alias === editingAlias);
      if (current) {
        usedSet.delete(`${current.txHash}:${current.outputIndex}`);
      }
    }
    return availableOutputs.filter(
      (output) => !usedSet.has(`${output.txHash}:${output.outputIndex}`)
    );
  }, [availableOutputs, configEntries, editingAlias]);

  const unusedReady = useMemo(() => unusedOutputs.filter((o) => o.isReady), [unusedOutputs]);
  const unusedPending = useMemo(() => unusedOutputs.filter((o) => !o.isReady), [unusedOutputs]);
  const hasPendingUnused = unusedPending.length > 0;

  // Poll for pending collateral confirmations while the dialog is open and
  // at least one unused UTXO is still waiting on confirmations. The user may
  // want to switch collateral to a UTXO that becomes ready mid-edit.
  useEffect(() => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
    if (isOpen && hasPendingUnused) {
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
  }, [isOpen, hasPendingUnused]);

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

  // NEW (m-redesign-masternode-edit-dialog 2026-06-11):
  // Copy the current private key to the clipboard. Uses the shared
  // writeToClipboard helper which handles the restricted-context fallback
  // (textarea + execCommand). Gates the 2-second Check icon swap on the
  // helper's boolean return so failed copies do NOT show a misleading success
  // indicator. Cancels any in-flight prior timer to handle rapid re-copies.
  const handleCopyPrivateKey = useCallback(async () => {
    if (!formData.privateKey) return;
    if (copyTimerRef.current) {
      clearTimeout(copyTimerRef.current);
      copyTimerRef.current = null;
    }
    const ok = await writeToClipboard(formData.privateKey);
    if (!ok) return;
    if (!mountedRef.current) return;
    setCopiedField('privateKey');
    copyTimerRef.current = setTimeout(() => {
      if (mountedRef.current) {
        setCopiedField(null);
      }
      copyTimerRef.current = null;
    }, 2000);
  }, [formData.privateKey]);

  const handleTogglePrivateKey = useCallback(() => {
    setShowPrivateKey((v) => !v);
  }, []);

  // Network-aware IP:Port hint. BlockchainInfo.chain is "main" / "test" /
  // "regtest" per `internal/gui/core/types.go`. Fallback to mainnet hint on
  // unknown or null chain so the user always sees a usable port reference.
  const getPortHint = useCallback((): string => {
    const chain = blockchainInfo?.chain;
    if (chain === 'test') return t('config.form.testnetPort');
    if (chain === 'regtest') return t('config.form.regtestPort');
    return t('config.form.ipHint');
  }, [blockchainInfo?.chain, t]);

  // Validate form. Memoized so handleSaveClick's deps array doesn't need to
  // either eslint-disable the missing reference or silently re-create the
  // callback per render. See code-review W1 (m-redesign-masternode-edit-dialog
  // 2026-06-11).
  const validateForm = useCallback((): boolean => {
    const errors: Record<string, string> = {};

    // Alias validation
    if (!formData.alias.trim()) {
      errors.alias = t('config.validation.aliasRequired');
    } else if (!/^[a-zA-Z0-9_]+$/.test(formData.alias)) {
      errors.alias = t('config.validation.aliasFormat');
    } else if (
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
  }, [formData, editingAlias, configEntries, t]);

  // NEW (m-redesign-masternode-edit-dialog 2026-06-11): private worker that
  // performs the actual UpdateMasternodeConfig call. Lifted verbatim from the
  // prior handleSubmit body so handleSaveClick can intercept the Save flow,
  // open the Change Key confirmation when needed, and delegate the actual
  // update to executeUpdate on confirm.
  const executeUpdate = useCallback(async () => {
    if (!editingAlias) {
      setError(sanitizeErrorMessage('No alias selected for editing'));
      return;
    }
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
      await UpdateMasternodeConfig(editingAlias, entry);
      if (mountedRef.current) {
        setSuccessMessage(t('config.updateSuccess', { alias: entry.alias }));
        // Close after a brief success-message linger so the user sees the
        // confirmation. The parent's onClose fetchMasternodes() will refresh
        // the row data.
        setTimeout(() => {
          if (mountedRef.current) {
            onClose();
            resetForm();
          }
        }, 600);
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
  }, [editingAlias, formData, onClose, resetForm, t]);

  // NEW (m-redesign-masternode-edit-dialog 2026-06-11): Save click handler
  // that gates the destructive private-key change behind a confirmation
  // dialog. The three-condition gate:
  //   (a) editingAlias non-empty (edit mode)
  //   (b) originalPrivateKeyRef.current non-empty (real entry was loaded;
  //       dummy aliases get straight-through behavior so save fails at the
  //       backend with the legacy "entry not found" rather than blocking on
  //       an irrelevant confirmation)
  //   (c) formData.privateKey.trim() !== originalPrivateKeyRef.current.trim()
  //       (the key actually changed — typing then reverting to the same value
  //       does not prompt)
  const handleSaveClick = useCallback(() => {
    if (!validateForm()) return;
    if (
      editingAlias &&
      originalPrivateKeyRef.current !== '' &&
      formData.privateKey.trim() !== originalPrivateKeyRef.current.trim()
    ) {
      setPendingChangeKey(true);
      return;
    }
    void executeUpdate();
  }, [editingAlias, formData, executeUpdate, validateForm]);

  // Ref so the keyboard handler always calls the latest handleSaveClick
  const handleSaveClickRef = useRef(handleSaveClick);
  handleSaveClickRef.current = handleSaveClick;

  // Handle cancel form — close the dialog directly (no list-mode fallback)
  const handleCancel = useCallback(() => {
    onClose();
    resetForm();
    setError(null);
  }, [onClose, resetForm]);

  // Handle collateral selection
  const handleCollateralChange = useCallback((value: string) => {
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
  }, []);

  // -------------------------------------------------------------------------
  // Inline validation status — drives the ✓ / ✗ indicator in each input's
  // right padding plus the dynamic border color. Mirrors the canonical
  // RecipientField pattern (`RecipientField.tsx:75-160`).
  // -------------------------------------------------------------------------
  const aliasOk = useMemo(() => {
    if (!formData.alias.trim()) return false;
    if (!/^[a-zA-Z0-9_]+$/.test(formData.alias)) return false;
    if (formData.alias !== editingAlias && configEntries.some((e) => e.alias === formData.alias)) {
      return false;
    }
    return true;
  }, [formData.alias, editingAlias, configEntries]);

  const ipOk = useMemo(() => {
    if (!formData.ip.includes(':')) return false;
    const [ip, port] = formData.ip.split(':');
    if (!ip || !port) return false;
    const portNum = parseInt(port, 10);
    return !isNaN(portNum) && portNum >= 1 && portNum <= 65535;
  }, [formData.ip]);

  const privateKeyOk = formData.privateKey.trim() !== '';
  const collateralOk = formData.txHash !== '';

  // NEW (m-masternode-edit-dialog-layout-polish 2026-06-11):
  // All 4 input fields render a neutral grey border regardless of validation
  // state. The colored borders (#27ae60 valid / #ff6666 invalid) added visual
  // noise without conveying actionable information that the ✓/✗ indicator on
  // the right edge and the error text under the input don't already convey.
  // The renderIndicator() helper below is preserved verbatim — it remains the
  // canonical validation-state signal.
  const NEUTRAL_BORDER_COLOR = '#3a3a3a';

  const renderIndicator = (hasError: boolean, ok: boolean): React.ReactNode => {
    if (hasError) {
      return (
        <span style={{ ...statusIndicatorStyle, color: '#ff6666' }} aria-hidden="true">
          ✗
        </span>
      );
    }
    if (ok) {
      return (
        <span style={{ ...statusIndicatorStyle, color: '#27ae60' }} aria-hidden="true">
          ✓
        </span>
      );
    }
    return null;
  };

  // -------------------------------------------------------------------------
  // Collateral UTXO options for the shared RowsPerPageSelect<string>.
  //
  // RowsPerPageSelect renders each option AS the label (it has no separate
  // value/label slot — see `RowsPerPageSelect.tsx:208-237`). To satisfy the
  // SC's option-label format while keeping the value resolvable, we use the
  // rendered label as the opaque option string AND maintain a reverse
  // `Map<label, {txHash, outputIndex}>` for lookup on change. Collisions are
  // structurally impossible at realistic wallet UTXO counts because every
  // label embeds `${txHash.substring(0, 8)}…:${outputIndex}` (a per-UTXO
  // unique fragment).
  //
  // The currently-selected UTXO is prepended as a synthetic "Current: …"
  // option if it is absent from the unused lists (e.g. it was claimed by
  // another entry mid-edit). Without this the select would visually drop the
  // user's current selection on render.
  // -------------------------------------------------------------------------
  const collateralOptions = useMemo(() => {
    const labels: string[] = [];
    const labelMap = new Map<string, { txHash: string; outputIndex: number }>();

    // NEW (m-masternode-edit-dialog-layout-polish 2026-06-11):
    // Confirmations count dropped from both labels so the option fits in the
    // ~368px Collateral column post the IP+Collateral 2-col grid (math: dialog
    // 800px - px-6 outer padding 48px = 752px content - 16px gap = 736px / 2 =
    // 368px per column). The ` — not ready` suffix on pending is preserved as
    // the load-bearing semantic signal that the UTXO cannot yet be used as
    // collateral regardless of how many confirmations remain. Ready UTXOs need
    // no status suffix because their presence in the list already conveys
    // selectability.
    //
    // Trade-off accepted: pending labels at worst-case lengths (e.g. `Platinum ·
    // 100,000,000 TWINS · 12345678…:N — not ready` ~409px content) exceed the
    // 368px trigger column. The trigger may visually ellipsis-truncate the
    // selected-pending case (rare workflow — users typically select READY
    // UTXOs). The popover (`shared/components/RowsPerPageSelect.tsx:198`) sets
    // `minWidth: '100%'` with no maxWidth and no overflow:hidden on options, so
    // the popover auto-grows to fit content — the ` — not ready` suffix is
    // always visible during selection. Trigger truncation only affects the
    // already-selected pending state, which is correctly blocked at save time
    // by the backend `entry not found` check.
    const formatReady = (o: MasternodeOutput): string =>
      `${o.tier} · ${o.amount.toLocaleString()} TWINS · ${o.txHash.substring(0, 8)}…:${o.outputIndex}`;

    const formatPending = (o: MasternodeOutput): string =>
      `${o.tier} · ${o.amount.toLocaleString()} TWINS · ${o.txHash.substring(0, 8)}…:${o.outputIndex} — not ready`;

    // Synthetic option for the currently-selected UTXO if not in unused lists.
    if (formData.txHash) {
      const inUnused = unusedOutputs.some(
        (o) => o.txHash === formData.txHash && o.outputIndex === formData.outputIndex
      );
      if (!inUnused) {
        const syntheticLabel = `${t('config.form.currentCollateral')}: ${formData.txHash.substring(0, 8)}…:${formData.outputIndex}`;
        labels.push(syntheticLabel);
        labelMap.set(syntheticLabel, { txHash: formData.txHash, outputIndex: formData.outputIndex });
      }
    }

    unusedReady.forEach((o) => {
      const label = formatReady(o);
      labels.push(label);
      labelMap.set(label, { txHash: o.txHash, outputIndex: o.outputIndex });
    });

    unusedPending.forEach((o) => {
      const label = formatPending(o);
      labels.push(label);
      labelMap.set(label, { txHash: o.txHash, outputIndex: o.outputIndex });
    });

    return { labels, labelMap };
  }, [formData.txHash, formData.outputIndex, unusedOutputs, unusedReady, unusedPending, t]);

  // Currently selected label (the option string to display in the trigger).
  // Falls back to the empty string when nothing is selected — RowsPerPageSelect
  // renders the empty string in the trigger which is acceptable for an empty
  // initial state. The user has to actively pick a UTXO to satisfy validation.
  const currentCollateralLabel = useMemo(() => {
    if (!formData.txHash) return '';
    for (const [label, value] of collateralOptions.labelMap.entries()) {
      if (value.txHash === formData.txHash && value.outputIndex === formData.outputIndex) {
        return label;
      }
    }
    return '';
  }, [collateralOptions, formData.txHash, formData.outputIndex]);

  const handleSelectCollateralLabel = useCallback(
    (label: string) => {
      const entry = collateralOptions.labelMap.get(label);
      if (!entry) {
        // The placeholder "no collateral available" pseudo-option falls here;
        // don't mutate form state.
        return;
      }
      handleCollateralChange(`${entry.txHash}:${entry.outputIndex}`);
    },
    [collateralOptions.labelMap, handleCollateralChange]
  );

  // Handle keyboard events
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept Escape/Enter while the destructive confirmation
      // dialog is open — its own keyboard handler owns those keys for that
      // overlay layer.
      if (pendingChangeKey) return;
      if (e.key === 'Escape') {
        handleCancel();
      } else if (e.key === 'Enter' && !isSubmitting) {
        handleSaveClickRef.current();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, isSubmitting, handleCancel, pendingChangeKey]);

  if (!isOpen) return null;

  const aliasHasError = !!formErrors.alias;
  const ipHasError = !!formErrors.ip;
  const privateKeyHasError = !!formErrors.privateKey;
  const collateralHasError = !!formErrors.collateral;

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          handleCancel();
        }
      }}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="masternode-edit-title"
        className="w-[800px] max-h-[600px] flex flex-col shadow-xl"
        style={cardChromeStyle}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between px-6 py-4"
          style={{ borderBottom: '1px solid #3a3a3a' }}
        >
          <h2 id="masternode-edit-title" className="text-lg font-semibold text-[#ddd]">
            {t('config.editTitle')}
          </h2>
          <IconButton
            onClick={handleCancel}
            title={t('common:buttons.close', { defaultValue: 'Close' })}
            ariaLabel={t('common:buttons.close', { defaultValue: 'Close' })}
            icon={<X size={14} />}
          />
        </div>

        {/* Error Display */}
        {error && (
          <div className="px-6 py-2">
            <Banner variant="error" message={error} />
          </div>
        )}

        {/* Success Display */}
        {successMessage && (
          <div
            className="px-6 py-2"
            style={{
              backgroundColor: 'rgba(39, 174, 96, 0.12)',
              borderBottom: '1px solid #27ae60',
            }}
          >
            <p className="text-sm" style={{ color: '#27ae60' }}>
              {successMessage}
            </p>
          </div>
        )}

        {/* Edit Form */}
        <div className="flex-1 overflow-auto px-6 py-4">
          <div className="space-y-4">
            {/* Alias Field */}
            <div>
              <label style={labelStyle}>
                {t('config.form.alias')} <span style={requiredMarkStyle}>*</span>
              </label>
              <div style={{ position: 'relative' }}>
                <input
                  type="text"
                  value={formData.alias}
                  onChange={(e) => {
                    setFormData((prev) => ({ ...prev, alias: e.target.value }));
                    setFormErrors((prev) => ({ ...prev, alias: '' }));
                  }}
                  placeholder={t('config.form.aliasPlaceholder')}
                  maxLength={50}
                  style={{
                    ...inputBaseStyle,
                    paddingRight: inputWithIndicatorPaddingRight,
                    borderColor: NEUTRAL_BORDER_COLOR,
                  }}
                />
                {renderIndicator(aliasHasError, aliasOk)}
              </div>
              {formErrors.alias && <p style={errorTextStyle}>{formErrors.alias}</p>}
            </div>

            {/* NEW (m-masternode-edit-dialog-layout-polish 2026-06-11):
                IP + Collateral share a row in a 2-col grid. Both fields hold
                single-line content; pairing them semantically as "where"
                (IP/server) + "what" (UTXO/collateral). Private Key stays
                full-width below because of its 3 trailing IconButtons +
                Generate button which need the full row width. `alignItems:
                'start'` keeps cells top-aligned so the asymmetric hint text
                heights (Mainnet port hint vs Collateral tier hint) do not
                shift cell content. */}
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 1fr',
                gap: '16px',
                alignItems: 'start',
              }}
            >
              {/* IP:Port Field */}
              <div>
                <label style={labelStyle}>
                  {t('config.form.ip')} <span style={requiredMarkStyle}>*</span>
                </label>
                <div style={{ position: 'relative' }}>
                  <input
                    type="text"
                    value={formData.ip}
                    onChange={(e) => {
                      setFormData((prev) => ({ ...prev, ip: e.target.value }));
                      setFormErrors((prev) => ({ ...prev, ip: '' }));
                    }}
                    placeholder={t('config.form.ipPlaceholder')}
                    style={{
                      ...inputBaseStyle,
                      paddingRight: inputWithIndicatorPaddingRight,
                      borderColor: NEUTRAL_BORDER_COLOR,
                    }}
                  />
                  {renderIndicator(ipHasError, ipOk)}
                </div>
                {formErrors.ip && <p style={errorTextStyle}>{formErrors.ip}</p>}
                <p style={hintTextStyle}>{getPortHint()}</p>
              </div>

              {/* Collateral UTXO Field */}
              <div>
                <label style={labelStyle}>
                  {t('config.form.collateral')} <span style={requiredMarkStyle}>*</span>
                </label>
                <div
                  style={{
                    position: 'relative',
                    display: 'flex',
                    alignItems: 'center',
                    border: `1px solid ${NEUTRAL_BORDER_COLOR}`,
                    borderRadius: '4px',
                    backgroundColor: '#252525',
                    padding: '0',
                    width: '100%',
                  }}
                >
                  <div style={{ flex: 1, padding: '0' }}>
                    {collateralOptions.labels.length === 0 ? (
                      // Static disabled placeholder — when no UTXOs are available
                      // we render a read-only div instead of forcing a no-op
                      // through RowsPerPageSelect. The user sees an honest
                      // disabled state, screen readers announce the placeholder
                      // as static text (not a fake selection), and validation
                      // still correctly blocks Save because `formData.txHash`
                      // stays empty. Closes code-review W2.
                      <div
                        role="textbox"
                        aria-readonly="true"
                        aria-label={t('config.form.collateral')}
                        style={{
                          width: '100%',
                          padding: '7px 28px 7px 10px',
                          fontSize: '12px',
                          color: '#666',
                          cursor: 'not-allowed',
                          userSelect: 'none',
                        }}
                      >
                        {t('config.form.noCollateralAvailable')}
                      </div>
                    ) : (
                      <RowsPerPageSelect<string>
                        value={currentCollateralLabel || collateralOptions.labels[0]}
                        options={collateralOptions.labels}
                        onChange={handleSelectCollateralLabel}
                        ariaLabel={t('config.form.collateral')}
                        align="left"
                        triggerStyle={{
                          width: '100%',
                          minWidth: '100%',
                          padding: '7px 28px 7px 10px',
                          backgroundColor: 'transparent',
                          border: 'none',
                          borderRadius: '4px',
                          fontSize: '12px',
                          color: '#ddd',
                        }}
                      />
                    )}
                  </div>
                  {renderIndicator(collateralHasError, collateralOk)}
                </div>
                {formErrors.collateral && (
                  <p style={errorTextStyle}>{formErrors.collateral}</p>
                )}
                {hasPendingUnused && (
                  <p style={pendingHintTextStyle}>{t('config.form.pendingCollateralHint')}</p>
                )}
                <p style={hintTextStyle}>{t('config.form.collateralHint')}</p>
              </div>
            </div>

            {/* Private Key Field */}
            {/* NEW (m-masternode-edit-dialog-layout-polish 2026-06-11):
                alignItems: 'center' so Show/Copy IconButtons sit on the input
                vertical centerline rather than stretching across label+input+
                indicator block (was 'stretch'). The Generate button is taller
                (~36px from px-4 py-2) than the 24x24 IconButtons; centering
                aligns all three around the input's center axis. */}
            <div>
              <label style={labelStyle}>
                {t('config.form.privateKey')} <span style={requiredMarkStyle}>*</span>
              </label>
              <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                <div style={{ position: 'relative', flex: 1 }}>
                  <input
                    type={showPrivateKey ? 'text' : 'password'}
                    value={formData.privateKey}
                    onChange={(e) => {
                      setFormData((prev) => ({ ...prev, privateKey: e.target.value }));
                      setFormErrors((prev) => ({ ...prev, privateKey: '' }));
                    }}
                    placeholder={t('config.form.privateKeyPlaceholder')}
                    autoComplete="new-password"
                    style={{
                      ...inputBaseStyle,
                      paddingRight: inputWithIndicatorPaddingRight,
                      fontFamily: 'monospace',
                      borderColor: NEUTRAL_BORDER_COLOR,
                    }}
                  />
                  {renderIndicator(privateKeyHasError, privateKeyOk)}
                </div>
                <IconButton
                  onClick={handleTogglePrivateKey}
                  title={
                    showPrivateKey
                      ? t('config.form.privateKeyHide')
                      : t('config.form.privateKeyShow')
                  }
                  ariaLabel={
                    showPrivateKey
                      ? t('config.form.privateKeyHide')
                      : t('config.form.privateKeyShow')
                  }
                  icon={showPrivateKey ? <EyeOff size={14} /> : <Eye size={14} />}
                />
                <IconButton
                  onClick={handleCopyPrivateKey}
                  disabled={!formData.privateKey}
                  title={t('config.form.privateKeyCopy')}
                  ariaLabel={t('config.form.privateKeyCopy')}
                  icon={
                    copiedField === 'privateKey' ? (
                      <Check size={14} color="#27ae60" />
                    ) : (
                      <Copy size={14} />
                    )
                  }
                />
                <button
                  type="button"
                  onClick={handleGenerateKey}
                  disabled={isGeneratingKey}
                  className="px-4 py-2 text-sm bg-[#4a7c59] border border-[#5a8c69] text-white font-medium rounded-md hover:bg-[#5a8c69] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1 flex-shrink-0"
                >
                  <Key size={14} className={isGeneratingKey ? 'animate-pulse' : ''} />
                  {t('config.form.generateKey')}
                </button>
              </div>
              {formErrors.privateKey && <p style={errorTextStyle}>{formErrors.privateKey}</p>}
              {copiedField === 'privateKey' && (
                <p style={{ ...hintTextStyle, color: '#27ae60' }}>
                  {t('config.form.privateKeyCopied')}
                </p>
              )}
              <p style={hintTextStyle}>{t('config.form.privateKeyHint')}</p>
            </div>
          </div>

          {/* Form Action Buttons */}
          <div
            className="flex items-center justify-end gap-2 mt-6 pt-4"
            style={{ borderTop: '1px solid #3a3a3a' }}
          >
            <button
              type="button"
              onClick={handleCancel}
              disabled={isSubmitting}
              style={{
                padding: '8px 16px',
                fontSize: '12px',
                backgroundColor: '#383838',
                border: '1px solid #4a4a4a',
                borderRadius: '6px',
                color: '#ccc',
                cursor: isSubmitting ? 'not-allowed' : 'pointer',
                opacity: isSubmitting ? 0.5 : 1,
                transition: 'background-color 0.15s',
              }}
            >
              {t('config.buttons.cancel')}
            </button>
            <button
              type="button"
              onClick={handleSaveClick}
              disabled={isSubmitting}
              style={{
                padding: '8px 16px',
                fontSize: '12px',
                fontWeight: 500,
                backgroundColor: '#4a7c59',
                border: '1px solid #5a8c69',
                borderRadius: '6px',
                color: '#fff',
                cursor: isSubmitting ? 'not-allowed' : 'pointer',
                opacity: isSubmitting ? 0.7 : 1,
                transition: 'background-color 0.15s',
              }}
            >
              {isSubmitting ? t('config.buttons.saving') : t('config.buttons.save')}
            </button>
          </div>
        </div>
      </div>

      {/* Change Key confirmation dialog. zIndex 60 sits above the parent
          dialog (z-50 from the outer overlay). Mirrors the
          m-restyle-masternode-config-dialog (2026-06-10) precedent. */}
      <SimpleConfirmDialog
        isOpen={pendingChangeKey}
        isDestructive
        zIndex={60}
        title={t('config.changeKeyConfirm.title')}
        message={t('config.changeKeyConfirm.message')}
        confirmText={t('config.changeKeyConfirm.confirm')}
        cancelText={t('config.buttons.cancel')}
        onConfirm={() => {
          setPendingChangeKey(false);
          void executeUpdate();
        }}
        onCancel={() => setPendingChangeKey(false)}
      />
    </div>
  );
};
