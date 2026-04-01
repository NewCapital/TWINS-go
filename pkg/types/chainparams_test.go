package types

import (
	"encoding/hex"
	"testing"
	"time"
)

func TestMasternodeTierConstants(t *testing.T) {
	// Test that tier constants are properly defined
	tiers := []MasternodeTier{
		MasternodeTierBronze,
		MasternodeTierSilver,
		MasternodeTierGold,
		MasternodeTierPlatinum,
	}

	// Check that they have different values
	for i, tier1 := range tiers {
		for j, tier2 := range tiers {
			if i != j && tier1 == tier2 {
				t.Errorf("Tier %d and %d have the same value", i, j)
			}
		}
	}
}

func TestChainParamsGetTierCollateral(t *testing.T) {
	params := MainnetParams()

	// Test valid tiers
	bronzeCollateral := params.GetTierCollateral(MasternodeTierBronze)
	if bronzeCollateral != 100000000000000 { // 1M TWINS in satoshis
		t.Errorf("Expected Bronze collateral 100000000000000, got %d", bronzeCollateral)
	}

	silverCollateral := params.GetTierCollateral(MasternodeTierSilver)
	if silverCollateral != 500000000000000 { // 5M TWINS in satoshis
		t.Errorf("Expected Silver collateral 500000000000000, got %d", silverCollateral)
	}

	goldCollateral := params.GetTierCollateral(MasternodeTierGold)
	if goldCollateral != 2000000000000000 { // 20M TWINS in satoshis
		t.Errorf("Expected Gold collateral 2000000000000000, got %d", goldCollateral)
	}

	platinumCollateral := params.GetTierCollateral(MasternodeTierPlatinum)
	if platinumCollateral != 10000000000000000 { // 100M TWINS in satoshis
		t.Errorf("Expected Platinum collateral 10000000000000000, got %d", platinumCollateral)
	}

	// Test invalid tier (should return 0)
	invalidTier := MasternodeTier(999)
	invalidCollateral := params.GetTierCollateral(invalidTier)
	if invalidCollateral != 0 {
		t.Errorf("Expected 0 for invalid tier, got %d", invalidCollateral)
	}
}

func TestMustParseHashLittleEndian(t *testing.T) {
	const hexStr = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

	hash := MustParseHash(hexStr)

	if hash.String() != hexStr {
		t.Fatalf("expected String() to return %s, got %s", hexStr, hash.String())
	}

	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("failed to decode hex string: %v", err)
	}

	for i := 0; i < len(hash); i++ {
		expected := bytes[len(bytes)-1-i]
		if hash[i] != expected {
			t.Fatalf("byte %d: expected 0x%02x, got 0x%02x", i, expected, hash[i])
		}
	}
}

func TestChainParamsIsValidTier(t *testing.T) {
	params := MainnetParams()

	// Test valid collateral amounts
	validCollaterals := []int64{
		100000000000000,   // Bronze
		500000000000000,   // Silver
		2000000000000000,  // Gold
		10000000000000000, // Platinum
	}

	for _, collateral := range validCollaterals {
		if !params.IsValidTier(collateral) {
			t.Errorf("Collateral %d should be valid", collateral)
		}
	}

	// Test invalid collateral amounts
	invalidCollaterals := []int64{
		0,
		1000000,
		50000000000000,    // Between Bronze and Silver
		1000000000000000,  // Between Silver and Gold
		5000000000000000,  // Between Gold and Platinum
		20000000000000000, // Above Platinum
	}

	for _, collateral := range invalidCollaterals {
		if params.IsValidTier(collateral) {
			t.Errorf("Collateral %d should not be valid", collateral)
		}
	}
}

func TestChainParamsGetTierFromCollateral(t *testing.T) {
	params := MainnetParams()

	// Test valid collaterals
	testCases := []struct {
		collateral    int64
		expectedTier  MasternodeTier
		shouldBeValid bool
	}{
		{100000000000000, MasternodeTierBronze, true},
		{500000000000000, MasternodeTierSilver, true},
		{2000000000000000, MasternodeTierGold, true},
		{10000000000000000, MasternodeTierPlatinum, true},
		{123456789, MasternodeTierBronze, false}, // Invalid amount
	}

	for _, tc := range testCases {
		tier, valid := params.GetTierFromCollateral(tc.collateral)
		if valid != tc.shouldBeValid {
			t.Errorf("Collateral %d: expected valid=%v, got valid=%v", tc.collateral, tc.shouldBeValid, valid)
		}
		if tc.shouldBeValid && tier != tc.expectedTier {
			t.Errorf("Collateral %d: expected tier %d, got tier %d", tc.collateral, tc.expectedTier, tier)
		}
	}
}

