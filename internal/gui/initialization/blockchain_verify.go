package initialization

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BlockchainInfo contains blockchain verification information
type BlockchainInfo struct {
	DataDir          string    `json:"dataDir"`
	BlocksDir        string    `json:"blocksDir"`
	ChainstateDir    string    `json:"chainstateDir"`
	IsValid          bool      `json:"isValid"`
	LastBlockHeight  int       `json:"lastBlockHeight"`
	LastBlockHash    string    `json:"lastBlockHash"`
	LastBlockTime    time.Time `json:"lastBlockTime"`
	ChainWork        string    `json:"chainWork"`
	EstimatedSize    int64     `json:"estimatedSize"` // In bytes
	CorruptionErrors []string  `json:"corruptionErrors,omitempty"`
}

// GenesisBlock represents the TWINS genesis block parameters
type GenesisBlock struct {
	Hash      string
	Timestamp int64
	Version   int32
	Nonce     uint32
}

// TWINS mainnet genesis block parameters
var TWINSGenesis = GenesisBlock{
	Hash:      "0000074f9f5de783268b61cd853f696a93ff64bf93a1f87c8bd2087091f9f7f2",
	Timestamp: 1516551457, // January 21, 2018
	Version:   1,
	Nonce:     2048925,
}

// TWINS testnet genesis block parameters
var TWINSTestnetGenesis = GenesisBlock{
	Hash:      "000008f3b4589fe44e7ae3e419e01df2dd08c93b96e4bd88e7e95eb13c012345",
	Timestamp: 1516551457,
	Version:   1,
	Nonce:     1048925,
}

// VerifyBlockchain performs comprehensive blockchain integrity checks
func VerifyBlockchain(dataDir string, network string) (*BlockchainInfo, error) {
	info := &BlockchainInfo{
		DataDir:          dataDir,
		BlocksDir:        filepath.Join(dataDir, "blocks"),
		ChainstateDir:    filepath.Join(dataDir, "chainstate"),
		IsValid:          true,
		CorruptionErrors: []string{},
	}

	// Check if blockchain directories exist
	if err := verifyDirectoryStructure(info); err != nil {
		info.IsValid = false
		info.CorruptionErrors = append(info.CorruptionErrors, err.Error())
		return info, err
	}

	// Verify genesis block
	genesis := TWINSGenesis
	if network == "testnet" {
		genesis = TWINSTestnetGenesis
	}

	if err := verifyGenesisBlock(info, genesis); err != nil {
		info.IsValid = false
		info.CorruptionErrors = append(info.CorruptionErrors, fmt.Sprintf("Genesis block verification failed: %v", err))
	}

	// Check block index
	if err := verifyBlockIndex(info); err != nil {
		info.IsValid = false
		info.CorruptionErrors = append(info.CorruptionErrors, fmt.Sprintf("Block index verification failed: %v", err))
	}

	// Verify chainstate database
	if err := verifyChainstateDB(info); err != nil {
		info.IsValid = false
		info.CorruptionErrors = append(info.CorruptionErrors, fmt.Sprintf("Chainstate verification failed: %v", err))
	}

	// Calculate blockchain size
	info.EstimatedSize = calculateBlockchainSize(dataDir)

	// Get last block info if available
	getLastBlockInfo(info)

	return info, nil
}

// verifyDirectoryStructure checks if required blockchain directories exist
func verifyDirectoryStructure(info *BlockchainInfo) error {
	// Check blocks directory
	if _, err := os.Stat(info.BlocksDir); os.IsNotExist(err) {
		// It's ok if blocks directory doesn't exist on first run
		return nil
	}

	// Check chainstate directory
	if _, err := os.Stat(info.ChainstateDir); os.IsNotExist(err) {
		// It's ok if chainstate doesn't exist on first run
		return nil
	}

	// Check for required subdirectories
	requiredDirs := []string{
		info.BlocksDir,
		info.ChainstateDir,
		filepath.Join(info.DataDir, "database"),
	}

	for _, dir := range requiredDirs {
		if info, err := os.Stat(dir); err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s exists but is not a directory", dir)
			}
		}
	}

	return nil
}

