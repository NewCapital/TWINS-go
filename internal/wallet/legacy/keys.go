// Package legacy provides Bitcoin/TWINS C++ compatible data structures
// for wallet.dat persistence layer.
package legacy

import (
	"bytes"
	"errors"
	"io"

	"github.com/twins-dev/twins-core/internal/wallet/serialization"
)

var (
	// ErrInvalidPubKey indicates an invalid public key format
	ErrInvalidPubKey = errors.New("invalid public key format")
	// ErrInvalidPrivKey indicates an invalid private key format
	ErrInvalidPrivKey = errors.New("invalid private key format")
)

// CPubKey represents a secp256k1 public key in Bitcoin format
// Compressed: 33 bytes (0x02/0x03 prefix + 32-byte X coordinate)
// Uncompressed: 65 bytes (0x04 prefix + 32-byte X + 32-byte Y)
type CPubKey []byte

// NewCPubKey creates a CPubKey from raw bytes
func NewCPubKey(data []byte) (CPubKey, error) {
	if len(data) != 33 && len(data) != 65 {
		return nil, ErrInvalidPubKey
	}
	if len(data) == 33 && data[0] != 0x02 && data[0] != 0x03 {
		return nil, ErrInvalidPubKey
	}
	if len(data) == 65 && data[0] != 0x04 {
		return nil, ErrInvalidPubKey
	}
	return CPubKey(data), nil
}

// IsCompressed returns true if the public key is in compressed format
func (pk CPubKey) IsCompressed() bool {
	return len(pk) == 33
}

// IsValid checks if the public key format is valid
func (pk CPubKey) IsValid() bool {
	if len(pk) != 33 && len(pk) != 65 {
		return false
	}
	if len(pk) == 33 && pk[0] != 0x02 && pk[0] != 0x03 {
		return false
	}
	if len(pk) == 65 && pk[0] != 0x04 {
		return false
	}
	return true
}

// Serialize writes the public key to the writer
// Public keys are serialized as variable-length byte arrays
func (pk CPubKey) Serialize(w io.Writer) error {
	return serialization.WriteVarBytes(w, []byte(pk))
}

// Deserialize reads a public key from the reader
func (pk *CPubKey) Deserialize(r io.Reader) error {
	data, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	key, err := NewCPubKey(data)
	if err != nil {
		return err
	}
	*pk = key
	return nil
}

// Bytes returns the raw public key bytes
func (pk CPubKey) Bytes() []byte {
	return []byte(pk)
}

// CPrivKey represents a secp256k1 private key in DER-encoded format
// This matches the Bitcoin Core format (variable length, typically 279 bytes)
type CPrivKey []byte

// NewCPrivKey creates a CPrivKey from DER-encoded bytes
func NewCPrivKey(data []byte) (CPrivKey, error) {
	if len(data) == 0 {
		return nil, ErrInvalidPrivKey
	}
	// Basic DER format validation (should start with sequence tag)
	if data[0] != 0x30 {
		return nil, ErrInvalidPrivKey
	}
	return CPrivKey(data), nil
}

// Serialize writes the private key to the writer
func (pk CPrivKey) Serialize(w io.Writer) error {
	return serialization.WriteVarBytes(w, []byte(pk))
}

// Deserialize reads a private key from the reader
func (pk *CPrivKey) Deserialize(r io.Reader) error {
	data, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	key, err := NewCPrivKey(data)
	if err != nil {
		return err
	}
	*pk = key
	return nil
}

// Bytes returns the raw DER-encoded private key bytes
func (pk CPrivKey) Bytes() []byte {
	return []byte(pk)
}

// CKeyMetadata represents metadata associated with a key
// Corresponds to C++ CKeyMetadata in wallet.h
type CKeyMetadata struct {
	Version      int32  // Key metadata version
	CreateTime   int64  // Creation timestamp (Unix time)
	HDKeyPath    string // BIP32 derivation path (e.g., "m/44'/0'/0'/0/0")
	HDMasterKeyID []byte // Master key fingerprint (4 bytes)
}

// NewCKeyMetadata creates new key metadata with current timestamp
func NewCKeyMetadata(createTime int64) *CKeyMetadata {
	return &CKeyMetadata{
		Version:    1,
		CreateTime: createTime,
	}
}

