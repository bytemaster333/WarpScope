package warp

import (
	"fmt"

	avawarp "github.com/ava-labs/avalanchego/vms/platformvm/warp"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp/payload"
	"github.com/ava-labs/libevm/accounts/abi"
	"github.com/ava-labs/libevm/core/types"
)

// ParsedWarpMessage contains the decoded components extracted from a
// SendWarpMessage event log or from a signed message bytes blob.
type ParsedWarpMessage struct {
	// UnsignedMsg is the decoded unsigned Warp message.
	UnsignedMsg *avawarp.UnsignedMessage

	// BitSetSig is the BitSetSignature when parsing a signed message.
	// Nil when only an unsigned message was available (e.g., from the source event).
	BitSetSig *avawarp.BitSetSignature

	// AddressedCall is the decoded payload when the message carries an
	// AddressedCall (standard for Teleporter messages).
	AddressedCall *payload.AddressedCall

	// RawMsgBytes are UnsignedMsg.Bytes() — the codec-serialized unsigned message
	// bytes that validators sign. Pass these directly to bls.Verify.
	RawMsgBytes []byte
}

// DecodeUnsignedMsgFromLog decodes the unsigned Warp message from a
// SendWarpMessage event log's Data field.
//
// The Data field contains one ABI-encoded `bytes` parameter holding the full
// unsigned Warp message bytes. Topics[1]=sender, Topics[2]=messageID are indexed
// and pre-decoded by the EVM adapter; only Data needs further decoding here.
func DecodeUnsignedMsgFromLog(log *types.Log) (*ParsedWarpMessage, error) {
	if len(log.Data) == 0 {
		return nil, fmt.Errorf("SendWarpMessage log has empty Data field")
	}

	// ABI-decode the single non-indexed `bytes message` parameter from log.Data.
	bytesType, err := abi.NewType("bytes", "", nil)
	if err != nil {
		return nil, fmt.Errorf("build ABI bytes type: %w", err)
	}
	values, err := abi.Arguments{{Type: bytesType}}.Unpack(log.Data)
	if err != nil {
		return nil, fmt.Errorf("ABI-unpack SendWarpMessage data: %w", err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("ABI-unpack returned no values")
	}
	msgBytes, ok := values[0].([]byte)
	if !ok {
		return nil, fmt.Errorf("ABI-unpack: expected []byte, got %T", values[0])
	}

	return parseUnsignedBytes(msgBytes)
}

// ParseUnsignedMessageBytes parses raw unsigned Warp message bytes into a
// ParsedWarpMessage. Use this when you have the raw bytes (e.g., stored from
// a previously decoded log) rather than a live log object.
func ParseUnsignedMessageBytes(msgBytes []byte) (*ParsedWarpMessage, error) {
	return parseUnsignedBytes(msgBytes)
}

// parseUnsignedBytes parses raw unsigned Warp message bytes.
func parseUnsignedBytes(msgBytes []byte) (*ParsedWarpMessage, error) {
	unsignedMsg, err := avawarp.ParseUnsignedMessage(msgBytes)
	if err != nil {
		return nil, fmt.Errorf("parse unsigned Warp message: %w", err)
	}

	parsed := &ParsedWarpMessage{
		UnsignedMsg: unsignedMsg,
		RawMsgBytes: unsignedMsg.Bytes(),
	}

	// Attempt to decode the AddressedCall payload; non-fatal if it fails
	// (e.g., the message uses a different payload type).
	if ac, err := decodeAddressedCall(unsignedMsg.Payload); err == nil {
		parsed.AddressedCall = ac
	}

	return parsed, nil
}

// ParseSignedMessage decodes a fully signed Warp message (UnsignedMessage +
// BitSetSignature) from its codec-serialized bytes.
// Returns an error if the signature is not a BitSetSignature.
func ParseSignedMessage(msgBytes []byte) (*ParsedWarpMessage, error) {
	msg, err := avawarp.ParseMessage(msgBytes)
	if err != nil {
		return nil, fmt.Errorf("parse signed Warp message: %w", err)
	}

	bss, ok := msg.Signature.(*avawarp.BitSetSignature)
	if !ok {
		return nil, fmt.Errorf("unsupported signature type %T; WarpScope requires *warp.BitSetSignature", msg.Signature)
	}

	parsed := &ParsedWarpMessage{
		UnsignedMsg: &msg.UnsignedMessage,
		BitSetSig:   bss,
		RawMsgBytes: msg.UnsignedMessage.Bytes(),
	}

	if ac, err := decodeAddressedCall(msg.UnsignedMessage.Payload); err == nil {
		parsed.AddressedCall = ac
	}

	return parsed, nil
}

// decodeAddressedCall decodes a Warp payload as an AddressedCall.
func decodeAddressedCall(payloadBytes []byte) (*payload.AddressedCall, error) {
	p, err := payload.Parse(payloadBytes)
	if err != nil {
		return nil, err
	}
	ac, ok := p.(*payload.AddressedCall)
	if !ok {
		return nil, fmt.Errorf("payload is %T, not *payload.AddressedCall", p)
	}
	return ac, nil
}

// ValidateNetworkID returns an error when the unsigned message's NetworkID
// does not match the expected network.
func ValidateNetworkID(unsignedMsg *avawarp.UnsignedMessage, expectedNetworkID uint32) error {
	if unsignedMsg.NetworkID != expectedNetworkID {
		return fmt.Errorf(
			"NetworkID mismatch: message carries %d, expected %d (%s); "+
				"possible cross-network replay or relayer misconfiguration",
			unsignedMsg.NetworkID, expectedNetworkID, networkName(expectedNetworkID),
		)
	}
	return nil
}

func networkName(id uint32) string {
	switch id {
	case 1:
		return "Mainnet"
	case 5:
		return "Fuji"
	default:
		return "unknown"
	}
}
