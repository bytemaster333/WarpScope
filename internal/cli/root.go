// Package cli defines the WarpScope Cobra command tree.
package cli

import (
	"github.com/spf13/cobra"
)

// rootCmd is the top-level "warpscope" command.
var rootCmd = &cobra.Command{
	Use:   "warpscope",
	Short: "Cross-chain Warp message failure attribution for Avalanche",
	Long: `WarpScope diagnoses why Avalanche Interchain Messaging (ICM/Warp) messages
are stuck or failed. It combines P-Chain validator set queries, BLS threshold
verification, and on-chain event correlation to produce actionable diagnostics.

No other tool in the Avalanche ecosystem performs mathematical Failure Attribution:
determining whether a message failed due to validator churn, insufficient BLS
stake weight, relayer misconfiguration, or cryptographic invalidity.

Examples:

  # Diagnose a stuck cross-chain message
  warpscope diagnose 0xabc123... \
    --source-rpc   https://api.avax-test.network/ext/bc/C/rpc \
    --dest-rpc     https://<l1-rpc-url>/ext/bc/<chainID>/rpc \
    --pchain-rpc   https://api.avax-test.network \
    --subnet-id    <subnetID> \
    --network      fuji

  # List validators and their BLS key registration status
  warpscope validators \
    --pchain-rpc  https://api.avax.network \
    --subnet-id   <subnetID>`,
}

// Execute runs the root command. Called from main().
func Execute() error {
	rootCmd.SilenceErrors = true // errors are printed by main() to avoid duplication
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(newDiagnoseCmd())
	rootCmd.AddCommand(newValidatorsCmd())
}
