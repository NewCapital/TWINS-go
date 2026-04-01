import React from 'react';
import { useTranslation } from 'react-i18next';
import { Masternode } from '@/shared/types/masternode.types';

export interface MasternodesActionsProps {
  selectedMasternode: Masternode | null;
  isLoading: boolean;
  isStartingMasternode: boolean;
  onStartAlias: () => void;
  onStartAll: () => void;
  onStartMissing: () => void;
  onUpdateStatus: () => void;
  onConfigure: () => void;
  onSetupWizard: () => void;
}

export const MasternodesActions: React.FC<MasternodesActionsProps> = ({
  selectedMasternode,
  isLoading,
  isStartingMasternode,
  onStartAlias,
  onStartAll,
  onStartMissing,
  onUpdateStatus,
  onConfigure,
  onSetupWizard,
}) => {
  const { t } = useTranslation('masternode');

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      gap: '8px',
      marginTop: '8px',
      padding: '4px 0'
    }}>
      <button
        className="qt-button"
        disabled={!selectedMasternode || isStartingMasternode}
        onClick={onStartAlias}
        style={{
          padding: '4px 12px',
          fontSize: '12px',
          opacity: selectedMasternode && !isStartingMasternode ? 1 : 0.5,
          cursor: selectedMasternode && !isStartingMasternode ? 'pointer' : 'not-allowed'
        }}
      >
        {t('actions.startAlias')}
      </button>
      <button
        className="qt-button"
        disabled={isStartingMasternode}
        onClick={onStartAll}
        style={{
          padding: '4px 12px',
          fontSize: '12px',
          opacity: isStartingMasternode ? 0.5 : 1,
          cursor: isStartingMasternode ? 'not-allowed' : 'pointer'
        }}
      >
        {t('actions.startAll')}
      </button>
      <button
        className="qt-button"
        disabled={isStartingMasternode}
        onClick={onStartMissing}
        style={{
          padding: '4px 12px',
          fontSize: '12px',
          opacity: isStartingMasternode ? 0.5 : 1,
          cursor: isStartingMasternode ? 'not-allowed' : 'pointer'
        }}
      >
        {t('actions.startMissing')}
      </button>
      <button
        className="qt-button"
        disabled={isLoading}
        onClick={onUpdateStatus}
        style={{
          padding: '4px 12px',
          fontSize: '12px',
          opacity: isLoading ? 0.5 : 1,
          cursor: isLoading ? 'not-allowed' : 'pointer'
        }}
      >
        {t('actions.update')}
      </button>
      <button
        className="qt-button"
        onClick={onConfigure}
        style={{
          padding: '4px 12px',
          fontSize: '12px',
          cursor: 'pointer'
        }}
      >
        {t('actions.configure')}
      </button>
      <button
        className="qt-button"
        onClick={onSetupWizard}
        style={{
          padding: '4px 12px',
          fontSize: '12px',
          cursor: 'pointer',
          backgroundColor: '#0066cc',
          borderColor: '#0055aa'
        }}
      >
        {t('wizard.title')}
      </button>
      <div style={{ flex: 1 }} /> {/* Spacer */}
    </div>
  );
};
