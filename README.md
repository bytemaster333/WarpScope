# WarpScope

> Cross-Chain Observability for Avalanche Interchain Messaging

[![Go 1.22+](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://go.dev/dl/)
[![CGO Required](https://img.shields.io/badge/CGO-required-orange)](https://pkg.go.dev/cmd/cgo)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## Overview

As Avalanche L1 deployments multiply, cross-chain messaging (ICM/Warp) failures become invisible.
Glacier and AvaCloud tell you *that* a message is stuck. The AWM Relayer emits operational metrics.
Teleporter CLI decodes events. But **no existing tool tells you *why*.**

WarpScope is the missing `strace` for Avalanche cross-chain messages. It performs **mathematical
failure attribution** by combining three data sources into a single diagnostic pipeline:

- **P-Chain validator set snapshots** — captures the exact validator set at signing height
- **BLS12-381 threshold cryptography** — verifies aggregate signatures via `supranational/blst`
- **On-chain Teleporter event correlation** — traces the message from source `SendWarpMessage`
  through destination `ReceiveCrossChainMessage` to `MessageExecutionFailed`

The result is a structured `DiagnosticReport` with a machine-readable `rootCause`, per-check
evidence, and actionable remediation steps — suitable for both human review and CI/alerting pipelines.

---

## Key Features

### 1. Smart Mode — Zero Config for Common Cases

Pass only the transaction hash and `--network fuji`. WarpScope auto-fills everything else:

```
[auto] source-rpc     → https://api.avax-test.network/ext/bc/C/rpc  (Fuji Testnet default)
[auto] pchain-rpc     → https://api.avax-test.network  (Fuji Testnet default)
[auto] subnet-id      → 11111111111111111111111111111111LpoYY  (detected via platform.validatedBy)
[auto] pchain-height  → 2014823  (estimated from block 38421001 at 2026-04-13 18:31:00 UTC)
```

Three auto-detection passes:
1. **RPC defaults** — from a built-in network profile (`fuji` / `mainnet`)
2. **Subnet ID** — ABI-decodes the `SendWarpMessage` log → parses the unsigned Warp message →
   calls `platform.validatedBy(sourceChainID)` on the P-Chain
3. **P-Chain signing height** — reads the source block timestamp → queries `platform.getTimestamp`
   and `platform.getHeight` → estimates via linear interpolation (~3 s/block)

All failures are non-fatal: warnings are printed to stderr and safe fallbacks are used so the
diagnosis always proceeds.

### 2. Failure Attribution Engine

Seven ordered diagnostic checks, each targeting a distinct failure mode:

| # | Check | Category | Severity |
|---|---|---|---|
| 1 | NetworkID match | `INVALID_NETWORK_ID` | CRITICAL |
| 2 | Relayer pickup detection | `RELAYER_NEVER_PICKED_UP` | CRITICAL |
| 3 | Destination execution | `DESTINATION_EXECUTION_FAILED` | CRITICAL |
| 4 | BLS stake weight threshold | `INSUFFICIENT_STAKE_WEIGHT` | WARNING |
| 5 | Validator set stability | `VALIDATOR_SET_CHANGED` | WARNING |
| 6 | Full BLS signature verification | `BLS_VERIFICATION_FAILED` | CRITICAL |
| 7 | ACP-181 epoch boundary race | `EPOCH_BOUNDARY_RACE` | WARNING |

The first CRITICAL failure becomes the `rootCause`. JSON output exposes `isHealthy` and
`rootCause` for scripting.

### 3. Mathematical BLS Verification

When a signed message is available, WarpScope performs the full cryptographic proof:

1. Decode `BitSetSignature.Signers` bitset → collect validator indices
2. Sort validators **lexicographically by compressed BLS public key bytes** (canonical ordering)
3. Accumulate signed weight; check `signedWeight × 100 ≥ totalWeight × 67`
4. `bls.AggregatePublicKeys(collectedPKs)` → single aggregate key
5. `blst.Verify(aggregatePK, aggregateSig, unsignedMsg.Bytes())`

Reports exact signed-weight percentage, not just pass/fail.

---

## Failure Categories

| Category | Severity | How It's Detected | Remediation |
|---|---|---|---|
| `HEALTHY` | — | All checks pass | — |
| `RELAYER_NEVER_PICKED_UP` | CRITICAL | `ReceiveCrossChainMessage` absent on destination | Restart relayer; check source chain filter config |
| `DESTINATION_EXECUTION_FAILED` | CRITICAL | `MessageExecutionFailed` event present | Fix destination contract; call `retryMessageExecution()` |
| `BLS_VERIFICATION_FAILED` | CRITICAL | `bls.Verify()` returns false | Re-sign message with current validator set |
| `INVALID_NETWORK_ID` | CRITICAL | `UnsignedMessage.NetworkID` ≠ expected | Check relayer network configuration |
| `INVALID_SOURCE_CHAIN_ID` | CRITICAL | `SourceChainID` mismatch in Warp message | Verify source chain RPC URL |
| `QUORUM_CONFIG_MISMATCH` | CRITICAL | Destination `quorumNumerator` ≠ relayer config | Inspect Warp precompile on destination chain |
| `INSUFFICIENT_STAKE_WEIGHT` | WARNING | BLS-capable validator weight < 67% of total | Validators must register BLS keys via P-Chain |
| `VALIDATOR_SET_CHANGED` | WARNING | Validators removed since signing height | Call `retrySendCrossChainMessage` to re-aggregate |
| `EPOCH_BOUNDARY_RACE` | WARNING | ≥1 ACP-181 epoch (150 blocks, ~7.5 min) elapsed | Re-aggregate signatures at current P-Chain height |

---

## Architecture

```
                      ┌─────────────────────────────────────────┐
  Source Chain  ──────►  Engine.Diagnose(ctx, txHash)            │
                      │                                          │
  P-Chain       ──────►  1. Fetch SendWarpMessage event          │
                      │  2. Decode UnsignedMessage               │──► DiagnosticReport
  Dest Chain    ──────►  3. Query validator sets (H + current)   │      rootCause
                      │  4. Search for delivery/exec events      │      isHealthy
                      │  5. Run 7 ordered checks                 │      checks[]
                      │  6. Assemble report                      │      summary
                      └─────────────────────────────────────────┘
```

**Ports-and-adapters design**: `chain.EVMClient` and `chain.PChainClient` interfaces isolate all
RPC calls from business logic. The diagnosis engine is fully testable with hand-written fakes — no
live network required for unit tests.

---

## Installation

### Prerequisites

- **Go 1.22+** — download from [go.dev](https://go.dev/dl/)
- **CGO enabled** — required for `supranational/blst` (BLS12-381 C library)
- **GCC or Clang**:
  - Ubuntu/Debian: `sudo apt-get install build-essential`
  - macOS: `xcode-select --install`
  - Windows: [tdm-gcc](https://jmeubank.github.io/tdm-gcc/) or WSL2

> **Note:** Alpine Linux is **not supported** — musl libc is incompatible with blst.
> Use `golang:1.22-bookworm` for Docker builds.

### Build from Source

```bash
git clone https://github.com/bytemaster333/warpscope.git
cd warpscope

CGO_ENABLED=1 make build   # produces ./bin/warpscope
CGO_ENABLED=1 make test    # run unit tests
```

Verify your GCC setup:

```bash
make check-cgo
```

---

## Usage

### Smart Mode (Recommended)

The simplest invocation — let WarpScope figure out the rest:

```bash
./bin/warpscope diagnose 0xYOUR_TX_HASH --network fuji
```

Add a destination chain RPC to enable delivery detection:

```bash
./bin/warpscope diagnose 0xYOUR_TX_HASH \
  --network  fuji \
  --dest-rpc https://<l1-rpc>/ext/bc/<chainID>/rpc
```

### Manual Mode (Full Control)

Override every parameter explicitly:

```bash
./bin/warpscope diagnose 0xYOUR_TX_HASH \
  --source-rpc    https://api.avax-test.network/ext/bc/C/rpc \
  --dest-rpc      https://<l1-rpc>/ext/bc/<chainID>/rpc \
  --pchain-rpc    https://api.avax-test.network \
  --subnet-id     29uVeLPJB1eQJkzRemU8g8wZDnzt5pHe4bZqAYp5sMk3UJqd6j \
  --pchain-height 2014500 \
  --network       fuji \
  --output        table
```

### JSON Output for CI / Scripting

```bash
./bin/warpscope diagnose 0xYOUR_TX_HASH --network fuji --output json \
  | jq '{rootCause: .rootCause, healthy: .isHealthy}'
```

Exit code is `1` when `isHealthy` is false, making it pipeline-friendly.

### Validator Audit

Check which validators in a subnet have registered BLS keys and whether the 67% quorum is reachable:

```bash
./bin/warpscope validators \
  --pchain-rpc https://api.avax-test.network \
  --subnet-id  29uVeLPJB1eQJkzRemU8g8wZDnzt5pHe4bZqAYp5sMk3UJqd6j
```

### Discover a Recent Fuji Teleporter Transaction

```bash
CGO_ENABLED=1 go run ./cmd/fuji-finder/ \
  --source-rpc https://api.avax-test.network/ext/bc/C/rpc
```

Scans the last 2,000 C-Chain blocks (~2.7 hours) for `SendWarpMessage` events and prints the
tx hash, block number, timestamp, and message ID in JSON — ready to pipe into `diagnose`.

---

## Sample Output

```
  WarpScope Diagnostic Report
  Transaction:    0x7a3f9b2c...607182
  Message ID:     0xdeadbeef...001234
  Timestamp:      2026-04-13T18:00:00Z
  P-Chain Height: 2014500
  Verdict:        FAILED
  Root Cause:     RELAYER_NEVER_PICKED_UP

       CHECK                    STATUS    SEVERITY    DESCRIPTION
----+-----------------------+--------+----------+-------------------------------------------
  ✓    NetworkID                PASS      CRITICAL    Skipped (no parsed Warp message)
  ✗    RelayerPickup            FAIL      CRITICAL    No ReceiveCrossChainMessage on destination
  ✓    DestinationExecution     PASS      CRITICAL    No delivery found; check not applicable
  ✓    StakeWeightThreshold     PASS      WARNING     BLS validators hold 100.0% (threshold 67.0%)
  ✓    ValidatorSetStability    PASS      WARNING     No validator set churn detected
  ✓    BLSSignatureValidity     PASS      CRITICAL    Skipped (no BitSetSignature available)
  ✓    EpochBoundaryRace        PASS      WARNING     No epoch boundary detected

  Message 0xdead…1234: ROOT CAUSE=RELAYER_NEVER_PICKED_UP — No ReceiveCrossChainMessage
  event found on the destination chain. Possible causes: relayer not running,
  misconfigured source chain filter, or validator set too fragmented to aggregate
  67% stake weight.
```

Run all five failure scenarios locally (no network required):

```bash
CGO_ENABLED=1 go run ./cmd/demo-run/
```

---

## How It Works

`Engine.Diagnose` executes a six-step pipeline:

1. **Fetch** `SendWarpMessage` event from the source transaction receipt.
   If absent → immediate `RELAYER_NEVER_PICKED_UP` verdict.
2. **Decode** the unsigned Warp message bytes (ABI-decoded from `log.Data`).
   Parse errors are non-fatal; structural checks are skipped gracefully.
3. **Query P-Chain** for validator sets at both the signing height and the current height.
   Validator sets are sorted lexicographically by compressed BLS public key bytes — this is the
   canonical ordering that determines BitSet index assignments.
4. **Search destination chain** for `ReceiveCrossChainMessage` and `MessageExecutionFailed`
   events within a configurable block window (default: last 50,000 blocks).
5. **Run 7 checks** in priority order. Each check is independent and returns a `CheckResult`
   with `Passed`, `Severity`, `Category`, and a human-readable `Description`.
6. **Assemble** the `DiagnosticReport`: first CRITICAL failure → `rootCause`; all checks pass
   → `isHealthy = true`.

Key protocol detail: the `BitSetSignature` embedded in a delivered Warp message does **not**
include the P-Chain height used for signing. This is why `--pchain-height` exists (and Smart Mode
estimates it).

---

## Roadmap

| Milestone | Status | Highlights |
|---|---|---|
| **MVP v0.1** | ✅ Shipped | Smart Mode, 7 failure categories, BLS mathematical verification, table + JSON output, `fuji-finder` tx discovery tool |
| **V1 — Indexer** | 🔲 Planned | Binary-search P-Chain block timestamps for sub-50 ms height resolution; `warpscope watch` for real-time stream monitoring; complete `INVALID_SOURCE_CHAIN_ID` and `QUORUM_CONFIG_MISMATCH` checks |
| **V2 — Dashboard** | 🔲 Planned | Web UI; Prometheus metrics exporter; Slack / PagerDuty webhooks; historical message tracking across all Avalanche L1s |

---

## Reference

| Constant | Value |
|---|---|
| Warp precompile | `0x0200000000000000000000000000000000000005` |
| TeleporterMessenger v1.0.0 | `0x253b2784c75e510dD0fF1da844684a1aC0aa5fcf` |
| Primary Network subnet ID | `11111111111111111111111111111111LpoYY` |
| Default quorum | 67 / 100 stake weight |
| ACP-181 epoch length | 150 P-Chain blocks (~7.5 min) |
| BLS library | `supranational/blst` v0.3.14 |
| Key dependencies | `avalanchego` v1.13.5 · `subnet-evm` v0.7.9 · `libevm` v1.13.14-0.3.0.rc.6 |

---

## Contributing

```bash
# Fork → clone → create feature branch
git checkout -b feat/my-improvement

# Run unit tests (no network required)
CGO_ENABLED=1 make test

# Run the 5-scenario smoke test (no network required)
CGO_ENABLED=1 go run ./cmd/demo-run/

# Submit PR
```

CGO must be enabled for all builds and tests. All new failure categories should have a
corresponding unit test using the fake `EVMClient` / `PChainClient` pattern in
`internal/diagnosis/engine_test.go`.

---

## License

MIT — see [LICENSE](LICENSE).