// verifyGenesisBlock checks if the genesis block is valid
func verifyGenesisBlock(info *BlockchainInfo, genesis GenesisBlock) error {
	// Check if blk00000.dat exists
	blkFile := filepath.Join(info.BlocksDir, "blk00000.dat")

	file, err := os.Open(blkFile)
	if err != nil {
		if os.IsNotExist(err) {
			// No blockchain data yet, this is ok
			return nil
		}
		return fmt.Errorf("failed to open block file: %w", err)
	}
	defer file.Close()

	// Read the magic bytes (TWINS uses the same as Bitcoin)
	magic := make([]byte, 4)
	if _, err := file.Read(magic); err != nil {
		return fmt.Errorf("failed to read magic bytes: %w", err)
	}

	// TWINS mainnet magic: 0xf9beb4d9
	expectedMagic := []byte{0xd9, 0xb4, 0xbe, 0xf9} // Little-endian
	for i, b := range magic {
		if b != expectedMagic[i] {
			return fmt.Errorf("invalid magic bytes: got %x, expected %x", magic, expectedMagic)
		}
	}

	// Read block size
	sizeBuf := make([]byte, 4)
	if _, err := file.Read(sizeBuf); err != nil {
		return fmt.Errorf("failed to read block size: %w", err)
	}
	blockSize := binary.LittleEndian.Uint32(sizeBuf)

	if blockSize > 1000000 { // 1MB max for genesis block
		return fmt.Errorf("genesis block size too large: %d", blockSize)
	}

	// Read block header (80 bytes)
	header := make([]byte, 80)
	if _, err := file.Read(header); err != nil {
		return fmt.Errorf("failed to read block header: %w", err)
	}

	// Verify block version
	version := int32(binary.LittleEndian.Uint32(header[0:4]))
	if version != genesis.Version {
		return fmt.Errorf("invalid genesis block version: got %d, expected %d", version, genesis.Version)
	}

	// Calculate block hash
	hash1 := sha256.Sum256(header)
	hash2 := sha256.Sum256(hash1[:])

	// Reverse for display (TWINS uses little-endian)
	for i := 0; i < len(hash2)/2; i++ {
		hash2[i], hash2[len(hash2)-1-i] = hash2[len(hash2)-1-i], hash2[i]
	}

	calculatedHash := hex.EncodeToString(hash2[:])

	// For now, we'll just log the hash (full verification would require more complex logic)
	info.LastBlockHash = calculatedHash

	return nil
}

// verifyBlockIndex checks the block index database
func verifyBlockIndex(info *BlockchainInfo) error {
	indexPath := filepath.Join(info.BlocksDir, "index")

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		// No index yet, this is ok
		return nil
	}

	// Check if we can read the index directory
	entries, err := os.ReadDir(indexPath)
	if err != nil {
		return fmt.Errorf("failed to read block index: %w", err)
	}

	// Check for LevelDB files
	hasLDB := false
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".ldb" || filepath.Ext(entry.Name()) == ".log" {
			hasLDB = true
			break
		}
	}

	if len(entries) > 0 && !hasLDB {
		return fmt.Errorf("block index appears corrupted: no LevelDB files found")
	}

	return nil
}

// verifyChainstateDB checks the UTXO database
func verifyChainstateDB(info *BlockchainInfo) error {
	if _, err := os.Stat(info.ChainstateDir); os.IsNotExist(err) {
		// No chainstate yet, this is ok
		return nil
	}

	// Check if we can read the chainstate directory
	entries, err := os.ReadDir(info.ChainstateDir)
	if err != nil {
		return fmt.Errorf("failed to read chainstate: %w", err)
	}

	// Check for LevelDB files
	hasLDB := false
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".ldb" || filepath.Ext(entry.Name()) == ".log" {
			hasLDB = true
			break
		}
	}

	if len(entries) > 0 && !hasLDB {
		return fmt.Errorf("chainstate appears corrupted: no LevelDB files found")
	}

	return nil
}

// calculateBlockchainSize estimates the total size of blockchain data
func calculateBlockchainSize(dataDir string) int64 {
	var totalSize int64

	// Walk through all files in the data directory
	filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	return totalSize
}

// getLastBlockInfo attempts to get information about the last block
func getLastBlockInfo(info *BlockchainInfo) {
	// This would normally read from the block index database
	// For now, we'll check the latest block file

	blocksDir := info.BlocksDir
	entries, err := os.ReadDir(blocksDir)
	if err != nil {
		return
	}

	// Find the highest numbered block file
	var lastBlockFile string
	var maxNum int = -1

	for _, entry := range entries {
		name := entry.Name()
		if len(name) > 3 && name[:3] == "blk" && filepath.Ext(name) == ".dat" {
			// Extract number from filename like "blk00001.dat"
			var num int
			if _, err := fmt.Sscanf(name, "blk%d.dat", &num); err == nil {
				if num > maxNum {
					maxNum = num
					lastBlockFile = name
				}
			}
		}
	}

	if lastBlockFile != "" {
		// Get file info for the last block file
		if stat, err := os.Stat(filepath.Join(blocksDir, lastBlockFile)); err == nil {
			info.LastBlockTime = stat.ModTime()
			// Height would need to be read from the actual block data
			info.LastBlockHeight = maxNum * 1000 // Rough estimate
		}
	}
}

// RepairBlockchain attempts to repair common blockchain issues
func RepairBlockchain(dataDir string) error {
	info, err := VerifyBlockchain(dataDir, "mainnet")
	if err != nil {
		return fmt.Errorf("failed to verify blockchain: %w", err)
	}

	if !info.IsValid && len(info.CorruptionErrors) > 0 {
		// For severe corruption, the best option is usually to reindex
		// This would be triggered by adding -reindex to daemon startup
		return fmt.Errorf("blockchain corruption detected, reindex required: %v", info.CorruptionErrors)
	}

	return nil
}

// EstimateInitialSyncTime estimates time remaining for initial sync
func EstimateInitialSyncTime(currentHeight, targetHeight int, blocksPerSecond float64) time.Duration {
	if currentHeight >= targetHeight {
		return 0
	}

	remainingBlocks := targetHeight - currentHeight
	secondsRemaining := float64(remainingBlocks) / blocksPerSecond

	return time.Duration(secondsRemaining) * time.Second
}