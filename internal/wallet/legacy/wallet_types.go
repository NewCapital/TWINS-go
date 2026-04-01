package legacy

import (
	"fmt"
	"io"

	"github.com/twins-dev/twins-core/internal/wallet/serialization"
	"github.com/twins-dev/twins-core/pkg/crypto"
)

// CWalletTx represents a wallet transaction with metadata
// Extends basic transaction with wallet-specific information
// Corresponds to C++ CWalletTx in wallet.h
type CWalletTx struct {
	// Base transaction data (version, vin, vout, locktime)
	TxHash []byte // 32-byte transaction hash

	// Wallet-specific metadata
	TimeReceived int64             // nTimeReceived - When transaction was first seen
	OrderPos     int64             // nOrderPos - Position in transaction list for display ordering
	FromAccount  string            // strFromAccount - Legacy account system (deprecated)
	MapValue     map[string]string // mapValue - Key-value metadata (comments, labels, etc.)
}

// NewCWalletTx creates a new wallet transaction
func NewCWalletTx(txHash []byte, timeReceived int64) *CWalletTx {
	return &CWalletTx{
		TxHash:       txHash,
		TimeReceived: timeReceived,
		OrderPos:     -1, // -1 means not yet assigned
		MapValue:     make(map[string]string),
	}
}

// Serialize writes wallet transaction to the writer
// Note: This is a simplified version. Full implementation would include
// complete CTransaction serialization (version, vin, vout, locktime)
func (wtx *CWalletTx) Serialize(w io.Writer) error {
	// Write transaction hash (32 bytes fixed)
	if err := serialization.WriteFixedBytes(w, wtx.TxHash); err != nil {
		return err
	}

	// Write time received
	if err := serialization.WriteInt64(w, wtx.TimeReceived); err != nil {
		return err
	}

	// Write order position
	if err := serialization.WriteInt64(w, wtx.OrderPos); err != nil {
		return err
	}

	// Write from account
	if err := serialization.WriteString(w, wtx.FromAccount); err != nil {
		return err
	}

	// Write mapValue (map<string, string>)
	if err := serialization.WriteCompactSize(w, uint64(len(wtx.MapValue))); err != nil {
		return err
	}
	for key, value := range wtx.MapValue {
		if err := serialization.WriteString(w, key); err != nil {
			return err
		}
		if err := serialization.WriteString(w, value); err != nil {
			return err
		}
	}

	return nil
}

// Deserialize reads wallet transaction from the reader
func (wtx *CWalletTx) Deserialize(r io.Reader) error {
	// Read transaction hash
	txHash, err := serialization.ReadFixedBytes(r, 32)
	if err != nil {
		return err
	}
	wtx.TxHash = txHash

	// Read time received
	timeReceived, err := serialization.ReadInt64(r)
	if err != nil {
		return err
	}
	wtx.TimeReceived = timeReceived

	// Read order position
	orderPos, err := serialization.ReadInt64(r)
	if err != nil {
		return err
	}
	wtx.OrderPos = orderPos

	// Read from account
	fromAccount, err := serialization.ReadString(r)
	if err != nil {
		return err
	}
	wtx.FromAccount = fromAccount

	// Read mapValue
	mapSize, err := serialization.ReadCompactSize(r)
	if err != nil {
		return err
	}
	wtx.MapValue = make(map[string]string, mapSize)
	for i := uint64(0); i < mapSize; i++ {
		key, err := serialization.ReadString(r)
		if err != nil {
			return err
		}
		value, err := serialization.ReadString(r)
		if err != nil {
			return err
		}
		wtx.MapValue[key] = value
	}

	return nil
}

// CHDAccount represents an HD account with external/internal chain counters
// Corresponds to C++ CHDAccount in hdchain.h
type CHDAccount struct {
	ExternalCounter uint32 // Counter for external chain (receiving addresses)
	InternalCounter uint32 // Counter for internal chain (change addresses)
}

