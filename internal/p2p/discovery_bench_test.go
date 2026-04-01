package p2p

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// Benchmark utilities

func setupBenchDiscovery(b *testing.B) *PeerDiscovery {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	tmpDir := b.TempDir()

	pd := NewPeerDiscovery(DiscoveryConfig{
		Logger:         logger,
		Network:        "testnet",
		Seeds:          nil,
		DNSSeeds:       nil,
		MaxPeers:       100000,
		DataDir:        tmpDir,
		DNSSeedEnabled: true,
	})

	return pd
}

func createBenchAddress(i int) *NetAddress {
	// Create diverse addresses across different /16 networks
	octet1 := byte((i / 65536) % 256)
	octet2 := byte((i / 256) % 256)
	octet3 := byte(i % 256)
	octet4 := byte((i * 17) % 256) // Add some variety

	ip := net.IPv4(octet1, octet2, octet3, octet4)

	return &NetAddress{
		Time:     uint32(time.Now().Unix()),
		Services: SFNodeNetwork,
		IP:       ip,
		Port:     37817,
	}
}

// Benchmarks

func BenchmarkAddAddress(b *testing.B) {
	pd := setupBenchDiscovery(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		addr := createBenchAddress(i)
		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	b.StopTimer()

	// Report performance
	ops := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(ops, "addresses/sec")
}

func BenchmarkGetAddresses(b *testing.B) {
	pd := setupBenchDiscovery(b)

	// Pre-populate with 10,000 addresses
	for i := 0; i < 10000; i++ {
		addr := createBenchAddress(i)
		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pd.GetAddresses(100)
	}

	b.StopTimer()

	// Report time per batch
	timePerBatch := b.Elapsed() / time.Duration(b.N)
	b.ReportMetric(float64(timePerBatch.Microseconds()), "µs/batch")
}

func BenchmarkWeightedSelection(b *testing.B) {
	pd := setupBenchDiscovery(b)

	// Test with different pool sizes
	poolSizes := []int{10, 100, 1000}

	for _, size := range poolSizes {
		b.Run(fmt.Sprintf("pool_size_%d", size), func(b *testing.B) {
			// Create pool
			pool := make(map[string]*KnownAddress)
			for i := 0; i < size; i++ {
				addr := createBenchAddress(i)
				known := &KnownAddress{
					Addr:        addr,
					LastSuccess: time.Now().Add(-1 * time.Hour),
					LastSeen:    time.Now(),
					Services:    SFNodeNetwork,
					Attempts:    int32(i % 5),
				}
				pool[addr.String()] = known
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				pd.selectWeighted(pool)
			}

			b.StopTimer()

			// Report time per selection
			timePerSelect := b.Elapsed() / time.Duration(b.N)
			b.ReportMetric(float64(timePerSelect.Nanoseconds()), "ns/select")
		})
	}
}

func BenchmarkPriorityCalculation(b *testing.B) {
	pd := setupBenchDiscovery(b)

	// Create test address
	addr := createBenchAddress(0)
	known := &KnownAddress{
		Addr:        addr,
		LastSuccess: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork,
		Attempts:    2,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pd.calculateAddressPriority(known)
	}

	b.StopTimer()

	// Report time per calculation
	timePerCalc := b.Elapsed() / time.Duration(b.N)
	b.ReportMetric(float64(timePerCalc.Nanoseconds()), "ns/calc")
}

func BenchmarkCalculateChance(b *testing.B) {
	pd := setupBenchDiscovery(b)

	// Create test address
	addr := createBenchAddress(0)
	known := &KnownAddress{
		Addr:        addr,
		LastAttempt: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork,
		Attempts:    3,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pd.calculateChance(known)
	}

	b.StopTimer()

	// Report time per calculation
	timePerCalc := b.Elapsed() / time.Duration(b.N)
	b.ReportMetric(float64(timePerCalc.Nanoseconds()), "ns/calc")
}

func BenchmarkPersistence(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	tmpDir := b.TempDir()

	// Create addresses to save
	addresses := make(map[string]*KnownAddress)
	for i := 0; i < 10000; i++ {
		addr := createBenchAddress(i)
		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}
		addresses[addr.String()] = known
	}

	addrDB := NewAddressDB(tmpDir, logger)

	b.Run("Save", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			if err := addrDB.Save(addresses); err != nil {
				b.Fatal(err)
			}
		}

		b.StopTimer()

		// Report time per save
		timePerSave := b.Elapsed() / time.Duration(b.N)
		b.ReportMetric(float64(timePerSave.Milliseconds()), "ms/save")
	})

	// Save once for load benchmark
	if err := addrDB.Save(addresses); err != nil {
		b.Fatal(err)
	}

	b.Run("Load", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			if _, err := addrDB.Load(); err != nil {
				b.Fatal(err)
			}
		}

		b.StopTimer()

		// Report time per load
		timePerLoad := b.Elapsed() / time.Duration(b.N)
		b.ReportMetric(float64(timePerLoad.Milliseconds()), "ms/load")
	})
}

