import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { X, ChevronLeft, ChevronRight, Check, Server, Clock, Wallet, RefreshCw, Key, Globe, Tag, CheckCircle, AlertCircle } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { sanitizeErrorMessage } from '@/shared/utils/sanitize';
import {
  GetMasternodeConfig,
  AddMasternodeConfig,
  GenerateMasternodeKey,
  GetMasternodeOutputs,
  StartMasternode,
} from '@wailsjs/go/main/App';

// Types matching backend structures
interface MasternodeOutput {
  txHash: string;
  outputIndex: number;
  amount: number;
  tier: string;
  confirmations: number;
  isReady: boolean;
}

interface MasternodeConfigEntry {
  alias: string;
  ip: string;
  privateKey: string;
  txHash: string;
  outputIndex: number;
}

interface MasternodeSetupWizardProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: () => void;
}

// Tier definitions
const TIERS = [
  { id: 'bronze', name: 'Bronze', collateral: 1000000, weight: 1 },
  { id: 'silver', name: 'Silver', collateral: 5000000, weight: 5 },
  { id: 'gold', name: 'Gold', collateral: 20000000, weight: 20 },
  { id: 'platinum', name: 'Platinum', collateral: 100000000, weight: 100 },
] as const;

type TierType = typeof TIERS[number]['id'];

// Wizard state
interface WizardState {
  step: number;
  tier: TierType | null;
  collateral: MasternodeOutput | null;
  privateKey: string;
  ipAddress: string;
  port: number;
  alias: string;
}

const TOTAL_STEPS = 7;
const DEFAULT_PORT = 37817;

