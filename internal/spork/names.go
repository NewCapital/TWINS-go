// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package spork

// GetSporkName returns the name of a spork by ID
func GetSporkName(id int32) string {
	switch id {
	case SporkMaxValue:
		return "SPORK_5_MAX_VALUE"
	case SporkMasternodeScanning:
		return "SPORK_7_MASTERNODE_SCANNING"
	case SporkMasternodePaymentEnforcement:
		return "SPORK_8_MASTERNODE_PAYMENT_ENFORCEMENT"
	case SporkMasternodePayUpdatedNodes:
		return "SPORK_10_MASTERNODE_PAY_UPDATED_NODES"
	case SporkNewProtocolEnforcement:
		return "SPORK_14_NEW_PROTOCOL_ENFORCEMENT"
	case SporkNewProtocolEnforcement2:
		return "SPORK_15_NEW_PROTOCOL_ENFORCEMENT_2"
	case SporkTwinsEnableMasternodeTiers:
		return "SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS"
	case SporkTwinsMinStakeAmount:
		return "SPORK_TWINS_02_MIN_STAKE_AMOUNT"
	default:
		return "Unknown"
	}
}

// GetSporkID returns the spork ID by name
func GetSporkID(name string) int32 {
	switch name {
	case "SPORK_5_MAX_VALUE":
		return SporkMaxValue
	case "SPORK_7_MASTERNODE_SCANNING":
		return SporkMasternodeScanning
	case "SPORK_8_MASTERNODE_PAYMENT_ENFORCEMENT":
		return SporkMasternodePaymentEnforcement
	case "SPORK_10_MASTERNODE_PAY_UPDATED_NODES":
		return SporkMasternodePayUpdatedNodes
	case "SPORK_14_NEW_PROTOCOL_ENFORCEMENT":
		return SporkNewProtocolEnforcement
	case "SPORK_15_NEW_PROTOCOL_ENFORCEMENT_2":
		return SporkNewProtocolEnforcement2
	case "SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS":
		return SporkTwinsEnableMasternodeTiers
	case "SPORK_TWINS_02_MIN_STAKE_AMOUNT":
		return SporkTwinsMinStakeAmount
	default:
		return -1
	}
}