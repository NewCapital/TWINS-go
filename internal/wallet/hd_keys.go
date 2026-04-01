package wallet

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/twins-dev/twins-core/pkg/crypto"
)

const (
	// HardenedKeyStart is the starting index for hardened keys
	HardenedKeyStart = 0x80000000

	// BIP44Purpose is the BIP44 purpose constant
	BIP44Purpose = 44 + HardenedKeyStart

	// TWINSCoinType is the TWINS coin type in BIP44
	TWINSCoinType = 970 + HardenedKeyStart // Using 970 as TWINS coin type (from chainparams.cpp:311)
)

// HDKey represents a hierarchical deterministic key
type HDKey struct {
	key         *crypto.PrivateKey
	pubKey      *crypto.PublicKey
	chainCode   []byte
	depth       uint8
	parentFP    []byte
	childIndex  uint32
	network     NetworkType
	isPrivate   bool
}

// NewHDKeyFromSeed creates a new HD key from a seed
func NewHDKeyFromSeed(seed []byte, network NetworkType) (*HDKey, error) {
	if len(seed) < 16 || len(seed) > 64 {
		return nil, fmt.Errorf("seed length must be between 16 and 64 bytes")
	}

	// Generate master key using HMAC-SHA512
	mac := hmac.New(sha512.New, []byte("Bitcoin seed"))
	mac.Write(seed)
	lr := mac.Sum(nil)

	// Split into key and chain code
	keyBytes := lr[:32]
	chainCode := lr[32:]

	// Create private key
	privKey, err := crypto.ParsePrivateKeyFromBytes(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create private key: %w", err)
	}

	pubKey := privKey.PublicKey()

	return &HDKey{
		key:         privKey,
		pubKey:      pubKey,
		chainCode:   chainCode,
		depth:       0,
		parentFP:    make([]byte, 4),
		childIndex:  0,
		network:     network,
		isPrivate:   true,
	}, nil
}

// DeriveChild derives a child key at the given index
func (k *HDKey) DeriveChild(index uint32) (*HDKey, error) {
	if !k.isPrivate {
		return nil, fmt.Errorf("cannot derive child from public key")
	}

	isHardened := index >= HardenedKeyStart

	// Prepare data for HMAC
	var data []byte
	if isHardened {
		// Hardened child: 0x00 || private_key || index
		data = append([]byte{0x00}, k.key.Bytes()...)
	} else {
		// Normal child: compressed_public_key || index (BIP32 uses compressed format)
		data = k.pubKey.CompressedBytes()
	}

	// Append index
	indexBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(indexBytes, index)
	data = append(data, indexBytes...)

	// Calculate HMAC-SHA512
	mac := hmac.New(sha512.New, k.chainCode)
	mac.Write(data)
	lr := mac.Sum(nil)

	keyBytes := lr[:32]
	chainCode := lr[32:]

	// Create child private key by adding parent key
	childKey, err := addPrivateKeys(k.key, keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to derive child key: %w", err)
	}

	// Calculate parent fingerprint
	parentFP := k.Fingerprint()

	return &HDKey{
		key:         childKey,
		pubKey:      childKey.PublicKey(),
		chainCode:   chainCode,
		depth:       k.depth + 1,
		parentFP:    parentFP,
		childIndex:  index,
		network:     k.network,
		isPrivate:   true,
	}, nil
}

// DerivePath derives a key from a full BIP44 path
func (k *HDKey) DerivePath(path *KeyPath) (*HDKey, error) {
	// Start with master key
	current := k

	// Derive: m/44'/cointype'/account'/change/index
	indices := []uint32{
		BIP44Purpose,                     // 44'
		path.CoinType + HardenedKeyStart, // cointype'
		path.Account + HardenedKeyStart,  // account'
		path.Change,                      // change (0 or 1)
		path.Index,                       // index
	}

	for _, index := range indices {
		child, err := current.DeriveChild(index)
		if err != nil {
			return nil, fmt.Errorf("failed to derive child at index %d: %w", index, err)
		}
		current = child
	}

	return current, nil
}

// Fingerprint returns the fingerprint of the key
func (k *HDKey) Fingerprint() []byte {
	hash := crypto.Hash160(k.pubKey.Bytes())
	return hash[:4]
}

// GetAddress returns the address for this key
func (k *HDKey) GetAddress() (string, error) {
	// Create P2PKH address from public key (use compressed format)
	pubKeyHash := crypto.Hash160(k.pubKey.CompressedBytes())

	// Add version byte (TWINS mainnet: 0x49, testnet: 0x6f)
	var version byte
	switch k.network {
	case MainNet:
		version = crypto.MainNetPubKeyHashAddrID // 0x49 - TWINS W... addresses
	case TestNet:
		version = crypto.TestNetPubKeyHashAddrID // 0x6f
	case RegTest:
		version = crypto.TestNetPubKeyHashAddrID // 0x6f
	default:
		return "", fmt.Errorf("unknown network type")
	}

	// Create address payload
	payload := append([]byte{version}, pubKeyHash...)

	// Calculate checksum
	checksum := crypto.DoubleHash256(payload)[:4]

	// Append checksum
	fullPayload := append(payload, checksum...)

	// Encode to base58
	address := crypto.Base58Encode(fullPayload)

	return address, nil
}

