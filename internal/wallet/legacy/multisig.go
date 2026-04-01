package legacy

import (
	"io"

	"github.com/twins-dev/twins-core/internal/wallet/serialization"
)

// CMultisigAddress represents a serializable multisig P2SH address
// Corresponds to legacy wallet.dat multisig address storage
type CMultisigAddress struct {
	Address      string   // P2SH address (Base58Check encoded)
	RedeemScript string   // Hex-encoded redeem script
	NRequired    int32    // Number of required signatures
	Keys         []string // Public keys or addresses used in multisig
	Account      string   // Associated account name
	CreatedAt    int64    // Unix timestamp (seconds since epoch)
}

// Serialize writes multisig address to the writer
// Format matches legacy C++ wallet.dat multisig address serialization
func (ma *CMultisigAddress) Serialize(w io.Writer) error {
	// Write P2SH address
	if err := serialization.WriteString(w, ma.Address); err != nil {
		return err
	}

	// Write redeem script (hex-encoded)
	if err := serialization.WriteString(w, ma.RedeemScript); err != nil {
		return err
	}

	// Write number of required signatures
	if err := serialization.WriteInt32(w, ma.NRequired); err != nil {
		return err
	}

	// Write keys array (count + each key)
	if err := serialization.WriteCompactSize(w, uint64(len(ma.Keys))); err != nil {
		return err
	}
	for _, key := range ma.Keys {
		if err := serialization.WriteString(w, key); err != nil {
			return err
		}
	}

	// Write account name
	if err := serialization.WriteString(w, ma.Account); err != nil {
		return err
	}

	// Write creation timestamp (Unix seconds)
	if err := serialization.WriteInt64(w, ma.CreatedAt); err != nil {
		return err
	}

	return nil
}

// Deserialize reads multisig address from the reader
func (ma *CMultisigAddress) Deserialize(r io.Reader) error {
	// Read P2SH address
	address, err := serialization.ReadString(r)
	if err != nil {
		return err
	}
	ma.Address = address

	// Read redeem script
	redeemScript, err := serialization.ReadString(r)
	if err != nil {
		return err
	}
	ma.RedeemScript = redeemScript

	// Read number of required signatures
	nrequired, err := serialization.ReadInt32(r)
	if err != nil {
		return err
	}
	ma.NRequired = nrequired

	// Read keys array
	keysCount, err := serialization.ReadCompactSize(r)
	if err != nil {
		return err
	}
	ma.Keys = make([]string, keysCount)
	for i := uint64(0); i < keysCount; i++ {
		key, err := serialization.ReadString(r)
		if err != nil {
			return err
		}
		ma.Keys[i] = key
	}

	// Read account name
	account, err := serialization.ReadString(r)
	if err != nil {
		return err
	}
	ma.Account = account

	// Read creation timestamp
	createdAt, err := serialization.ReadInt64(r)
	if err != nil {
		return err
	}
	ma.CreatedAt = createdAt

	return nil
}