// CHDChain represents HD wallet chain state
// Corresponds to C++ CHDChain in hdchain.h
type CHDChain struct {
	Version      int32  // Chain version
	ChainID      []byte // Chain ID (SHA256 of seed, 32 bytes)
	Crypted      bool   // Whether seed is encrypted
	Seed         []byte // BIP32 seed (encrypted if Crypted=true)
	Mnemonic     []byte // BIP39 mnemonic (optional)
	MnemonicPass []byte // Mnemonic passphrase (optional)
	// MapAccounts stores per-account counters (key is account index, usually 0)
	MapAccounts map[uint32]CHDAccount
	// Convenience fields for default account (index 0)
	ExternalCounter uint32 // Counter for external chain (receiving addresses)
	InternalCounter uint32 // Counter for internal chain (change addresses)
}

// NewCHDChain creates a new HD chain with chain ID derived from seed
func NewCHDChain(seed []byte) *CHDChain {
	// Calculate chain ID as DoubleHash256 of seed (SHA256(SHA256(seed)))
	// This matches the Bitcoin standard and wallet.go validation at line 645
	chainID := crypto.DoubleHash256(seed)

	return &CHDChain{
		Version:         1,
		ChainID:         chainID,
		Crypted:         false,
		Seed:            seed,
		Mnemonic:        []byte{},
		MnemonicPass:    []byte{},
		MapAccounts:     make(map[uint32]CHDAccount),
		ExternalCounter: 0,
		InternalCounter: 0,
	}
}

// Serialize writes HD chain to the writer
// Matches legacy C++ CHDChain serialization format
func (hd *CHDChain) Serialize(w io.Writer) error {
	// Write version
	if err := serialization.WriteInt32(w, hd.Version); err != nil {
		return err
	}

	// Write chain ID (32 bytes)
	if err := serialization.WriteFixedBytes(w, hd.ChainID); err != nil {
		return err
	}

	// Write crypted flag
	cryptedByte := byte(0)
	if hd.Crypted {
		cryptedByte = byte(1)
	}
	if err := serialization.WriteByte(w, cryptedByte); err != nil {
		return err
	}

	// Write seed (variable length)
	if err := serialization.WriteVarBytes(w, hd.Seed); err != nil {
		return err
	}

	// Write mnemonic (variable length)
	if err := serialization.WriteVarBytes(w, hd.Mnemonic); err != nil {
		return err
	}

	// Write mnemonic passphrase (variable length)
	if err := serialization.WriteVarBytes(w, hd.MnemonicPass); err != nil {
		return err
	}

	// Write mapAccounts as map<uint32, CHDAccount>
	// Format: CompactSize(count) + [for each: uint32 key + uint32 external + uint32 internal]
	if err := serialization.WriteCompactSize(w, uint64(len(hd.MapAccounts))); err != nil {
		return err
	}
	for accountID, account := range hd.MapAccounts {
		if err := serialization.WriteUint32(w, accountID); err != nil {
			return err
		}
		if err := serialization.WriteUint32(w, account.ExternalCounter); err != nil {
			return err
		}
		if err := serialization.WriteUint32(w, account.InternalCounter); err != nil {
			return err
		}
	}

	return nil
}

// Deserialize reads HD chain from the reader
// Matches legacy C++ CHDChain deserialization format
func (hd *CHDChain) Deserialize(r io.Reader) error {
	// Read version
	version, err := serialization.ReadInt32(r)
	if err != nil {
		return err
	}
	hd.Version = version

	// Read chain ID (32 bytes)
	chainID, err := serialization.ReadFixedBytes(r, 32)
	if err != nil {
		return err
	}
	hd.ChainID = chainID

	// Read crypted flag
	cryptedByte, err := serialization.ReadByte(r)
	if err != nil {
		return err
	}
	hd.Crypted = cryptedByte != 0

	// Read seed (variable length)
	seed, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	hd.Seed = seed

	// Read mnemonic (variable length)
	mnemonic, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	hd.Mnemonic = mnemonic

	// Read mnemonic passphrase (variable length)
	mnemonicPass, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	hd.MnemonicPass = mnemonicPass

	// Read mapAccounts as map<uint32, CHDAccount>
	// Format: CompactSize(count) + [for each: uint32 key + uint32 external + uint32 internal]
	accountCount, err := serialization.ReadCompactSize(r)
	if err != nil {
		return err
	}

	// Bounds check to prevent DoS via memory exhaustion
	const maxAccounts = 1000
	if accountCount > maxAccounts {
		return fmt.Errorf("invalid account count: %d exceeds maximum %d", accountCount, maxAccounts)
	}

	hd.MapAccounts = make(map[uint32]CHDAccount, accountCount)
	for i := uint64(0); i < accountCount; i++ {
		accountID, err := serialization.ReadUint32(r)
		if err != nil {
			return err
		}
		externalCounter, err := serialization.ReadUint32(r)
		if err != nil {
			return err
		}
		internalCounter, err := serialization.ReadUint32(r)
		if err != nil {
			return err
		}
		hd.MapAccounts[accountID] = CHDAccount{
			ExternalCounter: externalCounter,
			InternalCounter: internalCounter,
		}
	}

	// Set convenience fields from default account (index 0)
	if account, ok := hd.MapAccounts[0]; ok {
		hd.ExternalCounter = account.ExternalCounter
		hd.InternalCounter = account.InternalCounter
	}

	return nil
}

