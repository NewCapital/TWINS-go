import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { GetDefaultDataDirectory, GetStoredDataDirectory, SelectDataDirectory, CheckDiskSpace, ValidateDataDirectory, InitializeDataDirectory } from '@wailsjs/go/main/App';

interface IntroDialogProps {
  onComplete: (dataDirectory: string) => void;
  onCancel?: () => void;
}

interface DiskSpaceInfo {
  available: number;
  required: number;
  hasSpace: boolean;
}

export const IntroDialog: React.FC<IntroDialogProps> = ({ onComplete, onCancel }) => {
  const { t } = useTranslation('common');
  const [useDefault, setUseDefault] = useState(true); // Start with default selected initially
  const [customDirectory, setCustomDirectory] = useState('');
  const [defaultDirectory, setDefaultDirectory] = useState('');
  const [diskSpace, setDiskSpace] = useState<DiskSpaceInfo | null>(null);
  const [isValidating, setIsValidating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<string>('');
  const [isInitializing, setIsInitializing] = useState(false);

  useEffect(() => {
    // Load default directory and stored preferences on mount
    const initializeDialog = async () => {
      try {
        // Get both default and stored directories
        const [defaultDir, storedDir] = await Promise.all([
          GetDefaultDataDirectory(),
          GetStoredDataDirectory()
        ]);

        setDefaultDirectory(defaultDir);

        // Determine initial state based on stored preferences
        if (!storedDir || storedDir === '') {
          // No preferences file exists - select "Use the default data directory"
          setUseDefault(true);
          setCustomDirectory(defaultDir);
          checkDiskSpaceForDirectory(defaultDir);
        } else if (storedDir === defaultDir) {
          // Preferences exist and stored dir is same as default - select "Use the default data directory"
          setUseDefault(true);
          setCustomDirectory(defaultDir);
          checkDiskSpaceForDirectory(defaultDir);
        } else {
          // Preferences exist and stored dir is different from default - select "Use a custom data directory"
          setUseDefault(false);
          setCustomDirectory(storedDir);
          checkDiskSpaceForDirectory(storedDir);
        }
      } catch (err) {
        console.error('Failed to initialize dialog:', err);
        setError('Failed to initialize data directory settings');
        // Fall back to default directory
        try {
          const defaultDir = await GetDefaultDataDirectory();
          setDefaultDirectory(defaultDir);
          setUseDefault(true);
          setCustomDirectory(defaultDir);
          checkDiskSpaceForDirectory(defaultDir);
        } catch (fallbackErr) {
          console.error('Failed to get default directory:', fallbackErr);
        }
      }
    };

    initializeDialog();
  }, []);

  const getCurrentDirectory = () => {
    return useDefault ? defaultDirectory : customDirectory;
  };

  const checkDiskSpaceForDirectory = async (dir: string) => {
    if (!dir) return;

    try {
      const spaceInfo = await CheckDiskSpace(dir);
      setDiskSpace(spaceInfo);

      // Check if directory exists
      try {
        await ValidateDataDirectory(dir);
        if (spaceInfo.hasSpace) {
          setStatusMessage('Directory already exists. Add /name if you intend to create a new directory here.');
        }
      } catch {
        setStatusMessage('');
      }

      if (!spaceInfo.hasSpace) {
        setError(null);
        setStatusMessage('Error: Cannot create data directory here.');
      } else {
        setError(null);
      }
    } catch (err) {
      console.error('Failed to check disk space:', err);
      setStatusMessage('Error: Cannot create data directory here.');
    }
  };

  const formatBytes = (bytes: number): string => {
    const gb = bytes / (1000 * 1000 * 1000); // Using 1000 like the old GUI
    return Math.round(gb).toString();
  };

  const handleBrowse = async () => {
    try {
      const selectedDir = await SelectDataDirectory();
      if (selectedDir) {
        setCustomDirectory(selectedDir);
        setUseDefault(false);
        await checkDiskSpaceForDirectory(selectedDir);
      }
    } catch (err) {
      console.error('Failed to select directory:', err);
      setStatusMessage('Error: Failed to select directory');
    }
  };

  const handleRadioChange = (useDefaultOption: boolean) => {
    setUseDefault(useDefaultOption);
    const dir = useDefaultOption ? defaultDirectory : customDirectory;
    if (dir) {
      checkDiskSpaceForDirectory(dir);
    }
  };

  const handleCustomDirectoryChange = (value: string) => {
    setCustomDirectory(value);
    if (!useDefault && value) {
      checkDiskSpaceForDirectory(value);
    }
  };

  const handleContinue = async () => {
    const selectedDir = getCurrentDirectory();
    if (!selectedDir) return;

    setIsValidating(true);
    setError(null);

    try {
      // Validate the selected directory
      await ValidateDataDirectory(selectedDir);

      // Initialize the data directory structure
      setIsInitializing(true);
      await InitializeDataDirectory(selectedDir);

      // Success - call the completion callback
      onComplete(selectedDir);
    } catch (err) {
      console.error('Failed to initialize:', err);
      setError(err instanceof Error ? err.message : 'Failed to initialize data directory');
      setIsInitializing(false);
    } finally {
      setIsValidating(false);
    }
  };

  const canContinue = getCurrentDirectory() && diskSpace?.hasSpace && !statusMessage.includes('Error');

  return (
    <div
      className="flex flex-col h-screen"
      style={{
        backgroundColor: '#f0f0f0',
      }}
    >
      {/* Main Content - fills entire window */}
      <div className="flex-1 px-6 py-4 overflow-hidden">
        {/* Welcome text - italic like in old GUI */}
        <p className="mb-3" style={{ color: '#000000', fontSize: '13px', fontStyle: 'italic' }}>
          {t('intro.welcome')}
        </p>

        {/* Explanation text */}
        <p className="mb-2" style={{ color: '#000000', fontSize: '13px' }}>
          {t('intro.chooseDataDir')}
        </p>

        {/* Size warning */}
        <p className="mb-3" style={{ color: '#000000', fontSize: '13px' }}>
          {t('intro.dataDirDescription')}
        </p>

        {/* Radio Options */}
        <div>
          {/* Default directory option */}
          <div className="mb-1">
            <label className="flex items-center cursor-pointer">
              <input
                type="radio"
                name="datadir"
                checked={useDefault}
                onChange={() => handleRadioChange(true)}
                className="mr-2"
                style={{
                  width: '16px',
                  height: '16px',
                  accentColor: '#007AFF'
                }}
              />
              <span style={{ color: '#000000', fontSize: '13px' }}>
                {t('intro.defaultDir')}
              </span>
            </label>
          </div>

          {/* Custom directory option */}
          <div className="mb-2">
            <label className="flex items-center cursor-pointer">
              <input
                type="radio"
                name="datadir"
                checked={!useDefault}
                onChange={() => handleRadioChange(false)}
                className="mr-2"
                style={{
                  width: '16px',
                  height: '16px',
                  accentColor: '#007AFF'
                }}
              />
              <span style={{ color: '#000000', fontSize: '13px' }}>
                {t('intro.customDir')}
              </span>
            </label>
          </div>

          {/* Custom directory input - slightly indented */}
          <div className="flex gap-2 mb-2" style={{ paddingLeft: '20px' }}>
            <input
              type="text"
              value={customDirectory}
              onChange={(e) => handleCustomDirectoryChange(e.target.value)}
              disabled={useDefault}
              className="flex-1"
              style={{
                padding: '4px 8px',
                fontSize: '13px',
                backgroundColor: useDefault ? '#f0f0f0' : '#ffffff',
                border: '1px solid #c0c0c0',
                borderRadius: '3px',
                color: useDefault ? '#999999' : '#000000',
                height: '26px',
              }}
            />
            <button
              onClick={handleBrowse}
              disabled={useDefault}
              style={{
                padding: '4px 12px',
                fontSize: '13px',
                backgroundColor: useDefault ? '#f0f0f0' : '#ffffff',
                border: '1px solid #c0c0c0',
                borderRadius: '3px',
                color: useDefault ? '#999999' : '#000000',
                cursor: useDefault ? 'default' : 'pointer',
                height: '26px',
                minWidth: '40px',
              }}
            >
              ...
            </button>
          </div>

          {/* Free space and status messages - slightly indented to align with input */}
          <div style={{ paddingLeft: '20px' }}>
            {diskSpace && (
              <p style={{
                color: diskSpace.hasSpace ? '#000000' : '#800000',
                fontSize: '13px',
                marginBottom: '4px'
              }}>
                {formatBytes(diskSpace.available)} GB of free space available.
              </p>
            )}

            {statusMessage && (
              <p style={{
                color: statusMessage.includes('Error') ? '#800000' : '#666666',
                fontSize: '13px'
              }}>
                {statusMessage}
              </p>
            )}
          </div>
        </div>

        {/* Error Message */}
        {error && (
          <div className="mt-3" style={{ color: '#800000', fontSize: '13px' }}>
            {error}
          </div>
        )}
      </div>

      {/* Footer Buttons - at bottom of window */}
      <div
        className="px-6 py-3 flex justify-end gap-3"
      >
        <button
          onClick={onCancel}
          disabled={isValidating || isInitializing}
          style={{
            padding: '4px 16px',
            fontSize: '13px',
            backgroundColor: '#ffffff',
            border: '1px solid #c0c0c0',
            borderRadius: '4px',
            color: '#000000',
            cursor: 'pointer',
            height: '28px',
          }}
        >
          {t('buttons.cancel')}
        </button>
        <button
          onClick={handleContinue}
          disabled={!canContinue || isValidating || isInitializing}
          style={{
            padding: '4px 20px',
            fontSize: '13px',
            backgroundColor: canContinue ? '#007AFF' : '#f0f0f0',
            border: canContinue ? '1px solid #005ACC' : '1px solid #c0c0c0',
            borderRadius: '4px',
            color: canContinue ? '#ffffff' : '#999999',
            cursor: canContinue ? 'pointer' : 'default',
            height: '28px',
            fontWeight: '500',
          }}
        >
          {t('buttons.ok')}
        </button>
      </div>
    </div>
  );
};

export default IntroDialog;