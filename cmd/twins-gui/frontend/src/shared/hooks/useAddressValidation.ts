import { useState, useEffect, useCallback, useRef } from 'react';
import { ValidateAddress } from '@wailsjs/go/main/App';
import { sanitizeAddress, isAddressSafe } from '@/utils/addressSanitizer';
import { sharedValidationCache } from '@/utils/validationCache';

export interface AddressValidation {
  isvalid: boolean;
  address: string;
  ismine: boolean;
  iswatchonly: boolean;
  isscript: boolean;
  pubkey: string;
  account: string;
}

export interface ValidationState {
  validation: AddressValidation | null;
  isValidating: boolean;
  error: string | null;
}

/**
 * Custom hook for validating TWINS addresses with debounced API calls
 *
 * Features:
 * - Debounced validation to minimize API calls
 * - Result caching with TTL (1 minute) to improve performance
 * - Request cancellation support for rapid input changes
 *
 * Note: AbortController is used to prevent state updates from stale requests,
 * but Wails doesn't support request cancellation at the transport level.
 * The backend call will complete but the result will be ignored if aborted.
 *
 * @param address - The address to validate
 * @param debounceMs - Debounce delay in milliseconds (default: 300ms)
 */
export function useAddressValidation(address: string, debounceMs: number = 300) {
  const [state, setState] = useState<ValidationState>({
    validation: null,
    isValidating: false,
    error: null
  });

  const abortControllerRef = useRef<AbortController | null>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef<boolean>(true);
  const requestIdRef = useRef<number>(0);

  const validateAddress = useCallback(async (addr: string) => {
    // Skip validation for empty addresses
    if (!addr || addr.trim() === '') {
      if (mountedRef.current) {
        setState({
          validation: null,
          isValidating: false,
          error: null
        });
      }
      return;
    }

    // Sanitize the address to prevent injection attacks
    const sanitizedAddress = sanitizeAddress(addr);

    // Check if address is safe to send to backend
    if (sanitizedAddress && !isAddressSafe(sanitizedAddress)) {
      if (mountedRef.current) {
        setState({
          validation: {
            isvalid: false,
            address: sanitizedAddress,
            ismine: false,
            iswatchonly: false,
            isscript: false,
            pubkey: '',
            account: ''
          },
          isValidating: false,
          error: 'Address contains invalid characters'
        });
      }
      return;
    }

    // Check shared cache first
    const cached = sharedValidationCache.get(sanitizedAddress);
    if (cached) {
      if (mountedRef.current) {
        setState({
          validation: cached.validation,
          isValidating: false,
          error: null
        });
      }
      return;
    }

    // Cancel any previous request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    // Create new abort controller for this request
    abortControllerRef.current = new AbortController();

    // Increment request ID to track the latest request
    const currentRequestId = ++requestIdRef.current;

    if (mountedRef.current) {
      setState(prev => ({
        ...prev,
        isValidating: true,
        error: null
      }));
    }

    try {
      const result = await ValidateAddress(sanitizedAddress);

      // Check if this is still the latest request, not aborted, and component is mounted
      if (currentRequestId !== requestIdRef.current ||
          abortControllerRef.current?.signal.aborted ||
          !mountedRef.current) {
        return;
      }

      // Cache the result in the shared cache
      sharedValidationCache.set(sanitizedAddress, result);

      if (mountedRef.current) {
        setState({
          validation: result,
          isValidating: false,
          error: null
        });
      }
    } catch (err) {
      // Don't update state if this isn't the latest request, was aborted, or component unmounted
      if (currentRequestId !== requestIdRef.current ||
          abortControllerRef.current?.signal.aborted ||
          !mountedRef.current) {
        return;
      }

      if (mountedRef.current) {
        setState({
          validation: null,
          isValidating: false,
          error: err instanceof Error ? err.message : 'Failed to validate address'
        });
      }
    }
  }, []);

  // Cleanup effect for mounted ref
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    // Clear any existing timeout
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    // If address is empty, clear validation immediately
    if (!address || address.trim() === '') {
      if (mountedRef.current) {
        setState({
          validation: null,
          isValidating: false,
          error: null
        });
      }
      return;
    }

    // Set up debounced validation
    timeoutRef.current = setTimeout(() => {
      validateAddress(address);
    }, debounceMs);

    // Cleanup function
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [address, debounceMs, validateAddress]);

  // Clear cache function for testing or manual refresh
  const clearCache = useCallback(() => {
    sharedValidationCache.clear();
  }, []);

  return {
    ...state,
    clearCache
  };
}

/**
 * Get validation status and message for UI display
 */
export function getValidationStatus(state: ValidationState): {
  status: 'idle' | 'validating' | 'valid' | 'invalid' | 'warning' | 'error';
  message: string;
} {
  if (state.error) {
    return {
      status: 'error',
      message: state.error
    };
  }

  if (state.isValidating) {
    return {
      status: 'validating',
      message: 'Validating address...'
    };
  }

  if (!state.validation) {
    return {
      status: 'idle',
      message: ''
    };
  }

  if (!state.validation.isvalid) {
    return {
      status: 'invalid',
      message: 'Invalid TWINS address format'
    };
  }

  if (state.validation.iswatchonly) {
    return {
      status: 'warning',
      message: 'This is a watch-only address (you do not control it)'
    };
  }

  if (state.validation.ismine) {
    return {
      status: 'valid',
      message: 'Your own address'
    };
  }

  return {
    status: 'valid',
    message: 'Valid TWINS address'
  };
}