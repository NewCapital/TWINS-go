# TWINS Core (Go)

**Status: BETA** | Protocol Compatibility: 100% with legacy C++ nodes

Modern Go implementation of the TWINS cryptocurrency. Fully compatible with the existing TWINS network — connects to the same peers, validates the same blocks, and works with the same wallets.

## Differences from Legacy (C++) Version

| | Legacy (C++) | Go (this) |
|---|---|---|
| Language | C++ (Qt) | Go |
| Database | LevelDB/BerkeleyDB | Pebble |
| Build | autotools, ~5 min | `go build`, seconds |
| Binary size | ~30 MB + Qt deps | ~15 MB, no deps |
| Config format | `twins.conf` (key=value) | `twinsd.yml` (YAML) + legacy `twins.conf` support |
| Wallet GUI | Qt-based | Wails (web-based) |
| Memory usage | Higher | 30-50% lower |
| Codebase | ~575 files | ~380 files |

What stays the same:
- PoS consensus rules, stake modifiers, kernel hash validation
- P2P protocol (magic bytes, message formats, protocol version 70928)
- Masternode tiers (Bronze/Silver/Gold/Platinum)
- RPC interface compatibility
- Wallet migration from legacy `wallet.dat` (BerkeleyDB) — automatic on first start

## Quick Start

### Build
```bash
go build ./cmd/twinsd       # daemon
go build ./cmd/twins-cli    # CLI client
```

### Run
```bash
./twinsd start > /dev/null 2>&1 &   # start daemon in background
./twins-cli getinfo                  # check status
```

## Migrating from Legacy Wallet

If you have an existing TWINS wallet from the C++ version, copy your `wallet.dat` to the Go daemon's data directory (`~/.twins/`).

On Linux both daemons use `~/.twins/` by default, so no copy is needed — just stop the old daemon and start the new one.

On macOS the legacy Qt wallet stores data in `~/Library/Application Support/TWINS/`:

```bash
# 1. Stop the legacy daemon
twins-cli stop

# 2. Copy wallet (macOS only — Linux uses the same dir)
cp ~/Library/Application\ Support/TWINS/wallet.dat ~/.twins/wallet.dat

# 3. Start the Go daemon
./twinsd start > /dev/null 2>&1 &
```

The Go daemon reads the legacy `wallet.dat` (BerkeleyDB) and migrates it to its own format on first start. Your balances, addresses, and keys will be available after migration.

**Important:** Always back up your `wallet.dat` before switching between daemon versions.

## Configuration

The daemon uses `~/.twins/twinsd.yml` (YAML) as the primary config. Legacy `twins.conf` is also supported as fallback.

Priority: CLI flags > environment variables > twinsd.yml > twins.conf > defaults

## Requirements

- Go 1.25 or later

## Protocol Compatibility

100% compatible with legacy C++ nodes:
- PoS difficulty: ppcoin-style per-block retargeting (blocks >400)
- Masternode broadcast: Dual-key structure with Hash160 signatures
- P2P protocol: Network magic `0x2f1cd30a`, protocol version 70928
- No chain fork risk

## License

MIT License - See `COPYING` for details.
