package warp

import (
	"fmt"

	"github.com/ava-labs/libevm/accounts/abi"
	"github.com/ava-labs/libevm/common"
)

// TeleporterPayload contains the decoded Teleporter-layer fields from an
// AddressedCall whose SourceAddress is the TeleporterMessenger contract.
type TeleporterPayload struct {
	// DestinationChainID is the target blockchain ID (bytes32).
	DestinationChainID common.Hash

	// DestinationAddress is the contract to call on the destination chain.
	DestinationAddress common.Address

	// SourceAddress is the TeleporterMessenger address on the source chain.
	// For Teleporter v1.0.0 this is always 0x253b2784c75e510dD0fF1da844684a1aC0aa5fcf.
	SourceAddress common.Address

	// MessageNonce is the monotonically increasing message counter.
	MessageNonce uint64

	// RawPayload contains the full ABI-encoded TeleporterMessage bytes for
	// callers that need the full struct.
	RawPayload []byte
}

// DecodeTeleporterPayload extracts Teleporter-layer metadata from a ParsedWarpMessage.
// It validates that the AddressedCall.SourceAddress is the TeleporterMessenger,
// then ABI-decodes the inner payload as a TeleporterMessage struct.
//
// Returns an error if:
//   - the message has no AddressedCall payload
//   - the SourceAddress is not the TeleporterMessenger address
func DecodeTeleporterPayload(parsed *ParsedWarpMessage) (*TeleporterPayload, error) {
	if parsed.AddressedCall == nil {
		return nil, fmt.Errorf("Warp message has no AddressedCall payload; not a Teleporter message")
	}

	// The SourceAddress in AddressedCall should be the TeleporterMessenger address.
	srcAddr := common.BytesToAddress(parsed.AddressedCall.SourceAddress)
	if srcAddr != TeleporterAddress {
		return nil, fmt.Errorf(
			"AddressedCall.SourceAddress is %s, expected TeleporterMessenger %s; "+
				"this Warp message was not sent by TeleporterMessenger",
			srcAddr.Hex(), TeleporterAddress.Hex(),
		)
	}

	// ABI-decode the inner payload as a TeleporterMessage.
	// We only extract the fields we need for diagnostics.
	tp, err := decodeTeleporterMessageMinimal(parsed.AddressedCall.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode TeleporterMessage from AddressedCall payload: %w", err)
	}
	tp.SourceAddress = srcAddr
	tp.RawPayload = parsed.AddressedCall.Payload

	return tp, nil
}

// decodeTeleporterMessageMinimal ABI-decodes the fields needed for diagnostics
// from a raw TeleporterMessage ABI-encoded payload.
func decodeTeleporterMessageMinimal(data []byte) (*TeleporterPayload, error) {
	// Use the TeleporterABI loaded in events.go to decode the message struct.
	// The SendCrossChainMessage event's `message` parameter uses the same struct.
	method, ok := TeleporterABI.Events["SendCrossChainMessage"]
	if !ok {
		return nil, fmt.Errorf("SendCrossChainMessage not found in Teleporter ABI")
	}

	// Find the `message` argument (tuple type).
	var msgArg abi.Argument
	for _, arg := range method.Inputs {
		if arg.Name == "message" {
			msgArg = arg
			break
		}
	}
	if msgArg.Name == "" {
		return nil, fmt.Errorf("message argument not found in SendCrossChainMessage event")
	}

	values, err := abi.Arguments{msgArg}.Unpack(data)
	if err != nil {
		return nil, fmt.Errorf("ABI-unpack TeleporterMessage: %w", err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("ABI-unpack returned no values")
	}

	// The decoded value is a struct represented as a map or anonymous struct.
	// Use type assertion to extract the fields we care about.
	msgMap, ok := values[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("decoded TeleporterMessage is %T, expected map", values[0])
	}

	tp := &TeleporterPayload{}

	if destChainID, ok := msgMap["destinationBlockchainID"].([32]byte); ok {
		tp.DestinationChainID = destChainID
	}
	if destAddr, ok := msgMap["destinationAddress"].(common.Address); ok {
		tp.DestinationAddress = destAddr
	}

	return tp, nil
}
