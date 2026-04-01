package wallet

import (
	"math/rand"
	"sort"
)

// CoinSelectionResult holds the result of coin selection.
type CoinSelectionResult struct {
	Selected []*UTXO
	Total    int64
}

// selectCoinsMinConf selects UTXOs with at least minConf confirmations
// (and at least minConfMine for our own change outputs) to cover targetValue.
// Uses knapsack algorithm with largest-first fallback.
// Legacy: CWallet::SelectCoinsMinConf (wallet.cpp:1432)
func selectCoinsMinConf(spendable []*UTXO, targetValue int64, minConf int32, minConfMine int32, currentHeight uint32, coinbaseMaturity int32, spendZeroConfChange bool) *CoinSelectionResult {
	// Filter UTXOs by confirmation requirements
	eligible := make([]*UTXO, 0, len(spendable))
	for _, utxo := range spendable {
		confirmations := int32(0)
		if utxo.BlockHeight >= 0 && currentHeight >= uint32(utxo.BlockHeight) {
			confirmations = int32(currentHeight) - utxo.BlockHeight + 1
		}

		// Check maturity for coinbase/stake
		if (utxo.IsCoinbase || utxo.IsStake) && confirmations < coinbaseMaturity {
			continue
		}

		// Determine required confirmations for this UTXO
		requiredConf := minConf
		if utxo.IsChange {
			requiredConf = minConfMine
			// Allow zero-conf change if enabled
			// Legacy: wallet.cpp:1454 (nDepthFrom == 0 with fIsFromMe)
			if spendZeroConfChange && confirmations == 0 {
				requiredConf = 0
			}
		}

		if confirmations < requiredConf {
			continue
		}

		eligible = append(eligible, utxo)
	}

	if len(eligible) == 0 {
		return nil
	}

	// Check if exact match exists or if total is insufficient
	totalAvailable := int64(0)
	for _, utxo := range eligible {
		totalAvailable += utxo.Output.Value
		if utxo.Output.Value == targetValue {
			return &CoinSelectionResult{
				Selected: []*UTXO{utxo},
				Total:    utxo.Output.Value,
			}
		}
	}

	if totalAvailable < targetValue {
		return nil
	}

	// If total equals target exactly, use all
	if totalAvailable == targetValue {
		return &CoinSelectionResult{
			Selected: eligible,
			Total:    totalAvailable,
		}
	}

	// Sort ascending by value for knapsack
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Output.Value < eligible[j].Output.Value
	})

	// Track the smallest single coin that is larger than target (coinLowestLarger)
	// Legacy: wallet.cpp:1476
	var coinLowestLarger *UTXO
	for i := len(eligible) - 1; i >= 0; i-- {
		if eligible[i].Output.Value >= targetValue {
			coinLowestLarger = eligible[i]
		} else {
			break // sorted ascending, so all remaining are smaller
		}
	}

	// Filter to coins smaller than target + CENT for knapsack.
	// Legacy: wallet.cpp:2544 uses (n < nTargetValue + CENT) for vValue set.
	smallerCoins := make([]*UTXO, 0, len(eligible))
	for _, utxo := range eligible {
		if utxo.Output.Value < targetValue+CentThreshold {
			smallerCoins = append(smallerCoins, utxo)
		}
	}

	// Try knapsack with target value
	bestResult := approximateBestSubset(smallerCoins, targetValue)

	// Legacy 2-pass CENT logic:
	// If we found a solution >= target, try again targeting (target + CENT)
	// to see if we can avoid creating small change.
	// Legacy: wallet.cpp:1500-1511
	if bestResult != nil && bestResult.Total >= targetValue {
		betterResult := approximateBestSubset(smallerCoins, targetValue+CentThreshold)
		if betterResult != nil && betterResult.Total >= targetValue+CentThreshold {
			bestResult = betterResult
		}
	}

	// Compare knapsack result with coinLowestLarger.
	// Legacy: wallet.cpp:2590-2591
	//   if ((nBest != nTargetValue && nBest < nTargetValue + CENT) || coinLowestLarger.first <= nBest)
	// Prefer coinLowestLarger when:
	//   (a) knapsack result is not exact AND produces small change (< CENT) that is uneconomical, OR
	//   (b) coinLowestLarger is smaller/equal to the knapsack total (fewer inputs = cheaper)
	if bestResult != nil && bestResult.Total >= targetValue {
		if coinLowestLarger != nil {
			knapsackNotExact := bestResult.Total != targetValue
			knapsackSmallChange := bestResult.Total < targetValue+CentThreshold
			coinLowestLargerCheaper := coinLowestLarger.Output.Value <= bestResult.Total

			if (knapsackNotExact && knapsackSmallChange) || coinLowestLargerCheaper {
				return &CoinSelectionResult{
					Selected: []*UTXO{coinLowestLarger},
					Total:    coinLowestLarger.Output.Value,
				}
			}
		}
		return bestResult
	}

	// Knapsack couldn't find a subset, use coinLowestLarger if available
	if coinLowestLarger != nil {
		return &CoinSelectionResult{
			Selected: []*UTXO{coinLowestLarger},
			Total:    coinLowestLarger.Output.Value,
		}
	}

	// Last resort: largest-first greedy (fallback beyond legacy behavior)
	return selectLargestFirst(eligible, targetValue)
}

