package cli

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	avawarp "github.com/ava-labs/avalanchego/vms/platformvm/warp"
	"github.com/ava-labs/libevm/accounts/abi"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/types"
	"github.com/ava-labs/libevm/crypto"
	"github.com/ava-labs/subnet-evm/ethclient"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/bytemaster333/warpscope/internal/chain"
	"github.com/bytemaster333/warpscope/internal/diagnosis"
	"github.com/bytemaster333/warpscope/internal/output"
	"github.com/spf13/cobra"
)

// ─── Network profiles ─────────────────────────────────────────────────────────

type networkProfile struct {
	sourceCChainRPC string
	pchainRPC       string
	label           string
}

var knownNetworks = map[string]networkProfile{
	"fuji": {
		sourceCChainRPC: "https://api.avax-test.network/ext/bc/C/rpc",
		pchainRPC:       "https://api.avax-test.network",
		label:           "Fuji Testnet",
	},
	"testnet": {
		sourceCChainRPC: "https://api.avax-test.network/ext/bc/C/rpc",
		pchainRPC:       "https://api.avax-test.network",
		label:           "Fuji Testnet",
	},
	"mainnet": {
		sourceCChainRPC: "https://api.avax.network/ext/bc/C/rpc",
		pchainRPC:       "https://api.avax.network",
		label:           "Mainnet",
	},
}

const primaryNetworkSubnetID = "11111111111111111111111111111111LpoYY"

// ─── Precomputed topics ───────────────────────────────────────────────────────

var (
	sendWarpTopic       = crypto.Keccak256Hash([]byte("SendWarpMessage(address,bytes32,bytes)"))
	warpPrecompileAddr  = common.HexToAddress("0x0200000000000000000000000000000000000005")
)

// ─── Command ──────────────────────────────────────────────────────────────────

func newDiagnoseCmd() *cobra.Command {
	var (
		sourceRPC     string
		destRPC       string
		pchainRPC     string
		subnetIDStr   string
		network       string
		outputFormat  string
		pchainHeight  uint64
		destFromBlock uint64
		destToBlock   uint64
	)

	cmd := &cobra.Command{
		Use:   "diagnose <tx-hash>",
		Short: "Diagnose a stuck or failed cross-chain Warp message",
		Long: `Diagnose runs the WarpScope Failure Attribution engine on the source
transaction that emitted the SendWarpMessage event.

Smart Mode: when --network fuji (or mainnet) is given, most flags become optional.
WarpScope automatically:
  • fills in default RPC URLs for the chosen network
  • detects the Subnet ID from the Warp message inside the transaction
  • estimates the P-Chain signing height from the source block timestamp

Minimal usage (Smart Mode):
  warpscope diagnose <tx-hash> --network fuji

Full usage:
  warpscope diagnose <tx-hash> \
    --source-rpc   https://api.avax-test.network/ext/bc/C/rpc \
    --dest-rpc     https://<l1-rpc>/ext/bc/<chainID>/rpc \
    --pchain-rpc   https://api.avax-test.network \
    --subnet-id    29uVeLPJB1eQJkzRemU8g8wZDnzt5pHe4bZqAYp5sMk3UJqd6j \
    --network      fuji \
    --output       table`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true, // don't print usage on runtime errors (e.g. network failures)
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiagnose(cmd.Context(), args[0], diagnoseOptions{
				sourceRPC:     sourceRPC,
				destRPC:       destRPC,
				pchainRPC:     pchainRPC,
				subnetIDStr:   subnetIDStr,
				network:       network,
				outputFormat:  outputFormat,
				pchainHeight:  pchainHeight,
				destFromBlock: destFromBlock,
				destToBlock:   destToBlock,
			})
		},
	}

	cmd.Flags().StringVar(&sourceRPC, "source-rpc", "", "Source chain RPC URL (auto: C-Chain default for --network)")
	cmd.Flags().StringVar(&destRPC, "dest-rpc", "", "Destination chain RPC URL (enables delivery detection)")
	cmd.Flags().StringVar(&pchainRPC, "pchain-rpc", "", "P-Chain RPC URL (auto: default for --network)")
	cmd.Flags().StringVar(&subnetIDStr, "subnet-id", "", "Source subnet ID (auto: detected from Warp message)")
	cmd.Flags().StringVar(&network, "network", "mainnet", "Avalanche network: mainnet|fuji")
	cmd.Flags().StringVar(&outputFormat, "output", "table", "Output format: table|json")
	cmd.Flags().Uint64Var(&pchainHeight, "pchain-height", 0, "P-Chain signing height (auto: estimated from block timestamp)")
	cmd.Flags().Uint64Var(&destFromBlock, "dest-from-block", 0, "Destination chain start block for event search")
	cmd.Flags().Uint64Var(&destToBlock, "dest-to-block", 0, "Destination chain end block for event search")

	// No MarkFlagRequired — smart mode fills in missing values automatically.
	return cmd
}

