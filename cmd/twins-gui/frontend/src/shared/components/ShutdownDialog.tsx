import React, { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { EventsOn, EventsOff } from '@wailsjs/runtime/runtime';
import './ShutdownDialog.css';

/**
 * ShutdownDialog Component
 *
 * Displays a shutdown window exactly like Qt wallet's ShutdownWindow.
 * Based on TWINS-Core/src/qt/utilitydialog.cpp lines 154-183.
 *
 * Qt implementation:
 * - Simple QWidget with QVBoxLayout
 * - Single QLabel with two lines of text separated by <br /><br />
 * - No styling, uses system defaults
 * - Cannot be closed by user
 */
const ShutdownDialog: React.FC = () => {
  const { t } = useTranslation('common');

  useEffect(() => {
    // Just listen for completion to close
    EventsOn('shutdown:complete', () => {
      console.log('Shutdown complete');
    });

    return () => {
      EventsOff('shutdown:complete');
    };
  }, []);

  return (
    <div className="shutdown-overlay">
      <div className="shutdown-window">
        {/* Exactly matching Qt: single label with <br /><br /> between lines */}
        <div className="shutdown-label">
          {t('shutdown.message')}
          <br />
          <br />
          {t('shutdown.pleaseWait')}
        </div>
      </div>
    </div>
  );
};

export default ShutdownDialog;