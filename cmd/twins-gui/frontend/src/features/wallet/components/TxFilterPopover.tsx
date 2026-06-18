import React, { useEffect, useRef } from 'react';

interface TxFilterPopoverProps {
  /** Element the popover anchors to. When null, popover renders nothing. */
  anchorEl: HTMLElement | null;
  isOpen: boolean;
  onClose: () => void;
  children: React.ReactNode;
  /** Minimum width in px. Defaults to 240. */
  width?: number;
  /** Padding inside the popover card. Defaults to '12px'. */
  padding?: string;
}

/**
 * Shared popover shell for filter editors and the `+ Add filter` dropdown.
 * Mirrors the lifecycle pattern of the ExplorerButton popover in
 * TransactionDetailsDialog.tsx: positioned via anchorEl.getBoundingClientRect()
 * with viewport-overflow flip, closes on outside-mousedown / capture-phase
 * Escape / capture-phase scroll / window resize.
 *
 * Phase 1 of the chip-based filter bar.
 */
export const TxFilterPopover: React.FC<TxFilterPopoverProps> = ({
  anchorEl,
  isOpen,
  onClose,
  children,
  width = 240,
  padding = '12px',
}) => {
  const popoverRef = useRef<HTMLDivElement>(null);

  // Close on outside mousedown.
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (
        popoverRef.current &&
        !popoverRef.current.contains(target) &&
        anchorEl &&
        !anchorEl.contains(target)
      ) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [isOpen, anchorEl, onClose]);

  // Close on Escape. Capture phase + stopPropagation so parent dialogs don't
  // also fire their Escape handler.
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener('keydown', handler, true);
    return () => document.removeEventListener('keydown', handler, true);
  }, [isOpen, onClose]);

  // Close on scroll / resize: the popover uses fixed positioning captured at
  // open time; reflowing the parent would otherwise visually decouple it.
  // BUT: scrolls INSIDE the popover (e.g. Type editor's overflowY: auto list)
  // must NOT dismiss it. Filter the capture-phase scroll listener so it only
  // fires for scroll events whose target is OUTSIDE the popover element.
  useEffect(() => {
    if (!isOpen) return;
    const resizeHandler = () => onClose();
    const scrollHandler = (e: Event) => {
      const target = e.target as Node | null;
      // Document/window-level scrolls have target === document; treat as outside.
      if (target && popoverRef.current && popoverRef.current.contains(target)) {
        return;
      }
      onClose();
    };
    window.addEventListener('resize', resizeHandler);
    document.addEventListener('scroll', scrollHandler, true);
    return () => {
      window.removeEventListener('resize', resizeHandler);
      document.removeEventListener('scroll', scrollHandler, true);
    };
  }, [isOpen, onClose]);

  if (!isOpen || !anchorEl) return null;

  const rect = anchorEl.getBoundingClientRect();
  // Viewport-overflow flip: if the popover would overflow the right edge,
  // anchor its right edge to the anchor's right edge instead of left.
  const leftFromLeft = rect.left;
  const overflowsRight = leftFromLeft + width > window.innerWidth - 10;
  const positionStyle: React.CSSProperties = overflowsRight
    ? { right: window.innerWidth - rect.right, top: rect.bottom + 4 }
    : { left: leftFromLeft, top: rect.bottom + 4 };

  return (
    <div
      ref={popoverRef}
      style={{
        position: 'fixed',
        ...positionStyle,
        minWidth: `${width}px`,
        backgroundColor: '#2f2f2f',
        border: '1px solid #3a3a3a',
        borderRadius: '6px',
        padding,
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.6)',
        zIndex: 60,
      }}
    >
      {children}
    </div>
  );
};
