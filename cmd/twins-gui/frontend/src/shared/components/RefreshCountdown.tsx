import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';

export interface RefreshCountdownProps {
  countdown: number;
  total: number;
  mode: 'display' | 'interactive';
  onRefresh?: () => void;
  isLoading?: boolean;
  size?: number;
}

export const RefreshCountdown: React.FC<RefreshCountdownProps> = ({
  countdown,
  total,
  mode,
  onRefresh,
  isLoading = false,
  size = 28,
}) => {
  const { t } = useTranslation('masternode');
  const [isHovered, setIsHovered] = useState(false);

  const isInteractive = mode === 'interactive' && !isLoading;

  // Ring geometry
  const strokeWidth = Math.max(2, size / 14);
  const padding = 1;
  const radius = (size / 2) - (strokeWidth / 2) - padding;
  const circumference = 2 * Math.PI * radius;
  const center = size / 2;

  // Progress: fraction of time remaining (1 = full, 0 = empty)
  const progress = total > 0 ? Math.max(0, Math.min(countdown / total, 1)) : 0;
  const dashOffset = circumference * (1 - progress);

  // Colors
  const arcColor = isHovered && isInteractive ? '#2ecc71' : '#27ae60';
  const bgRingColor = '#3a3a3a';

  // Font size scales with ring
  const fontSize = Math.max(8, Math.round(size * 0.35));

  const handleClick = () => {
    if (isInteractive && onRefresh) {
      onRefresh();
    }
  };

  const tooltip = mode === 'interactive'
    ? t('refreshCountdown.clickToRefresh')
    : t('refreshCountdown.tooltip', { seconds: countdown });

  return (
    <div
      style={{
        width: size,
        height: size,
        position: 'relative',
        cursor: isInteractive ? 'pointer' : 'default',
        opacity: isLoading ? 0.5 : 1,
        transition: 'opacity 0.2s',
        flexShrink: 0,
      }}
      onClick={handleClick}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      title={tooltip}
      role={mode === 'interactive' ? 'button' : undefined}
      aria-label={tooltip}
    >
      <svg width={size} height={size} style={{ position: 'absolute', top: 0, left: 0 }}>
        {/* Background ring */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke={bgRingColor}
          strokeWidth={strokeWidth}
        />
        {/* Progress arc — depletes clockwise */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke={arcColor}
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={dashOffset}
          strokeLinecap="round"
          transform={`rotate(-90 ${center} ${center})`}
          style={{ transition: 'stroke-dashoffset 0.3s linear, stroke 0.15s' }}
        />
      </svg>

      {/* Center text: seconds remaining */}
      {!isLoading && (
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: size,
            height: size,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontSize,
            fontWeight: 500,
            color: '#ddd',
            lineHeight: 1,
            userSelect: 'none',
          }}
        >
          {countdown}
        </div>
      )}

      {/* Loading spinner */}
      {isLoading && (
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: size,
            height: size,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontSize: fontSize - 1,
            color: '#888',
            lineHeight: 1,
          }}
        >
          ...
        </div>
      )}
    </div>
  );
};
