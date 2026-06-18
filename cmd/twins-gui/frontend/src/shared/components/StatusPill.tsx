import React from 'react';

export type StatusPillTone = 'success' | 'warning' | 'error' | 'neutral';

const PILL_COLORS: Record<StatusPillTone, string> = {
  success: '#27ae60',
  warning: '#ff9966',
  error: '#ff6666',
  neutral: '#888888',
};

const hexToRGBA = (hex: string, alpha: number): string => {
  const h = hex.replace('#', '');
  const r = parseInt(h.substring(0, 2), 16);
  const g = parseInt(h.substring(2, 4), 16);
  const b = parseInt(h.substring(4, 6), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
};

export interface StatusPillProps {
  tone: StatusPillTone;
  label: string;
}

export const StatusPill: React.FC<StatusPillProps> = ({ tone, label }) => {
  const color = PILL_COLORS[tone];
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        backgroundColor: hexToRGBA(color, 0.15),
        border: `1px solid ${hexToRGBA(color, 0.4)}`,
        borderRadius: '999px',
        padding: '2px 8px',
        fontSize: '11px',
        fontWeight: 500,
        color,
        whiteSpace: 'nowrap',
      }}
    >
      {label}
    </span>
  );
};