type diagnoseOptions struct {
	sourceRPC     string
	destRPC       string
	pchainRPC     string
	subnetIDStr   string
	network       string
	outputFormat  string
	pchainHeight  uint64
	destFromBlock uint64
	destToBlock   uint64
}

// ─── Smart mode ───────────────────────────────────────────────────────────────

func autoLog(key, value, reason string) {
	if reason != "" {
		fmt.Fprintf(os.Stderr, "[auto] %-14s → %s  (%s)\n", key, value, reason)
	} else {
		fmt.Fprintf(os.Stderr, "[auto] %-14s → %s\n", key, value)
	}
}

// applySmartDefaults fills in any missing opts fields in three passes:
//  1. RPC URLs from the --network profile
//  2. Subnet ID from the Warp message in the source tx
//  3. P-Chain height estimated from the block timestamp
//
// All failures are non-fatal: a warning is printed and a safe fallback is used.
func applySmartDefaults(ctx context.Context, opts *diagnoseOptions, txHash common.Hash) {
	profile, hasProfile := knownNetworks[strings.ToLower(opts.network)]

	// Pass 1 — RPC defaults.
	if hasProfile {
		if opts.sourceRPC == "" {
			opts.sourceRPC = profile.sourceCChainRPC
			autoLog("source-rpc", opts.sourceRPC, profile.label+" default")
		}
		if opts.pchainRPC == "" {
			opts.pchainRPC = profile.pchainRPC
			autoLog("pchain-rpc", opts.pchainRPC, profile.label+" default")
		}
	}
	if opts.sourceRPC == "" || opts.pchainRPC == "" {
		return // can't do passes 2/3 without RPC
	}

	// Pass 2 & 3 — connect to source chain once to get receipt + block header.
	if opts.subnetIDStr != "" && opts.pchainHeight != 0 {
		return // nothing left to auto-detect
	}

	connCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(connCtx, opts.sourceRPC)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[auto] WARNING: could not connect to source chain: %v\n", err)
		fallbackSubnetAndHeight(ctx, opts)
		return
	}
	defer client.Close()

	// Fetch receipt.
	receipt, err := client.TransactionReceipt(connCtx, txHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[auto] WARNING: could not fetch tx receipt: %v\n", err)
		fallbackSubnetAndHeight(ctx, opts)
		return
	}

	// Fetch block header for timestamp.
	var txBlockTime time.Time
	header, err := client.HeaderByNumber(connCtx, new(big.Int).SetUint64(receipt.BlockNumber.Uint64()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[auto] WARNING: could not fetch block header: %v\n", err)
	} else {
		txBlockTime = time.Unix(int64(header.Time), 0).UTC()
	}

	// Pass 2 — Subnet ID.
	if opts.subnetIDStr == "" {
		sourceChainID := extractSourceChainID(receipt)
		subnetResolved := false
		if sourceChainID != ids.Empty {
			pCtx, pCancel := context.WithTimeout(ctx, 15*time.Second)
			defer pCancel()
			subnetID, err := chain.ResolveSubnetForChainID(pCtx, opts.pchainRPC, sourceChainID)
			if err == nil {
				if subnetID == ids.Empty {
					opts.subnetIDStr = primaryNetworkSubnetID
					autoLog("subnet-id", opts.subnetIDStr, "Primary Network (platform.validatedBy)")
				} else {
					opts.subnetIDStr = subnetID.String()
					autoLog("subnet-id", opts.subnetIDStr, "detected via platform.validatedBy")
				}
				subnetResolved = true
			} else {
				fmt.Fprintf(os.Stderr, "[auto] WARNING: platform.validatedBy failed: %v\n", err)
			}
		}
		if !subnetResolved {
			opts.subnetIDStr = primaryNetworkSubnetID
			autoLog("subnet-id", opts.subnetIDStr, "fallback: Primary Network")
		}
	}

	// Pass 3 — P-Chain height.
	if opts.pchainHeight == 0 {
		if !txBlockTime.IsZero() {
			pCtx, pCancel := context.WithTimeout(ctx, 15*time.Second)
			defer pCancel()
			estimated, err := chain.EstimatePChainHeight(pCtx, opts.pchainRPC, txBlockTime)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[auto] WARNING: height estimation failed: %v\n", err)
			} else {
				opts.pchainHeight = estimated
				autoLog("pchain-height", fmt.Sprintf("%d", estimated),
					fmt.Sprintf("estimated from block %d at %s",
						receipt.BlockNumber.Uint64(),
						txBlockTime.Format("2006-01-02 15:04:05 UTC")))
			}
		} else {
			fmt.Fprintf(os.Stderr, "[auto] pchain-height  : 0 — using current (block time unavailable)\n")
		}
	}
}

