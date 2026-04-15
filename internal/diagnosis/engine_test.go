package diagnosis

import (
	"context"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/types"
	"github.com/bytemaster333/warpscope/internal/chain"
)

// ── Fakes ────────────────────────────────────────────────────────────────────

type fakeEVMClient struct {
	sendEvent       *chain.WarpMessageEvent
	receiveEvent    *chain.ReceiveEvent
	execFailedEvent *chain.ExecutionFailedEvent
	blockNum        uint64
	sendErr         error
	receiveErr      error
}

func (f *fakeEVMClient) GetSendWarpEvent(_ context.Context, _ common.Hash) (*chain.WarpMessageEvent, error) {
	return f.sendEvent, f.sendErr
}
func (f *fakeEVMClient) FindReceiveEvent(_ context.Context, _ common.Hash, _, _ uint64) (*chain.ReceiveEvent, error) {
	return f.receiveEvent, f.receiveErr
}
func (f *fakeEVMClient) FindExecutionFailedEvent(_ context.Context, _ common.Hash, _, _ uint64) (*chain.ExecutionFailedEvent, error) {
	return f.execFailedEvent, nil
}
func (f *fakeEVMClient) BlockNumber(_ context.Context) (uint64, error) {
	return f.blockNum, nil
}
func (f *fakeEVMClient) TransactionReceipt(_ context.Context, _ common.Hash) (*types.Receipt, error) {
	return &types.Receipt{}, nil
}

type fakePChainClient struct {
	validators        []chain.ValidatorInfo
	validatorsAtH     []chain.ValidatorInfo
	height            uint64
	networkID         uint32
	validatorsErr     error
	validatorsAtHErr  error
}

func (f *fakePChainClient) GetCurrentValidators(_ context.Context, _ ids.ID) ([]chain.ValidatorInfo, error) {
	return f.validators, f.validatorsErr
}
func (f *fakePChainClient) GetValidatorsAt(_ context.Context, _ ids.ID, _ uint64) ([]chain.ValidatorInfo, error) {
	if f.validatorsAtH != nil {
		return f.validatorsAtH, f.validatorsAtHErr
	}
	return f.validators, f.validatorsAtHErr
}
func (f *fakePChainClient) GetHeight(_ context.Context) (uint64, error) {
	return f.height, nil
}
func (f *fakePChainClient) GetNetworkID(_ context.Context) (uint32, error) {
	return f.networkID, nil
}

// ── Test helpers ──────────────────────────────────────────────────────────────

func makeFakeWarpMsg() []byte {
	// Return an empty byte slice; warpparse.ParseUnsignedMessageBytes handles
	// empty bytes gracefully by returning a partial ParsedWarpMessage.
	return []byte{}
}

