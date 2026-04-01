package blockchain

import (
	"fmt"

	"github.com/twins-dev/twins-core/pkg/types"
)

// Checkpoint represents a blockchain checkpoint
type Checkpoint struct {
	Height uint32
	Hash   types.Hash
}

// CheckpointManager manages blockchain checkpoints for consensus validation
type CheckpointManager struct {
	checkpoints map[uint32]types.Hash
	badBlocks   map[uint32]types.Hash // Blacklisted blocks
	lastHeight  uint32
}

// NewCheckpointManager creates a new checkpoint manager with hardcoded checkpoints
func NewCheckpointManager(network string) *CheckpointManager {
	cm := &CheckpointManager{
		checkpoints: make(map[uint32]types.Hash),
		badBlocks:   make(map[uint32]types.Hash),
		lastHeight:  0,
	}

	switch network {
	case "mainnet":
		cm.loadMainnetCheckpoints()
	case "testnet":
		cm.loadTestnetCheckpoints()
	case "regtest":
		// No checkpoints for regtest
	}

	return cm
}

// loadMainnetCheckpoints loads the hardcoded mainnet checkpoints from legacy chainparams.cpp
func (cm *CheckpointManager) loadMainnetCheckpoints() {
	// Critical checkpoints from legacy/src/chainparams.cpp lines 55-142
	checkpoints := []Checkpoint{
		{0, types.MustParseHash("0000071cf2d95aec5ba4818418756c93cb12cd627191710e8969f2f35c3530de")}, // genesis
		{2000, types.MustParseHash("70362c3e307213d37dcc57d89f64d9bdeb7779e368f1c34c9b24af0dce72a6ae")},
		{50000, types.MustParseHash("f5c1fe5c20a9f1fd981f5292d49293e59dd6fba685021994891f02667dce086b")},
		{80000, types.MustParseHash("917998d20d8447861ef56dfc401bfa026ed5230953e20c046db2634f2b7e96df")}, // FakePoS attack region
		{81000, types.MustParseHash("90555f5617101e868f41faf66c976b63c7a86b6c521c6f7e40f4aed8dd88be55")},
		{82000, types.MustParseHash("71ab39d5584b00e3b82e4e5bf35eb686278bbee50c02c6006f02d971ec52e57f")},
		{83000, types.MustParseHash("0c05d80f0773e8d1a306ddcaa0c3d28c460e18e327a5b4f9369e0920bb91cf09")},
		{84123, types.MustParseHash("1db6bab4574c294c9b5378e23997bc8a383f4ae25934041fb380634c1678d2c1")},
		{85000, types.MustParseHash("4b74d103475d7d606de8534a87bb3449fdf11e92e9dd6baa5fb7f1fe12656e9b")},
		{87000, types.MustParseHash("6514126e1b1439b9b8a16657caff1f9bb7742d355e2e825a9a46e48aa99ee91e")},
		{89000, types.MustParseHash("4ea0c0c13808fc908de9d24b318bba3330eb4626d7188ee089b42264db90fb17")},
		{93000, types.MustParseHash("9068c3e6fb672ff33b91c35ae3fa466f4f5b57da53d332bdfb339ca5bf2219a2")},
		{100000, types.MustParseHash("db0569db100c482e19789c6459ccfe906415fe5bbfc2d5bacfc93a4c03938c36")},
		{106344, types.MustParseHash("1570eb5464f9d586ee0de79524b3c360b54e53610ebe5cf3964cf58637db366e")}, // split
		{110000, types.MustParseHash("b94b2bfe0cf63604784d28dce62f78c29c492d2fb276ce1b78661d82985e7365")},
		{113000, types.MustParseHash("06dec9e49608a776876766b1a9edc690c1d5e5f53e0e59b1f85829cb8331827f")},
		{113500, types.MustParseHash("d053ed7eb99b639c4b45817b4c7a00699e1dc8e5043f474ab8b351da1bd3ee52")},
		{113715, types.MustParseHash("0b6252798e7628257417e927dc3f1e9cad56fe2042700d2a8803923659953d8e")},
		{113719, types.MustParseHash("81923b7058cf8b3eb3e2e69f89bc23f290ab386cc8149c7e4b2adcff12e826df")}, // Last block before split
		{113720, types.MustParseHash("d31998ff85218e6cac477cd7d7c7bfa91009a58d4371ab1bed148d4cbf72158f")}, // mainnet
		{113871, types.MustParseHash("44fc9bc5c0b1a35aa2f1dc3c7ee1722722873e2dfb0c013ecbea66caa4ec86a5")}, // before split
		{113872, types.MustParseHash("13bb2e1c7c95dd40c631d0fb314b6b4b27b583bffa1e5c5e4535ae34ee02fb68")}, // mainnet
		{114000, types.MustParseHash("42d1dea87efa95cac8d1ede5d2ed37f6f99929edd9b17e946dbd9e2e520528a8")},
		{114500, types.MustParseHash("5bd14bcb6b82cc1b0f073f902db59ecafa5d9d364cb6a91bd78b7377b7a61921")},
		{115200, types.MustParseHash("9b23c51a7d288f0c2aa82601cfbd3499c23d0696194a75afa8b2ef4aca909eaa")},
		{116000, types.MustParseHash("42d9dd5852a3cf0d9a799e4eb1ab8b54609aef25a1c649562d7453dae48b77b8")},
		{117000, types.MustParseHash("6e6983dccf2a30662c5ae82370c6911580dd79ce40a1ddf56cb732d36f7d9687")},
		{117250, types.MustParseHash("98a0eefbfe111c51784944b2c650b2cc5ba749b21cba837d9734a1a45a4c36f2")},
		{200000, types.MustParseHash("c0a39556e6d7f0c80c7efc479640151c0db31f38afcf91dd72f6a2b9897c76d2")},
		{230000, types.MustParseHash("eee7bfa3489249b8547cb539468cb7df139606f88d303afa2b175e317e8c865e")},
		{250000, types.MustParseHash("c3ed34dde5ebd0a679667ab37891a5079207847ba55ee6ba40872e693a2c0b67")},
		{280000, types.MustParseHash("85ac815a816a8c98cbdd9c10c26edd71c4edd7eb87cdc473e650e12981307ffe")},
		{300000, types.MustParseHash("9e1eb7bfc58e66ad4ff62ca20b6c0aa838c2a6151100c574548317affb214212")},
		{314883, types.MustParseHash("07ed00a702b3e78a6a1664befb950ecb08e112ad1f4af65f3d6b08d593b84929")},
		{316999, types.MustParseHash("867f64f36a34f99881b19e9e4802149b3f37978085245fb37be6fd1a0583f459")},
		{317000, types.MustParseHash("1657f2baa2b635805daccb32adbe7b5da847b2e96f7a7da8b213cccc6c90f262")}, // CRITICAL: Protocol enforcement (70922)
		{317001, types.MustParseHash("ed7a7c72e9c31ef0e9e08da12a0ac57ccd87ff0ddf0e6694e4ce1edd9761b23b")},
		{317005, types.MustParseHash("44405d602b2dca51798def580721c768280cb57f729bf74c02d586caa0b22812")},
		{317167, types.MustParseHash("a5e4d2c9ebb2411119970a6e185e6e0d7f8b14e2d769cefc4c7c442b36cc7e39")},
		{317168, types.MustParseHash("6b3ed5c27f90e5d463a7991c95933f7918e9fc1c3cc070333280e1dc4d211497")}, // known split
		{320500, types.MustParseHash("013b9e3d4cd2511507ba73c93a71cb53095a78e81c9256deb14d94f9706efa7c")},
		{321000, types.MustParseHash("f952755ad4ced9dcd70ab83686548d53ca690cd5bad6e60e43e76231f2ff1992")},
		{322000, types.MustParseHash("850c8f67919f8dc23679e0b5ef1e2599034b729efc6f44387462efb3e1f95e70")},
		{323000, types.MustParseHash("622192c9e38d1cfcfe76fe9659bc89d430d17b04c031e119b4698fdf31c6435f")},
		{323987, types.MustParseHash("4a2d43e8b9a22cd6789d26aecf103d70035d6d038a669336125d802aa9b19c78")}, // 70923 enforcement
		{323989, types.MustParseHash("410704a99d95725af169c8805607bb09881c7c840105b7430a1e007ff38943c5")},
		{324000, types.MustParseHash("6b7e2a40ed3fc09c93fbd4e3073de6c2ddbe0b048499686e537257515e0bc165")},
		{325000, types.MustParseHash("93cd883cf9858fb944459a1aa3a30d3487372b21666d72bb2ebd4d0b1a39e580")},
		{322999, types.MustParseHash("d22553c89e2bcf2880da68f06ac677a535388583ead3b3a14671f19be43f6e77")},
		{332120, types.MustParseHash("27045d11d68c210997565a8a7b1ff8783af10c864f00e773f6355a860a349d0d")},
		{333499, types.MustParseHash("be482c0c599bacba2b3673482c56a5a7804924883d2c8269f2a7e4d1a8bd2e28")},
		{333500, types.MustParseHash("fa4657c82b54cb75510d3b0f14e4a5131f6ca972a5522806ba78cedc3e4fa02b")}, // Min stake fix
		{333510, types.MustParseHash("206a4fd16dcf58c549f13faebf7dba877041b9edb379f3386561bf9df7901cff")},
		{340000, types.MustParseHash("28a4de165cf11170b035515972e232c711981434f50b62400b36d58e4929a95c")},
		{350000, types.MustParseHash("e5954e2a84643ffc67f0ec6d98a768918844be9a58558c01df14eb482cdc6b70")},
		{360000, types.MustParseHash("89475bd848b0086430bc6532ba4b695abea900ab84b10c1260b5f236bd635e79")},
		{375600, types.MustParseHash("f71da43f75eedda8c86003d891a254254c484f8044a79089df101be0aa0a52c8")},
		{382000, types.MustParseHash("045aec823149c164bad0341de41f98e844f228ede271cf9c671b190805487de4")}, // 70924 enforcement
		{382050, types.MustParseHash("6f510b37501369a22d9a1f9e3f3ceec4a476e731b15215b678a9cf908b71ced6")},
		{382150, types.MustParseHash("bf2d5cdc298a332f2ee1ec7caf45ec10f5a6c08b52946cc076ae7edd89133351")},
		{394756, types.MustParseHash("8aa66d6721481b39813df844677c6142e39a6e068584b5503a9d48621ad0c9e4")},
		{420000, types.MustParseHash("ade10823f13d1f5f6b9f3a4722b8956878d17d257482768680b0bf4bc0493201")},
		{441201, types.MustParseHash("593456c5007465e968b7487fd55b2e36dbfb18dda45109551c5d4a83067e0a5e")}, // split
		{441397, types.MustParseHash("777d3d58642fae753b67d503362195607006a1c017e89b082a27123d080b784d")},
		{442325, types.MustParseHash("1d16bad5a8da7dc3c0bc2f417ec35d2c7ff558a7ee95bf0f731e308324b4dfa4")},
		{500000, types.MustParseHash("9f5aaff290c48d5a8cd53e3b7cb7eea911a5f10831b0a946348a1db1a645a2ec")},
		{550000, types.MustParseHash("c6f21271fbdaec10dfee16d9cf4c4d073d78b1282d1aa4414001a4ec5fc05008")},
		{650000, types.MustParseHash("8942cf0b3850d448289ebe663e074c122bf0d36361bf6dd374aeeffa2b9c340b")},
		{682400, types.MustParseHash("89708f4d054771b2079854b415fb8d336d9f16c9b5cd1342d3b233229a35c97c")}, // preparing update for 70926
		{907970, types.MustParseHash("3d9698bc000068035cebe5ed382ae5bb660b8debe271dffed70f914de22bc9aa")},
		{907995, types.MustParseHash("a626b8fe541f3a07ba0f59836cc631dd81db07296a158287635e67fd2ba5225a")}, // Network recovery & fork. 08.2022 (start)
		{907996, types.MustParseHash("4faaf1b056965328980ae1b6caeca7428698f916110e473e39c161543d32a767")},
		{907997, types.MustParseHash("a99b449ac84f53f3a9c7aba594654b2a7f803f94b25b0f835337e160e8e93f3a")},
		{907998, types.MustParseHash("1b629d939db0c4f71e42d206b59ea0f2c0f1019c8b5f46b66241e9ea25e8854a")},
		{907999, types.MustParseHash("3714adc51af727462177131098c5ed6f46489350eeab716c8f96b744afdc88d0")},
		{908000, types.MustParseHash("f9f9d700a25f1b8a98ed53829278180a75da9781bde525a0cc90a1ef9b2fa8fc")}, // CRITICAL: Network recovery fork
		{908001, types.MustParseHash("58dfee65f4fe0d7d97e5618c10c5cb68ee50e9a210189ab8e1aa0dae659087a8")},
		{908002, types.MustParseHash("a43c4301f66e211818915d2692a6455addbc19e299590cd13d78c061764094e0")},
		{908003, types.MustParseHash("6a13fffd1d02ad616682ed92ec72076bdfab0453fc9aa6624a846640daa539d3")},
		{908004, types.MustParseHash("cacf903e1d9c05a182d7dd480ffb9349e9d9de1242f5740c453194aec19f5b88")},
		{908005, types.MustParseHash("033d4929561bf239b1d8c32a702690f1fae903ef4031ae6f320159ba8b52b5fe")},
		{908006, types.MustParseHash("1e8900e8a6872310af5d7171a2252a390846c4bc5e73c8ad35f7e2ff35f460f9")},
		{908007, types.MustParseHash("7933e12ba1321b30385f37254fa40d62cf064fd60766c65cfc97179770125720")},
		{908614, types.MustParseHash("e94ff6dd468d909e14d447762bb52051e41ea0bf91b10d48869fd093cdb540df")}, // Network recovery & fork. 08.2022 (end)
		{909389, types.MustParseHash("bd5a38aa452c21e6c6039cd464fe60366bbed8964735088391ee0c6cca68bd02")},
		{1000000, types.MustParseHash("bae87eafb178c491ceb8bd91f42a6a79bfc819edb56c34a9359ef39c0355c904")},
		{1100000, types.MustParseHash("59672b45b3dc70ddec92a8d96a715c7887d05b9848c64f9d7b9f981eada9ebe7")},
		{1200000, types.MustParseHash("4244ee62d68cb62953f0cfa8845288b341b4fe96dce6702c89c0c1ea7e441777")},
		{1300000, types.MustParseHash("354f146aa6ea110c89a4fc44ad70cef6ddaa988e279394b9a6f1c8bb16c3c3bb")},
		{1400000, types.MustParseHash("9dd68068645ca254b948cdfd12a2e3c179fddb581666b57ba06a80f009ee27b6")},
		{1500000, types.MustParseHash("f3aea73b8854713c00abff4ff642c877858c0530203d40d20d0ad8ad2329e48c")},
		{1600000, types.MustParseHash("d85de4c53bf48e89b6083e168110dc71cbc3e6fe9f0ecdc263ce5443fe41f782")},
	}

	// Add all checkpoints to the manager
	for _, checkpoint := range checkpoints {
		cm.checkpoints[checkpoint.Height] = checkpoint.Hash
		if checkpoint.Height > cm.lastHeight {
			cm.lastHeight = checkpoint.Height
		}
	}

	// Bad blocks blacklist from legacy chainparams.cpp lines 147-152
	badBlocks := []Checkpoint{
		{907998, types.MustParseHash("91ee7fb080b75f37cd96223134d52cccb9fdc19f2698c65a4343356a1421e0b0")},
		{907999, types.MustParseHash("75759383da8f63daa3980f5b1767287a204124c317083780d457ebd6d15d4639")}, // 29.7 - 908000
		{908000, types.MustParseHash("3a1daa15d6ab64d79d4f71e40dfde714fd5f0d181b12a4123d9e555b216fe358")},
	}

	// Add bad blocks to the blacklist
	for _, badBlock := range badBlocks {
		cm.badBlocks[badBlock.Height] = badBlock.Hash
	}
}

