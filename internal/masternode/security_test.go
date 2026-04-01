// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test WIF key for testing (not a real key with funds)
const testWIF = "93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg"

func TestSecureKey_NewAndClear(t *testing.T) {
	// Create secure key
	sk := NewSecureKey(testWIF)
	require.NotNil(t, sk)
	assert.True(t, sk.IsSet())

	// Clear the key
	sk.Clear()
	assert.False(t, sk.IsSet())

	// Clear again should be safe
	sk.Clear()
	assert.False(t, sk.IsSet())
}

func TestSecureKey_Empty(t *testing.T) {
	// Empty string
	sk := NewSecureKey("")
	assert.False(t, sk.IsSet())

	// Nil key
	var nilKey *SecureKey
	assert.False(t, nilKey.IsSet())
	nilKey.Clear() // Should not panic
}

func TestSecureKey_DecodeWIF(t *testing.T) {
	sk := NewSecureKey(testWIF)
	defer sk.Clear()

	privKey, err := sk.DecodeWIF()
	require.NoError(t, err)
	require.NotNil(t, privKey)

	// Verify the public key can be derived
	pubKey := privKey.PublicKey()
	require.NotNil(t, pubKey)
}

func TestSecureKey_DecodeWIF_AfterClear(t *testing.T) {
	sk := NewSecureKey(testWIF)
	sk.Clear()

	privKey, err := sk.DecodeWIF()
	assert.Error(t, err)
	assert.Nil(t, privKey)
	assert.Contains(t, err.Error(), "empty")
}

func TestSecureKey_String(t *testing.T) {
	// Set key should not expose data
	sk := NewSecureKey(testWIF)
	defer sk.Clear()
	assert.Equal(t, "<secure-key-set>", sk.String())
	assert.NotContains(t, sk.String(), testWIF)

	// Cleared key
	sk.Clear()
	assert.Equal(t, "<unset>", sk.String())

	// Nil key
	var nilKey *SecureKey
	assert.Equal(t, "<unset>", nilKey.String())
}
