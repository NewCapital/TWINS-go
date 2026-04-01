import React from 'react';
import { useTranslation } from 'react-i18next';
import type { BlockSummary } from '@/store/slices/explorerSlice';
import { ChevronLeft, ChevronRight } from 'lucide-react';

interface BlockListProps {
  blocks: BlockSummary[];
  isLoading: boolean;
  currentPage: number;
  totalBlocks: number;
  blocksPerPage: number;
  onBlockClick: (query: string) => void;
  onPageChange: (page: number) => void;
}

export const BlockList: React.FC<BlockListProps> = ({
  blocks,
  isLoading,
  currentPage,
  totalBlocks,
  blocksPerPage,
  onBlockClick,
  onPageChange,
}) => {
  const { t } = useTranslation('common');
  const totalPages = Math.ceil(totalBlocks / blocksPerPage);

  const formatTime = (timeStr: string) => {
    const date = new Date(timeStr);
    return date.toLocaleString();
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Table Header */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '80px 1fr 160px 80px 80px',
          gap: '8px',
          padding: '8px 12px',
          backgroundColor: '#1a1a1a',
          borderBottom: '1px solid #3a3a3a',
          fontSize: '11px',
          fontWeight: 'bold',
          color: '#888888',
        }}
      >
        <div>{t('explorer.height')}</div>
        <div>{t('explorer.hash')}</div>
        <div>{t('explorer.time')}</div>
        <div style={{ textAlign: 'center' }}>{t('explorer.txCount')}</div>
        <div style={{ textAlign: 'right' }}>{t('explorer.size')}</div>
      </div>

      {/* Table Body */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {isLoading ? (
          <div style={{ padding: '24px', textAlign: 'center', color: '#888888' }}>
            {t('explorer.loadingBlocks')}
          </div>
        ) : blocks.length === 0 ? (
          <div style={{ padding: '24px', textAlign: 'center', color: '#888888' }}>
            {t('explorer.noBlocks')}
          </div>
        ) : (
          blocks.map((block) => (
            <div
              key={block.hash}
              onClick={() => onBlockClick(block.hash)}
              style={{
                display: 'grid',
                gridTemplateColumns: '80px 1fr 160px 80px 80px',
                gap: '8px',
                padding: '8px 12px',
                borderBottom: '1px solid #2a2a2a',
                fontSize: '11px',
                cursor: 'pointer',
                transition: 'background-color 0.15s',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.backgroundColor = '#252525';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = 'transparent';
              }}
            >
              <div style={{ color: '#4a9eff' }}>{block.height}</div>
              <div
                style={{
                  color: '#dddddd',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  fontFamily: 'monospace',
                }}
              >
                {block.hash}
              </div>
              <div style={{ color: '#888888' }}>{formatTime(block.time)}</div>
              <div style={{ textAlign: 'center', color: '#dddddd' }}>{block.tx_count}</div>
              <div style={{ textAlign: 'right', color: '#888888' }}>{block.size} B</div>
            </div>
          ))
        )}
      </div>

      {/* Pagination */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '8px 12px',
          borderTop: '1px solid #3a3a3a',
          backgroundColor: '#1a1a1a',
        }}
      >
        <div style={{ fontSize: '11px', color: '#888888' }}>
          {totalBlocks > 0 && (
            <>
              {t('explorer.showing', {
                start: currentPage * blocksPerPage + 1,
                end: Math.min((currentPage + 1) * blocksPerPage, totalBlocks),
                total: totalBlocks
              })}
            </>
          )}
        </div>
        <div style={{ display: 'flex', gap: '4px' }}>
          <button
            className="qt-button"
            onClick={() => onPageChange(currentPage - 1)}
            disabled={currentPage === 0 || isLoading}
            style={{ padding: '4px 8px', fontSize: '11px' }}
          >
            <ChevronLeft size={14} />
          </button>
          <span style={{ padding: '4px 12px', fontSize: '11px', color: '#888888' }}>
            {t('explorer.page', { current: currentPage + 1, total: totalPages || 1 })}
          </span>
          <button
            className="qt-button"
            onClick={() => onPageChange(currentPage + 1)}
            disabled={currentPage >= totalPages - 1 || isLoading}
            style={{ padding: '4px 8px', fontSize: '11px' }}
          >
            <ChevronRight size={14} />
          </button>
        </div>
      </div>
    </div>
  );
};