func BenchmarkMarkOperations(b *testing.B) {
	pd := setupBenchDiscovery(b)

	// Pre-populate with addresses
	addresses := make([]*NetAddress, 1000)
	for i := 0; i < 1000; i++ {
		addr := createBenchAddress(i)
		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}
		pd.AddAddress(known, nil, SourceDNS)
		addresses[i] = addr
	}

	b.Run("MarkAttempt", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			addr := addresses[i%len(addresses)]
			pd.MarkAttempt(addr)
		}
	})

	b.Run("MarkSuccess", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			addr := addresses[i%len(addresses)]
			pd.MarkSuccess(addr)
		}
	})

	b.Run("MarkBad", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			addr := addresses[i%len(addresses)]
			pd.MarkBad(addr, "benchmark test")
		}
	})
}

func BenchmarkCleanupAddresses(b *testing.B) {
	pd := setupBenchDiscovery(b)

	// Pre-populate with mix of good, bad, and old addresses
	for i := 0; i < 10000; i++ {
		addr := createBenchAddress(i)

		var lastSeen time.Time
		var attempts int32
		var isBad bool

		// Create realistic mix
		switch i % 4 {
		case 0: // Recent good address
			lastSeen = time.Now()
			attempts = 1
			isBad = false
		case 1: // Old address (should be cleaned)
			lastSeen = time.Now().Add(-8 * 24 * time.Hour)
			attempts = 0
			isBad = false
		case 2: // Bad address with failures (should be cleaned)
			lastSeen = time.Now()
			attempts = 12
			isBad = false
		case 3: // Marked bad (should be cleaned)
			lastSeen = time.Now()
			attempts = 0
			isBad = true
		}

		known := &KnownAddress{
			Addr:     addr,
			LastSeen: lastSeen,
			Services: SFNodeNetwork,
			Attempts: attempts,
			IsBad:    isBad,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pd.cleanupAddresses()
	}

	b.StopTimer()

	// Report time per cleanup
	timePerCleanup := b.Elapsed() / time.Duration(b.N)
	b.ReportMetric(float64(timePerCleanup.Milliseconds()), "ms/cleanup")
}

func BenchmarkNetworkGrouping(b *testing.B) {
	// Create diverse test IPs
	ips := make([]net.IP, 1000)
	for i := 0; i < 1000; i++ {
		ips[i] = net.IPv4(byte(i/256), byte(i%256), byte(i*3%256), byte(i*7%256))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ip := ips[i%len(ips)]
		getNetworkGroup(ip)
	}

	b.StopTimer()

	// Report time per grouping
	timePerGroup := b.Elapsed() / time.Duration(b.N)
	b.ReportMetric(float64(timePerGroup.Nanoseconds()), "ns/group")
}

func BenchmarkConcurrentAccess(b *testing.B) {
	pd := setupBenchDiscovery(b)

	// Pre-populate with addresses
	for i := 0; i < 1000; i++ {
		addr := createBenchAddress(i)
		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Run concurrent operations
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Mix of read and write operations
			switch i % 4 {
			case 0:
				pd.GetAddresses(10)
			case 1:
				addr := createBenchAddress(i)
				known := &KnownAddress{
					Addr:     addr,
					LastSeen: time.Now(),
					Services: SFNodeNetwork,
				}
				pd.AddAddress(known, nil, SourceDNS)
			case 2:
				addr := createBenchAddress(i % 1000)
				pd.MarkAttempt(addr)
			case 3:
				addr := createBenchAddress(i % 1000)
				pd.MarkSuccess(addr)
			}
			i++
		}
	})
}
