import React from 'react';

export interface DashboardCardProps {
  title: string;
  children: React.ReactNode;
  headerLeft?: React.ReactNode;
  headerRight?: React.ReactNode;
  // Optional inline overrides merged into the card chrome. Intended for
  // layout-flow properties only (e.g. `flex`, `minHeight`, `maxHeight`,
  // `width`) so the card can participate in a flex column layout — the
  // Overview Recent Transactions card uses `{ flex: 1, minHeight: 0 }` to
  // stretch and fill the remaining viewport space below the 2x2 grid.
  // DO NOT override the card's intentional chrome tokens (`display`,
  // `flexDirection`, `gap`, `backgroundColor`, `border`, `borderRadius`,
  // `padding`) — those properties form the canonical Receive-language card
  // shell and changing them silently breaks the header/body flex layout.
  // The merge order (`{ ...cardStyle, ...style }`) lets `style` win on
  // conflicts; this prop is an escape hatch, not a theming surface.
  style?: React.CSSProperties;
}

const cardStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  padding: '16px 20px',
  display: 'flex',
  flexDirection: 'column',
  gap: '12px',
};

const headerStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: '8px',
};

const titleStyle: React.CSSProperties = {
  fontSize: '13px',
  fontWeight: 600,
  color: '#ccc',
};

const headerRightStyle: React.CSSProperties = {
  fontSize: '11px',
  color: '#888',
  fontWeight: 500,
  letterSpacing: '0.5px',
};

export const DashboardCard: React.FC<DashboardCardProps> = ({ title, children, headerLeft, headerRight, style }) => {
  return (
    <div style={style ? { ...cardStyle, ...style } : cardStyle}>
      <div style={headerStyle}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
          {headerLeft !== undefined && headerLeft !== null && headerLeft}
          <span style={titleStyle}>{title}</span>
        </div>
        {headerRight !== undefined && headerRight !== null && (
          // <div> (not <span>) so callers can pass block-level content
          // (e.g. flex rows with icons/buttons) without invalid HTML nesting.
          // Existing string callsites (BalancesStrip's `unitLabel`, etc.)
          // continue to render identically because the inline font tokens
          // are applied as-is via `style` regardless of the host element.
          <div style={headerRightStyle}>{headerRight}</div>
        )}
      </div>
      {children}
    </div>
  );
};
