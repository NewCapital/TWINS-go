import React from 'react';

export interface SendFeeControlsProps {
  feeRate: number;
  sliderPosition: number;
  onSliderChange: (position: number, rate: number) => void;
  estimateFeeAvailable?: boolean;
  onChooseCustomFee?: () => void;
}

export const SendFeeControls: React.FC<SendFeeControlsProps> = ({
  feeRate,
  sliderPosition,
  onSliderChange,
  estimateFeeAvailable = true,
  onChooseCustomFee,
}) => {
  const handleSliderChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const position = parseInt(e.target.value);
    // Calculate fee rate based on slider position
    // 0 = 0.0001 TWINS/kB (normal), 100 = 0.001 TWINS/kB (fast)
    const minRate = 0.0001;
    const maxRate = 0.001;
    const rate = minRate + (maxRate - minRate) * (position / 100);
    onSliderChange(position, rate);
  };

  return (
    <div className="qt-frame-secondary" style={{
      marginBottom: '8px',
      padding: '8px',
      border: '1px solid #4a4a4a',
      borderRadius: '2px',
      backgroundColor: '#3a3a3a'
    }}>
      <div className="qt-vbox" style={{ gap: '12px' }}>
        {/* Confirmation Time Slider */}
        <div className="qt-hbox" style={{ alignItems: 'center', gap: '8px' }}>
          <span className="qt-label" style={{ fontSize: '11px', whiteSpace: 'nowrap' }}>
            Confirmation time:
          </span>
          <span className="qt-label" style={{ fontSize: '11px', color: '#aaa' }}>
            normal
          </span>
          <input
            type="range"
            min="0"
            max="100"
            value={sliderPosition}
            onChange={handleSliderChange}
            style={{
              flex: 1,
              height: '4px',
              borderRadius: '2px',
              backgroundColor: '#555',
              outline: 'none',
              cursor: 'pointer',
            }}
          />
          <span className="qt-label" style={{ fontSize: '11px', color: '#aaa' }}>
            fast
          </span>
        </div>

        {/* Smart Fee Message / Warning */}
        <div style={{
          fontSize: '11px',
          color: estimateFeeAvailable ? '#999' : '#e6a700',
          fontStyle: 'italic',
          textAlign: 'center',
          padding: '4px 0',
          backgroundColor: estimateFeeAvailable ? 'transparent' : 'rgba(230, 167, 0, 0.1)',
          borderRadius: '2px'
        }}>
          {estimateFeeAvailable
            ? '(Smart fee not initialized yet. This usually takes a few blocks...)'
            : '⚠ Fee estimation unavailable - using default fee rate. Ensure sufficient balance for fees.'}
        </div>

        {/* Transaction Fee Display */}
        <div className="qt-hbox" style={{ alignItems: 'center', gap: '12px' }}>
          <span className="qt-label" style={{ fontSize: '12px', fontWeight: 'bold' }}>
            Transaction Fee:
          </span>
          <span className="qt-label" style={{ fontSize: '12px' }}>
            {feeRate.toFixed(8)} TWINS/kB
          </span>
          <button
            type="button"
            className="qt-button"
            style={{ padding: '3px 12px', fontSize: '11px', marginLeft: 'auto' }}
            onClick={onChooseCustomFee}
            disabled={!onChooseCustomFee}
          >
            Choose...
          </button>
        </div>
      </div>
    </div>
  );
};