// Serialize writes key metadata to the writer
// Format: version, createTime, optional HD path and master key ID
func (km *CKeyMetadata) Serialize(w io.Writer) error {
	// Write version
	if err := serialization.WriteInt32(w, km.Version); err != nil {
		return err
	}

	// Write creation time
	if err := serialization.WriteInt64(w, km.CreateTime); err != nil {
		return err
	}

	// Write HD key path (optional, empty string if not HD key)
	if err := serialization.WriteString(w, km.HDKeyPath); err != nil {
		return err
	}

	// Write master key ID (optional, empty if not HD key)
	if err := serialization.WriteVarBytes(w, km.HDMasterKeyID); err != nil {
		return err
	}

	return nil
}

// Deserialize reads key metadata from the reader
func (km *CKeyMetadata) Deserialize(r io.Reader) error {
	// Read version
	version, err := serialization.ReadInt32(r)
	if err != nil {
		return err
	}
	km.Version = version

	// Read creation time
	createTime, err := serialization.ReadInt64(r)
	if err != nil {
		return err
	}
	km.CreateTime = createTime

	// Read HD key path
	hdKeyPath, err := serialization.ReadString(r)
	if err != nil {
		return err
	}
	km.HDKeyPath = hdKeyPath

	// Read master key ID
	masterKeyID, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	km.HDMasterKeyID = masterKeyID

	return nil
}

// CMasterKey represents an encrypted master key for wallet encryption
// Corresponds to C++ CMasterKey in crypter.h
type CMasterKey struct {
	EncryptedKey              []byte // vchCryptedKey - 32-byte key encrypted with passphrase
	Salt                      []byte // vchSalt - 8-byte salt for key derivation
	DerivationMethod          uint32 // nDerivationMethod - 0 (EVP_sha512) or 1 (scrypt)
	DeriveIterations          uint32 // nDeriveIterations - KDF iteration count
	OtherDerivationParameters []byte // vchOtherDerivationParameters - Additional params (scrypt)
}

// Derivation methods
const (
	DerivationMethodSHA512 = 0 // EVP_sha512 (legacy, 25000 iterations default)
	DerivationMethodScrypt = 1 // scrypt (newer, configurable parameters)
)

// NewCMasterKey creates a new master key with specified parameters
func NewCMasterKey(encryptedKey, salt []byte, method, iterations uint32) *CMasterKey {
	return &CMasterKey{
		EncryptedKey:     encryptedKey,
		Salt:             salt,
		DerivationMethod: method,
		DeriveIterations: iterations,
	}
}

// Serialize writes master key to the writer
func (mk *CMasterKey) Serialize(w io.Writer) error {
	// Write encrypted key
	if err := serialization.WriteVarBytes(w, mk.EncryptedKey); err != nil {
		return err
	}

	// Write salt
	if err := serialization.WriteVarBytes(w, mk.Salt); err != nil {
		return err
	}

	// Write derivation method
	if err := serialization.WriteUint32(w, mk.DerivationMethod); err != nil {
		return err
	}

	// Write iteration count
	if err := serialization.WriteUint32(w, mk.DeriveIterations); err != nil {
		return err
	}

	// Write additional parameters
	if err := serialization.WriteVarBytes(w, mk.OtherDerivationParameters); err != nil {
		return err
	}

	return nil
}

// Deserialize reads master key from the reader
func (mk *CMasterKey) Deserialize(r io.Reader) error {
	// Read encrypted key
	encryptedKey, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	mk.EncryptedKey = encryptedKey

	// Read salt
	salt, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	mk.Salt = salt

	// Read derivation method
	method, err := serialization.ReadUint32(r)
	if err != nil {
		return err
	}
	mk.DerivationMethod = method

	// Read iteration count
	iterations, err := serialization.ReadUint32(r)
	if err != nil {
		return err
	}
	mk.DeriveIterations = iterations

	// Read additional parameters
	params, err := serialization.ReadVarBytes(r)
	if err != nil {
		return err
	}
	mk.OtherDerivationParameters = params

	return nil
}

// SerializeToBytes serializes the structure to a byte slice
func SerializeToBytes(s interface {
	Serialize(io.Writer) error
}) ([]byte, error) {
	var buf bytes.Buffer
	if err := s.Serialize(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DeserializeFromBytes deserializes a structure from a byte slice
func DeserializeFromBytes(data []byte, s interface {
	Deserialize(io.Reader) error
}) error {
	r := bytes.NewReader(data)
	return s.Deserialize(r)
}
