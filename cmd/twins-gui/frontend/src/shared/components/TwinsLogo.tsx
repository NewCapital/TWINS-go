import React from 'react';

interface TwinsLogoProps {
  size?: number;
  className?: string;
}

export const TwinsLogo: React.FC<TwinsLogoProps> = ({ size = 48, className = '' }) => {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 100 100"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
    >
      {/* Background circle */}
      <circle
        cx="50"
        cy="50"
        r="45"
        fill="#1a1a1a"
        stroke="#2a2a2a"
        strokeWidth="2"
      />

      {/* W letter shape with gradient */}
      <defs>
        <linearGradient id="twinsGradient" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor="#27ae60" />
          <stop offset="50%" stopColor="#3498db" />
          <stop offset="100%" stopColor="#e74c3c" />
        </linearGradient>
      </defs>

      {/* Stylized W shape */}
      <path
        d="M 20 30 L 30 65 L 40 40 L 50 65 L 60 40 L 70 65 L 80 30"
        stroke="url(#twinsGradient)"
        strokeWidth="4"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
      />

      {/* Double W effect for "twins" concept */}
      <path
        d="M 20 30 L 30 65 L 40 40 L 50 65 L 60 40 L 70 65 L 80 30"
        stroke="url(#twinsGradient)"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
        opacity="0.5"
        transform="translate(0, 3)"
      />

      {/* Center dot accent */}
      <circle
        cx="50"
        cy="50"
        r="3"
        fill="#27ae60"
      />
    </svg>
  );
};

// Alternative simpler logo
export const TwinsLogoSimple: React.FC<TwinsLogoProps> = ({ size = 48, className = '' }) => {
  return (
    <div
      className={className}
      style={{
        width: size,
        height: size,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'linear-gradient(135deg, #27ae60 0%, #3498db 100%)',
        borderRadius: '8px',
        position: 'relative',
      }}
    >
      <div
        style={{
          fontSize: size * 0.5,
          fontWeight: 'bold',
          color: '#ffffff',
          textShadow: '0 2px 4px rgba(0,0,0,0.3)',
        }}
      >
        W
      </div>
      <div
        style={{
          fontSize: size * 0.15,
          color: '#ffffff',
          position: 'absolute',
          bottom: '15%',
          letterSpacing: '1px',
          fontWeight: '600',
        }}
      >
        TWINS
      </div>
    </div>
  );
};