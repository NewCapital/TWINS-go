import React, { useState, useCallback } from 'react';
import { Lock, AlertTriangle, ChevronDown, ChevronRight, Eye, EyeOff } from 'lucide-react';
import type { config } from '../../../../../wailsjs/go/models';
import type { DaemonSettingValue } from '../../../../store/slices/optionsSlice';

// Category display titles
const categoryTitle = (cat: string): string => {
  const titles: Record<string, string> = {
    staking: 'Staking Settings',
    wallet: 'Wallet Settings',
    network: 'Network Settings',
    rpc: 'RPC Settings',
    masternode: 'Masternode Settings',
    logging: 'Logging Settings',
    sync: 'Sync Settings',
  };
  return titles[cat] || `${cat.charAt(0).toUpperCase() + cat.slice(1)} Settings`;
};

// Keys that hold sensitive secrets and should render as password fields with show/hide toggle
const SENSITIVE_KEYS = new Set(['masternode.privateKey', 'rpc.password', 'rpc.username']);

// Convert satoshis to a human-readable TWINS string (trims trailing zeros)
function satoshisToTWINS(satoshis: unknown): string {
  const n = Number(satoshis);
  if (!isFinite(n) || satoshis === '' || satoshis == null) return '';
  return (n / 1e8).toFixed(8).replace(/0+$/, '').replace(/\.$/, '') || '0';
}

// Returns true when the meta.units string implies the value is expressed in satoshis
function isSatoshiField(meta: config.SettingMeta): boolean {
  const u = (meta.units as unknown as string | undefined) ?? '';
  return u.includes('satoshis');
}

export interface DaemonCategorySectionProps {
  filterCategories: string[];
  metadata: config.SettingMeta[];
  daemonValues: Record<string, DaemonSettingValue>;
  pendingChanges: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
}