// Sign signs data with the private key
func (k *HDKey) Sign(data []byte) ([]byte, error) {
	if !k.isPrivate {
		return nil, fmt.Errorf("cannot sign with public key")
	}

	sig, err := k.key.Sign(data)
	if err != nil {
		return nil, err
	}

	return sig.Bytes(), nil
}

// PublicKey returns the public key
func (k *HDKey) PublicKey() *crypto.PublicKey {
	return k.pubKey
}

// PrivateKey returns the private key
func (k *HDKey) PrivateKey() *crypto.PrivateKey {
	if !k.isPrivate {
		return nil
	}
	return k.key
}

// SerializePublic serializes the extended public key
func (k *HDKey) SerializePublic() []byte {
	buf := new(bytes.Buffer)

	// Version (4 bytes) - mainnet: 0x0488B21E, testnet: 0x043587CF
	var version uint32
	if k.network == MainNet {
		version = 0x0488B21E
	} else {
		version = 0x043587CF
	}
	binary.Write(buf, binary.BigEndian, version)

	// Depth (1 byte)
	buf.WriteByte(k.depth)

	// Parent fingerprint (4 bytes)
	buf.Write(k.parentFP)

	// Child index (4 bytes)
	binary.Write(buf, binary.BigEndian, k.childIndex)

	// Chain code (32 bytes)
	buf.Write(k.chainCode)

	// Public key (33 bytes compressed)
	buf.Write(k.pubKey.CompressedBytes())

	// Calculate checksum
	checksum := crypto.DoubleHash256(buf.Bytes())[:4]

	// Append checksum
	buf.Write(checksum)

	return buf.Bytes()
}

// SerializePrivate serializes the extended private key
func (k *HDKey) SerializePrivate() []byte {
	if !k.isPrivate {
		return nil
	}

	buf := new(bytes.Buffer)

	// Version (4 bytes) - mainnet: 0x0488ADE4, testnet: 0x04358394
	var version uint32
	if k.network == MainNet {
		version = 0x0488ADE4
	} else {
		version = 0x04358394
	}
	binary.Write(buf, binary.BigEndian, version)

	// Depth (1 byte)
	buf.WriteByte(k.depth)

	// Parent fingerprint (4 bytes)
	buf.Write(k.parentFP)

	// Child index (4 bytes)
	binary.Write(buf, binary.BigEndian, k.childIndex)

	// Chain code (32 bytes)
	buf.Write(k.chainCode)

	// Private key (33 bytes: 0x00 + 32 bytes key)
	buf.WriteByte(0x00)
	buf.Write(k.key.Bytes())

	// Calculate checksum
	checksum := crypto.DoubleHash256(buf.Bytes())[:4]

	// Append checksum
	buf.Write(checksum)

	return buf.Bytes()
}

// String returns the base58-encoded extended key
func (k *HDKey) String() string {
	if k.isPrivate {
		return crypto.Base58Encode(k.SerializePrivate())
	}
	return crypto.Base58Encode(k.SerializePublic())
}

// Neuter returns a public-only version of the key
func (k *HDKey) Neuter() *HDKey {
	return &HDKey{
		key:         nil,
		pubKey:      k.pubKey,
		chainCode:   k.chainCode,
		depth:       k.depth,
		parentFP:    k.parentFP,
		childIndex:  k.childIndex,
		network:     k.network,
		isPrivate:   false,
	}
}

// IsHardened returns whether the key is hardened
func (k *HDKey) IsHardened() bool {
	return k.childIndex >= HardenedKeyStart
}

// Depth returns the depth of the key in the hierarchy
func (k *HDKey) Depth() uint8 {
	return k.depth
}

// ChildIndex returns the child index
func (k *HDKey) ChildIndex() uint32 {
	return k.childIndex
}

// ChainCode returns the chain code
func (k *HDKey) ChainCode() []byte {
	return k.chainCode
}

// addPrivateKeys adds two private keys together (for HD key derivation)
func addPrivateKeys(baseKey *crypto.PrivateKey, addition []byte) (*crypto.PrivateKey, error) {
	// Get the base key bytes
	baseBytes := baseKey.Bytes()

	// Parse addition as big int
	addInt := new(big.Int).SetBytes(addition)
	baseInt := new(big.Int).SetBytes(baseBytes)

	// Add the two values modulo the secp256k1 curve order (NOT P256!)
	// Bitcoin/TWINS use secp256k1, not NIST P-256
	curveOrder := btcec.S256().N
	resultInt := new(big.Int).Add(baseInt, addInt)
	resultInt.Mod(resultInt, curveOrder)

	// Ensure result is not zero
	if resultInt.Sign() == 0 {
		return nil, fmt.Errorf("result key is zero")
	}

	// Convert back to bytes (32 bytes)
	resultBytes := resultInt.Bytes()
	if len(resultBytes) < 32 {
		// Pad with leading zeros
		padded := make([]byte, 32)
		copy(padded[32-len(resultBytes):], resultBytes)
		resultBytes = padded
	}

	// Parse as private key
	return crypto.ParsePrivateKeyFromBytes(resultBytes)
}