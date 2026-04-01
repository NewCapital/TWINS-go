import React, { useEffect, useCallback } from 'react';
import { X } from 'lucide-react';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { InformationTab, ConsoleTab, NetworkTrafficTab, PeersTab, WalletRepairTab } from './tabs';
import { ToolsTab, TOOLS_TAB_NAMES, type ToolsTabValue } from '../constants';

export const ToolsDialog: React.FC = () => {
  const {
    isToolsDialogOpen,
    toolsActiveTab,
    closeToolsDialog,
    setToolsActiveTab,
  } = useStore(useShallow((s) => ({
    isToolsDialogOpen: s.isToolsDialogOpen,
    toolsActiveTab: s.toolsActiveTab,
    closeToolsDialog: s.closeToolsDialog,
    setToolsActiveTab: s.setToolsActiveTab,
  })));

  const handleClose = useCallback(() => {
    closeToolsDialog();
  }, [closeToolsDialog]);

  // Handle Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isToolsDialogOpen) {
        handleClose();
      }
    };
    if (isToolsDialogOpen) {
      document.addEventListener('keydown', handleKeyDown);
    }
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isToolsDialogOpen, handleClose]);

  if (!isToolsDialogOpen) return null;

  const renderTabContent = () => {
    switch (toolsActiveTab) {
      case ToolsTab.Information: return <InformationTab />;
      case ToolsTab.Console: return <ConsoleTab />;
      case ToolsTab.NetworkTraffic: return <NetworkTrafficTab />;
      case ToolsTab.Peers: return <PeersTab />;
      case ToolsTab.WalletRepair: return <WalletRepairTab />;
      default: return null;
    }
  };

  return (
    <>
      {/* Backdrop */}
      <div
        style={{
          position: 'fixed',
          inset: 0,
          backgroundColor: 'rgba(0, 0, 0, 0.6)',
          zIndex: 1000,
        }}
        onClick={handleClose}
      />

      {/* Dialog */}
      <div
        style={{
          position: 'fixed',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          width: '900px',
          height: '620px',
          backgroundColor: '#2b2b2b',
          borderRadius: '8px',
          boxShadow: '0 4px 20px rgba(0, 0, 0, 0.5)',
          zIndex: 1001,
          display: 'flex',
          flexDirection: 'column',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: '16px 20px',
            borderBottom: '1px solid #444',
          }}
        >
          <h2 style={{ margin: 0, color: '#fff', fontSize: '18px', fontWeight: 500 }}>
            Tools Window
          </h2>
          <button
            onClick={handleClose}
            style={{
              background: 'none',
              border: 'none',
              color: '#888',
              cursor: 'pointer',
              padding: '4px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <X size={20} />
          </button>
        </div>

        {/* Tab Bar */}
        <div
          style={{
            display: 'flex',
            borderBottom: '1px solid #444',
            backgroundColor: '#333',
          }}
        >
          {TOOLS_TAB_NAMES.map((tab, index) => (
            <button
              key={tab}
              onClick={() => setToolsActiveTab(index as ToolsTabValue)}
              style={{
                padding: '12px 20px',
                backgroundColor: toolsActiveTab === index ? '#2b2b2b' : 'transparent',
                border: 'none',
                borderBottom: toolsActiveTab === index ? '2px solid #4a9eff' : '2px solid transparent',
                color: toolsActiveTab === index ? '#fff' : '#aaa',
                cursor: 'pointer',
                fontSize: '13px',
                fontWeight: toolsActiveTab === index ? 500 : 400,
                transition: 'all 0.15s ease',
              }}
            >
              {tab}
            </button>
          ))}
        </div>

        {/* Content Area */}
        <div style={{ flex: 1, overflow: 'hidden' }}>
          {renderTabContent()}
        </div>
      </div>
    </>
  );
};
