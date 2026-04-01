/**
 * ConfirmationRing Component
 * Renders a circular SVG progress ring around a transaction type icon
 * to indicate confirmation progress (0-6+ confirmations).
 *
 * Replaces the legacy PNG overlay icons (clock1-5.png, transaction0/2.png)
 * with a clean, scalable CSS/SVG solution.
 *
 * Pulse animation class (.confirmation-ring-pulse) is defined in qt-theme.css.
 */

import React from 'react';

interface ConfirmationRingProps {
  typeIcon: string;
  confirmations: number;
  isConflicted?: boolean;
  isCoinstake?: boolean;
  maturesIn?: number;
  size: number;
}

const CONFIRMED_THRESHOLD = 6;
const MATURITY_THRESHOLD = 60;

export const ConfirmationRing: React.FC<ConfirmationRingProps> = ({
  typeIcon,
  confirmations,
  isConflicted = false,
  isCoinstake = false,
  maturesIn = 0,
  size,
}) => {
  const confs = Math.max(0, confirmations);
  const isImmature = isCoinstake && maturesIn > 0;
  const isConfirmed = confs >= CONFIRMED_THRESHOLD && !isConflicted && !isImmature;

  // Ring geometry
  const strokeWidth = Math.max(2, size / 18);
  const padding = 1;
  const radius = (size / 2) - (strokeWidth / 2) - padding;
  const circumference = 2 * Math.PI * radius;
  const center = size / 2;

  // Icon size leaves room for ring + gap
  const iconSize = size - (strokeWidth + padding) * 2 - 2;

  // No ring needed for confirmed transactions
  if (isConfirmed) {
    return (
      <div style={{ width: size, height: size, position: 'relative' }}>
        <img
          src={typeIcon}
          alt="Transaction Type"
          style={{
            width: size,
            height: size,
            objectFit: 'contain',
          }}
        />
      </div>
    );
  }

  // Immature coinstake: orange maturity progress ring (out of 60 blocks)
  if (isImmature) {
    const maturedBlocks = MATURITY_THRESHOLD - maturesIn;
    const maturityProgress = Math.min(Math.max(maturedBlocks, 0) / MATURITY_THRESHOLD, 1);
    const maturityDashOffset = circumference * (1 - maturityProgress);
    return (
      <div style={{ width: size, height: size, position: 'relative', overflow: 'visible' }}>
        <svg width={size} height={size} style={{ position: 'absolute', top: 0, left: 0 }}>
          <circle cx={center} cy={center} r={radius} fill="none" stroke="#4a4a4a" strokeWidth={strokeWidth} />
          <circle
            cx={center} cy={center} r={radius} fill="none"
            stroke="#f97316"
            strokeWidth={strokeWidth}
            strokeDasharray={circumference}
            strokeDashoffset={maturityDashOffset}
            strokeLinecap="round"
            transform={`rotate(-90 ${center} ${center})`}
          />
        </svg>
        <img
          src={typeIcon}
          alt="Transaction Type"
          style={{
            position: 'absolute',
            top: (size - iconSize) / 2,
            left: (size - iconSize) / 2,
            width: iconSize,
            height: iconSize,
            objectFit: 'contain',
          }}
        />
      </div>
    );
  }

  // Colors
  const needsRedRing = confs === 0 || isConflicted;
  const ringColor = needsRedRing ? '#ef4444' : '#22c55e';
  const bgRingColor = '#4a4a4a';

  // Progress: 0 confs or conflicted = full ring in red, 1-5 = partial green arc
  const showFullRing = needsRedRing;
  const progress = showFullRing ? 0 : Math.min(confs / CONFIRMED_THRESHOLD, 1);
  const dashOffset = circumference * (1 - progress);

  return (
    <div
      style={{
        width: size,
        height: size,
        position: 'relative',
        overflow: 'visible',
      }}
      className={confs === 0 && !isConflicted ? 'confirmation-ring-pulse' : undefined}
    >
      {/* SVG ring */}
      <svg
        width={size}
        height={size}
        style={{ position: 'absolute', top: 0, left: 0 }}
      >
        {showFullRing ? (
          /* 0-conf or conflicted: single full-color ring */
          <circle
            cx={center}
            cy={center}
            r={radius}
            fill="none"
            stroke={ringColor}
            strokeWidth={strokeWidth}
          />
        ) : (
          /* 1-5 confs: background ring + green progress arc */
          <>
            <circle
              cx={center}
              cy={center}
              r={radius}
              fill="none"
              stroke={bgRingColor}
              strokeWidth={strokeWidth}
            />
            <circle
              cx={center}
              cy={center}
              r={radius}
              fill="none"
              stroke={ringColor}
              strokeWidth={strokeWidth}
              strokeDasharray={circumference}
              strokeDashoffset={dashOffset}
              strokeLinecap="round"
              transform={`rotate(-90 ${center} ${center})`}
            />
          </>
        )}
      </svg>

      {/* Type icon centered inside */}
      <img
        src={typeIcon}
        alt="Transaction Type"
        style={{
          position: 'absolute',
          top: (size - iconSize) / 2,
          left: (size - iconSize) / 2,
          width: iconSize,
          height: iconSize,
          objectFit: 'contain',
        }}
      />

      {/* Conflicted badge */}
      {isConflicted && (
        <div
          style={{
            position: 'absolute',
            bottom: -1,
            right: -1,
            width: Math.max(14, size / 3),
            height: Math.max(14, size / 3),
            borderRadius: '50%',
            backgroundColor: '#ef4444',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontSize: Math.max(9, size / 4.5),
            fontWeight: 'bold',
            color: '#fff',
            lineHeight: 1,
            border: '1.5px solid #2b2b2b',
          }}
        >
          !
        </div>
      )}
    </div>
  );
};
