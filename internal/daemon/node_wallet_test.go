package daemon

import (
	"testing"

	"github.com/twins-dev/twins-core/internal/config"
	"github.com/twins-dev/twins-core/internal/wallet"
)

func TestApplyConfigFileSettings_ZeroMinTxFee(t *testing.T) {
	wc := wallet.DefaultConfig()
	if wc.MinTxFee == 0 {
		t.Fatal("wallet default MinTxFee should be non-zero for this test to be meaningful")
	}

	fc := config.DefaultConfig()
	fc.Wallet.MinTxFee = 0 // User explicitly set to 0

	applyConfigFileSettings(wc, fc)

	if wc.MinTxFee != 0 {
		t.Errorf("expected MinTxFee=0 (user-set), got %d", wc.MinTxFee)
	}
}

func TestApplyConfigFileSettings_ZeroMaxTxFee(t *testing.T) {
	wc := wallet.DefaultConfig()
	if wc.MaxTxFee == 0 {
		t.Fatal("wallet default MaxTxFee should be non-zero for this test to be meaningful")
	}

	fc := config.DefaultConfig()
	fc.Wallet.MaxTxFee = 0 // User explicitly set to 0

	applyConfigFileSettings(wc, fc)

	if wc.MaxTxFee != 0 {
		t.Errorf("expected MaxTxFee=0 (user-set), got %d", wc.MaxTxFee)
	}
}

func TestApplyConfigFileSettings_NonZeroValues(t *testing.T) {
	wc := wallet.DefaultConfig()
	fc := config.DefaultConfig()

	// Set non-zero custom values
	fc.Wallet.PayTxFee = 5000
	fc.Wallet.MinTxFee = 20000
	fc.Wallet.MaxTxFee = 500000000
	fc.Wallet.TxConfirmTarget = 3
	fc.Wallet.Keypool = 2000
	fc.Wallet.SpendZeroConfChange = true
	fc.Wallet.CreateWalletBackups = 5
	fc.Wallet.BackupPath = "/tmp/backups"
	fc.Staking.ReserveBalance = 100000000

	applyConfigFileSettings(wc, fc)

	if wc.FeePerKB != 5000 {
		t.Errorf("FeePerKB: expected 5000, got %d", wc.FeePerKB)
	}
	if wc.MinTxFee != 20000 {
		t.Errorf("MinTxFee: expected 20000, got %d", wc.MinTxFee)
	}
	if wc.MaxTxFee != 500000000 {
		t.Errorf("MaxTxFee: expected 500000000, got %d", wc.MaxTxFee)
	}
	if wc.TxConfirmTarget != 3 {
		t.Errorf("TxConfirmTarget: expected 3, got %d", wc.TxConfirmTarget)
	}
	if wc.AccountLookahead != 2000 {
		t.Errorf("AccountLookahead: expected 2000, got %d", wc.AccountLookahead)
	}
	if !wc.SpendZeroConfChange {
		t.Error("SpendZeroConfChange: expected true")
	}
	if wc.CreateWalletBackups != 5 {
		t.Errorf("CreateWalletBackups: expected 5, got %d", wc.CreateWalletBackups)
	}
	if wc.BackupPath != "/tmp/backups" {
		t.Errorf("BackupPath: expected /tmp/backups, got %q", wc.BackupPath)
	}
	if wc.ReserveBalance != 100000000 {
		t.Errorf("ReserveBalance: expected 100000000, got %d", wc.ReserveBalance)
	}
}

func TestApplyConfigFileSettings_PayTxFeeZeroKeepsWalletDefault(t *testing.T) {
	wc := wallet.DefaultConfig()
	originalFee := wc.FeePerKB

	fc := config.DefaultConfig()
	fc.Wallet.PayTxFee = 0 // 0 means "use wallet dynamic fee default"

	applyConfigFileSettings(wc, fc)

	if wc.FeePerKB != originalFee {
		t.Errorf("PayTxFee=0 should preserve wallet default %d, got %d", originalFee, wc.FeePerKB)
	}
}
