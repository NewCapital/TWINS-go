import React from 'react';
import { useTranslation } from 'react-i18next';
import { UseFormRegister, FieldError } from 'react-hook-form';
import { Clipboard, BookOpen, UserPlus, X } from 'lucide-react';
import { useAddressValidation, getValidationStatus } from '@/shared/hooks/useAddressValidation';

// Quick client-side address format check (same regex as Send.tsx / AddressBookDialog.tsx)
const TWINS_ADDRESS_REGEX = /^[Wamn][a-km-zA-HJ-NP-Z1-9]{33}$/;

interface RecipientFieldErrors {
  address?: FieldError;
  amount?: FieldError;
  label?: FieldError;
}

interface RecipientFieldProps {
  index: number;
  register: UseFormRegister<any>;
  address: string;
  label: string;
  showRemoveButton: boolean;
  onRemove: () => void;
  onUseMaximum?: () => void;
  onAddressBookPick?: () => void;
  onSaveToAddressBook?: (address: string, label: string) => void;
  errors?: RecipientFieldErrors;
}

const RecipientFieldComponent: React.FC<RecipientFieldProps> = ({
  index,
  register,
  address,
  label,
  showRemoveButton,
  onRemove,
  onUseMaximum,
  onAddressBookPick,
  onSaveToAddressBook,
  errors
}) => {
  const { t } = useTranslation('wallet');
  // Use address validation hook for this specific recipient
  const addressValidation = useAddressValidation(address || '');
  const validationStatus = getValidationStatus(addressValidation);

  // Generate IDs for accessibility
  const addressInputId = `recipient-address-${index}`;
  const addressErrorId = `recipient-address-error-${index}`;
  const labelInputId = `recipient-label-${index}`;
  const amountInputId = `recipient-amount-${index}`;

  return (
    <div className="qt-vbox" style={{ gap: '8px' }}>
      {/* Recipient header with remove button - shown only when multiple recipients */}
      {showRemoveButton && (
        <div className="qt-hbox" style={{
          alignItems: 'center',
          justifyContent: 'space-between',
          marginBottom: '-4px',
        }}>
          <span className="qt-label" style={{
            fontSize: '11px',
            color: '#999',
          }}>
            {t('send.recipients.title', { number: index + 1 })}
          </span>
          <button
            type="button"
            onClick={onRemove}
            className="qt-button-icon"
            style={{
              padding: '3px 6px',
              minHeight: '24px',
              backgroundColor: '#404040',
              border: '1px solid #555',
              borderRadius: '2px',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '4px',
            }}
            title={t('send.recipients.remove')}
          >
            <X size={12} />
            <span style={{ fontSize: '11px' }}>{t('send.recipients.removeLabel')}</span>
          </button>
        </div>
      )}
      {/* Pay To Field */}
      <div className="qt-hbox" style={{ alignItems: 'center', gap: '6px' }}>
        <label
          htmlFor={addressInputId}
          className="qt-label"
          style={{
            width: '65px',
            textAlign: 'right',
            fontSize: '12px'
          }}
        >
          {t('send.payTo')}:
        </label>
        <div style={{ flex: 1, position: 'relative' }}>
          <input
            {...register(`recipients.${index}.address`)}
            id={addressInputId}
            type="text"
            className="qt-input"
            placeholder={t('send.payToPlaceholder')}
            autoCapitalize="off"
            autoCorrect="off"
            autoComplete="off"
            spellCheck={false}
            aria-invalid={!!errors?.address || validationStatus.status === 'invalid' || validationStatus.status === 'error'}
            aria-describedby={errors?.address?.message || validationStatus.message ? addressErrorId : undefined}
            style={{
              width: '100%',
              padding: '3px 5px',
              paddingRight: validationStatus.status !== 'idle' ? '25px' : '5px',
              fontSize: '11px',
              backgroundColor: '#2b2b2b',
              border: `1px solid ${
                errors?.address ? '#cc0000' :
                validationStatus.status === 'invalid' || validationStatus.status === 'error' ? '#cc0000' :
                validationStatus.status === 'valid' ? '#00aa00' :
                validationStatus.status === 'warning' ? '#ffaa00' :
                '#1a1a1a'
              }`,
              borderRadius: '2px'
            }}
          />
          {/* Validation status indicator for each recipient */}
          {validationStatus.status !== 'idle' && (
            <span style={{
              position: 'absolute',
              right: '5px',
              top: '50%',
              transform: 'translateY(-50%)',
              fontSize: '12px',
              pointerEvents: 'none'
            }}>
              {validationStatus.status === 'validating' && '⏳'}
              {validationStatus.status === 'valid' && '✓'}
              {validationStatus.status === 'invalid' && '✗'}
              {validationStatus.status === 'warning' && '⚠️'}
              {validationStatus.status === 'error' && '❌'}
            </span>
          )}
        </div>
        <button
          type="button"
          className="qt-button-icon"
          style={{
            padding: '3px',
            minWidth: '24px',
            height: '24px',
            backgroundColor: '#404040',
            border: '1px solid #555',
            borderRadius: '2px'
          }}
          title={t('common:buttons.paste')}
        >
          <Clipboard size={12} />
        </button>
        <button
          type="button"
          className="qt-button-icon"
          onClick={onAddressBookPick}
          style={{
            padding: '3px',
            minWidth: '24px',
            height: '24px',
            backgroundColor: '#404040',
            border: '1px solid #555',
            borderRadius: '2px',
            cursor: 'pointer',
          }}
          title={t('common:buttons.addressBook')}
        >
          <BookOpen size={12} />
        </button>
        {onSaveToAddressBook && (() => {
          // Enable when address passes async validation OR matches format regex
          // (regex fallback avoids delay when address is populated from picker)
          const addressOk = validationStatus.status === 'valid' || TWINS_ADDRESS_REGEX.test(address);
          const canSave = addressOk && !!label.trim();
          return (
            <button
              type="button"
              className="qt-button-icon"
              onClick={() => onSaveToAddressBook(address, label)}
              disabled={!canSave}
              style={{
                padding: '3px',
                minWidth: '24px',
                height: '24px',
                backgroundColor: canSave ? '#404040' : '#333',
                border: '1px solid #555',
                borderRadius: '2px',
                cursor: canSave ? 'pointer' : 'default',
                opacity: canSave ? 1 : 0.5,
              }}
              title={t('send.saveToAddressBook')}
            >
              <UserPlus size={12} />
            </button>
          );
        })()}
      </div>

      {/* Label Field */}
      <div className="qt-hbox" style={{ alignItems: 'center', gap: '6px' }}>
        <label
          htmlFor={labelInputId}
          className="qt-label"
          style={{
            width: '65px',
            textAlign: 'right',
            fontSize: '12px'
          }}
        >
          {t('send.label')}:
        </label>
        <input
          {...register(`recipients.${index}.label`)}
          id={labelInputId}
          type="text"
          className="qt-input"
          placeholder={t('send.labelPlaceholder')}
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          style={{
            flex: 1,
            padding: '3px 5px',
            fontSize: '11px',
            backgroundColor: '#2b2b2b',
            border: '1px solid #1a1a1a',
            borderRadius: '2px'
          }}
        />
      </div>

      {/* Amount Field */}
      <div className="qt-hbox" style={{ alignItems: 'center', gap: '6px' }}>
        <label
          htmlFor={amountInputId}
          className="qt-label"
          style={{
            width: '65px',
            textAlign: 'right',
            fontSize: '12px'
          }}
        >
          {t('send.amount')}:
        </label>
        <input
          {...register(`recipients.${index}.amount`)}
          id={amountInputId}
          type="text"
          className="qt-input"
          placeholder={t('send.amountPlaceholder')}
          aria-label={t('send.amount')}
          aria-invalid={!!errors?.amount}
          style={{
            flex: 1,
            padding: '3px 5px',
            fontSize: '11px',
            backgroundColor: '#2b2b2b',
            border: `1px solid ${errors?.amount ? '#cc0000' : '#1a1a1a'}`,
            borderRadius: '2px'
          }}
        />
        {onUseMaximum && (
          <button
            type="button"
            onClick={onUseMaximum}
            className="qt-button"
            style={{
              padding: '2px 6px',
              fontSize: '10px',
              backgroundColor: '#404040',
              border: '1px solid #555',
              borderRadius: '2px',
              cursor: 'pointer',
              whiteSpace: 'nowrap'
            }}
            title={t('send.recipients.useMaximum')}
          >
            {t('common:buttons.max')}
          </button>
        )}
        <span className="qt-label" style={{ fontSize: '11px', minWidth: '45px' }}>
          {t('common:units.twins')}
        </span>
      </div>

      {/* Form validation error messages */}
      {errors?.address && (
        <div
          id={addressErrorId}
          role="alert"
          aria-live="polite"
          style={{
            marginLeft: '71px',
            marginTop: '-4px',
            fontSize: '11px',
            color: '#cc0000'
          }}
        >
          {errors.address.message}
        </div>
      )}
      {errors?.amount && (
        <div
          role="alert"
          aria-live="polite"
          style={{
            marginLeft: '71px',
            marginTop: '-4px',
            fontSize: '11px',
            color: '#cc0000'
          }}
        >
          {errors.amount.message}
        </div>
      )}
      {/* Backend validation messages (only show if no form errors) */}
      {!errors?.address && validationStatus.message && (
        <div
          id={addressErrorId}
          role="alert"
          aria-live="polite"
          style={{
            marginLeft: '71px',
            marginTop: '-4px',
            fontSize: '11px',
            color: validationStatus.status === 'valid' ? '#00aa00' :
                   validationStatus.status === 'warning' ? '#ffaa00' : '#cc0000'
          }}
        >
          {validationStatus.message}
        </div>
      )}
    </div>
  );
};

// Export without memoization to ensure react-hook-form registration works properly
export const RecipientField = RecipientFieldComponent;