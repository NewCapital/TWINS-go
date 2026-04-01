import React, { useEffect, useCallback, useRef, useState } from 'react';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { GetLatestBlocks, GetExplorerBlock, GetExplorerTransaction, GetExplorerAddress, GetAddressTransactions, ExplorerSearch } from '@wailsjs/go/main/App';
import { BlockList } from '../components/BlockList';
import { BlockDetail } from '../components/BlockDetail';
import { TransactionDetail } from '../components/TransactionDetail';
import { AddressView } from '../components/AddressView';
import { SearchBar } from '../components/SearchBar';
import type { AddressTx, AddressInfo } from '@/store/slices/explorerSlice';

const TX_BATCH_SIZE = 50;

export const ExplorerPage: React.FC = () => {
  const {
    view,
    blocks,
    currentBlock,
    currentTransaction,
    explorerAddress,
    searchQuery,
    parentContext,
    blocksPage,
    blocksPerPage,
    totalBlocks,
    isLoadingBlocks,
    isLoadingBlock,
    isLoadingTransaction,
    isLoadingAddress,
    isSearching,
    error,
    setView,
    setBlocks,
    setCurrentBlock,
    setCurrentTransaction,
    setExplorerAddress,
    setSearchQuery,
    setParentContext,
    setBlocksPage,
    setTotalBlocks,
    setLoadingBlocks,
    setLoadingBlock,
    setLoadingTransaction,
    setLoadingAddress,
    setSearching,
    setError,
  } = useStore(useShallow((state) => ({
    view: state.view,
    blocks: state.blocks,
    currentBlock: state.currentBlock,
    currentTransaction: state.currentTransaction,
    explorerAddress: state.explorerAddress,
    searchQuery: state.searchQuery,
    parentContext: state.parentContext,
    blocksPage: state.blocksPage,
    blocksPerPage: state.blocksPerPage,
    totalBlocks: state.totalBlocks,
    isLoadingBlocks: state.isLoadingBlocks,
    isLoadingBlock: state.isLoadingBlock,
    isLoadingTransaction: state.isLoadingTransaction,
    isLoadingAddress: state.isLoadingAddress,
    isSearching: state.isSearching,
    error: state.error,
    setView: state.setView,
    setBlocks: state.setBlocks,
    setCurrentBlock: state.setCurrentBlock,
    setCurrentTransaction: state.setCurrentTransaction,
    setExplorerAddress: state.setExplorerAddress,
    setSearchQuery: state.setSearchQuery,
    setParentContext: state.setParentContext,
    setBlocksPage: state.setBlocksPage,
    setTotalBlocks: state.setTotalBlocks,
    setLoadingBlocks: state.setLoadingBlocks,
    setLoadingBlock: state.setLoadingBlock,
    setLoadingTransaction: state.setLoadingTransaction,
    setLoadingAddress: state.setLoadingAddress,
    setSearching: state.setSearching,
    setError: state.setError,
  })));

  const isLoadingRef = useRef(false);
  const [isLoadingTx, setIsLoadingTx] = useState(false);
  const [txLoadedCount, setTxLoadedCount] = useState(0);
  const txLoadingRef = useRef(false);
  const currentAddressRef = useRef<string | null>(null);
  const addressInfoRef = useRef<AddressInfo | null>(null);
  const txTimeoutRef = useRef<number | null>(null);

  // Fetch latest blocks
  const fetchBlocks = useCallback(async (page: number = 0) => {
    if (isLoadingRef.current) return;
    isLoadingRef.current = true;
    setLoadingBlocks(true);
    setError(null);

    try {
      const offset = page * blocksPerPage;
      const result = await GetLatestBlocks(blocksPerPage, offset);
      if (result) {
        setBlocks(result);
        // Update total blocks from first block height if available
        if (result.length > 0 && page === 0) {
          setTotalBlocks(result[0].height + 1);
        }
      }
    } catch (err) {
      console.error('Failed to fetch blocks:', err);
      setError('Failed to fetch blocks');
    } finally {
      isLoadingRef.current = false;
      setLoadingBlocks(false);
    }
  }, [blocksPerPage, setBlocks, setTotalBlocks, setLoadingBlocks, setError]);

  // Fetch block details (from block list - no parent, or from navigation like Previous/Next)
  const fetchBlock = useCallback(async (query: string) => {
    // Block view always goes back to blocks list
    setParentContext(null);
    setLoadingBlock(true);
    setError(null);

    try {
      const result = await GetExplorerBlock(query);
      if (result) {
        setCurrentBlock(result);
        setView('block');
      }
    } catch (err) {
      console.error('Failed to fetch block:', err);
      setError('Block not found');
    } finally {
      setLoadingBlock(false);
    }
  }, [setParentContext, setCurrentBlock, setView, setLoadingBlock, setError]);

  // Fetch transaction details (from block - parent is block)
  const fetchTransaction = useCallback(async (txid: string) => {
    // Save current block as parent so Back returns to it
    if (view === 'block' && currentBlock) {
      setParentContext({ view: 'block', blockHash: currentBlock.hash });
    } else {
      setParentContext(null);
    }

    setLoadingTransaction(true);
    setError(null);

    try {
      const result = await GetExplorerTransaction(txid);
      if (result) {
        setCurrentTransaction(result);
        setView('transaction');
      }
    } catch (err) {
      console.error('Failed to fetch transaction:', err);
      setError('Transaction not found');
    } finally {
      setLoadingTransaction(false);
    }
  }, [view, currentBlock, setParentContext, setCurrentTransaction, setView, setLoadingTransaction, setError]);

  // Progressive transaction loading with race condition protection
  const loadTransactionsBatch = useCallback(async (address: string, offset: number, existingTxs: AddressTx[]) => {
    // Guard: check if already loading or address changed
    if (txLoadingRef.current) return;
    if (currentAddressRef.current !== address) return;

    txLoadingRef.current = true;
    setIsLoadingTx(true);

    try {
      const result = await GetAddressTransactions(address, TX_BATCH_SIZE, offset);

      // Double-check address hasn't changed during async operation
      if (currentAddressRef.current !== address) {
        return;
      }

      if (result) {
        const newTxs = [...existingTxs, ...result.transactions];

        // Calculate total_received and total_sent from transactions
        let totalReceived = 0;
        let totalSent = 0;
        for (const tx of newTxs) {
          if (tx.amount >= 0) {
            totalReceived += tx.amount;
          } else {
            totalSent += Math.abs(tx.amount);
          }
        }

        // Update store with new transactions using ref for base info
        if (addressInfoRef.current) {
          setExplorerAddress({
            ...addressInfoRef.current,
            transactions: newTxs,
            total_received: totalReceived,
            total_sent: totalSent,
          });
        }

        setTxLoadedCount(newTxs.length);

        // Continue loading if there's more
        if (result.has_more && currentAddressRef.current === address) {
          txLoadingRef.current = false;
          // Use setTimeout to avoid blocking UI, store ref for cleanup
          txTimeoutRef.current = window.setTimeout(() => {
            loadTransactionsBatch(address, offset + result.transactions.length, newTxs);
          }, 10);
        }
      }
    } catch (err) {
      console.error('Failed to fetch transactions batch:', err);
    } finally {
      txLoadingRef.current = false;
      setIsLoadingTx(false);
    }
  }, [setExplorerAddress]);

  // Fetch address info (basic info first, then transactions progressively)
  const fetchAddress = useCallback(async (address: string) => {
    // Keep the same parent context (the original block)
    // So Back from address goes to block, not to transaction

    // Reset transaction loading state
    currentAddressRef.current = address;
    txLoadingRef.current = false;
    setTxLoadedCount(0);
    setIsLoadingTx(false);

    setLoadingAddress(true);
    setError(null);

    try {
      // First, get basic address info (fast)
      const result = await GetExplorerAddress(address, 0);
      if (result) {
        // Store base address info in ref for batch updates
        addressInfoRef.current = result;

        // Set address with empty transactions, show page immediately
        setExplorerAddress({
          ...result,
          transactions: [],
        });
        setView('address');
        setLoadingAddress(false);

        // Start loading transactions progressively
        loadTransactionsBatch(address, 0, []);
      }
    } catch (err) {
      console.error('Failed to fetch address:', err);
      setError('Address not found');
      setLoadingAddress(false);
    }
  }, [setExplorerAddress, setView, setLoadingAddress, setError, loadTransactionsBatch]);

  // Search handler
  const handleSearch = useCallback(async (query: string) => {
    if (!query.trim()) return;

    setSearching(true);
    setSearchQuery(query);
    setError(null);

    try {
      const result = await ExplorerSearch(query);
      if (result) {
        switch (result.type) {
          case 'block':
            if (result.block) {
              setCurrentBlock(result.block);
              setView('block');
            }
            break;
          case 'transaction':
            if (result.transaction) {
              setCurrentTransaction(result.transaction);
              setView('transaction');
            }
            break;
          case 'address':
            if (result.address) {
              // For search, also use progressive loading
              currentAddressRef.current = result.address.address;
              addressInfoRef.current = result.address;
              txLoadingRef.current = false;
              setTxLoadedCount(0);
              setExplorerAddress({
                ...result.address,
                transactions: [],
              });
              setView('address');
              loadTransactionsBatch(result.address.address, 0, []);
            }
            break;
          case 'not_found':
            setError(`Nothing found for "${query}"`);
            break;
        }
      }
    } catch (err) {
      console.error('Search failed:', err);
      setError('Search failed');
    } finally {
      setSearching(false);
    }
  }, [setSearchQuery, setSearching, setCurrentBlock, setCurrentTransaction, setExplorerAddress, setView, setError, loadTransactionsBatch]);

  // Initial fetch
  useEffect(() => {
    fetchBlocks(0);
  }, [fetchBlocks]);

  // Cleanup on view change and component unmount
  useEffect(() => {
    if (view !== 'address') {
      currentAddressRef.current = null;
      addressInfoRef.current = null;
      txLoadingRef.current = false;
      // Cancel pending timeout to prevent memory leak
      if (txTimeoutRef.current) {
        clearTimeout(txTimeoutRef.current);
        txTimeoutRef.current = null;
      }
    }
  }, [view]);

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (txTimeoutRef.current) {
        clearTimeout(txTimeoutRef.current);
      }
    };
  }, []);

  // Page change handler
  const handlePageChange = (page: number) => {
    setBlocksPage(page);
    fetchBlocks(page);
  };

  // Navigation handlers
  const handleBlockClick = (query: string) => {
    fetchBlock(query);
  };

  const handleTxClick = (txid: string) => {
    fetchTransaction(txid);
  };

  const handleAddressClick = (address: string) => {
    fetchAddress(address);
  };

  const handleBack = useCallback(async () => {
    setError(null);
    currentAddressRef.current = null;

    // If we have a parent block, go back to it
    if (parentContext?.view === 'block' && parentContext.blockHash) {
      setLoadingBlock(true);
      try {
        const result = await GetExplorerBlock(parentContext.blockHash);
        if (result) {
          setCurrentBlock(result);
          setView('block');
          setParentContext(null); // Clear parent - block's back goes to list
        }
      } catch (err) {
        console.error('Failed to fetch parent block:', err);
        // Fallback to blocks list
        setView('blocks');
      } finally {
        setLoadingBlock(false);
      }
      return;
    }

    // Default: go back to blocks list
    setView('blocks');
    setCurrentBlock(null);
    setCurrentTransaction(null);
    setExplorerAddress(null);
    setParentContext(null);
  }, [parentContext, setView, setCurrentBlock, setCurrentTransaction, setExplorerAddress, setParentContext, setError, setLoadingBlock]);

  // Refresh handler
  const handleRefresh = () => {
    switch (view) {
      case 'blocks':
        fetchBlocks(blocksPage);
        break;
      case 'block':
        if (currentBlock) fetchBlock(currentBlock.hash);
        break;
      case 'transaction':
        if (currentTransaction) fetchTransaction(currentTransaction.txid);
        break;
      case 'address':
        if (explorerAddress) fetchAddress(explorerAddress.address);
        break;
    }
  };

  // Render current view
  const renderView = () => {
    switch (view) {
      case 'block':
        return (
          <BlockDetail
            block={currentBlock}
            isLoading={isLoadingBlock}
            onTxClick={handleTxClick}
            onBlockClick={handleBlockClick}
            onAddressClick={handleAddressClick}
            onBack={handleBack}
          />
        );
      case 'transaction':
        return (
          <TransactionDetail
            transaction={currentTransaction}
            isLoading={isLoadingTransaction}
            onAddressClick={handleAddressClick}
            onTxClick={handleTxClick}
            onBlockClick={handleBlockClick}
            onBack={handleBack}
          />
        );
      case 'address':
        return (
          <AddressView
            addressInfo={explorerAddress}
            isLoading={isLoadingAddress}
            isLoadingTx={isLoadingTx}
            txLoadedCount={txLoadedCount}
            onTxClick={handleTxClick}
            onBack={handleBack}
          />
        );
      default:
        return (
          <BlockList
            blocks={blocks}
            isLoading={isLoadingBlocks}
            currentPage={blocksPage}
            totalBlocks={totalBlocks}
            blocksPerPage={blocksPerPage}
            onBlockClick={handleBlockClick}
            onPageChange={handlePageChange}
          />
        );
    }
  };

  return (
    <div className="qt-frame" style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div className="qt-vbox" style={{ padding: '8px', height: '100%', display: 'flex', flexDirection: 'column' }}>
        {/* Page Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '8px' }}>
          <div className="qt-header-label" style={{ fontSize: '18px' }}>
            BLOCK EXPLORER
          </div>
          <button
            className="qt-button"
            onClick={handleRefresh}
            disabled={isLoadingBlocks || isLoadingBlock || isLoadingTransaction || isLoadingAddress || isSearching}
            style={{ padding: '4px 12px', fontSize: '11px' }}
          >
            Refresh
          </button>
        </div>

        {/* Search Bar */}
        <SearchBar
          value={searchQuery}
          isSearching={isSearching}
          onSearch={handleSearch}
          onChange={setSearchQuery}
        />

        {/* Error Message */}
        {error && (
          <div style={{
            padding: '8px 12px',
            marginBottom: '8px',
            fontSize: '12px',
            color: '#ff6666',
            backgroundColor: '#3a1a1a',
            border: '1px solid #ff6666',
            borderRadius: '2px',
          }}>
            {error}
          </div>
        )}

        {/* Content Area */}
        <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          {renderView()}
        </div>
      </div>
    </div>
  );
};

export { ExplorerPage as Explorer };