export const DaemonCategorySection: React.FC<DaemonCategorySectionProps> = ({
  filterCategories,
  metadata,
  daemonValues,
  pendingChanges,
  onChange,
}) => {
  const [collapsedCategories, setCollapsedCategories] = useState<Set<string>>(() => new Set(filterCategories));
  // Set of keys where the sensitive value is currently revealed
  const [revealedKeys, setRevealedKeys] = useState<Set<string>>(new Set());

  const toggleReveal = useCallback((key: string) => {
    setRevealedKeys(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const getWorkingValue = (key: string): unknown => {
    return pendingChanges[key] !== undefined
      ? pendingChanges[key]
      : daemonValues[key]?.value;
  };

  const toggleCategory = useCallback((cat: string) => {
    setCollapsedCategories(prev => {
      const next = new Set(prev);
      if (next.has(cat)) {
        next.delete(cat);
      } else {
        next.add(cat);
      }
      return next;
    });
  }, []);

  // Filter and group metadata by category
  const groupedSettings = filterCategories
    .map(cat => ({
      category: cat,
      settings: metadata.filter(m => m.category === cat),
    }))
    .filter(g => g.settings.length > 0);

  const renderControl = (meta: config.SettingMeta) => {
    const value = getWorkingValue(meta.key);
    const isLocked = daemonValues[meta.key]?.locked ?? false;
    const settingType = meta.type as unknown as string;
    const isPending = pendingChanges[meta.key] !== undefined;
    const isSensitive = SENSITIVE_KEYS.has(meta.key);
    const isRevealed = revealedKeys.has(meta.key);

    const inputBase: React.CSSProperties = {
      padding: '4px 8px',
      backgroundColor: isLocked ? '#333' : '#3a3a3a',
      border: isPending ? '1px solid #4a9eff' : '1px solid #555',
      borderRadius: '3px',
      color: isLocked ? '#888' : '#fff',
      fontSize: '13px',
    };

    switch (settingType) {
      case 'bool':
        return (
          <input
            type="checkbox"
            checked={!!value}
            onChange={(e) => onChange(meta.key, e.target.checked)}
            disabled={isLocked}
            style={{ marginRight: '8px', cursor: isLocked ? 'not-allowed' : 'pointer' }}
          />
        );

      case 'int':
      case 'int64':
      case 'uint32':
      case 'float64': {
        const showTwins = isSatoshiField(meta);
        return (
          <div>
            <input
              type="number"
              value={value != null ? String(value) : ''}
              onChange={(e) => {
                if (e.target.value === '') {
                  onChange(meta.key, 0);
                  return;
                }
                const parsed = settingType === 'float64'
                  ? parseFloat(e.target.value)
                  : parseInt(e.target.value, 10);
                if (!isNaN(parsed)) {
                  onChange(meta.key, parsed);
                }
              }}
              disabled={isLocked}
              min={meta.validation?.min}
              max={meta.validation?.max}
              step={settingType === 'float64' ? '0.00000001' : '1'}
              style={{ ...inputBase, width: '120px' }}
            />
            {showTwins && value != null && value !== '' && (
              <div style={{ color: '#888', fontSize: '11px', marginTop: '2px' }}>
                ≈ {satoshisToTWINS(value)} {(meta.units as unknown as string).replace('satoshis', 'TWINS')}
              </div>
            )}
          </div>
        );
      }

      case 'string':
        if (meta.validation?.options && meta.validation.options.length > 0) {
          return (
            <select
              value={String(value ?? '')}
              onChange={(e) => onChange(meta.key, e.target.value)}
              disabled={isLocked}
              style={{ ...inputBase, width: '200px' }}
            >
              {meta.validation.options.map(opt => (
                <option key={opt} value={opt}>{opt}</option>
              ))}
            </select>
          );
        }

        // Sensitive fields (private key, password): password input with reveal toggle
        if (isSensitive) {
          return (
            <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
              <input
                type={isRevealed ? 'text' : 'password'}
                value={String(value ?? '')}
                onChange={(e) => onChange(meta.key, e.target.value)}
                disabled={isLocked}
                autoComplete="new-password"
                style={{ ...inputBase, width: '200px' }}
              />
              <button
                type="button"
                onClick={() => toggleReveal(meta.key)}
                disabled={isLocked}
                title={isRevealed ? 'Hide value' : 'Show value'}
                style={{
                  background: 'none',
                  border: '1px solid #555',
                  borderRadius: '3px',
                  color: '#aaa',
                  cursor: isLocked ? 'not-allowed' : 'pointer',
                  padding: '4px 6px',
                  display: 'flex',
                  alignItems: 'center',
                }}
              >
                {isRevealed ? <EyeOff size={13} /> : <Eye size={13} />}
              </button>
            </div>
          );
        }

        // logging.output: add a hint about absolute vs relative paths
        if (meta.key === 'logging.output') {
          return (
            <div>
              <input
                type="text"
                value={String(value ?? '')}
                onChange={(e) => onChange(meta.key, e.target.value)}
                disabled={isLocked}
                style={{ ...inputBase, width: '200px' }}
              />
              <div style={{ color: '#888', fontSize: '11px', marginTop: '2px' }}>
                "stdout", "stderr", or an absolute file path. Relative paths resolve from the data directory.
              </div>
            </div>
          );
        }

        return (
          <input
            type="text"
            value={String(value ?? '')}
            onChange={(e) => onChange(meta.key, e.target.value)}
            disabled={isLocked}
            style={{ ...inputBase, width: '200px' }}
          />
        );

      case '[]string': {
        // Textarea with one entry per line — much more readable than a comma-separated single line
        const lines = Array.isArray(value)
          ? (value as string[]).join('\n')
          : String(value ?? '');
        return (
          <textarea
            value={lines}
            onChange={(e) => {
              const items = e.target.value.split('\n').map(s => s.trim()).filter(Boolean);
              onChange(meta.key, items);
            }}
            disabled={isLocked}
            placeholder="one entry per line"
            rows={4}
            style={{
              ...inputBase,
              width: '280px',
              resize: 'vertical',
              fontFamily: 'monospace',
              lineHeight: '1.4',
            }}
          />
        );
      }

      default:
        return (
          <span style={{ color: '#888', fontSize: '12px' }}>
            Unsupported type: {settingType}
          </span>
        );
    }
  };

  const renderBadges = (meta: config.SettingMeta) => {
    if (meta.hotReload) {
      return (
        <>
          {' '}
          <span
            title="Changes apply immediately without restart"
            style={{
              color: '#4caf50',
              fontSize: '10px',
              marginLeft: '4px',
              border: '1px solid #4caf50',
              borderRadius: '3px',
              padding: '1px 4px',
              verticalAlign: 'middle',
              whiteSpace: 'nowrap',
            }}
          >
            Live
          </span>
        </>
      );
    }
    return (
      <>
        {' '}
        <span
          title="Requires daemon restart to take effect"
          style={{ color: '#ffa500', marginLeft: '4px' }}
        >
          <AlertTriangle size={13} style={{ display: 'inline', verticalAlign: 'middle' }} />
        </span>
      </>
    );
  };

  const renderLockIcon = (meta: config.SettingMeta) => {
    const flag = meta.cliFlag || '';
    const envVar = meta.envVar || '';
    const title = flag
      ? `Overridden by CLI flag: --${flag}`
      : envVar
        ? `Overridden by environment variable: ${envVar}`
        : 'Overridden by external configuration';
    return (
      <span
        title={title}
        style={{ color: '#888', marginLeft: '8px' }}
      >
        <Lock size={13} style={{ display: 'inline', verticalAlign: 'middle' }} />
      </span>
    );
  };

  const renderSetting = (meta: config.SettingMeta) => {
    const isLocked = daemonValues[meta.key]?.locked ?? false;
    const settingType = meta.type as unknown as string;
    const isBool = settingType === 'bool';
    const description = meta.description as unknown as string | undefined;

    return (
      <div
        key={meta.key}
        style={{
          display: 'flex',
          alignItems: isBool ? 'center' : 'flex-start',
          marginBottom: '10px',
          opacity: meta.deprecated ? 0.5 : 1,
        }}
      >
        {isBool ? (
          <>
            {renderControl(meta)}
            <div style={{ flex: 1 }}>
              <label style={{ color: '#ddd', fontSize: '13px', cursor: isLocked ? 'default' : 'pointer' }}>
                {meta.label}
                {meta.units && !isSatoshiField(meta) && (
                  <span style={{ color: '#888', fontSize: '12px', marginLeft: '4px' }}>
                    ({meta.units})
                  </span>
                )}
                {renderBadges(meta)}
                {isLocked && renderLockIcon(meta)}
              </label>
              {description && (
                <div style={{ color: '#666', fontSize: '11px', marginTop: '2px' }}>
                  {description}
                </div>
              )}
            </div>
          </>
        ) : (
          <>
            <div style={{ width: '200px', flexShrink: 0, paddingTop: '4px' }}>
              <label style={{ color: '#ddd', fontSize: '13px' }}>
                {meta.label}
                {renderBadges(meta)}
                {isLocked && renderLockIcon(meta)}
              </label>
              {description && (
                <div style={{ color: '#666', fontSize: '11px', marginTop: '2px', lineHeight: '1.3' }}>
                  {description}
                </div>
              )}
            </div>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '8px' }}>
              {renderControl(meta)}
              {meta.units && !isSatoshiField(meta) && (
                <span style={{ color: '#888', fontSize: '12px', paddingTop: '6px' }}>
                  {meta.units}
                </span>
              )}
            </div>
          </>
        )}
      </div>
    );
  };

  if (groupedSettings.length === 0) {
    return (
      <div style={{ color: '#888', fontSize: '13px', padding: '16px', textAlign: 'center' }}>
        No settings available
      </div>
    );
  }

  return (
    <div style={{ padding: '16px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
      {groupedSettings.map(({ category, settings }) => {
        const isCollapsed = collapsedCategories.has(category);
        return (
          <div
            key={category}
            style={{
              border: '1px solid #555',
              borderRadius: '4px',
              backgroundColor: '#2a2a2a',
            }}
          >
            <button
              onClick={() => toggleCategory(category)}
              style={{
                display: 'flex',
                alignItems: 'center',
                width: '100%',
                padding: '10px 12px',
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                gap: '6px',
              }}
            >
              {isCollapsed
                ? <ChevronRight size={14} style={{ color: '#aaa' }} />
                : <ChevronDown size={14} style={{ color: '#aaa' }} />
              }
              <span style={{
                color: '#aaa',
                fontSize: '11px',
                textTransform: 'uppercase',
                letterSpacing: '0.5px',
                flex: 1,
                textAlign: 'left',
              }}>
                {categoryTitle(category)}
              </span>
              <span style={{ color: '#666', fontSize: '11px' }}>
                {settings.length} {settings.length === 1 ? 'setting' : 'settings'}
              </span>
            </button>

            {!isCollapsed && (
              <div style={{ padding: '4px 12px 12px 12px', borderTop: '1px solid #444' }}>
                {settings.map(renderSetting)}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
};

export default DaemonCategorySection;
