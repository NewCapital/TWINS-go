import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Bookmark, Check, Pencil, Trash2, Plus, MoreHorizontal } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTransactions } from '@/store/useStore';
import {
  matchesViewSnapshot,
  type CurrentTransactionsState,
} from '@/shared/utils/transactionViewMatching';
import { TxFilterPopover } from './TxFilterPopover';
import { SimpleConfirmDialog } from '@/shared/components/SimpleConfirmDialog';
import { IconButton } from '@/shared/components/IconButton';

const pillButton: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: '6px',
  padding: '6px 12px',
  fontSize: '12px',
  fontWeight: 500,
  color: '#ccc',
  backgroundColor: '#383838',
  border: '1px solid #4a4a4a',
  borderRadius: '999px',
  cursor: 'pointer',
  transition: 'background-color 0.15s, border-color 0.15s',
  whiteSpace: 'nowrap',
};

const sectionHeaderStyle: React.CSSProperties = {
  padding: '6px 12px 2px',
  fontSize: '10px',
  fontWeight: 500,
  color: '#888',
  textTransform: 'uppercase',
  letterSpacing: '0.5px',
};

const viewRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '6px',
  padding: '6px 8px 6px 12px',
  fontSize: '12px',
  color: '#ddd',
  backgroundColor: 'transparent',
  border: 'none',
  borderRadius: '4px',
  cursor: 'pointer',
  width: '100%',
  textAlign: 'left',
  transition: 'background-color 0.15s',
};

const dividerStyle: React.CSSProperties = {
  height: '1px',
  backgroundColor: '#3a3a3a',
  margin: '4px 0',
};

const inputStyle: React.CSSProperties = {
  backgroundColor: '#252525',
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  padding: '6px 8px',
  fontSize: '12px',
  color: '#ddd',
  outline: 'none',
  width: '100%',
};

const footerActionStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '6px',
  padding: '8px 12px',
  fontSize: '12px',
  fontWeight: 500,
  color: '#27ae60',
  backgroundColor: 'transparent',
  border: 'none',
  cursor: 'pointer',
  width: '100%',
  textAlign: 'left',
  borderRadius: '4px',
};