// approximateBestSubset uses stochastic optimization to find a subset of coins
// close to the target value. Runs 1000 iterations with random inclusion.
// Legacy: ApproximateBestSubset (wallet.cpp:1358-1425)
func approximateBestSubset(coins []*UTXO, targetValue int64) *CoinSelectionResult {
	if len(coins) == 0 {
		return nil
	}

	const iterations = 1000

	// Initialize with "use all coins" as fallback, matching legacy C++.
	// Legacy: wallet.cpp:2329-2330 (vfBest = all true, nBest = nTotalLower)
	totalAll := int64(0)
	for _, c := range coins {
		totalAll += c.Output.Value
	}
	if totalAll < targetValue {
		return nil
	}

	bestTotal := totalAll
	bestSelection := make([]bool, len(coins))
	for i := range bestSelection {
		bestSelection[i] = true
	}

	for i := 0; i < iterations; i++ {
		included := make([]bool, len(coins))
		total := int64(0)
		reachedTarget := false

		// Random walk: go through coins in random order, include with 50% probability
		// on first pass, include all remaining on second pass if needed
		for pass := 0; pass < 2 && !reachedTarget; pass++ {
			for j := 0; j < len(coins); j++ {
				if included[j] {
					continue
				}
				// First pass: include with 50% probability
				// Second pass: include all remaining
				if pass == 1 || rand.Intn(2) == 0 {
					total += coins[j].Output.Value
					included[j] = true
					if total >= targetValue {
						reachedTarget = true
						// Try to improve by removing coins that push us over
						// Legacy: wallet.cpp:1409-1421
						for k := len(coins) - 1; k >= 0; k-- {
							if included[k] && total-coins[k].Output.Value >= targetValue {
								total -= coins[k].Output.Value
								included[k] = false
							}
						}
						break
					}
				}
			}
		}

		// Update best if this is closer to target (but still >= target)
		if reachedTarget {
			if total < bestTotal {
				bestTotal = total
				copy(bestSelection, included)
			}
			// Perfect match - stop early
			if total == targetValue {
				break
			}
		}
	}

	selected := make([]*UTXO, 0)
	for i, inc := range bestSelection {
		if inc {
			selected = append(selected, coins[i])
		}
	}

	return &CoinSelectionResult{
		Selected: selected,
		Total:    bestTotal,
	}
}

// filterByConfirmations filters UTXOs by the same confirmation requirements
// used in selectCoinsMinConf. Extracted so the size-aware fallback in SelectUTXOs
// can get the eligible set for a given tier without running the full knapsack.
func filterByConfirmations(spendable []*UTXO, minConf int32, minConfMine int32, currentHeight uint32, coinbaseMaturity int32, spendZeroConfChange bool) []*UTXO {
	eligible := make([]*UTXO, 0, len(spendable))
	for _, utxo := range spendable {
		confirmations := int32(0)
		if utxo.BlockHeight >= 0 && currentHeight >= uint32(utxo.BlockHeight) {
			confirmations = int32(currentHeight) - utxo.BlockHeight + 1
		}

		if (utxo.IsCoinbase || utxo.IsStake) && confirmations < coinbaseMaturity {
			continue
		}

		requiredConf := minConf
		if utxo.IsChange {
			requiredConf = minConfMine
			if spendZeroConfChange && confirmations == 0 {
				requiredConf = 0
			}
		}

		if confirmations < requiredConf {
			continue
		}

		eligible = append(eligible, utxo)
	}
	return eligible
}

// selectLargestFirst selects UTXOs largest-first until target is met.
// Used as a fallback when knapsack cannot find a valid subset.
func selectLargestFirst(coins []*UTXO, targetValue int64) *CoinSelectionResult {
	// Sort descending by value
	sorted := make([]*UTXO, len(coins))
	copy(sorted, coins)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Output.Value > sorted[j].Output.Value
	})

	selected := make([]*UTXO, 0)
	total := int64(0)
	for _, utxo := range sorted {
		selected = append(selected, utxo)
		total += utxo.Output.Value
		if total >= targetValue {
			return &CoinSelectionResult{
				Selected: selected,
				Total:    total,
			}
		}
	}

	// Insufficient funds
	return nil
}
