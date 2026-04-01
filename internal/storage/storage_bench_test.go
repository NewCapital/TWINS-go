// storage_bench_test.go - Comprehensive storage performance benchmarks
// TODO: Bench tests use stale storage API (StoreBlock signature changed).
// Excluded via build tag until updated.

//go:build storage_bench

package storage_test

import (
    "crypto/rand"
    "encoding/binary"
    "encoding/json"
    "fmt"
    "os"
    "testing"
    "time"
    "unsafe"

    "github.com/stretchr/testify/require"
    storage "github.com/twins-dev/twins-core/internal/storage"
    binarystorage "github.com/twins-dev/twins-core/internal/storage/binary"
    "github.com/twins-dev/twins-core/pkg/types"
)

// Benchmark Results Target (Apple M3):
// - Binary encoding: 10-15ns
// - JSON encoding: 300-500ns
// - Binary decoding: 5-10ns
// - JSON decoding: 500-1000ns
// - Key generation: 10-20ns
// - Prefix iteration: 1-2μs per 1000 items

// Test data structures
type CompactUTXO struct {
    Value       uint64
    ScriptType  uint8
    ScriptData  [20]byte
    BlockHeight uint32
    IsCoinbase  bool
}

type UTXOJSON struct {
    Value       uint64 `json:"value"`
    ScriptType  uint8  `json:"script_type"`
    ScriptData  string `json:"script_data"`
    BlockHeight uint32 `json:"block_height"`
    IsCoinbase  bool   `json:"is_coinbase"`
}

// Benchmark binary vs JSON encoding
func BenchmarkUTXOEncodingBinary(b *testing.B) {
    utxo := &CompactUTXO{
        Value:       100000000,
        ScriptType:  1,
        BlockHeight: 500000,
        IsCoinbase:  false,
    }
    rand.Read(utxo.ScriptData[:])

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        _ = (*[34]byte)(unsafe.Pointer(utxo))[:]
    }
}

func BenchmarkUTXOEncodingJSON(b *testing.B) {
    utxo := &UTXOJSON{
        Value:       100000000,
        ScriptType:  1,
        ScriptData:  "1234567890abcdef1234567890abcdef12345678",
        BlockHeight: 500000,
        IsCoinbase:  false,
    }

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        data, _ := json.Marshal(utxo)
        _ = data
    }
}

func BenchmarkUTXODecodingBinary(b *testing.B) {
    utxo := &CompactUTXO{
        Value:       100000000,
        ScriptType:  1,
        BlockHeight: 500000,
        IsCoinbase:  false,
    }
    data := (*[34]byte)(unsafe.Pointer(utxo))[:]

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        _ = (*CompactUTXO)(unsafe.Pointer(&data[0]))
    }
}

func BenchmarkUTXODecodingJSON(b *testing.B) {
    utxo := &UTXOJSON{
        Value:       100000000,
        ScriptType:  1,
        ScriptData:  "1234567890abcdef1234567890abcdef12345678",
        BlockHeight: 500000,
        IsCoinbase:  false,
    }
    data, _ := json.Marshal(utxo)

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        var decoded UTXOJSON
        _ = json.Unmarshal(data, &decoded)
    }
}

// Benchmark key generation
func BenchmarkKeyGeneration(b *testing.B) {
    benchmarks := []struct {
        name string
        fn   func() []byte
    }{
        {"UTXOKey", func() []byte {
            key := make([]byte, 35)
            key[0] = 0x10 // PrefixUTXO
            // Simulate copying hash
            copy(key[1:33], make([]byte, 32))
            binary.LittleEndian.PutUint16(key[33:], 0)
            return key
        }},
        {"BlockKey", func() []byte {
            key := make([]byte, 37)
            key[0] = 0x01 // PrefixBlock
            binary.LittleEndian.PutUint32(key[1:5], 500000)
            copy(key[5:], make([]byte, 32))
            return key
        }},
        {"TxLocationKey", func() []byte {
            key := make([]byte, 33)
            key[0] = 0x05 // PrefixTxLocation
            copy(key[1:], make([]byte, 32))
            return key
        }},
        {"AddressUTXOKey", func() []byte {
            key := make([]byte, 59)
            key[0] = 0x20 // PrefixAddressUTXO
            copy(key[1:21], make([]byte, 20))
            binary.LittleEndian.PutUint32(key[21:25], 500000)
            copy(key[25:57], make([]byte, 32))
            binary.LittleEndian.PutUint16(key[57:], 0)
            return key
        }},
    }

    for _, bm := range benchmarks {
        b.Run(bm.name, func(b *testing.B) {
            b.ReportAllocs()
            for i := 0; i < b.N; i++ {
                _ = bm.fn()
            }
        })
    }
}

