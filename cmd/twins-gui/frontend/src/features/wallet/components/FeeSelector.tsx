import React, { useState } from 'react';

interface FeeSelectorProps {
  feeRate: number; // Current fee rate in TWINS/kB
  onChooseFee: () => void;
}

export const FeeSelector: React.FC<FeeSelectorProps> = ({
  feeRate,
  onChooseFee,
}) => {
  const [sliderValue, setSliderValue] = useState(0); // 0 = normal, 100 = fast

  // Format fee rate for display (8 decimal places)
  const formatFeeRate = (rate: number): string => {
    return rate.toFixed(8);
  };

  return (
    <div className="qt-vbox" style={{ gap: '8px' }}>
      {/* Confirmation Time Slider */}
      <div className="qt-hbox" style={{ alignItems: 'center', gap: '8px' }}>
        <span className="qt-label" style={{ fontSize: '12px' }}>
          Confirmation time:
        </span>
        <span className="qt-label" style={{ fontSize: '12px' }}>
          normal
        </span>
        <input
          type="range"
          min="0"
          max="100"
          value={sliderValue}
          onChange={(e) => setSliderValue(parseInt(e.target.value))}
          style={{
            flex: 1,
            height: '4px',
            backgroundColor: '#4a4a4a',
            borderRadius: '2px',
            cursor: 'pointer',
          }}
        />
        <span className="qt-label" style={{ fontSize: '12px' }}>
          fast
        </span>
      </div>

      {/* Smart Fee Message */}
      <div className="qt-label" style={{
        fontSize: '11px',
        color: '#888',
        textAlign: 'center',
      }}>
        (Smart fee not initialized yet. This usually takes a few blocks...)
      </div>

      {/* Transaction Fee Display */}
      <div className="qt-hbox" style={{
        justifyContent: 'space-between',
        alignItems: 'center',
        marginTop: '4px',
      }}>
        <span className="qt-label" style={{ fontSize: '12px' }}>
          Transaction Fee:
        </span>
        <div className="qt-hbox" style={{ alignItems: 'center', gap: '8px' }}>
          <span className="qt-label" style={{ fontSize: '12px' }}>
            {formatFeeRate(feeRate)} TWINS/kB
          </span>
          <button
            type="button"
            onClick={onChooseFee}
            className="qt-button"
            style={{
              padding: '2px 10px',
              fontSize: '11px',
            }}
          >
            Choose...
          </button>
        </div>
      </div>
    </div>
  );
};
