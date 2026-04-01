import React, { useState, useEffect, useCallback, useRef } from 'react';
import { X, Copy, Check, ChevronDown, AlertCircle, CheckCircle2 } from 'lucide-react';
import { useSignVerify } from '@/store/useStore';
import {
  SignMessage,
  VerifyMessage,
  GetReceivingAddresses,
  CopyToClipboard,
} from '@wailsjs/go/main/App';
import { UnlockWalletDialog } from './UnlockWalletDialog';
import { useWalletAction } from '@/shared/hooks/useWalletAction';
import type { SignVerifyTab } from '@/store/slices/signVerifySlice';

interface WalletAddress {
  address: string;
  label: string;
}

export const SignVerifyMessageDialog: React.FC = () => {
  const {
    isSignVerifyDialogOpen,
    signVerifyActiveTab,
    closeSignVerifyDialog,
    setSignVerifyActiveTab,
  } = useSignVerify();

  // Sign tab state
  const [signAddress, setSignAddress] = useState('');
  const [signMessage, setSignMessage] = useState('');
  const [signResult, setSignResult] = useState('');
  const [signError, setSignError] = useState('');
  const [isSigning, setIsSigning] = useState(false);
  const [signCopied, setSignCopied] = useState(false);

  // Verify tab state
  const [verifyAddress, setVerifyAddress] = useState('');
  const [verifyMessage, setVerifyMessage] = useState('');
  const [verifySignature, setVerifySignature] = useState('');
  const [verifyResult, setVerifyResult] = useState<'valid' | 'invalid' | null>(null);
  const [verifyError, setVerifyError] = useState('');
  const [isVerifying, setIsVerifying] = useState(false);

  // Shared state
  const [walletAddresses, setWalletAddresses] = useState<WalletAddress[]>([]);
  const [showAddressDropdown, setShowAddressDropdown] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const mountedRef = useRef(true);

  // Wallet unlock hook — restore to prior state (e.g. staking-only) after signing
  const { showUnlockDialog, executeWithUnlock, unlockDialogProps } = useWalletAction({
    restoreAfter: true,
  });

  // Load wallet addresses when dialog opens
  useEffect(() => {
    mountedRef.current = true;
    if (isSignVerifyDialogOpen) {
      GetReceivingAddresses().then((addresses) => {
        if (mountedRef.current) {
          setWalletAddresses(
            addresses.map((a: any) => ({ address: a.address, label: a.label }))
          );
        }
      }).catch(() => {});
    }
    return () => { mountedRef.current = false; };
  }, [isSignVerifyDialogOpen]);

  // Close address dropdown on click outside
  useEffect(() => {
    const handleMouseDown = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setShowAddressDropdown(false);
      }
    };
    if (showAddressDropdown) {
      document.addEventListener('mousedown', handleMouseDown);
    }
    return () => document.removeEventListener('mousedown', handleMouseDown);
  }, [showAddressDropdown]);

  const handleClose = useCallback(() => {
    closeSignVerifyDialog();
  }, [closeSignVerifyDialog]);

  // Handle Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isSignVerifyDialogOpen && !showUnlockDialog) {
        handleClose();
      }
    };
    if (isSignVerifyDialogOpen) {
      document.addEventListener('keydown', handleKeyDown);
    }
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isSignVerifyDialogOpen, handleClose, showUnlockDialog]);

  const performSign = useCallback(async () => {
    setIsSigning(true);
    setSignError('');

    try {
      const signature = await SignMessage(signAddress.trim(), signMessage);
      if (mountedRef.current) {
        setSignResult(signature);
      }
    } catch (err: any) {
      if (mountedRef.current) {
        setSignError(err?.message || String(err));
      }
    } finally {
      if (mountedRef.current) {
        setIsSigning(false);
      }
    }
  }, [signAddress, signMessage]);

  const handleSign = useCallback(async () => {
    setSignError('');
    setSignResult('');

    if (!signAddress.trim()) {
      setSignError('Please enter a TWINS address.');
      return;
    }
    if (!signMessage.trim()) {
      setSignError('Please enter a message to sign.');
      return;
    }

    // Unlock wallet if needed, then sign
    await executeWithUnlock(async () => {
      await performSign();
    });
  }, [signAddress, signMessage, executeWithUnlock, performSign]);

  const handleVerify = useCallback(async () => {
    setVerifyError('');
    setVerifyResult(null);

    if (!verifyAddress.trim()) {
      setVerifyError('Please enter a TWINS address.');
      return;
    }
    if (!verifyMessage.trim()) {
      setVerifyError('Please enter the original message.');
      return;
    }
    if (!verifySignature.trim()) {
      setVerifyError('Please enter the signature to verify.');
      return;
    }

    setIsVerifying(true);

    try {
      const valid = await VerifyMessage(verifyAddress.trim(), verifySignature.trim(), verifyMessage);
      if (mountedRef.current) {
        setVerifyResult(valid ? 'valid' : 'invalid');
      }
    } catch (err: any) {
      if (mountedRef.current) {
        setVerifyError(err?.message || String(err));
      }
    } finally {
      if (mountedRef.current) {
        setIsVerifying(false);
      }
    }
  }, [verifyAddress, verifyMessage, verifySignature]);

  const handleCopySignature = useCallback(async () => {
    try {
      await CopyToClipboard(signResult);
      setSignCopied(true);
      setTimeout(() => setSignCopied(false), 2000);
    } catch {
      // Fallback
      navigator.clipboard?.writeText(signResult);
      setSignCopied(true);
      setTimeout(() => setSignCopied(false), 2000);
    }
  }, [signResult]);

  const handleClearSign = useCallback(() => {
    setSignAddress('');
    setSignMessage('');
    setSignResult('');
    setSignError('');
    setSignCopied(false);
  }, []);

  const handleClearVerify = useCallback(() => {
    setVerifyAddress('');
    setVerifyMessage('');
    setVerifySignature('');
    setVerifyResult(null);
    setVerifyError('');
  }, []);

  const handleSelectAddress = useCallback((address: string) => {
    setSignAddress(address);
    setShowAddressDropdown(false);
  }, []);

  if (!isSignVerifyDialogOpen) return null;

  const tabs: { id: SignVerifyTab; label: string }[] = [
    { id: 'sign', label: 'Sign Message' },
    { id: 'verify', label: 'Verify Message' },
  ];

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
          width: '640px',
          backgroundColor: '#2b2b2b',
          borderRadius: '8px',
          boxShadow: '0 4px 20px rgba(0, 0, 0, 0.5)',
          zIndex: 1001,
          display: 'flex',
          flexDirection: 'column',
          maxHeight: '90vh',
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
            Sign/Verify Message
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
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setSignVerifyActiveTab(tab.id)}
              style={{
                padding: '12px 24px',
                backgroundColor: signVerifyActiveTab === tab.id ? '#2b2b2b' : 'transparent',
                border: 'none',
                borderBottom: signVerifyActiveTab === tab.id ? '2px solid #4a9eff' : '2px solid transparent',
                color: signVerifyActiveTab === tab.id ? '#fff' : '#aaa',
                cursor: 'pointer',
                fontSize: '13px',
                fontWeight: signVerifyActiveTab === tab.id ? 500 : 400,
                transition: 'all 0.15s ease',
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Content Area */}
        <div style={{ padding: '20px', overflowY: 'auto' }}>
          {signVerifyActiveTab === 'sign' ? (
            <SignTab
              address={signAddress}
              message={signMessage}
              result={signResult}
              error={signError}
              isSigning={isSigning}
              copied={signCopied}
              walletAddresses={walletAddresses}
              showDropdown={showAddressDropdown}
              dropdownRef={dropdownRef}
              onAddressChange={setSignAddress}
              onMessageChange={setSignMessage}
              onSign={handleSign}
              onCopy={handleCopySignature}
              onClear={handleClearSign}
              onToggleDropdown={() => setShowAddressDropdown(!showAddressDropdown)}
              onSelectAddress={handleSelectAddress}
            />
          ) : (
            <VerifyTab
              address={verifyAddress}
              message={verifyMessage}
              signature={verifySignature}
              result={verifyResult}
              error={verifyError}
              isVerifying={isVerifying}
              onAddressChange={setVerifyAddress}
              onMessageChange={setVerifyMessage}
              onSignatureChange={setVerifySignature}
              onVerify={handleVerify}
              onClear={handleClearVerify}
            />
          )}
        </div>
      </div>

      {/* Unlock Wallet Dialog */}
      {showUnlockDialog && (
        <UnlockWalletDialog
          isOpen={showUnlockDialog}
          {...unlockDialogProps}
          zIndex={1002}
          temporaryUnlock
        />
      )}
    </>
  );
};