// CExtPubKey represents an extended public key (BIP32)
// Matches C++ CExtPubKey serialization: [size=74][depth][fingerprint][child][chaincode][pubkey]
type CExtPubKey struct {
	Depth       uint8    // Depth in derivation path
	Fingerprint [4]byte  // Parent key fingerprint
	Child       uint32   // Child number
	ChainCode   [32]byte // Chain code
	PubKey      []byte   // 33-byte compressed public key
}

// Serialize writes extended public key to the writer
// Format: CompactSize(74) + 74 bytes of data
func (epk *CExtPubKey) Serialize(w io.Writer) error {
	// Write compact size (74 bytes)
	if err := serialization.WriteCompactSize(w, 74); err != nil {
		return err
	}

	// Write depth (1 byte)
	if err := serialization.WriteByte(w, epk.Depth); err != nil {
		return err
	}

	// Write fingerprint (4 bytes)
	if _, err := w.Write(epk.Fingerprint[:]); err != nil {
		return err
	}

	// Write child number (4 bytes big-endian for BIP32)
	childBytes := make([]byte, 4)
	childBytes[0] = byte(epk.Child >> 24)
	childBytes[1] = byte(epk.Child >> 16)
	childBytes[2] = byte(epk.Child >> 8)
	childBytes[3] = byte(epk.Child)
	if _, err := w.Write(childBytes); err != nil {
		return err
	}

	// Write chain code (32 bytes)
	if _, err := w.Write(epk.ChainCode[:]); err != nil {
		return err
	}

	// Write public key (33 bytes)
	if _, err := w.Write(epk.PubKey); err != nil {
		return err
	}

	return nil
}

// Deserialize reads extended public key from the reader
func (epk *CExtPubKey) Deserialize(r io.Reader) error {
	// Read compact size (should be 74)
	size, err := serialization.ReadCompactSize(r)
	if err != nil {
		return err
	}
	if size != 74 {
		return fmt.Errorf("invalid CExtPubKey size: %d, expected 74", size)
	}

	// Read depth (1 byte)
	epk.Depth, err = serialization.ReadByte(r)
	if err != nil {
		return err
	}

	// Read fingerprint (4 bytes)
	if _, err := io.ReadFull(r, epk.Fingerprint[:]); err != nil {
		return err
	}

	// Read child number (4 bytes big-endian for BIP32)
	childBytes := make([]byte, 4)
	if _, err := io.ReadFull(r, childBytes); err != nil {
		return err
	}
	epk.Child = uint32(childBytes[0])<<24 | uint32(childBytes[1])<<16 | uint32(childBytes[2])<<8 | uint32(childBytes[3])

	// Read chain code (32 bytes)
	if _, err := io.ReadFull(r, epk.ChainCode[:]); err != nil {
		return err
	}

	// Read public key (33 bytes)
	epk.PubKey = make([]byte, 33)
	if _, err := io.ReadFull(r, epk.PubKey); err != nil {
		return err
	}

	return nil
}