export const TxViewsMenu: React.FC = () => {
  const { t } = useTranslation('wallet');
  const {
    views,
    dateFilter,
    dateRangeFrom,
    dateRangeTo,
    typeFilter,
    searchText,
    minAmount,
    maxAmount,
    watchOnlyFilter,
    sortColumn,
    sortDirection,
    applyView,
    saveCurrentAs,
    renameView,
    deleteView,
    loadViews,
  } = useTransactions();

  const [isOpen, setIsOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const overflowRefs = useRef<Map<string, HTMLDivElement>>(new Map());

  // Inline editing state.
  const [savingNew, setSavingNew] = useState(false);
  const [newViewName, setNewViewName] = useState('');
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState('');
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  // Overflow menu open for a specific view id.
  const [overflowOpenId, setOverflowOpenId] = useState<string | null>(null);

  // One-shot lazy-seed on mount: ensures defaults populate even if the
  // component renders before any other code touches the views slice.
  useEffect(() => {
    loadViews();
    // loadViews is a stable Zustand action reference.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Dismiss the overflow sub-popover (Rename / Delete menu) on outside
  // mousedown and on capture-phase Escape. Without this, clicking elsewhere in
  // the parent Views popover leaves the overflow lingering, and pressing
  // Escape would dismiss the entire Views popover instead of just the
  // overflow. Matches the dismissal pattern used by TxFilterPopover.
  useEffect(() => {
    if (!overflowOpenId) return;
    const overflowEl = overflowRefs.current.get(overflowOpenId);
    const mouseHandler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (overflowEl && !overflowEl.contains(target)) {
        setOverflowOpenId(null);
      }
    };
    const keyHandler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        setOverflowOpenId(null);
      }
    };
    document.addEventListener('mousedown', mouseHandler);
    document.addEventListener('keydown', keyHandler, true);
    return () => {
      document.removeEventListener('mousedown', mouseHandler);
      document.removeEventListener('keydown', keyHandler, true);
    };
  }, [overflowOpenId]);

  const currentState: CurrentTransactionsState = useMemo(
    () => ({
      dateFilter,
      dateRangeFrom,
      dateRangeTo,
      typeFilter,
      searchText,
      minAmount,
      maxAmount,
      watchOnlyFilter,
      sortColumn,
      sortDirection,
    }),
    [
      dateFilter,
      dateRangeFrom,
      dateRangeTo,
      typeFilter,
      searchText,
      minAmount,
      maxAmount,
      watchOnlyFilter,
      sortColumn,
      sortDirection,
    ],
  );

  const matchingView = useMemo(
    () => views.find((v) => matchesViewSnapshot(currentState, v)) ?? null,
    [views, currentState],
  );

  const defaultViews = useMemo(() => views.filter((v) => v.isDefault), [views]);
  const userViews = useMemo(() => views.filter((v) => !v.isDefault), [views]);

  const handleClose = () => {
    setIsOpen(false);
    setSavingNew(false);
    setNewViewName('');
    setRenamingId(null);
    setRenameDraft('');
    setOverflowOpenId(null);
  };

  const handleApply = (id: string) => {
    applyView(id);
    handleClose();
  };

  const handleStartRename = (id: string, currentName: string) => {
    setRenamingId(id);
    setRenameDraft(currentName);
    setOverflowOpenId(null);
  };

  const handleCommitRename = () => {
    // Pass the trimmed draft for symmetry with saveCurrentAs's UI commit path.
    // The slice trims internally too, but mirroring the convention here keeps
    // any future UI-side length validation consistent with what's persisted.
    const trimmed = renameDraft.trim();
    if (renamingId && trimmed) {
      renameView(renamingId, trimmed);
    }
    setRenamingId(null);
    setRenameDraft('');
  };

  const handleStartSaveNew = () => {
    setSavingNew(true);
    setNewViewName('');
  };

  const handleCommitSaveNew = () => {
    const trimmed = newViewName.trim();
    if (trimmed) {
      saveCurrentAs(trimmed);
      setSavingNew(false);
      setNewViewName('');
    }
  };

  const handleConfirmDelete = () => {
    if (confirmDeleteId) {
      deleteView(confirmDeleteId);
    }
    setConfirmDeleteId(null);
  };

  // B5 (2026-05-22): when no saved view matches AND the current selection
  // differs from the canonical default (filter cleared, default `date desc`
  // sort), surface a `Views: Custom` label so the user sees their current
  // selection is unsaved. Bare `Views` is reserved for the truly-untouched
  // empty state so the pill doesn't shout at users who haven't customized
  // anything. Includes sort-only customizations per Codex R5 W1.
  const hasActiveFilters =
    dateFilter !== 'all' ||
    typeFilter.length > 0 ||
    searchText !== '' ||
    minAmount !== '' ||
    maxAmount !== '' ||
    watchOnlyFilter !== 'all' ||
    sortColumn !== 'date' ||
    sortDirection !== 'desc';

  const triggerLabel = matchingView
    ? t('transactions.views.viewsButtonActive', { name: matchingView.name })
    : hasActiveFilters
      ? t('transactions.views.viewsButtonActive', { name: t('transactions.views.customLabel') })
      : t('transactions.views.viewsButton');

  const renderViewRow = (
    view: typeof views[number],
    isMatching: boolean,
    showOverflow: boolean,
  ) => {
    const isRenaming = renamingId === view.id;
    return (
      <div
        key={view.id}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '4px',
          position: 'relative',
        }}
      >
        {isRenaming ? (
          <div style={{ flex: 1, padding: '4px 8px 4px 12px' }}>
            <input
              type="text"
              autoFocus
              value={renameDraft}
              onChange={(e) => setRenameDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  handleCommitRename();
                } else if (e.key === 'Escape') {
                  e.preventDefault();
                  setRenamingId(null);
                  setRenameDraft('');
                }
              }}
              style={inputStyle}
              aria-label={t('transactions.views.renameLabel')}
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              autoComplete="off"
            />
          </div>
        ) : (
          <button
            type="button"
            style={{
              ...viewRowStyle,
              color: isMatching ? '#27ae60' : '#ddd',
              fontWeight: isMatching ? 600 : 400,
            }}
            onClick={() => handleApply(view.id)}
            onMouseEnter={(e) => {
              e.currentTarget.style.backgroundColor = '#383838';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.backgroundColor = 'transparent';
            }}
          >
            <span style={{ width: '14px', display: 'inline-flex', justifyContent: 'center', flexShrink: 0 }}>
              {isMatching ? <Check size={12} color="#27ae60" /> : null}
            </span>
            <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {view.name}
            </span>
          </button>
        )}

        {showOverflow && !isRenaming && (
          <div style={{ paddingRight: '4px', flexShrink: 0 }}>
            <IconButton
              icon={<MoreHorizontal size={12} />}
              title={t('transactions.views.overflowLabel')}
              ariaLabel={t('transactions.views.overflowLabel')}
              onClick={() =>
                setOverflowOpenId(overflowOpenId === view.id ? null : view.id)
              }
            />
          </div>
        )}

        {overflowOpenId === view.id && (
          <div
            ref={(el) => {
              if (el) {
                overflowRefs.current.set(view.id, el);
              } else {
                overflowRefs.current.delete(view.id);
              }
            }}
            style={{
              position: 'absolute',
              top: '100%',
              right: '4px',
              marginTop: '2px',
              backgroundColor: '#2f2f2f',
              border: '1px solid #3a3a3a',
              borderRadius: '6px',
              padding: '4px',
              boxShadow: '0 4px 12px rgba(0, 0, 0, 0.6)',
              zIndex: 70,
              minWidth: '140px',
            }}
            role="menu"
          >
            <button
              type="button"
              role="menuitem"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                padding: '6px 10px',
                fontSize: '12px',
                color: '#ddd',
                backgroundColor: 'transparent',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer',
                width: '100%',
                textAlign: 'left',
              }}
              onClick={() => handleStartRename(view.id, view.name)}
              onMouseEnter={(e) => {
                e.currentTarget.style.backgroundColor = '#383838';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = 'transparent';
              }}
            >
              <Pencil size={12} />
              {t('transactions.views.renameLabel')}
            </button>
            <button
              type="button"
              role="menuitem"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                padding: '6px 10px',
                fontSize: '12px',
                color: '#ff6666',
                backgroundColor: 'transparent',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer',
                width: '100%',
                textAlign: 'left',
              }}
              onClick={() => {
                setOverflowOpenId(null);
                setConfirmDeleteId(view.id);
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.backgroundColor = '#383838';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = 'transparent';
              }}
            >
              <Trash2 size={12} />
              {t('transactions.views.deleteLabel')}
            </button>
          </div>
        )}
      </div>
    );
  };

  const deleteTarget = confirmDeleteId ? views.find((v) => v.id === confirmDeleteId) : null;

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        style={pillButton}
        onClick={() => setIsOpen(!isOpen)}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        title={triggerLabel}
        onMouseEnter={(e) => {
          e.currentTarget.style.backgroundColor = '#444';
          e.currentTarget.style.borderColor = '#5a5a5a';
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.backgroundColor = '#383838';
          e.currentTarget.style.borderColor = '#4a4a4a';
        }}
      >
        <Bookmark size={12} />
        <span style={{ maxWidth: '160px', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          {triggerLabel}
        </span>
      </button>

      <TxFilterPopover
        anchorEl={triggerRef.current}
        isOpen={isOpen}
        onClose={handleClose}
        width={260}
        padding="4px"
      >
        <div style={{ display: 'flex', flexDirection: 'column' }} role="menu" aria-label={t('transactions.views.viewsButton')}>
          {/* Default views section */}
          <div style={sectionHeaderStyle}>
            {t('transactions.views.defaultViewsHeader')}
          </div>
          {defaultViews.map((view) =>
            renderViewRow(view, matchingView?.id === view.id, false),
          )}

          <div style={dividerStyle} />

          {/* User views section */}
          <div style={sectionHeaderStyle}>
            {t('transactions.views.userViewsHeader')}
          </div>
          {userViews.length === 0 ? (
            <div
              style={{
                padding: '8px 12px',
                fontSize: '11px',
                fontStyle: 'italic',
                color: '#666',
              }}
            >
              {t('transactions.views.noSavedViews')}
            </div>
          ) : (
            userViews.map((view) =>
              renderViewRow(view, matchingView?.id === view.id, true),
            )
          )}

          <div style={dividerStyle} />

          {/* Footer: Save current view as... */}
          {savingNew ? (
            <div style={{ padding: '8px 12px', display: 'flex', flexDirection: 'column', gap: '6px' }}>
              <input
                type="text"
                autoFocus
                value={newViewName}
                onChange={(e) => setNewViewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleCommitSaveNew();
                  } else if (e.key === 'Escape') {
                    e.preventDefault();
                    setSavingNew(false);
                    setNewViewName('');
                  }
                }}
                style={inputStyle}
                placeholder={t('transactions.views.savePlaceholder')}
                aria-label={t('transactions.views.savePlaceholder')}
                autoCapitalize="off"
                autoCorrect="off"
                spellCheck={false}
                autoComplete="off"
              />
            </div>
          ) : (
            <button
              type="button"
              style={footerActionStyle}
              onClick={handleStartSaveNew}
              onMouseEnter={(e) => {
                e.currentTarget.style.backgroundColor = '#383838';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = 'transparent';
              }}
            >
              <Plus size={12} />
              {t('transactions.views.saveCurrentAs')}
            </button>
          )}
        </div>
      </TxFilterPopover>

      {deleteTarget && (
        <SimpleConfirmDialog
          isOpen={true}
          title={t('transactions.views.deleteConfirmTitle')}
          message={t('transactions.views.deleteConfirmMessage', { name: deleteTarget.name })}
          confirmText={t('transactions.views.deleteConfirmButton')}
          cancelText={t('transactions.views.cancelButton')}
          isDestructive={true}
          onConfirm={handleConfirmDelete}
          onCancel={() => setConfirmDeleteId(null)}
          zIndex={1010}
        />
      )}
    </>
  );
};
