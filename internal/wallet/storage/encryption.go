// Package storage provides wallet persistence and migration utilities
package storage

// IsWalletEncrypted checks if a BerkeleyDB wallet contains encrypted keys
// by looking for "mkey" entries which indicate encryption
func IsWalletEncrypted(entries []WalletEntry) bool {
	for _, entry := range entries {
		if parseKeyType(entry.Key) == "mkey" {
			return true
		}
	}
	return false
}
