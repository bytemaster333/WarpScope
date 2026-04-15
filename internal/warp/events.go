// Package warp provides Warp message parsing, Teleporter event decoding,
// and pre-computed event topic hashes for EVM log filtering.
//
// Note: the package is named "warp" to match its directory. Callers that also
// import avalanchego's warp package should alias one of them:
//
//	import warpparse "github.com/bytemaster333/warpscope/internal/warp"
//	import avawarp  "github.com/ava-labs/avalanchego/vms/platformvm/warp"
package warp

import (
	_ "embed"
	"strings"

	"github.com/ava-labs/libevm/accounts/abi"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/crypto"
)

// WarpPrecompileAddress is the deterministic address of the Warp precompile
// on all Subnet-EVM chains.
var WarpPrecompileAddress = common.HexToAddress("0x0200000000000000000000000000000000000005")

// TeleporterAddress is the deterministic address of TeleporterMessenger v1.0.0
// deployed via Nick's Method; identical on every chain.
var TeleporterAddress = common.HexToAddress("0x253b2784c75e510dD0fF1da844684a1aC0aa5fcf")

// Pre-computed event topic hashes used for EVM log filtering.
var (
	// SendWarpMessageTopic = keccak256("SendWarpMessage(address,bytes32,bytes)")
	// Log layout: Topics[0]=sig, Topics[1]=sender, Topics[2]=messageID; Data=ABI(bytes).
	SendWarpMessageTopic common.Hash

	// ReceiveCrossChainMessageTopic is derived from the parsed Teleporter ABI
	// to ensure the struct-parameter canonical tuple encoding is correct.
	// Topics: [0]=sig, [1]=messageID, [2]=sourceBlockchainID, [3]=deliverer.
	ReceiveCrossChainMessageTopic common.Hash

	// MessageExecutionFailedTopic is derived from the parsed Teleporter ABI.
	// Topics: [0]=sig, [1]=messageID, [2]=sourceBlockchainID.
	MessageExecutionFailedTopic common.Hash

	// TeleporterABI is the parsed Teleporter ABI, available for log data decoding.
	TeleporterABI abi.ABI
)

//go:embed teleporter_abi.json
var teleporterABIJSON string

func init() {
	// Compute SendWarpMessage topic from the canonical event signature string.
	// This is safe to hand-compute because the event has no struct parameters.
	SendWarpMessageTopic = crypto.Keccak256Hash([]byte("SendWarpMessage(address,bytes32,bytes)"))

	// Parse the embedded Teleporter ABI JSON. Using the ABI parser guarantees
	// that struct-parameter events get the correct tuple-encoded topic hash —
	// hand-computing these from strings is error-prone and fragile.
	var err error
	TeleporterABI, err = abi.JSON(strings.NewReader(teleporterABIJSON))
	if err != nil {
		panic("warp/events: parse embedded Teleporter ABI: " + err.Error())
	}

	ev, ok := TeleporterABI.Events["ReceiveCrossChainMessage"]
	if !ok {
		panic("warp/events: ReceiveCrossChainMessage not found in Teleporter ABI")
	}
	ReceiveCrossChainMessageTopic = ev.ID

	ev, ok = TeleporterABI.Events["MessageExecutionFailed"]
	if !ok {
		panic("warp/events: MessageExecutionFailed not found in Teleporter ABI")
	}
	MessageExecutionFailedTopic = ev.ID
}