// CHDPubKey represents a BIP32 extended public key with derivation info
// Corresponds to C++ CHDPubKey in hdchain.h
// Serialization: nVersion, extPubKey, hdchainID, nAccountIndex, nChangeIndex
type CHDPubKey struct {
	Version      int32      // Version (currently 1)
	ExtPubKey    CExtPubKey // Extended public key
	HDChainID    []byte     // 32-byte chain ID (SHA256 of seed)
	AccountIndex uint32     // Account index in BIP44 path
	ChangeIndex  uint32     // 0 = external, 1 = internal/change
}

// NewCHDPubKey creates a new HD public key
func NewCHDPubKey() *CHDPubKey {
	return &CHDPubKey{
		Version: 1,
	}
}

// GetKeyPath returns the BIP44 derivation path for this key
// Format: m/44'/{coin_type}'/{account}'/{change}/{child}
func (hpk *CHDPubKey) GetKeyPath(coinType uint32) string {
	return fmt.Sprintf("m/44'/%d'/%d'/%d/%d",
		coinType, hpk.AccountIndex, hpk.ChangeIndex, hpk.ExtPubKey.Child)
}

// Serialize writes HD public key to the writer
func (hpk *CHDPubKey) Serialize(w io.Writer) error {
	// Write version
	if err := serialization.WriteInt32(w, hpk.Version); err != nil {
		return err
	}

	// Write extended public key
	if err := hpk.ExtPubKey.Serialize(w); err != nil {
		return err
	}

	// Write hdchainID (32 bytes)
	if err := serialization.WriteFixedBytes(w, hpk.HDChainID); err != nil {
		return err
	}

	// Write account index
	if err := serialization.WriteUint32(w, hpk.AccountIndex); err != nil {
		return err
	}

	// Write change index
	if err := serialization.WriteUint32(w, hpk.ChangeIndex); err != nil {
		return err
	}

	return nil
}

// Deserialize reads HD public key from the reader
func (hpk *CHDPubKey) Deserialize(r io.Reader) error {
	// Read version
	version, err := serialization.ReadInt32(r)
	if err != nil {
		return err
	}
	hpk.Version = version

	// Read extended public key
	if err := hpk.ExtPubKey.Deserialize(r); err != nil {
		return err
	}

	// Read hdchainID (32 bytes)
	hdChainID, err := serialization.ReadFixedBytes(r, 32)
	if err != nil {
		return err
	}
	hpk.HDChainID = hdChainID

	// Read account index
	accountIndex, err := serialization.ReadUint32(r)
	if err != nil {
		return err
	}
	hpk.AccountIndex = accountIndex

	// Read change index
	changeIndex, err := serialization.ReadUint32(r)
	if err != nil {
		return err
	}
	hpk.ChangeIndex = changeIndex

	return nil
}

// CBlockLocator represents a block locator for wallet sync checkpoint
// Corresponds to C++ CBlockLocator
type CBlockLocator struct {
	BlockHashes [][]byte // Vector of block hashes (32 bytes each)
}

// NewCBlockLocator creates a new block locator
func NewCBlockLocator(hashes [][]byte) *CBlockLocator {
	return &CBlockLocator{
		BlockHashes: hashes,
	}
}

// Serialize writes block locator to the writer
func (bl *CBlockLocator) Serialize(w io.Writer) error {
	// Write number of hashes
	if err := serialization.WriteCompactSize(w, uint64(len(bl.BlockHashes))); err != nil {
		return err
	}

	// Write each hash (32 bytes)
	for _, hash := range bl.BlockHashes {
		if err := serialization.WriteFixedBytes(w, hash); err != nil {
			return err
		}
	}

	return nil
}

// Deserialize reads block locator from the reader
func (bl *CBlockLocator) Deserialize(r io.Reader) error {
	// Read number of hashes
	count, err := serialization.ReadCompactSize(r)
	if err != nil {
		return err
	}

	// Read each hash
	bl.BlockHashes = make([][]byte, count)
	for i := uint64(0); i < count; i++ {
		hash, err := serialization.ReadFixedBytes(r, 32)
		if err != nil {
			return err
		}
		bl.BlockHashes[i] = hash
	}

	return nil
}