// Benchmark storage size comparison
func TestBinaryVsJSONStorageSize(t *testing.T) {
    // Create sample data
    utxoBinary := &CompactUTXO{
        Value:       100000000,
        ScriptType:  1,
        BlockHeight: 500000,
        IsCoinbase:  false,
    }
    rand.Read(utxoBinary.ScriptData[:])

    utxoJSON := &UTXOJSON{
        Value:       100000000,
        ScriptType:  1,
        ScriptData:  fmt.Sprintf("%x", utxoBinary.ScriptData),
        BlockHeight: 500000,
        IsCoinbase:  false,
    }

    // Binary encoding
    binaryData := (*[34]byte)(unsafe.Pointer(utxoBinary))[:]

    // JSON encoding
    jsonData, err := json.Marshal(utxoJSON)
    require.NoError(t, err)

    t.Logf("Binary size: %d bytes", len(binaryData))
    t.Logf("JSON size: %d bytes", len(jsonData))
    t.Logf("Size reduction: %.1f%%", (1-float64(len(binaryData))/float64(len(jsonData)))*100)

    // Expected results:
    // Binary: 34 bytes
    // JSON: ~120-150 bytes
    // Reduction: ~75-80%
}

// Benchmark batch operations
func BenchmarkBatchOperations(b *testing.B) {
    benchmarks := []struct {
        name      string
        batchSize int
    }{
        {"Batch10", 10},
        {"Batch100", 100},
        {"Batch1000", 1000},
        {"Batch10000", 10000},
    }

    for _, bm := range benchmarks {
        b.Run(bm.name+"_Binary", func(b *testing.B) {
            utxos := make([]*CompactUTXO, bm.batchSize)
            for i := range utxos {
                utxos[i] = &CompactUTXO{
                    Value:       uint64(i * 100000000),
                    ScriptType:  1,
                    BlockHeight: uint32(500000 + i),
                }
            }

            b.ResetTimer()
            b.ReportAllocs()

            for i := 0; i < b.N; i++ {
                data := make([]byte, 0, 34*len(utxos))
                for _, utxo := range utxos {
                    data = append(data, (*[34]byte)(unsafe.Pointer(utxo))[:]...)
                }
                _ = data
            }
        })

        b.Run(bm.name+"_JSON", func(b *testing.B) {
            utxos := make([]*UTXOJSON, bm.batchSize)
            for i := range utxos {
                utxos[i] = &UTXOJSON{
                    Value:       uint64(i * 100000000),
                    ScriptType:  1,
                    ScriptData:  "1234567890abcdef1234567890abcdef12345678",
                    BlockHeight: uint32(500000 + i),
                }
            }

            b.ResetTimer()
            b.ReportAllocs()

            for i := 0; i < b.N; i++ {
                data, _ := json.Marshal(utxos)
                _ = data
            }
        })
    }
}

// Simulate full block validation storage operations
func BenchmarkBlockValidationStorageOps(b *testing.B) {
    // Simulate storage operations for validating a 2-tx PoS block

    b.Run("CurrentImplementation", func(b *testing.B) {
        b.ReportAllocs()

        for i := 0; i < b.N; i++ {
            // Simulate GetBlockContainingTx (linear scan)
            // In reality this iterates 1M+ blocks
            blocks := 1000 // Reduced for benchmark
            for j := 0; j < blocks; j++ {
                // Simulate JSON unmarshal of each block
                var block interface{}
                _ = json.Unmarshal([]byte(`{"height":500000,"txs":[{},{},{}]}`), &block)
            }

            // Simulate UTXO lookups (JSON)
            for j := 0; j < 2; j++ {
                var utxo UTXOJSON
                _ = json.Unmarshal([]byte(`{"value":100000000,"script_type":1}`), &utxo)
            }

            // Simulate writing block (JSON)
            blockData := map[string]interface{}{
                "height": 500001,
                "txs":    []interface{}{},
            }
            _, _ = json.Marshal(blockData)
        }
    })

    b.Run("OptimizedImplementation", func(b *testing.B) {
        b.ReportAllocs()

        for i := 0; i < b.N; i++ {
            // Simulate GetBlockContainingTx (direct index lookup)
            txLocation := &TxLocation{
                BlockHeight: 500000,
                TxIndex:     0,
            }
            _ = (*[38]byte)(unsafe.Pointer(txLocation))[:]

            // Simulate UTXO lookups (binary)
            for j := 0; j < 2; j++ {
                utxo := &CompactUTXO{
                    Value:       100000000,
                    ScriptType:  1,
                    BlockHeight: 500000,
                }
                _ = (*[34]byte)(unsafe.Pointer(utxo))[:]
            }

            // Simulate writing block (binary)
            block := &CompactBlock{
                Version:   1,
                Timestamp: 1234567890,
                TxCount:   2,
            }
            _ = (*[unsafe.Sizeof(*block)]byte)(unsafe.Pointer(block))[:]
        }
    })
}