func TestChainParamsGetTierRewardPercentage(t *testing.T) {
	params := MainnetParams()

	expectedRewards := map[MasternodeTier]int64{
		MasternodeTierBronze:   1000, // 10%
		MasternodeTierSilver:   2000, // 20%
		MasternodeTierGold:     3000, // 30%
		MasternodeTierPlatinum: 4000, // 40%
	}

	for tier, expectedReward := range expectedRewards {
		reward := params.GetTierRewardPercentage(tier)
		if reward != expectedReward {
			t.Errorf("Tier %d: expected reward %d, got %d", tier, expectedReward, reward)
		}
	}

	// Test invalid tier
	invalidTier := MasternodeTier(999)
	invalidReward := params.GetTierRewardPercentage(invalidTier)
	if invalidReward != 0 {
		t.Errorf("Invalid tier should return 0 reward, got %d", invalidReward)
	}
}

func TestMainnetParams(t *testing.T) {
	params := MainnetParams()

	// Test basic network properties
	if params.Name != "mainnet" {
		t.Errorf("Expected name 'mainnet', got '%s'", params.Name)
	}

	if params.DefaultPort != 37817 {
		t.Errorf("Expected default port 37817, got %d", params.DefaultPort)
	}

	if len(params.DNSSeeds) == 0 {
		t.Error("Mainnet should have DNS seeds")
	}

	// Test PoS parameters
	if params.StakeMinAge != 3*time.Hour {
		t.Errorf("Expected stake min age 3h, got %v", params.StakeMinAge)
	}

	if params.CoinbaseMaturity != 60 {
		t.Errorf("Expected coinbase maturity 60, got %d", params.CoinbaseMaturity)
	}

	// Test block parameters
	if params.TargetSpacing != 2*time.Minute {
		t.Errorf("Expected target spacing 2m, got %v", params.TargetSpacing)
	}

	if params.MaxBlockSize != 1000000 {
		t.Errorf("Expected max block size 1000000, got %d", params.MaxBlockSize)
	}

	// Test rewards
	if params.BlockReward != DefaultBlockReward {
		t.Errorf("Expected block reward %d, got %d", DefaultBlockReward, params.BlockReward)
	}

	// Test masternode tiers
	if len(params.MasternodeTiers) != 4 {
		t.Errorf("Expected 4 masternode tiers, got %d", len(params.MasternodeTiers))
	}

	// Test magic bytes are set
	zeroBytes := [4]byte{0, 0, 0, 0}
	if params.NetMagicBytes == zeroBytes {
		t.Error("Network magic bytes should not be zero")
	}

	const expectedGenesis = "0000071cf2d95aec5ba4818418756c93cb12cd627191710e8969f2f35c3530de"
	if params.GenesisHash.String() != expectedGenesis {
		t.Fatalf("expected genesis hash %s, got %s", expectedGenesis, params.GenesisHash.String())
	}
}

func TestTestnetParams(t *testing.T) {
	params := TestnetParams()

	// Test basic network properties
	if params.Name != "testnet" {
		t.Errorf("Expected name 'testnet', got '%s'", params.Name)
	}

	if params.DefaultPort != 37847 {
		t.Errorf("Expected default port 37847, got %d", params.DefaultPort)
	}

	// Test that it has different magic bytes from mainnet
	mainnetParams := MainnetParams()
	if params.NetMagicBytes == mainnetParams.NetMagicBytes {
		t.Error("Testnet should have different magic bytes from mainnet")
	}

	// Test faster parameters
	if params.StakeMinAge != 3*time.Hour {
		t.Errorf("Expected testnet stake min age 3h, got %v", params.StakeMinAge)
	}

	if params.TargetSpacing != 2*time.Minute {
		t.Errorf("Expected testnet target spacing 2m, got %v", params.TargetSpacing)
	}

	if params.CoinbaseMaturity != 15 {
		t.Errorf("Expected testnet coinbase maturity 15, got %d", params.CoinbaseMaturity)
	}

	// Test collateral requirements (same as mainnet for testnet)
	bronzeCollateral := params.GetTierCollateral(MasternodeTierBronze)
	if bronzeCollateral != 100000000000000 { // 1M TWINS in satoshis
		t.Errorf("Expected testnet Bronze collateral 100000000000000, got %d", bronzeCollateral)
	}
}