// loadTestnetCheckpoints loads testnet checkpoints (minimal for testnet)
func (cm *CheckpointManager) loadTestnetCheckpoints() {
	// For testnet, just add genesis
	cm.checkpoints[0] = types.MustParseHash("00000c538590ec8fc7c6725262788f25cb5cd4aa3120f1fcb4fe5f135f6a0eeb")
}

// ValidateCheckpoint checks if a block at the given height matches the expected checkpoint
func (cm *CheckpointManager) ValidateCheckpoint(height uint32, hash types.Hash) error {
	// Check if this is a blacklisted block
	if badHash, exists := cm.badBlocks[height]; exists {
		if hash == badHash {
			return fmt.Errorf("block %d with hash %s is blacklisted", height, hash)
		}
	}

	// Check if we have a checkpoint for this height
	if expectedHash, exists := cm.checkpoints[height]; exists {
		if hash != expectedHash {
			return fmt.Errorf("checkpoint mismatch at height %d: expected %s, got %s",
				height, expectedHash, hash)
		}
	}

	return nil
}

// GetCheckpoint returns the checkpoint at the given height if it exists
func (cm *CheckpointManager) GetCheckpoint(height uint32) (types.Hash, bool) {
	hash, exists := cm.checkpoints[height]
	return hash, exists
}