// fallbackSubnetAndHeight sets subnet/height to their safe defaults when the
// source chain is unreachable.
func fallbackSubnetAndHeight(ctx context.Context, opts *diagnoseOptions) {
	if opts.subnetIDStr == "" {
		opts.subnetIDStr = primaryNetworkSubnetID
		autoLog("subnet-id", opts.subnetIDStr, "fallback: Primary Network")
	}
	if opts.pchainHeight == 0 {
		fmt.Fprintf(os.Stderr, "[auto] pchain-height  : 0 — using current\n")
	}
}

// extractSourceChainID scans the receipt logs for a SendWarpMessage event,
// ABI-decodes the unsigned message bytes, and returns the SourceChainID.
// Returns ids.Empty if not found or parse fails.
func extractSourceChainID(receipt *types.Receipt) ids.ID {
	bytesType, err := abi.NewType("bytes", "", nil)
	if err != nil {
		return ids.Empty
	}
	abiArgs := abi.Arguments{{Type: bytesType}}

	for _, l := range receipt.Logs {
		if l == nil || l.Address != warpPrecompileAddr {
			continue
		}
		if len(l.Topics) < 1 || l.Topics[0] != sendWarpTopic {
			continue
		}
		values, err := abiArgs.Unpack(l.Data)
		if err != nil || len(values) == 0 {
			continue
		}
		msgBytes, ok := values[0].([]byte)
		if !ok || len(msgBytes) == 0 {
			continue
		}
		unsignedMsg, err := avawarp.ParseUnsignedMessage(msgBytes)
		if err != nil {
			continue
		}
		return unsignedMsg.SourceChainID
	}
	return ids.Empty
}

// ─── Main runner ──────────────────────────────────────────────────────────────

func runDiagnose(ctx context.Context, txHashStr string, opts diagnoseOptions) error {
	if !isHexHash(txHashStr) {
		return fmt.Errorf("invalid transaction hash: %q", txHashStr)
	}
	txHash := common.HexToHash(txHashStr)

	// Smart mode: fill in any missing options automatically.
	applySmartDefaults(ctx, &opts, txHash)

	// Validate RPC endpoints are present.
	if opts.sourceRPC == "" {
		return fmt.Errorf("--source-rpc is required (or use --network fuji/mainnet for auto-defaults)")
	}
	if opts.pchainRPC == "" {
		return fmt.Errorf("--pchain-rpc is required (or use --network fuji/mainnet for auto-defaults)")
	}
	if opts.subnetIDStr == "" {
		opts.subnetIDStr = primaryNetworkSubnetID
	}

	subnetID, err := parseSubnetID(opts.subnetIDStr)
	if err != nil {
		return fmt.Errorf("--subnet-id: %w", err)
	}

	expectedNetworkID, err := networkIDFromName(opts.network)
	if err != nil {
		return err
	}

	diagCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	sourceClient, err := chain.NewEVMClient(diagCtx, opts.sourceRPC)
	if err != nil {
		return fmt.Errorf("connect to source chain: %w", err)
	}

	var destClient chain.EVMClient
	if opts.destRPC != "" {
		destClient, err = chain.NewEVMClient(diagCtx, opts.destRPC)
		if err != nil {
			return fmt.Errorf("connect to destination chain: %w", err)
		}
	}

	pchainClient := chain.NewPChainClient(opts.pchainRPC)

	engine := &diagnosis.Engine{
		SourceClient:      sourceClient,
		DestClient:        destClient,
		PChain:            pchainClient,
		SubnetID:          subnetID,
		ExpectedNetworkID: expectedNetworkID,
		PChainHeight:      opts.pchainHeight,
		QuorumNum:         67,
		QuorumDen:         100,
		DestFromBlock:     opts.destFromBlock,
		DestToBlock:       opts.destToBlock,
	}

	report, err := engine.Diagnose(diagCtx, txHash)
	if err != nil {
		return fmt.Errorf("diagnosis failed: %w", err)
	}

	switch strings.ToLower(opts.outputFormat) {
	case "json":
		return output.WriteJSON(os.Stdout, report)
	default:
		output.WriteTable(os.Stdout, report)
		if !report.IsHealthy {
			os.Exit(1)
		}
		return nil
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func parseSubnetID(s string) (ids.ID, error) {
	if s == "" {
		return ids.Empty, nil
	}
	id, err := ids.FromString(s)
	if err == nil {
		return id, nil
	}
	s = strings.TrimPrefix(s, "0x")
	if len(s) == 64 {
		var idBytes [32]byte
		_, err := fmt.Sscanf(s, "%x", &idBytes)
		if err == nil {
			return ids.ID(idBytes), nil
		}
	}
	return ids.Empty, fmt.Errorf("cannot parse %q as CB58 or hex subnet ID", s)
}

func networkIDFromName(name string) (uint32, error) {
	switch strings.ToLower(name) {
	case "mainnet":
		return 1, nil
	case "fuji", "testnet":
		return 5, nil
	default:
		return 0, fmt.Errorf("unknown network %q; use 'mainnet' or 'fuji'", name)
	}
}

func isHexHash(s string) bool {
	s = strings.TrimPrefix(s, "0x")
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