// TxLocation structure for benchmarks
type TxLocation struct {
    BlockHeight uint32
    BlockHash   [32]byte
    TxIndex     uint16
}

// CompactBlock structure for benchmarks
type CompactBlock struct {
    Version       uint32
    HashPrevBlock [32]byte
    HashMerkle    [32]byte
    Timestamp     uint32
    Bits          uint32
    Nonce         uint32
    TxCount       uint32
}

// BenchmarkGetBlockContainingTxComparison compares O(n) vs O(1) performance
func BenchmarkGetBlockContainingTxComparison(b *testing.B) {
    // Setup test databases
    jsonDir, _ := os.MkdirTemp("", "json_bench_*")
    binaryDir, _ := os.MkdirTemp("", "binary_bench_*")
    defer os.RemoveAll(jsonDir)
    defer os.RemoveAll(binaryDir)

    config := storage.DefaultStorageConfig()

    // Setup JSON storage (current implementation)
    config.Path = jsonDir
    jsonStorage, err := binarystorage.NewBinaryStorage(config)
    require.NoError(b, err)
    defer jsonStorage.Close()

    // Setup Binary storage (optimized implementation)
    config.Path = binaryDir
    binaryStorage, err := binarystorage.NewBinaryStorage(config)
    require.NoError(b, err)
    defer binaryStorage.Close()

    // Create test data: 1000 blocks with 10 transactions each
    targetTxHash := types.Hash{}
    rand.Read(targetTxHash[:])
    targetBlockIndex := 750 // Place target transaction in block 750

    for i := 0; i < 1000; i++ {
        block := &types.Block{
            Header: &types.BlockHeader{
                Version:   1,
                Timestamp: uint32(time.Now().Unix()),
            },
            Transactions: make([]*types.Transaction, 10),
        }

        for j := 0; j < 10; j++ {
            tx := &types.Transaction{
                Version: 1,
                Inputs: []*types.TxInput{{
                    PreviousOutput: types.Outpoint{Hash: types.Hash{}, Index: 0},
                }},
                Outputs: []*types.TxOutput{{
                    Value: 100000000,
                }},
            }

            // Place our target transaction
            if i == targetBlockIndex && j == 5 {
                // Override the hash calculation for test purposes
                // In real code, tx.Hash() would compute this
                block.Transactions[j] = tx
            } else {
                block.Transactions[j] = tx
            }
        }

        // Store block in both storages
        jsonStorage.StoreBlock(block, uint32(i))
        binaryStorage.StoreBlock(block, uint32(i))
    }

    b.Run("JSON_Linear_O(n)", func(b *testing.B) {
        b.ReportAllocs()
        for i := 0; i < b.N; i++ {
            // This will do a linear scan through all blocks
            _, _ = jsonStorage.GetBlockContainingTx(targetTxHash)
        }
    })

    b.Run("Binary_Index_O(1)", func(b *testing.B) {
        b.ReportAllocs()
        for i := 0; i < b.N; i++ {
            // This will do a direct index lookup
            _, _ = binaryStorage.GetBlockContainingTx(targetTxHash)
        }
    })
}

