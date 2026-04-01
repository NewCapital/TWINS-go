/**
 * Shared validation cache for address validation
 * This prevents memory bloat when multiple recipients use validation
 */

interface CachedValidation {
  validation: any;
  timestamp: number;
}

class ValidationCache {
  private cache = new Map<string, CachedValidation>();
  private readonly maxSize = 100; // Global cache limit
  private readonly ttl = 60000; // 1 minute TTL

  /**
   * Get a cached validation result
   */
  get(address: string): CachedValidation | undefined {
    const cached = this.cache.get(address);

    if (!cached) {
      return undefined;
    }

    // Check if cache entry has expired
    if (Date.now() - cached.timestamp > this.ttl) {
      this.cache.delete(address);
      return undefined;
    }

    // Update LRU position by deleting and re-adding
    this.cache.delete(address);
    this.cache.set(address, cached);

    return cached;
  }

  /**
   * Set a validation result in the cache
   */
  set(address: string, validation: any): void {
    // If key exists, delete it first to update LRU position
    if (this.cache.has(address)) {
      this.cache.delete(address);
    }

    // Add to cache
    this.cache.set(address, {
      validation,
      timestamp: Date.now()
    });

    // Enforce size limit with LRU eviction
    if (this.cache.size > this.maxSize) {
      const entriesToRemove = this.cache.size - this.maxSize;
      const iterator = this.cache.keys();
      for (let i = 0; i < entriesToRemove; i++) {
        const keyToRemove = iterator.next().value;
        if (keyToRemove) {
          this.cache.delete(keyToRemove);
        }
      }
    }
  }

  /**
   * Clear the entire cache
   */
  clear(): void {
    this.cache.clear();
  }

  /**
   * Get current cache size
   */
  get size(): number {
    return this.cache.size;
  }
}

// Create a singleton instance shared across all components
export const sharedValidationCache = new ValidationCache();