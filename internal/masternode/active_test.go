// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/pkg/crypto"
)

func TestNewActiveMasternode(t *testing.T) {
	am := NewActiveMasternode()
	require.NotNil(t, am)
	assert.Equal(t, ActiveInitial, am.Status)
}

func TestActiveStatusString(t *testing.T) {
	tests := []struct {
		name     string
		status   ActiveStatus
		contains string
	}{
		{"Initial", ActiveInitial, "not yet activated"},
		{"SyncInProcess", ActiveSyncInProcess, "Sync in progress"},
		{"InputTooNew", ActiveInputTooNew, "confirmations"},
		{"NotCapable", ActiveNotCapable, "Not capable"},
		{"Started", ActiveStarted, "successfully started"},
		{"Unknown", ActiveStatus(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Contains(t, tc.status.String(), tc.contains)
		})
	}
}

func TestActiveMasternode_SetDependencies(t *testing.T) {
	am := NewActiveMasternode()
	manager := createTestManager(t)

	am.SetDependencies(manager, nil, true)

	assert.Equal(t, manager, am.manager)
	assert.True(t, am.isMainnet)
}

func TestActiveMasternode_SetSyncChecker(t *testing.T) {
	am := NewActiveMasternode()

	synced := false
	am.SetSyncChecker(func() bool {
		return synced
	})

	// Should be set
	assert.NotNil(t, am.isSynced)

	// Should work
	assert.False(t, am.isSynced())
	synced = true
	assert.True(t, am.isSynced())
}

func TestActiveMasternode_SetBalanceGetter(t *testing.T) {
	am := NewActiveMasternode()

	balance := int64(1000000)
	am.SetBalanceGetter(func() int64 {
		return balance
	})

	assert.NotNil(t, am.getBalance)
	assert.Equal(t, int64(1000000), am.getBalance())
}

func TestActiveMasternode_Initialize(t *testing.T) {
	am := NewActiveMasternode()
	am.SetDependencies(nil, nil, false) // testnet (non-mainnet for port validation)

	// Generate a test key and encode to WIF
	keyPair, err := crypto.GenerateKeyPair()
	require.NoError(t, err)

	// Encode to WIF (testnet version 0xEF)
	wif := keyPair.Private.EncodeWIF(0xEF, true)

	// Initialize with valid WIF and service address (testnet port)
	err = am.Initialize(wif, "127.0.0.1:37819")
	require.NoError(t, err)

	assert.NotNil(t, am.privateKey)
	assert.NotNil(t, am.PubKeyMasternode)
	assert.NotNil(t, am.ServiceAddr)
}

func TestActiveMasternode_Initialize_InvalidWIF(t *testing.T) {
	am := NewActiveMasternode()
	am.SetDependencies(nil, nil, true)

	err := am.Initialize("invalidwif", "127.0.0.1:37817")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid masternode private key")
}

func TestActiveMasternode_Initialize_InvalidAddress(t *testing.T) {
	am := NewActiveMasternode()
	am.SetDependencies(nil, nil, true)

	keyPair, err := crypto.GenerateKeyPair()
	require.NoError(t, err)
	wif := keyPair.Private.EncodeWIF(0xD4, true)

	err = am.Initialize(wif, "invalid-address")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service address")
}

func TestActiveMasternode_Initialize_WrongPort(t *testing.T) {
	am := NewActiveMasternode()
	am.SetDependencies(nil, nil, true) // mainnet

	keyPair, err := crypto.GenerateKeyPair()
	require.NoError(t, err)
	wif := keyPair.Private.EncodeWIF(0xD4, true)

	// Wrong port for mainnet (should be 37817)
	err = am.Initialize(wif, "127.0.0.1:12345")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port")
}

func TestActiveMasternode_GetStatus(t *testing.T) {
	am := NewActiveMasternode()

	// Initial status
	assert.Equal(t, "Node just started, not yet activated", am.GetStatus())

	// Not capable with reason
	am.Status = ActiveNotCapable
	am.NotCapableReason = "Hot node, waiting for remote activation"
	assert.Contains(t, am.GetStatus(), "Hot node")
}