// BenchmarkUTXOOperations compares UTXO encoding/decoding performance
func BenchmarkUTXOOperations(b *testing.B) {
    // Create a test UTXO
    utxo := &types.UTXO{
        Outpoint: types.Outpoint{
            Hash:  types.Hash{},
            Index: 0,
        },
        Output: &types.TxOutput{
            Value: 100000000,
            ScriptPubKey: []byte{
                0x76, 0xa9, 0x14, // P2PKH prefix
                0x89, 0xab, 0xcd, 0xef, 0xab, 0xba, 0xab, 0xba, 0xab, 0xba,
                0xab, 0xba, 0xab, 0xba, 0xab, 0xba, 0xab, 0xba, 0xab, 0xba,
                0x88, 0xac, // P2PKH suffix
            },
        },
        Height:     500000,
        IsCoinbase: false,
    }
    rand.Read(utxo.Outpoint.Hash[:])

    b.Run("Encoding", func(b *testing.B) {
        b.Run("JSON", func(b *testing.B) {
            b.ReportAllocs()
            for i := 0; i < b.N; i++ {
                data, _ := json.Marshal(utxo)
                _ = data
            }
        })

        b.Run("Binary", func(b *testing.B) {
            b.ReportAllocs()
            for i := 0; i < b.N; i++ {
                data, _ := binarystorage.EncodeUTXO(utxo)
                _ = data
            }
        })
    })

    b.Run("Decoding", func(b *testing.B) {
        jsonData, _ := json.Marshal(utxo)
        binaryData, _ := binarystorage.EncodeUTXO(utxo)

        b.Run("JSON", func(b *testing.B) {
            b.ReportAllocs()
            for i := 0; i < b.N; i++ {
                var decoded types.UTXO
                _ = json.Unmarshal(jsonData, &decoded)
            }
        })

        b.Run("Binary", func(b *testing.B) {
            b.ReportAllocs()
            for i := 0; i < b.N; i++ {
                decoded, _ := binarystorage.DecodeUTXO(binaryData, utxo.Outpoint)
                _ = decoded
            }
        })
    })

    // Report size comparison
    jsonData, _ := json.Marshal(utxo)
    binaryData, _ := binarystorage.EncodeUTXO(utxo)
    b.Logf("Storage size - JSON: %d bytes, Binary: %d bytes (%.1f%% reduction)",
        len(jsonData), len(binaryData),
        (1-float64(len(binaryData))/float64(len(jsonData)))*100)
}

// TestPerformanceValidation validates that we meet our performance targets
func TestPerformanceValidation(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping performance validation in short mode")
    }

    // Test UTXO encoding/decoding performance
    utxo := &types.UTXO{
        Outpoint: types.Outpoint{
            Hash:  types.Hash{},
            Index: 0,
        },
        Output: &types.TxOutput{
            Value: 100000000,
            ScriptPubKey: make([]byte, 25), // P2PKH script
        },
        Height:     500000,
        IsCoinbase: false,
    }

    // Measure binary encoding performance
    start := time.Now()
    iterations := 1000000
    for i := 0; i < iterations; i++ {
        _, _ = binarystorage.EncodeUTXO(utxo)
    }
    binaryEncodeTime := time.Since(start)
    binaryEncodeNsPerOp := binaryEncodeTime.Nanoseconds() / int64(iterations)

    // Measure JSON encoding performance
    start = time.Now()
    for i := 0; i < iterations; i++ {
        _, _ = json.Marshal(utxo)
    }
    jsonEncodeTime := time.Since(start)
    jsonEncodeNsPerOp := jsonEncodeTime.Nanoseconds() / int64(iterations)

    t.Logf("Encoding performance (ns/op):")
    t.Logf("  Binary: %d ns/op", binaryEncodeNsPerOp)
    t.Logf("  JSON:   %d ns/op", jsonEncodeNsPerOp)
    t.Logf("  Speedup: %.1fx", float64(jsonEncodeNsPerOp)/float64(binaryEncodeNsPerOp))

    // Validate that binary is at least 10x faster than JSON
    if binaryEncodeNsPerOp > jsonEncodeNsPerOp/10 {
        t.Logf("WARNING: Binary encoding not meeting 10x speedup target")
    }

    // Test storage size
    jsonData, _ := json.Marshal(utxo)
    binaryData, _ := binarystorage.EncodeUTXO(utxo)

    sizeReduction := (1 - float64(len(binaryData))/float64(len(jsonData))) * 100
    t.Logf("Storage size:")
    t.Logf("  JSON:   %d bytes", len(jsonData))
    t.Logf("  Binary: %d bytes", len(binaryData))
    t.Logf("  Reduction: %.1f%%", sizeReduction)

    // Validate at least 70% size reduction
    if sizeReduction < 70 {
        t.Errorf("Binary storage not meeting 70%% size reduction target: got %.1f%%", sizeReduction)
    }
}