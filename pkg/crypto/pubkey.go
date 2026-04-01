package crypto

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
)

// ParsePubKey parses a public key from bytes (compressed or uncompressed)
func ParsePubKey(pubKeyBytes []byte) (*PublicKey, error) {
	// Parse using btcec library
	btcecPubKey, err := btcec.ParsePubKey(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return &PublicKey{key: btcecPubKey}, nil
}