func TestActiveMasternode_IsStarted(t *testing.T) {
	am := NewActiveMasternode()

	assert.False(t, am.IsStarted())

	am.Status = ActiveStarted
	assert.True(t, am.IsStarted())
}

func TestActiveMasternode_ManageStatus_NotSynced(t *testing.T) {
	am := NewActiveMasternode()

	// Set sync checker that returns false
	am.SetSyncChecker(func() bool { return false })

	err := am.ManageStatus()
	require.NoError(t, err)
	assert.Equal(t, ActiveSyncInProcess, am.Status)
}

func TestActiveMasternode_ManageStatus_Synced(t *testing.T) {
	am := NewActiveMasternode()

	// Set sync checker that returns true
	am.SetSyncChecker(func() bool { return true })

	// Should transition from sync to initial
	am.Status = ActiveSyncInProcess
	err := am.ManageStatus()
	require.NoError(t, err)
	assert.Equal(t, ActiveNotCapable, am.Status)
}

func TestActiveMasternode_ManageStatus_HotNode(t *testing.T) {
	am := NewActiveMasternode()

	// Set sync checker and balance getter
	am.SetSyncChecker(func() bool { return true })
	am.SetBalanceGetter(func() int64 { return 0 }) // zero balance = hot node

	err := am.ManageStatus()
	require.NoError(t, err)
	assert.Equal(t, ActiveNotCapable, am.Status)
	assert.Equal(t, "Hot node, waiting for remote activation", am.NotCapableReason)
}

func TestActiveMasternode_ManageStatus_NoServiceAddr(t *testing.T) {
	am := NewActiveMasternode()

	am.SetSyncChecker(func() bool { return true })
	am.SetBalanceGetter(func() int64 { return 1000000 })

	err := am.ManageStatus()
	require.NoError(t, err)
	assert.Equal(t, ActiveNotCapable, am.Status)
	// LEGACY COMPAT: Now attempts auto-detection via GetLocal equivalent
	// Will fail with "Can't detect external address" if no public IP found
	assert.Equal(t, "Can't detect external address. Please use masternodeaddr configuration option.", am.NotCapableReason)
}

func TestActiveMasternode_Stop(t *testing.T) {
	am := NewActiveMasternode()
	am.Status = ActiveStarted

	am.Stop()

	assert.Equal(t, ActiveInitial, am.Status)
	assert.Empty(t, am.NotCapableReason)
}

func TestActiveMasternode_AutoManagement(t *testing.T) {
	am := NewActiveMasternode()

	// Initially not running
	assert.False(t, am.IsAutoManagementRunning())

	// Start auto-management
	am.StartAutoManagement()
	assert.True(t, am.IsAutoManagementRunning())

	// Double start should be safe (idempotent)
	am.StartAutoManagement()
	assert.True(t, am.IsAutoManagementRunning())

	// Stop auto-management
	am.StopAutoManagement()
	assert.False(t, am.IsAutoManagementRunning())

	// Double stop should be safe (idempotent)
	am.StopAutoManagement()
	assert.False(t, am.IsAutoManagementRunning())

	// Should be restartable after stop
	am.StartAutoManagement()
	assert.True(t, am.IsAutoManagementRunning())
	am.StopAutoManagement()
	assert.False(t, am.IsAutoManagementRunning())
}

func TestActiveMasternode_SendPing_NotStarted(t *testing.T) {
	am := NewActiveMasternode()

	err := am.SendPing()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestPingInterval(t *testing.T) {
	// Verify ping interval constant
	// Legacy: MASTERNODE_PING_SECONDS (5 * 60) = 5 minutes
	assert.Equal(t, 5*time.Minute, PingInterval)
}

func TestMinConfirmations(t *testing.T) {
	// Verify min confirmations constant
	assert.Equal(t, 15, MinConfirmations)
}