// ==========================================
// Sign Tab
// ==========================================

interface SignTabProps {
  address: string;
  message: string;
  result: string;
  error: string;
  isSigning: boolean;
  copied: boolean;
  walletAddresses: WalletAddress[];
  showDropdown: boolean;
  dropdownRef: React.RefObject<HTMLDivElement | null>;
  onAddressChange: (v: string) => void;
  onMessageChange: (v: string) => void;
  onSign: () => void;
  onCopy: () => void;
  onClear: () => void;
  onToggleDropdown: () => void;
  onSelectAddress: (addr: string) => void;
}

const SignTab: React.FC<SignTabProps> = ({
  address, message, result, error, isSigning, copied,
  walletAddresses, showDropdown, dropdownRef,
  onAddressChange, onMessageChange, onSign, onCopy, onClear,
  onToggleDropdown, onSelectAddress,
}) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
    {/* Description */}
    <p style={{ color: '#aaa', fontSize: '13px', margin: 0 }}>
      You can sign messages with your addresses to prove you own them. Be careful not to sign
      anything vague, as phishing attacks may try to trick you into signing your identity over to them.
      Only sign fully-detailed statements you agree to.
    </p>

    {/* Address Field */}
    <div>
      <label style={labelStyle}>TWINS Address</label>
      <div style={{ position: 'relative' }} ref={dropdownRef}>
        <div style={{ display: 'flex' }}>
          <input
            type="text"
            value={address}
            onChange={(e) => onAddressChange(e.target.value)}
            placeholder="Enter a TWINS address (e.g., W...)"
            style={{ ...inputStyle, flex: 1, borderTopRightRadius: 0, borderBottomRightRadius: 0 }}
          />
          <button
            onClick={onToggleDropdown}
            style={{
              ...buttonSecondaryStyle,
              borderTopLeftRadius: 0,
              borderBottomLeftRadius: 0,
              borderLeft: 'none',
              padding: '0 10px',
            }}
            title="Choose from wallet addresses"
          >
            <ChevronDown size={16} />
          </button>
        </div>
        {showDropdown && walletAddresses.length > 0 && (
          <div style={dropdownStyle}>
            {walletAddresses.map((wa) => (
              <div
                key={wa.address}
                onClick={() => onSelectAddress(wa.address)}
                style={dropdownItemStyle}
              >
                <span style={{ color: '#ddd', fontSize: '12px', fontFamily: 'monospace' }}>{wa.address}</span>
                {wa.label && <span style={{ color: '#888', fontSize: '11px', marginLeft: '8px' }}>({wa.label})</span>}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>

    {/* Message Field */}
    <div>
      <label style={labelStyle}>Message</label>
      <textarea
        value={message}
        onChange={(e) => onMessageChange(e.target.value)}
        placeholder="Enter the message you want to sign"
        rows={4}
        style={{ ...inputStyle, resize: 'vertical', minHeight: '80px' }}
      />
    </div>

    {/* Signature Output */}
    {result && (
      <div>
        <label style={labelStyle}>Signature</label>
        <div style={{ display: 'flex', gap: '8px' }}>
          <textarea
            readOnly
            value={result}
            rows={3}
            style={{ ...inputStyle, flex: 1, resize: 'none', fontFamily: 'monospace', fontSize: '12px', color: '#4caf50' }}
          />
          <button
            onClick={onCopy}
            style={{ ...buttonSecondaryStyle, alignSelf: 'flex-start', display: 'flex', alignItems: 'center', gap: '4px' }}
            title="Copy signature"
          >
            {copied ? <Check size={14} /> : <Copy size={14} />}
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
      </div>
    )}

    {/* Error */}
    {error && (
      <div style={errorStyle}>
        <AlertCircle size={14} />
        {error}
      </div>
    )}

    {/* Buttons */}
    <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
      <button onClick={onClear} style={buttonSecondaryStyle}>
        Clear All
      </button>
      <button
        onClick={onSign}
        disabled={isSigning}
        style={buttonPrimaryStyle}
      >
        {isSigning ? 'Signing...' : 'Sign Message'}
      </button>
    </div>
  </div>
);

// ==========================================
// Verify Tab
// ==========================================

interface VerifyTabProps {
  address: string;
  message: string;
  signature: string;
  result: 'valid' | 'invalid' | null;
  error: string;
  isVerifying: boolean;
  onAddressChange: (v: string) => void;
  onMessageChange: (v: string) => void;
  onSignatureChange: (v: string) => void;
  onVerify: () => void;
  onClear: () => void;
}

const VerifyTab: React.FC<VerifyTabProps> = ({
  address, message, signature, result, error, isVerifying,
  onAddressChange, onMessageChange, onSignatureChange, onVerify, onClear,
}) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
    {/* Description */}
    <p style={{ color: '#aaa', fontSize: '13px', margin: 0 }}>
      Enter the signing address, message, and signature below to verify the message.
      The message must match exactly — including line breaks, spaces, and tabs — or verification
      will fail. Do not add or remove whitespace when pasting. Be careful not to read more into
      the signature than what is in the signed message itself, to avoid being tricked by a
      man-in-the-middle attack.
    </p>

    {/* Address Field */}
    <div>
      <label style={labelStyle}>TWINS Address</label>
      <input
        type="text"
        value={address}
        onChange={(e) => onAddressChange(e.target.value)}
        placeholder="Enter the signer's TWINS address"
        style={inputStyle}
      />
    </div>

    {/* Message Field */}
    <div>
      <label style={labelStyle}>Message</label>
      <textarea
        value={message}
        onChange={(e) => onMessageChange(e.target.value)}
        placeholder="Enter the signed message"
        rows={4}
        style={{ ...inputStyle, resize: 'vertical', minHeight: '80px' }}
      />
    </div>

    {/* Signature Field */}
    <div>
      <label style={labelStyle}>Signature</label>
      <textarea
        value={signature}
        onChange={(e) => onSignatureChange(e.target.value)}
        placeholder="Enter the signature (base64)"
        rows={3}
        style={{ ...inputStyle, resize: 'vertical', fontFamily: 'monospace', fontSize: '12px' }}
      />
    </div>

    {/* Verification Result */}
    {result === 'valid' && (
      <div style={successStyle}>
        <CheckCircle2 size={16} />
        Message verification successful. The signature is valid and the message was signed by the owner
        of the provided address.
      </div>
    )}
    {result === 'invalid' && (
      <div style={errorStyle}>
        <AlertCircle size={16} />
        Message verification failed. The signature does not match the provided address and message.
      </div>
    )}

    {/* Error */}
    {error && (
      <div style={errorStyle}>
        <AlertCircle size={14} />
        {error}
      </div>
    )}

    {/* Buttons */}
    <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
      <button onClick={onClear} style={buttonSecondaryStyle}>
        Clear All
      </button>
      <button
        onClick={onVerify}
        disabled={isVerifying}
        style={buttonPrimaryStyle}
      >
        {isVerifying ? 'Verifying...' : 'Verify Message'}
      </button>
    </div>
  </div>
);

// ==========================================
// Shared Styles
// ==========================================

const labelStyle: React.CSSProperties = {
  display: 'block',
  color: '#ccc',
  fontSize: '13px',
  marginBottom: '6px',
  fontWeight: 500,
};

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '10px 12px',
  backgroundColor: '#1e1e1e',
  border: '1px solid #555',
  borderRadius: '4px',
  color: '#ddd',
  fontSize: '13px',
  outline: 'none',
  boxSizing: 'border-box',
};

const buttonPrimaryStyle: React.CSSProperties = {
  padding: '10px 20px',
  backgroundColor: '#4a9eff',
  color: '#fff',
  border: 'none',
  borderRadius: '4px',
  cursor: 'pointer',
  fontSize: '13px',
  fontWeight: 500,
};

const buttonSecondaryStyle: React.CSSProperties = {
  padding: '10px 16px',
  backgroundColor: '#3a3a3a',
  color: '#ccc',
  border: '1px solid #555',
  borderRadius: '4px',
  cursor: 'pointer',
  fontSize: '13px',
};

const errorStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '8px',
  padding: '10px 14px',
  backgroundColor: 'rgba(211, 47, 47, 0.15)',
  border: '1px solid rgba(211, 47, 47, 0.3)',
  borderRadius: '4px',
  color: '#ff6b6b',
  fontSize: '13px',
};

const successStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '8px',
  padding: '10px 14px',
  backgroundColor: 'rgba(76, 175, 80, 0.15)',
  border: '1px solid rgba(76, 175, 80, 0.3)',
  borderRadius: '4px',
  color: '#4caf50',
  fontSize: '13px',
};

const dropdownStyle: React.CSSProperties = {
  position: 'absolute',
  top: '100%',
  left: 0,
  right: 0,
  backgroundColor: '#333',
  border: '1px solid #555',
  borderRadius: '0 0 4px 4px',
  maxHeight: '200px',
  overflowY: 'auto',
  zIndex: 10,
};

const dropdownItemStyle: React.CSSProperties = {
  padding: '8px 12px',
  cursor: 'pointer',
  borderBottom: '1px solid #444',
  display: 'flex',
  alignItems: 'center',
};
