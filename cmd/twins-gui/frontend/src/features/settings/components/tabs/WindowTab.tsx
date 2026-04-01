import React from 'react';
import { GUISettings, SettingMetadata } from '../../../../store/slices/optionsSlice';

interface WindowTabProps {
  settings: Partial<GUISettings>;
  metadata: Record<string, SettingMetadata>;
  onChange: (key: string, value: unknown) => void;
}

export const WindowTab: React.FC<WindowTabProps> = ({ settings, metadata: _metadata, onChange }) => {
  // Note: metadata available for future use when CLI overrides are added to Window settings
  void _metadata;

  const hideTrayIcon = settings.fHideTrayIcon ?? false;
  const minimizeToTray = settings.fMinimizeToTray ?? false;

  return (
    <div style={{ padding: '16px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
      {/* Window Behavior Group */}
      <div style={{
        border: '1px solid #555',
        borderRadius: '4px',
        padding: '12px',
        backgroundColor: '#2a2a2a'
      }}>
        <div style={{
          color: '#aaa',
          fontSize: '11px',
          textTransform: 'uppercase',
          marginBottom: '12px',
          letterSpacing: '0.5px'
        }}>
          Window Behavior
        </div>

        {/* Minimize to Tray — disabled when tray icon is hidden */}
        <div style={{ marginBottom: '12px' }}>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <input
              type="checkbox"
              id="fMinimizeToTray"
              checked={minimizeToTray}
              disabled={hideTrayIcon}
              onChange={(e) => onChange('fMinimizeToTray', e.target.checked)}
              style={{ marginRight: '8px' }}
            />
            <label
              htmlFor="fMinimizeToTray"
              style={{ color: hideTrayIcon ? '#777' : '#ddd', fontSize: '13px' }}
            >
              Minimize to the tray instead of the taskbar
            </label>
          </div>
          {hideTrayIcon && (
            <div style={{ color: '#998800', fontSize: '11px', marginLeft: '24px', marginTop: '4px' }}>
              Disabled because "Hide tray icon" is enabled
            </div>
          )}
        </div>

        {/* Minimize on Close */}
        <div style={{ display: 'flex', alignItems: 'center' }}>
          <input
            type="checkbox"
            id="fMinimizeOnClose"
            checked={settings.fMinimizeOnClose ?? false}
            onChange={(e) => onChange('fMinimizeOnClose', e.target.checked)}
            style={{ marginRight: '8px' }}
          />
          <label htmlFor="fMinimizeOnClose" style={{ color: '#ddd', fontSize: '13px' }}>
            Minimize on close
          </label>
        </div>
      </div>

      {/* Tray Icon Group */}
      <div style={{
        border: '1px solid #555',
        borderRadius: '4px',
        padding: '12px',
        backgroundColor: '#2a2a2a'
      }}>
        <div style={{
          color: '#aaa',
          fontSize: '11px',
          textTransform: 'uppercase',
          marginBottom: '12px',
          letterSpacing: '0.5px'
        }}>
          System Tray
        </div>

        {/* Hide Tray Icon — disabled when minimize-to-tray is enabled */}
        <div>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <input
              type="checkbox"
              id="fHideTrayIcon"
              checked={hideTrayIcon}
              disabled={minimizeToTray}
              onChange={(e) => onChange('fHideTrayIcon', e.target.checked)}
              style={{ marginRight: '8px' }}
            />
            <label
              htmlFor="fHideTrayIcon"
              style={{ color: minimizeToTray ? '#777' : '#ddd', fontSize: '13px' }}
            >
              Hide tray icon
            </label>
          </div>
          {minimizeToTray && (
            <div style={{ color: '#998800', fontSize: '11px', marginLeft: '24px', marginTop: '4px' }}>
              Disabled because "Minimize to the tray instead of the taskbar" is enabled
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default WindowTab;