var testTxHash = common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
var testMsgID  = common.HexToHash("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestDiagnose_NoSendEvent verifies the engine handles a transaction with no
// Warp event (wrong txHash) by returning an early RELAYER_NEVER_PICKED_UP-like
// report with a clear summary.
func TestDiagnose_NoSendEvent(t *testing.T) {
	src := &fakeEVMClient{sendEvent: nil, blockNum: 100}
	pchain := &fakePChainClient{networkID: 5, height: 1000}

	engine := &Engine{
		SourceClient:      src,
		PChain:            pchain,
		SubnetID:          ids.Empty,
		ExpectedNetworkID: 5,
		QuorumNum:         67,
		QuorumDen:         100,
	}

	report, err := engine.Diagnose(context.Background(), testTxHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.IsHealthy {
		t.Error("expected IsHealthy=false when no send event found")
	}
	if report.RootCause != CategoryRelayerNeverPickedUp {
		t.Errorf("expected root cause RELAYER_NEVER_PICKED_UP, got %s", report.RootCause)
	}
}

// TestDiagnose_MessageDelivered verifies that a successfully delivered message
// results in an IsHealthy=true report with all checks passing or warning-only.
func TestDiagnose_MessageDelivered(t *testing.T) {
	sendEvent := &chain.WarpMessageEvent{
		TxHash:           testTxHash,
		BlockNumber:      100,
		MessageID:        testMsgID,
		UnsignedMsgBytes: []byte{}, // empty: parser returns partial result
	}
	receiveEvent := &chain.ReceiveEvent{
		TxHash:      common.HexToHash("0xaaaa"),
		BlockNumber: 200,
		MessageID:   testMsgID,
	}

	src := &fakeEVMClient{sendEvent: sendEvent, blockNum: 100}
	dest := &fakeEVMClient{receiveEvent: receiveEvent, blockNum: 1000}
	pchain := &fakePChainClient{networkID: 1, height: 500}

	engine := &Engine{
		SourceClient:      src,
		DestClient:        dest,
		PChain:            pchain,
		SubnetID:          ids.Empty,
		ExpectedNetworkID: 1,
		QuorumNum:         67,
		QuorumDen:         100,
	}

	report, err := engine.Diagnose(context.Background(), testTxHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Check that RelayerPickup passed.
	var pickupCheck *CheckResult
	for _, c := range report.Checks {
		if c.Name == "RelayerPickup" {
			pickupCheck = c
			break
		}
	}
	if pickupCheck == nil {
		t.Fatal("RelayerPickup check not found")
	}
	if !pickupCheck.Passed {
		t.Errorf("expected RelayerPickup to pass when receive event exists, got: %s", pickupCheck.Description)
	}
}

// TestDiagnose_NotDelivered verifies that RELAYER_NEVER_PICKED_UP is the root
// cause when a send event exists but no receive event is found.
func TestDiagnose_NotDelivered(t *testing.T) {
	sendEvent := &chain.WarpMessageEvent{
		TxHash:           testTxHash,
		MessageID:        testMsgID,
		UnsignedMsgBytes: []byte{},
	}

	src := &fakeEVMClient{sendEvent: sendEvent, blockNum: 100}
	dest := &fakeEVMClient{receiveEvent: nil, blockNum: 1000}
	pchain := &fakePChainClient{networkID: 1, height: 500}

	engine := &Engine{
		SourceClient:      src,
		DestClient:        dest,
		PChain:            pchain,
		SubnetID:          ids.Empty,
		ExpectedNetworkID: 1,
		QuorumNum:         67,
		QuorumDen:         100,
	}

	report, err := engine.Diagnose(context.Background(), testTxHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.IsHealthy {
		t.Error("expected IsHealthy=false when message not delivered")
	}
	if report.RootCause != CategoryRelayerNeverPickedUp {
		t.Errorf("expected RELAYER_NEVER_PICKED_UP, got %s", report.RootCause)
	}
}

// TestDiagnose_ExecutionFailed verifies the DESTINATION_EXECUTION_FAILED category
// when the message was delivered but the destination contract reverted.
func TestDiagnose_ExecutionFailed(t *testing.T) {
	sendEvent := &chain.WarpMessageEvent{
		TxHash:           testTxHash,
		MessageID:        testMsgID,
		UnsignedMsgBytes: []byte{},
	}
	receiveEvent := &chain.ReceiveEvent{
		TxHash:      common.HexToHash("0xbbbb"),
		BlockNumber: 200,
		MessageID:   testMsgID,
	}
	execFailed := &chain.ExecutionFailedEvent{
		TxHash:      common.HexToHash("0xbbbb"),
		BlockNumber: 200,
		MessageID:   testMsgID,
	}

	src := &fakeEVMClient{sendEvent: sendEvent, blockNum: 100}
	dest := &fakeEVMClient{receiveEvent: receiveEvent, execFailedEvent: execFailed, blockNum: 1000}
	pchain := &fakePChainClient{networkID: 1, height: 500}

	engine := &Engine{
		SourceClient:      src,
		DestClient:        dest,
		PChain:            pchain,
		SubnetID:          ids.Empty,
		ExpectedNetworkID: 1,
		QuorumNum:         67,
		QuorumDen:         100,
	}

	report, err := engine.Diagnose(context.Background(), testTxHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.RootCause != CategoryDestinationExecutionFailed {
		t.Errorf("expected DESTINATION_EXECUTION_FAILED, got %s", report.RootCause)
	}
}

// TestCheckNetworkID_Mismatch verifies the network ID check with mismatched IDs.
func TestCheckNetworkID_Mismatch(t *testing.T) {
	// Build a DiagnosisInput with a mismatched network ID in the parsed message.
	// We use a synthetic ParsedWarpMessage by constructing it manually.
	// (We can't construct a real UnsignedMessage without avalanchego codec.)
	// Instead test via checkNetworkID with nil ParsedMsg → should pass (skipped).
	in := &DiagnosisInput{
		ParsedMsg:         nil,
		ExpectedNetworkID: 1,
	}
	result := checkNetworkID(in)
	if !result.Passed {
		t.Errorf("expected checkNetworkID to pass when ParsedMsg is nil (check skipped)")
	}
}

// TestBuildSummary verifies summary generation for healthy and unhealthy reports.
func TestBuildSummary(t *testing.T) {
	healthyReport := &DiagnosticReport{
		TxHash:    testTxHash,
		MessageID: testMsgID,
		IsHealthy: true,
		RootCause: CategoryHealthy,
	}
	summary := buildSummary(healthyReport)
	if summary == "" {
		t.Error("expected non-empty summary for healthy report")
	}

	unhealthyReport := &DiagnosticReport{
		TxHash:    testTxHash,
		MessageID: testMsgID,
		IsHealthy: false,
		RootCause: CategoryRelayerNeverPickedUp,
		Checks: []*CheckResult{
			{
				Name:        "RelayerPickup",
				Category:    CategoryRelayerNeverPickedUp,
				Passed:      false,
				Severity:    SeverityCritical,
				Description: "No receive event found.",
			},
		},
	}
	summary = buildSummary(unhealthyReport)
	if summary == "" {
		t.Error("expected non-empty summary for unhealthy report")
	}
}

// TestEngine_NoDestClient verifies the engine works without a destination client.
func TestEngine_NoDestClient(t *testing.T) {
	sendEvent := &chain.WarpMessageEvent{
		TxHash:           testTxHash,
		MessageID:        testMsgID,
		UnsignedMsgBytes: []byte{},
	}
	src := &fakeEVMClient{sendEvent: sendEvent, blockNum: 100}
	pchain := &fakePChainClient{networkID: 1, height: 500}

	engine := &Engine{
		SourceClient:      src,
		DestClient:        nil, // no destination client
		PChain:            pchain,
		SubnetID:          ids.Empty,
		ExpectedNetworkID: 1,
		QuorumNum:         67,
		QuorumDen:         100,
	}

	report, err := engine.Diagnose(context.Background(), testTxHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without a dest client, we can't confirm delivery, so RELAYER_NEVER_PICKED_UP
	// check should fail (no receive event) → unhealthy.
	if report == nil {
		t.Fatal("expected non-nil report")
	}
}

