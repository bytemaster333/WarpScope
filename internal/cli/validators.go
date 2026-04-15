package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bytemaster333/warpscope/internal/chain"
	"github.com/bytemaster333/warpscope/internal/validator"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newValidatorsCmd() *cobra.Command {
	var (
		pchainRPC    string
		subnetIDStr  string
		height       uint64
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "validators",
		Short: "List validators and their BLS key registration status",
		Long: `List the active validator set for a subnet and show which validators
have registered BLS keys for Warp message signing.

Validators without BLS keys cannot participate in Warp signing. If too many
validators lack BLS keys, the subnet may be unable to reach the 67% stake-weight
quorum required for message delivery.

Example:
  warpscope validators \
    --pchain-rpc https://api.avax.network \
    --subnet-id  29uVeLPJB1eQJkzRemU8g8wZDnzt5pHe4bZqAYp5sMk3UJqd6j`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidators(cmd.Context(), validatorsOptions{
				pchainRPC:    pchainRPC,
				subnetIDStr:  subnetIDStr,
				height:       height,
				outputFormat: outputFormat,
			})
		},
	}

	cmd.Flags().StringVar(&pchainRPC, "pchain-rpc", "", "P-Chain RPC URL (required)")
	cmd.Flags().StringVar(&subnetIDStr, "subnet-id", "", "Subnet ID in CB58 or hex (required)")
	cmd.Flags().Uint64Var(&height, "height", 0, "P-Chain height (0 = current)")
	cmd.Flags().StringVar(&outputFormat, "output", "table", "Output format: table|json")

	_ = cmd.MarkFlagRequired("pchain-rpc")
	_ = cmd.MarkFlagRequired("subnet-id")

	return cmd
}

type validatorsOptions struct {
	pchainRPC    string
	subnetIDStr  string
	height       uint64
	outputFormat string
}

func runValidators(ctx context.Context, opts validatorsOptions) error {
	subnetID, err := parseSubnetID(opts.subnetIDStr)
	if err != nil {
		return fmt.Errorf("--subnet-id: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pchainClient := chain.NewPChainClient(opts.pchainRPC)

	var infos []chain.ValidatorInfo
	if opts.height > 0 {
		infos, err = pchainClient.GetValidatorsAt(queryCtx, subnetID, opts.height)
		if err != nil {
			return fmt.Errorf("GetValidatorsAt height=%d: %w", opts.height, err)
		}
	} else {
		infos, err = pchainClient.GetCurrentValidators(queryCtx, subnetID)
		if err != nil {
			return fmt.Errorf("GetCurrentValidators: %w", err)
		}
	}

	vs, err := validator.FromValidatorInfos(subnetID, opts.height, infos)
	if err != nil {
		return fmt.Errorf("build validator set: %w", err)
	}

	// Compute BLS coverage statistics.
	var blsWeight uint64
	for _, v := range vs.Validators {
		blsWeight += v.Weight
	}
	blsPct := 0.0
	if vs.TotalWeight > 0 {
		blsPct = float64(blsWeight) / float64(vs.TotalWeight) * 100
	}

	switch strings.ToLower(opts.outputFormat) {
	case "json":
		return writeValidatorsJSON(os.Stdout, vs, infos)
	default:
		writeValidatorsTable(os.Stdout, vs, infos, blsWeight, blsPct)
		return nil
	}
}

func writeValidatorsTable(w *os.File, vs *validator.ValidatorSet, allInfos []chain.ValidatorInfo, blsWeight uint64, blsPct float64) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Subnet:             %s\n", vs.SubnetID)
	fmt.Fprintf(w, "  Total Validators:   %d (%d with BLS keys)\n", len(allInfos), len(vs.Validators))
	fmt.Fprintf(w, "  Total Weight:       %d\n", vs.TotalWeight)
	fmt.Fprintf(w, "  BLS-capable Weight: %d (%.1f%%)\n", blsWeight, blsPct)
	if blsPct >= 67 {
		fmt.Fprintf(w, "  Warp Quorum:        ✓ 67%% threshold reachable\n")
	} else {
		fmt.Fprintf(w, "  Warp Quorum:        ✗ 67%% threshold NOT reachable (only %.1f%% BLS-capable)\n", blsPct)
	}
	fmt.Fprintln(w)

	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"NodeID", "Weight", "BLS Key", "Pubkey (first 8 bytes)"})
	table.SetBorder(false)
	table.SetColumnSeparator("  ")
	table.SetHeaderLine(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, info := range allInfos {
		hasKey := "✗"
		keyPreview := "(none)"
		if len(info.PublicKeyBytes) >= 8 {
			hasKey = "✓"
			keyPreview = fmt.Sprintf("0x%x…", info.PublicKeyBytes[:8])
		}
		table.Append([]string{
			info.NodeID.String(),
			fmt.Sprintf("%d", info.Weight),
			hasKey,
			keyPreview,
		})
	}
	table.Render()
	fmt.Fprintln(w)
}

func writeValidatorsJSON(w *os.File, vs *validator.ValidatorSet, allInfos []chain.ValidatorInfo) error {
	type entry struct {
		NodeID    string `json:"nodeID"`
		Weight    uint64 `json:"weight"`
		HasBLSKey bool   `json:"hasBLSKey"`
		PublicKey string `json:"publicKey,omitempty"`
	}
	type result struct {
		SubnetID        string  `json:"subnetID"`
		TotalValidators int     `json:"totalValidators"`
		BLSValidators   int     `json:"blsValidators"`
		TotalWeight     uint64  `json:"totalWeight"`
		Validators      []entry `json:"validators"`
	}

	entries := make([]entry, len(allInfos))
	for i, info := range allInfos {
		e := entry{
			NodeID:    info.NodeID.String(),
			Weight:    info.Weight,
			HasBLSKey: len(info.PublicKeyBytes) > 0,
		}
		if len(info.PublicKeyBytes) > 0 {
			e.PublicKey = fmt.Sprintf("0x%x", info.PublicKeyBytes)
		}
		entries[i] = e
	}

	r := result{
		SubnetID:        vs.SubnetID.String(),
		TotalValidators: len(allInfos),
		BLSValidators:   len(vs.Validators),
		TotalWeight:     vs.TotalWeight,
		Validators:      entries,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