func TestRegtestParams(t *testing.T) {
	params := RegtestParams()

	// Test basic network properties
	if params.Name != "regtest" {
		t.Errorf("Expected name 'regtest', got '%s'", params.Name)
	}

	if params.DefaultPort != 5467 {
		t.Errorf("Expected default port 5467, got %d", params.DefaultPort)
	}

	// Test no DNS seeds for regtest
	if len(params.DNSSeeds) != 0 {
		t.Errorf("Regtest should have no DNS seeds, got %d", len(params.DNSSeeds))
	}

	// Test very fast parameters
	if params.StakeMinAge != 1*time.Minute {
		t.Errorf("Expected regtest stake min age 1m, got %v", params.StakeMinAge)
	}

	if params.TargetSpacing != 2*time.Minute {
		t.Errorf("Expected regtest target spacing 2m, got %v", params.TargetSpacing)
	}

	if params.CoinbaseMaturity != 1 {
		t.Errorf("Expected regtest coinbase maturity 1, got %d", params.CoinbaseMaturity)
	}

	if params.DifficultyInterval != 10 {
		t.Errorf("Expected regtest difficulty interval 10, got %d", params.DifficultyInterval)
	}

	// Test minimal collateral requirements
	bronzeCollateral := params.GetTierCollateral(MasternodeTierBronze)
	if bronzeCollateral != 100000000 { // 1 TWINS in satoshis
		t.Errorf("Expected regtest Bronze collateral 100000000, got %d", bronzeCollateral)
	}

	platinumCollateral := params.GetTierCollateral(MasternodeTierPlatinum)
	if platinumCollateral != 10000000000 { // 100 TWINS in satoshis
		t.Errorf("Expected regtest Platinum collateral 10000000000, got %d", platinumCollateral)
	}
}

func TestNetworkParameterConsistency(t *testing.T) {
	networks := []*ChainParams{
		MainnetParams(),
		TestnetParams(),
		RegtestParams(),
	}

	for i, params := range networks {
		// Test that all networks have required masternode tiers
		if len(params.MasternodeTiers) != 4 {
			t.Errorf("Network %d should have 4 masternode tiers, got %d", i, len(params.MasternodeTiers))
		}

		// Test that all tiers are present
		requiredTiers := []MasternodeTier{
			MasternodeTierBronze,
			MasternodeTierSilver,
			MasternodeTierGold,
			MasternodeTierPlatinum,
		}

		for _, tier := range requiredTiers {
			if collateral := params.GetTierCollateral(tier); collateral <= 0 {
				t.Errorf("Network %d tier %d should have positive collateral, got %d", i, tier, collateral)
			}
		}

		// Test reward percentages are consistent
		for _, tier := range requiredTiers {
			reward := params.GetTierRewardPercentage(tier)
			if reward <= 0 {
				t.Errorf("Network %d tier %d should have positive reward percentage, got %d", i, tier, reward)
			}
		}

		// Test ascending collateral requirements
		bronze := params.GetTierCollateral(MasternodeTierBronze)
		silver := params.GetTierCollateral(MasternodeTierSilver)
		gold := params.GetTierCollateral(MasternodeTierGold)
		platinum := params.GetTierCollateral(MasternodeTierPlatinum)

		if bronze >= silver || silver >= gold || gold >= platinum {
			t.Errorf("Network %d: collateral requirements should be ascending", i)
		}

		// Test basic parameter validity
		if params.TargetSpacing <= 0 {
			t.Errorf("Network %d: target spacing should be positive", i)
		}

		if params.MaxBlockSize <= 0 {
			t.Errorf("Network %d: max block size should be positive", i)
		}

		if params.CoinbaseMaturity <= 0 {
			t.Errorf("Network %d: coinbase maturity should be positive", i)
		}

		if params.BlockReward <= 0 {
			t.Errorf("Network %d: block reward should be positive", i)
		}
	}
}

func TestRewardConstants(t *testing.T) {
	// Test that reward constants are reasonable
	if DefaultBlockReward <= 0 {
		t.Error("Default block reward should be positive")
	}

	// Test that reward percentages add up correctly (should be 100% = 10000 basis points)
	totalReward := DefaultMasternodeReward + DefaultStakeReward + DefaultDevFundReward
	if totalReward != 10000 {
		t.Errorf("Total reward percentages should equal 10000 basis points, got %d", totalReward)
	}

	// Test individual percentages are reasonable
	if DefaultMasternodeReward != 8000 { // 80%
		t.Errorf("Expected masternode reward 8000 bp, got %d", DefaultMasternodeReward)
	}

	if DefaultStakeReward != 1000 { // 10%
		t.Errorf("Expected stake reward 1000 bp, got %d", DefaultStakeReward)
	}

	if DefaultDevFundReward != 1000 { // 10%
		t.Errorf("Expected dev fund reward 1000 bp, got %d", DefaultDevFundReward)
	}
}

func BenchmarkGetTierCollateral(b *testing.B) {
	params := MainnetParams()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = params.GetTierCollateral(MasternodeTierBronze)
	}
}

func BenchmarkIsValidTier(b *testing.B) {
	params := MainnetParams()
	collateral := int64(100000000000000) // Bronze tier

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = params.IsValidTier(collateral)
	}
}

func BenchmarkGetTierFromCollateral(b *testing.B) {
	params := MainnetParams()
	collateral := int64(500000000000000) // Silver tier

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = params.GetTierFromCollateral(collateral)
	}
}