export const MasternodeSetupWizard: React.FC<MasternodeSetupWizardProps> = ({
  isOpen,
  onClose,
  onSuccess,
}) => {
  const { t } = useTranslation('masternode');
  const mountedRef = useRef(true);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Wizard state
  const [state, setState] = useState<WizardState>({
    step: 1,
    tier: null,
    collateral: null,
    privateKey: '',
    ipAddress: '',
    port: DEFAULT_PORT,
    alias: '',
  });

  // Data state
  const [availableOutputs, setAvailableOutputs] = useState<MasternodeOutput[]>([]);
  const [existingEntries, setExistingEntries] = useState<MasternodeConfigEntry[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [isGeneratingKey, setIsGeneratingKey] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createSuccess, setCreateSuccess] = useState(false);
  const [keyMode, setKeyMode] = useState<'generate' | 'existing'>('generate');

  // Validation errors
  const [validationErrors, setValidationErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (pollTimerRef.current) clearInterval(pollTimerRef.current);
    };
  }, []);

  // Fetch data when dialog opens
  const fetchData = useCallback(async () => {
    if (!mountedRef.current) return;
    setIsLoading(true);
    setError(null);

    try {
      const [outputs, config] = await Promise.all([
        GetMasternodeOutputs(),
        GetMasternodeConfig(),
      ]);
      if (mountedRef.current) {
        setAvailableOutputs(outputs || []);
        setExistingEntries(config || []);
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(t('config.fetchError'));
        console.error('Failed to fetch data:', err);
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
      }
    }
  }, [t]);

  useEffect(() => {
    if (isOpen) {
      fetchData();
      // Reset state when opening
      setState({
        step: 1,
        tier: null,
        collateral: null,
        privateKey: '',
        ipAddress: '',
        port: DEFAULT_PORT,
        alias: '',
      });
      setValidationErrors({});
      setCreateSuccess(false);
      setKeyMode('generate');
    }
  }, [isOpen, fetchData]);

  // Exclude UTXOs already assigned to existing masternode config entries
  const unusedOutputs = useMemo(() => {
    const usedUTXOSet = new Set(
      existingEntries.map(e => `${e.txHash}:${e.outputIndex}`)
    );
    return availableOutputs.filter(
      o => !usedUTXOSet.has(`${o.txHash}:${o.outputIndex}`)
    );
  }, [availableOutputs, existingEntries]);

  // Poll for confirmation updates when unused pending UTXOs exist
  const hasPendingOutputs = useMemo(() => unusedOutputs.some(o => !o.isReady), [unusedOutputs]);
  useEffect(() => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
    if (isOpen && hasPendingOutputs) {
      pollTimerRef.current = setInterval(async () => {
        if (!mountedRef.current) return;
        try {
          const outputs = await GetMasternodeOutputs();
          if (mountedRef.current) setAvailableOutputs(outputs || []);
        } catch {
          // Silently ignore poll errors
        }
      }, 30000);
    }
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [isOpen, hasPendingOutputs]);

  // Get tier collateral amount
  const getTierCollateral = (tierId: TierType): number => {
    const tier = TIERS.find(t => t.id === tierId);
    return tier?.collateral || 0;
  };

  // Filter outputs by selected tier — split into ready and pending
  const readyOutputs = state.tier
    ? unusedOutputs.filter(o => o.tier.toLowerCase() === state.tier && o.isReady)
    : [];
  const pendingOutputs = state.tier
    ? unusedOutputs.filter(o => o.tier.toLowerCase() === state.tier && !o.isReady)
    : [];

  // Check if tier is available (has at least one ready UTXO)
  const isTierAvailable = (tierId: TierType): boolean => {
    return unusedOutputs.some(o => o.tier.toLowerCase() === tierId && o.isReady);
  };

  // Check if tier has pending-only UTXOs (no ready ones)
  const isTierPending = (tierId: TierType): boolean => {
    return !isTierAvailable(tierId) &&
      unusedOutputs.some(o => o.tier.toLowerCase() === tierId && !o.isReady);
  };

  // Validate current step - wrapped in useCallback for handleNext dependency
  const validateStep = useCallback((): boolean => {
    const errors: Record<string, string> = {};

    switch (state.step) {
      case 2: // Tier selection
        if (!state.tier) {
          errors.tier = 'Please select a tier';
        }
        break;

      case 3: // Collateral selection
        if (!state.collateral) {
          errors.collateral = t('config.validation.collateralRequired');
        }
        break;

      case 4: // Private key
        if (!state.privateKey.trim()) {
          errors.privateKey = t('config.validation.privateKeyRequired');
        }
        break;

      case 5: // Network config
        if (!state.ipAddress.trim()) {
          errors.ip = t('wizard.step5.validation.ipRequired');
        } else {
          // IP validation with octet range check (0-255) - matches backend net.ParseIP behavior
          const ipRegex = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;
          const match = state.ipAddress.match(ipRegex);
          if (!match) {
            errors.ip = t('wizard.step5.validation.ipInvalid');
          } else {
            // Check each octet is in valid range (0-255)
            const octets = [match[1], match[2], match[3], match[4]].map(Number);
            const isValidOctets = octets.every(o => o >= 0 && o <= 255);
            if (!isValidOctets) {
              errors.ip = t('wizard.step5.validation.ipInvalid');
            }
          }
        }
        if (!state.port || state.port < 1 || state.port > 65535) {
          errors.port = t('wizard.step5.validation.portInvalid');
        }
        break;

      case 6: // Alias
        if (!state.alias.trim()) {
          errors.alias = t('wizard.step6.validation.required');
        } else if (!/^[a-zA-Z0-9_]+$/.test(state.alias)) {
          errors.alias = t('wizard.step6.validation.format');
        } else if (existingEntries.some(e => e.alias === state.alias)) {
          errors.alias = t('wizard.step6.validation.exists');
        }
        break;
    }

    setValidationErrors(errors);
    return Object.keys(errors).length === 0;
  }, [state, t, existingEntries]);

  // Handle next step - wrapped in useCallback for keyboard event handler
  const handleNext = useCallback(() => {
    if (validateStep()) {
      setState(prev => ({ ...prev, step: Math.min(prev.step + 1, TOTAL_STEPS) }));
    }
  }, [validateStep]);

  // Handle previous step
  const handleBack = useCallback(() => {
    setState(prev => ({ ...prev, step: Math.max(prev.step - 1, 1) }));
    setValidationErrors({});
  }, []);

  // Generate private key
  const handleGenerateKey = async () => {
    setIsGeneratingKey(true);
    setError(null);

    try {
      const key = await GenerateMasternodeKey();
      if (mountedRef.current) {
        setState(prev => ({ ...prev, privateKey: key }));
        setValidationErrors(prev => ({ ...prev, privateKey: '' }));
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

  // Create masternode
  const handleCreate = async () => {
    if (!state.collateral) return;

    setIsCreating(true);
    setError(null);

    try {
      const entry: MasternodeConfigEntry = {
        alias: state.alias.trim(),
        ip: `${state.ipAddress.trim()}:${state.port}`,
        privateKey: state.privateKey.trim(),
        txHash: state.collateral.txHash,
        outputIndex: state.collateral.outputIndex,
      };

      await AddMasternodeConfig(entry);
      if (mountedRef.current) {
        setCreateSuccess(true);
      }
    } catch (err) {
      if (mountedRef.current) {
        const errorMsg = err instanceof Error ? err.message : 'Unknown error';
        setError(t('wizard.step7.error', { error: sanitizeErrorMessage(errorMsg) }));
      }
    } finally {
      if (mountedRef.current) {
        setIsCreating(false);
      }
    }
  };

  // Start masternode and close - only calls onSuccess if start actually succeeds
  const handleStartAndClose = async () => {
    let startSucceeded = false;
    try {
      await StartMasternode(state.alias);
      startSucceeded = true;
    } catch (err) {
      console.error('Failed to start masternode:', err);
      // Still close dialog but don't call onSuccess since start failed
      // The masternode config was already created successfully
    }
    if (startSucceeded) {
      onSuccess?.();
    }
    onClose();
  };

  // Handle close - wrapped in useCallback for keyboard event handler
  const handleClose = useCallback(() => {
    if (createSuccess) {
      onSuccess?.();
    }
    onClose();
  }, [createSuccess, onSuccess, onClose]);

  // Handle keyboard events
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isCreating) {
        handleClose();
      } else if (e.key === 'Enter' && !isCreating && state.step < TOTAL_STEPS) {
        handleNext();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, isCreating, state.step, handleClose, handleNext]);

  if (!isOpen) return null;

  // Render step content
  const renderStepContent = () => {
    switch (state.step) {
      case 1:
        return (
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-semibold text-[#ddd] mb-2">{t('wizard.step1.subtitle')}</h3>
              <p className="text-sm text-[#aaa]">{t('wizard.step1.description')}</p>
            </div>

            {/* Requirements */}
            <div className="bg-[#333] rounded-lg p-4">
              <h4 className="text-sm font-semibold text-[#ddd] mb-3 flex items-center gap-2">
                <CheckCircle size={16} className="text-[#66ff66]" />
                {t('wizard.step1.requirements.title')}
              </h4>
              <ul className="space-y-2 text-sm text-[#aaa]">
                <li className="flex items-center gap-2">
                  <Server size={14} className="text-[#0088cc]" />
                  {t('wizard.step1.requirements.vps')}
                </li>
                <li className="flex items-center gap-2">
                  <Clock size={14} className="text-[#0088cc]" />
                  {t('wizard.step1.requirements.uptime')}
                </li>
                <li className="flex items-center gap-2">
                  <Wallet size={14} className="text-[#0088cc]" />
                  {t('wizard.step1.requirements.collateral')}
                </li>
                <li className="flex items-center gap-2">
                  <RefreshCw size={14} className="text-[#0088cc]" />
                  {t('wizard.step1.requirements.wallet')}
                </li>
              </ul>
            </div>

            {/* Tier Overview */}
            <div className="bg-[#333] rounded-lg p-4">
              <h4 className="text-sm font-semibold text-[#ddd] mb-3">{t('wizard.step1.tierOverview.title')}</h4>
              <p className="text-xs text-[#999] mb-3">{t('wizard.step1.tierOverview.description')}</p>
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="bg-[#2b2b2b] p-2 rounded">{t('wizard.step1.tierOverview.bronze')}</div>
                <div className="bg-[#2b2b2b] p-2 rounded">{t('wizard.step1.tierOverview.silver')}</div>
                <div className="bg-[#2b2b2b] p-2 rounded">{t('wizard.step1.tierOverview.gold')}</div>
                <div className="bg-[#2b2b2b] p-2 rounded">{t('wizard.step1.tierOverview.platinum')}</div>
              </div>
            </div>

            <p className="text-xs text-[#ffaa00] italic">{t('wizard.step1.note')}</p>
          </div>
        );

      case 2:
        return (
          <div className="space-y-4">
            <div>
              <h3 className="text-lg font-semibold text-[#ddd] mb-2">{t('wizard.step2.subtitle')}</h3>
              <p className="text-sm text-[#aaa]">{t('wizard.step2.description')}</p>
            </div>

            <div className="grid grid-cols-2 gap-3">
              {TIERS.map(tier => {
                const available = isTierAvailable(tier.id);
                const pending = isTierPending(tier.id);
                const selected = state.tier === tier.id;

                return (
                  <button
                    key={tier.id}
                    onClick={() => {
                      if (available) {
                        setState(prev => ({ ...prev, tier: tier.id, collateral: null }));
                        setValidationErrors(prev => ({ ...prev, tier: '' }));
                      }
                    }}
                    disabled={!available}
                    className={`p-4 rounded-lg border-2 text-left transition-all ${
                      selected
                        ? 'border-[#0066cc] bg-[#0066cc20]'
                        : available
                        ? 'border-[#555] hover:border-[#777] bg-[#333]'
                        : 'border-[#444] bg-[#2a2a2a] opacity-50 cursor-not-allowed'
                    }`}
                  >
                    <div className="flex justify-between items-start mb-2">
                      <span className={`font-semibold ${selected ? 'text-[#0099ff]' : 'text-[#ddd]'}`}>
                        {tier.name}
                      </span>
                      {selected && <Check size={18} className="text-[#0099ff]" />}
                    </div>
                    <div className="text-xs text-[#999]">
                      <div>{t('wizard.step2.tierCard.collateral')}: {tier.collateral.toLocaleString()} TWINS</div>
                      <div>{t('wizard.step2.tierCard.weight')}: {tier.weight}x</div>
                    </div>
                    <div className={`text-xs mt-2 ${available ? 'text-[#66ff66]' : pending ? 'text-[#ffaa00]' : 'text-[#ff6666]'}`}>
                      {available
                        ? t('wizard.step2.tierCard.available')
                        : pending
                        ? t('wizard.step2.tierCard.waitingConfirmations')
                        : t('wizard.step2.tierCard.insufficient')}
                    </div>
                  </button>
                );
              })}
            </div>

            {validationErrors.tier && (
              <p className="text-xs text-[#ff6666]">{validationErrors.tier}</p>
            )}
          </div>
        );

      case 3:
        return (
          <div className="space-y-4">
            <div>
              <h3 className="text-lg font-semibold text-[#ddd] mb-2">{t('wizard.step3.subtitle')}</h3>
              <p className="text-sm text-[#aaa]">{t('wizard.step3.description')}</p>
            </div>

            {readyOutputs.length === 0 && pendingOutputs.length === 0 ? (
              <div className="bg-[#3a2a2a] p-4 rounded-lg text-sm text-[#ff6666]">
                {t('wizard.step3.noUtxos', {
                  tier: state.tier ?? 'unknown',
                  amount: state.tier ? getTierCollateral(state.tier).toLocaleString() : '0',
                })}
              </div>
            ) : (
              <div className="space-y-2 max-h-[300px] overflow-y-auto">
                {readyOutputs.map(output => {
                  const selected = state.collateral?.txHash === output.txHash &&
                    state.collateral?.outputIndex === output.outputIndex;

                  return (
                    <button
                      key={`${output.txHash}:${output.outputIndex}`}
                      onClick={() => {
                        setState(prev => ({ ...prev, collateral: output }));
                        setValidationErrors(prev => ({ ...prev, collateral: '' }));
                      }}
                      className={`w-full p-3 rounded-lg border-2 text-left transition-all ${
                        selected
                          ? 'border-[#0066cc] bg-[#0066cc20]'
                          : 'border-[#555] hover:border-[#777] bg-[#333]'
                      }`}
                    >
                      <div className="flex justify-between items-center">
                        <div className="font-mono text-xs text-[#aaa]">
                          {output.txHash.substring(0, 16)}...:{output.outputIndex}
                        </div>
                        {selected && <Check size={16} className="text-[#0099ff]" />}
                      </div>
                      <div className="flex justify-between items-center mt-1">
                        <span className="text-sm text-[#ddd]">
                          {output.amount.toLocaleString()} TWINS
                        </span>
                        <span className="text-xs text-[#999] capitalize">{output.tier}</span>
                      </div>
                    </button>
                  );
                })}

                {readyOutputs.length === 0 && pendingOutputs.length > 0 && (
                  <div className="bg-[#3a3020] p-3 rounded-lg text-sm text-[#ffaa00]">
                    {t('wizard.step3.waitingForConfirmations', { count: pendingOutputs[0].confirmations })}
                  </div>
                )}

                {pendingOutputs.map(output => (
                  <div
                    key={`${output.txHash}:${output.outputIndex}`}
                    style={{ opacity: 0.5, cursor: 'not-allowed' }}
                    className="w-full p-3 rounded-lg border-2 border-[#444] bg-[#2a2a2a] text-left"
                  >
                    <div className="flex justify-between items-center">
                      <div className="font-mono text-xs text-[#aaa]">
                        {output.txHash.substring(0, 16)}...:{output.outputIndex}
                      </div>
                      <span className="text-xs text-[#ffaa00] bg-[#3a3020] px-2 py-0.5 rounded-full">
                        {t('wizard.step3.pendingUtxo', { count: output.confirmations })}
                      </span>
                    </div>
                    <div className="flex justify-between items-center mt-1">
                      <span className="text-sm text-[#ddd]">
                        {output.amount.toLocaleString()} TWINS
                      </span>
                      <span className="text-xs text-[#999] capitalize">{output.tier}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {validationErrors.collateral && (
              <p className="text-xs text-[#ff6666]">{validationErrors.collateral}</p>
            )}
          </div>
        );

      case 4:
        return (
          <div className="space-y-4">
            <div>
              <h3 className="text-lg font-semibold text-[#ddd] mb-2">{t('wizard.step4.subtitle')}</h3>
              <p className="text-sm text-[#aaa]">{t('wizard.step4.description')}</p>
            </div>

            {/* Key mode selection */}
            <div className="flex gap-2">
              <button
                onClick={() => setKeyMode('generate')}
                className={`flex-1 py-2 px-4 rounded-lg text-sm transition-all ${
                  keyMode === 'generate'
                    ? 'bg-[#0066cc] text-white'
                    : 'bg-[#444] text-[#ddd] hover:bg-[#555]'
                }`}
              >
                {t('wizard.step4.generateNew')}
              </button>
              <button
                onClick={() => setKeyMode('existing')}
                className={`flex-1 py-2 px-4 rounded-lg text-sm transition-all ${
                  keyMode === 'existing'
                    ? 'bg-[#0066cc] text-white'
                    : 'bg-[#444] text-[#ddd] hover:bg-[#555]'
                }`}
              >
                {t('wizard.step4.useExisting')}
              </button>
            </div>

            {keyMode === 'generate' ? (
              <div className="space-y-3">
                <button
                  onClick={handleGenerateKey}
                  disabled={isGeneratingKey}
                  className="w-full py-3 px-4 bg-[#0066cc] text-white rounded-lg hover:bg-[#0055aa] transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
                >
                  <Key size={18} className={isGeneratingKey ? 'animate-pulse' : ''} />
                  {isGeneratingKey ? t('wizard.step4.generating') : t('wizard.step4.generateNew')}
                </button>

                {state.privateKey && (
                  <div className="bg-[#2a4a2a] border border-[#66ff66] p-3 rounded-lg">
                    <div className="text-xs text-[#66ff66] mb-1">{t('wizard.step4.keyGenerated')}</div>
                    <div className="font-mono text-xs text-[#ddd] break-all">{state.privateKey}</div>
                  </div>
                )}
              </div>
            ) : (
              <div>
                <input
                  type="text"
                  value={state.privateKey}
                  onChange={(e) => {
                    setState(prev => ({ ...prev, privateKey: e.target.value }));
                    setValidationErrors(prev => ({ ...prev, privateKey: '' }));
                  }}
                  placeholder={t('wizard.step4.keyPlaceholder')}
                  className="w-full px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border border-[#555] rounded focus:outline-none focus:border-[#0066cc] font-mono"
                />
              </div>
            )}

            <p className="text-xs text-[#ffaa00]">{t('wizard.step4.keyHint')}</p>

            {validationErrors.privateKey && (
              <p className="text-xs text-[#ff6666]">{validationErrors.privateKey}</p>
            )}
          </div>
        );

      case 5:
        return (
          <div className="space-y-4">
            <div>
              <h3 className="text-lg font-semibold text-[#ddd] mb-2">{t('wizard.step5.subtitle')}</h3>
              <p className="text-sm text-[#aaa]">{t('wizard.step5.description')}</p>
            </div>

            <div className="space-y-3">
              <div>
                <label className="block text-sm text-[#ddd] mb-1">
                  {t('wizard.step5.ipAddress')} <span className="text-[#ff6666]">*</span>
                </label>
                <div className="flex items-center gap-2">
                  <Globe size={18} className="text-[#666]" />
                  <input
                    type="text"
                    value={state.ipAddress}
                    onChange={(e) => {
                      setState(prev => ({ ...prev, ipAddress: e.target.value }));
                      setValidationErrors(prev => ({ ...prev, ip: '' }));
                    }}
                    placeholder={t('wizard.step5.ipPlaceholder')}
                    className={`flex-1 px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border rounded focus:outline-none focus:border-[#0066cc] ${
                      validationErrors.ip ? 'border-[#ff6666]' : 'border-[#555]'
                    }`}
                  />
                </div>
                {validationErrors.ip && (
                  <p className="text-xs text-[#ff6666] mt-1">{validationErrors.ip}</p>
                )}
              </div>

              <div>
                <label className="block text-sm text-[#ddd] mb-1">
                  {t('wizard.step5.port')} <span className="text-[#ff6666]">*</span>
                </label>
                <input
                  type="number"
                  value={state.port}
                  onChange={(e) => {
                    setState(prev => ({ ...prev, port: parseInt(e.target.value, 10) || 0 }));
                    setValidationErrors(prev => ({ ...prev, port: '' }));
                  }}
                  min={1}
                  max={65535}
                  className={`w-32 px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border rounded focus:outline-none focus:border-[#0066cc] ${
                    validationErrors.port ? 'border-[#ff6666]' : 'border-[#555]'
                  }`}
                />
                <p className="text-xs text-[#999] mt-1">{t('wizard.step5.portHint')}</p>
                {validationErrors.port && (
                  <p className="text-xs text-[#ff6666] mt-1">{validationErrors.port}</p>
                )}
              </div>
            </div>
          </div>
        );

      case 6:
        return (
          <div className="space-y-4">
            <div>
              <h3 className="text-lg font-semibold text-[#ddd] mb-2">{t('wizard.step6.subtitle')}</h3>
              <p className="text-sm text-[#aaa]">{t('wizard.step6.description')}</p>
            </div>

            <div>
              <label className="block text-sm text-[#ddd] mb-1">
                {t('wizard.step6.aliasLabel')} <span className="text-[#ff6666]">*</span>
              </label>
              <div className="flex items-center gap-2">
                <Tag size={18} className="text-[#666]" />
                <input
                  type="text"
                  value={state.alias}
                  onChange={(e) => {
                    setState(prev => ({ ...prev, alias: e.target.value }));
                    setValidationErrors(prev => ({ ...prev, alias: '' }));
                  }}
                  placeholder={t('wizard.step6.aliasPlaceholder')}
                  maxLength={50}
                  className={`flex-1 px-3 py-2 text-sm bg-[#2b2b2b] text-[#ddd] border rounded focus:outline-none focus:border-[#0066cc] ${
                    validationErrors.alias ? 'border-[#ff6666]' : 'border-[#555]'
                  }`}
                />
              </div>
              <p className="text-xs text-[#999] mt-1">{t('wizard.step6.aliasHint')}</p>
              {validationErrors.alias && (
                <p className="text-xs text-[#ff6666] mt-1">{validationErrors.alias}</p>
              )}
            </div>
          </div>
        );

      case 7:
        return (
          <div className="space-y-4">
            {createSuccess ? (
              <div className="text-center py-8">
                <CheckCircle size={64} className="text-[#66ff66] mx-auto mb-4" />
                <h3 className="text-lg font-semibold text-[#ddd] mb-2">
                  {t('wizard.step7.success', { alias: state.alias })}
                </h3>
                <p className="text-sm text-[#aaa]">{t('wizard.step7.successHint')}</p>
              </div>
            ) : (
              <>
                <div>
                  <h3 className="text-lg font-semibold text-[#ddd] mb-2">{t('wizard.step7.subtitle')}</h3>
                  <p className="text-sm text-[#aaa]">{t('wizard.step7.description')}</p>
                </div>

                <div className="bg-[#333] rounded-lg p-4 space-y-3">
                  <div className="flex justify-between">
                    <span className="text-sm text-[#999]">{t('wizard.step7.summary.tier')}</span>
                    <span className="text-sm text-[#ddd] capitalize">{state.tier}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm text-[#999]">{t('wizard.step7.summary.collateral')}</span>
                    <span className="text-sm text-[#ddd] font-mono">
                      {state.collateral?.txHash.substring(0, 12)}...:{state.collateral?.outputIndex}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm text-[#999]">{t('wizard.step7.summary.privateKey')}</span>
                    <span className="text-sm text-[#ddd] font-mono">
                      {state.privateKey.substring(0, 12)}...
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm text-[#999]">{t('wizard.step7.summary.network')}</span>
                    <span className="text-sm text-[#ddd]">{state.ipAddress}:{state.port}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm text-[#999]">{t('wizard.step7.summary.alias')}</span>
                    <span className="text-sm text-[#ddd]">{state.alias}</span>
                  </div>
                </div>

                {error && (
                  <div className="bg-[#4a2a2a] border border-[#ff6666] p-3 rounded-lg flex items-center gap-2">
                    <AlertCircle size={16} className="text-[#ff6666]" />
                    <span className="text-sm text-[#ff6666]">{error}</span>
                  </div>
                )}
              </>
            )}
          </div>
        );

      default:
        return null;
    }
  };

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
      onClick={(e) => {
        if (e.target === e.currentTarget && !isCreating) {
          handleClose();
        }
      }}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="wizard-title"
        className="bg-[#2b2b2b] rounded-lg shadow-xl w-[600px] max-h-[700px] flex flex-col"
        style={{ border: '1px solid #555' }}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#555]">
          <div>
            <h2 id="wizard-title" className="text-lg font-semibold text-[#ddd]">
              {t('wizard.title')}
            </h2>
            <p className="text-xs text-[#999]">
              {t('wizard.progress', { current: state.step, total: TOTAL_STEPS })}
            </p>
          </div>
          <button
            onClick={handleClose}
            disabled={isCreating}
            className="text-[#999] hover:text-[#ddd] transition-colors disabled:opacity-50"
            aria-label="Close dialog"
          >
            <X size={20} />
          </button>
        </div>

        {/* Progress bar */}
        <div className="px-6 py-2">
          <div className="flex gap-1">
            {Array.from({ length: TOTAL_STEPS }, (_, i) => (
              <div
                key={i}
                className={`flex-1 h-1 rounded-full transition-colors ${
                  i < state.step ? 'bg-[#0066cc]' : 'bg-[#444]'
                }`}
              />
            ))}
          </div>
        </div>

        {/* Step title */}
        <div className="px-6 py-2">
          <h3 className="text-sm font-medium text-[#0099ff]">
            {t(`wizard.step${state.step}.title`)}
          </h3>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto px-6 py-4">
          {isLoading ? (
            <div className="text-center py-8 text-[#999]">{t('config.loading')}</div>
          ) : (
            renderStepContent()
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-[#555]">
          <button
            onClick={handleClose}
            disabled={isCreating}
            className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50"
          >
            {t('wizard.buttons.cancel')}
          </button>

          <div className="flex gap-2">
            {state.step > 1 && !createSuccess && (
              <button
                onClick={handleBack}
                disabled={isCreating}
                className="px-4 py-2 text-sm bg-[#444] text-[#ddd] rounded hover:bg-[#555] transition-colors disabled:opacity-50 flex items-center gap-1"
              >
                <ChevronLeft size={16} />
                {t('wizard.buttons.back')}
              </button>
            )}

            {state.step < TOTAL_STEPS && (
              <button
                onClick={handleNext}
                disabled={isLoading}
                className="px-4 py-2 text-sm bg-[#0066cc] text-white rounded hover:bg-[#0055aa] transition-colors disabled:opacity-50 flex items-center gap-1"
              >
                {t('wizard.buttons.next')}
                <ChevronRight size={16} />
              </button>
            )}

            {state.step === TOTAL_STEPS && !createSuccess && (
              <button
                onClick={handleCreate}
                disabled={isCreating}
                className="px-4 py-2 text-sm bg-[#0066cc] text-white rounded hover:bg-[#0055aa] transition-colors disabled:opacity-50 flex items-center gap-1"
              >
                {isCreating ? (
                  <>
                    <RefreshCw size={16} className="animate-spin" />
                    {t('wizard.step7.creating')}
                  </>
                ) : (
                  <>
                    <Check size={16} />
                    {t('wizard.buttons.finish')}
                  </>
                )}
              </button>
            )}

            {createSuccess && (
              <button
                onClick={handleStartAndClose}
                className="px-4 py-2 text-sm bg-[#00aa66] text-white rounded hover:bg-[#009955] transition-colors flex items-center gap-1"
              >
                {t('wizard.buttons.startNow')}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