// GetLastCheckpointHeight returns the height of the last checkpoint
func (cm *CheckpointManager) GetLastCheckpointHeight() uint32 {
	return cm.lastHeight
}

// IsCheckpointHeight returns true if the given height has a checkpoint
func (cm *CheckpointManager) IsCheckpointHeight(height uint32) bool {
	_, exists := cm.checkpoints[height]
	return exists
}

// IsBadBlock checks if a block hash is in the bad blocks list
func (cm *CheckpointManager) IsBadBlock(height uint32, hash types.Hash) bool {
	if badHash, exists := cm.badBlocks[height]; exists {
		return hash == badHash
	}
	return false
}

// GetCheckpoints returns all checkpoints
func (cm *CheckpointManager) GetCheckpoints() map[uint32]types.Hash {
	return cm.checkpoints
}

// GetNearestCheckpoint returns the nearest checkpoint at or before the given height
func (cm *CheckpointManager) GetNearestCheckpoint(height uint32) (uint32, types.Hash, bool) {
	var nearestHeight uint32
	var nearestHash types.Hash
	found := false

	for checkHeight, checkHash := range cm.checkpoints {
		if checkHeight <= height && checkHeight > nearestHeight {
			nearestHeight = checkHeight
			nearestHash = checkHash
			found = true
		}
	}

	return nearestHeight, nearestHash, found
}
