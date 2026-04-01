import React from 'react';
import { Loader2 } from 'lucide-react';
import '@/styles/qt-theme.css';

interface LoadingSpinnerProps {
  size?: 'small' | 'medium' | 'large';
  message?: string;
  overlay?: boolean;
}

export const LoadingSpinner: React.FC<LoadingSpinnerProps> = ({
  size = 'medium',
  message,
  overlay = false,
}) => {
  const sizeMap = {
    small: 16,
    medium: 24,
    large: 32,
  };

  const spinner = (
    <div className="flex flex-col items-center justify-center gap-2 qt-fade-in">
      <Loader2
        size={sizeMap[size]}
        className="qt-loading"
        style={{ color: 'var(--qt-icon-overview)' }}
      />
      {message && (
        <span
          className="qt-small-text"
          style={{ color: 'var(--qt-text-secondary)' }}
        >
          {message}
        </span>
      )}
    </div>
  );

  if (overlay) {
    return (
      <div
        className="absolute inset-0 flex items-center justify-center qt-fade-in"
        style={{
          backgroundColor: 'rgba(0, 0, 0, 0.7)',
          backdropFilter: 'blur(2px)',
          zIndex: 999,
        }}
      >
        {spinner}
      </div>
    );
  }

  return spinner;
};

// Loading skeleton for content placeholders
interface LoadingSkeletonProps {
  height?: string;
  width?: string;
  className?: string;
}

export const LoadingSkeleton: React.FC<LoadingSkeletonProps> = ({
  height = '20px',
  width = '100%',
  className = '',
}) => {
  return (
    <div
      className={`qt-loading-pulse ${className}`}
      style={{
        height,
        width,
        backgroundColor: 'var(--qt-bg-secondary)',
        borderRadius: '4px',
      }}
    />
  );
};

// Loading dots for inline loading states
export const LoadingDots: React.FC = () => {
  return (
    <span className="inline-flex gap-1">
      <span
        className="qt-loading-pulse"
        style={{
          width: '4px',
          height: '4px',
          borderRadius: '50%',
          backgroundColor: 'var(--qt-text-secondary)',
          animationDelay: '0ms',
        }}
      />
      <span
        className="qt-loading-pulse"
        style={{
          width: '4px',
          height: '4px',
          borderRadius: '50%',
          backgroundColor: 'var(--qt-text-secondary)',
          animationDelay: '200ms',
        }}
      />
      <span
        className="qt-loading-pulse"
        style={{
          width: '4px',
          height: '4px',
          borderRadius: '50%',
          backgroundColor: 'var(--qt-text-secondary)',
          animationDelay: '400ms',
        }}
      />
    </span>
  );
};