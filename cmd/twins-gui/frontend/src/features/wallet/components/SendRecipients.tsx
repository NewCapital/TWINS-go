import React from 'react';
import { UseFormRegister, FieldArrayWithId, FieldErrors } from 'react-hook-form';
import { RecipientField } from './RecipientField';

// CSS for dark theme scrollbar
const scrollbarStyles = `
  .recipient-scroll-container::-webkit-scrollbar {
    width: 8px;
  }
  .recipient-scroll-container::-webkit-scrollbar-track {
    background: #2b2b2b;
    border-radius: 4px;
  }
  .recipient-scroll-container::-webkit-scrollbar-thumb {
    background: #555;
    border-radius: 4px;
  }
  .recipient-scroll-container::-webkit-scrollbar-thumb:hover {
    background: #666;
  }
`;

interface RecipientData {
  address: string;
  amount: string;
  label?: string;
}

export interface SendRecipientsProps {
  fields: FieldArrayWithId<any, 'recipients', 'id'>[];
  register: UseFormRegister<any>;
  watchedRecipients: RecipientData[];
  onRemove: (index: number) => void;
  onUseMaximum: (index: number) => void;
  onAddressBookPick?: (index: number) => void;
  onSaveToAddressBook?: (address: string, label: string) => void;
  errors?: FieldErrors<{ recipients: RecipientData[] }>;
}

export const SendRecipients: React.FC<SendRecipientsProps> = ({
  fields,
  register,
  watchedRecipients,
  onRemove,
  onUseMaximum,
  onAddressBookPick,
  onSaveToAddressBook,
  errors,
}) => {
  return (
    <>
      <style>{scrollbarStyles}</style>
      <div
        className="qt-frame-secondary recipient-scroll-container"
        style={{
          marginBottom: '8px',
          padding: '8px',
          border: '1px solid #4a4a4a',
          borderRadius: '2px',
          backgroundColor: '#3a3a3a',
          maxHeight: '280px',
          overflowY: 'auto',
          overflowX: 'hidden',
          scrollbarWidth: 'thin',
          scrollbarColor: '#555 #2b2b2b'
        }}>
        {fields.map((field, index) => (
          <div key={field.id}>
            {/* Add separator between recipients */}
            {index > 0 && (
              <div style={{
                borderTop: '1px solid #4a4a4a',
                margin: '8px 0',
              }} />
            )}

            <RecipientField
              index={index}
              register={register}
              address={watchedRecipients?.[index]?.address || ''}
              label={watchedRecipients?.[index]?.label || ''}
              showRemoveButton={fields.length > 1}
              onRemove={() => onRemove(index)}
              onUseMaximum={() => onUseMaximum(index)}
              onAddressBookPick={onAddressBookPick ? () => onAddressBookPick(index) : undefined}
              onSaveToAddressBook={onSaveToAddressBook}
              errors={errors?.recipients?.[index]}
            />
          </div>
        ))}
      </div>
    </>
  );
};
