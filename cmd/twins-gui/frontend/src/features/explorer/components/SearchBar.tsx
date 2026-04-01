import React, { useState } from 'react';
import { Search } from 'lucide-react';

interface SearchBarProps {
  value: string;
  isSearching: boolean;
  onSearch: (query: string) => void;
  onChange: (value: string) => void;
}

export const SearchBar: React.FC<SearchBarProps> = ({
  value,
  isSearching,
  onSearch,
  onChange,
}) => {
  const [localValue, setLocalValue] = useState(value);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (localValue.trim()) {
      onSearch(localValue.trim());
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setLocalValue(e.target.value);
    onChange(e.target.value);
  };

  return (
    <form onSubmit={handleSubmit} style={{ marginBottom: '12px' }}>
      <div style={{ display: 'flex', gap: '8px' }}>
        <div style={{ flex: 1, position: 'relative' }}>
          <input
            type="text"
            value={localValue}
            onChange={handleChange}
            placeholder="Search by block hash, height, transaction ID, or address..."
            className="qt-input"
            style={{
              width: '100%',
              padding: '8px 12px',
              paddingLeft: '36px',
              fontSize: '12px',
              backgroundColor: '#1e1e1e',
              border: '1px solid #3a3a3a',
              borderRadius: '2px',
              color: '#dddddd',
            }}
            disabled={isSearching}
          />
          <Search
            size={16}
            style={{
              position: 'absolute',
              left: '10px',
              top: '50%',
              transform: 'translateY(-50%)',
              color: '#666666',
            }}
          />
        </div>
        <button
          type="submit"
          className="qt-button qt-button-primary"
          disabled={isSearching || !localValue.trim()}
          style={{
            padding: '8px 16px',
            fontSize: '12px',
            minWidth: '80px',
          }}
        >
          {isSearching ? 'Searching...' : 'Search'}
        </button>
      </div>
    </form>
  );
};
