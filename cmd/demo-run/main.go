// demo-run: WarpScope Diagnosis Engine'in uçtan uca demosunu çalıştırır.
// Gerçek RPC bağlantısı olmadan, engine_test.go'daki fake istemcileri kullanarak
// tüm kontrolleri ve çıktı render'ını gerçek verilerle gösterir.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/types"
	"github.com/bytemaster333/warpscope/internal/chain"
	"github.com/bytemaster333/warpscope/internal/diagnosis"
	"github.com/bytemaster333/warpscope/internal/output"
)

// ─── Fake istemciler (ağ bağlantısı gerektirmez) ──────────────────────────────

type fakeEVMClient struct {
	sendEvent       *chain.WarpMessageEvent
	receiveEvent    *chain.ReceiveEvent
	execFailedEvent *chain.ExecutionFailedEvent
	blockNum        uint64
}

func (f *fakeEVMClient) GetSendWarpEvent(_ context.Context, _ common.Hash) (*chain.WarpMessageEvent, error) {
	return f.sendEvent, nil
}
func (f *fakeEVMClient) FindReceiveEvent(_ context.Context, _ common.Hash, _, _ uint64) (*chain.ReceiveEvent, error) {
	return f.receiveEvent, nil
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
	validatorsAt  []chain.ValidatorInfo
	currentVals   []chain.ValidatorInfo
	height        uint64
	currentHeight uint64
	networkID     uint32
}

func (f *fakePChainClient) GetCurrentValidators(_ context.Context, _ ids.ID) ([]chain.ValidatorInfo, error) {
	if f.currentVals != nil {
		return f.currentVals, nil
	}
	return f.validatorsAt, nil
}
func (f *fakePChainClient) GetValidatorsAt(_ context.Context, _ ids.ID, h uint64) ([]chain.ValidatorInfo, error) {
	return f.validatorsAt, nil
}
func (f *fakePChainClient) GetHeight(_ context.Context) (uint64, error) {
	if f.currentHeight > 0 {
		return f.currentHeight, nil
	}
	return f.height, nil
}
func (f *fakePChainClient) GetNetworkID(_ context.Context) (uint32, error) {
	return f.networkID, nil
}

// ─── Senaryo builder ──────────────────────────────────────────────────────────

func buildValidators(count int, withBLS bool) []chain.ValidatorInfo {
	vals := make([]chain.ValidatorInfo, count)
	for i := 0; i < count; i++ {
		v := chain.ValidatorInfo{
			NodeID: ids.NodeID{byte(i + 1)},
			Weight: 100 + uint64(i*50),
		}
		if withBLS {
			key := make([]byte, 48)
			key[0] = byte(i + 1)
			v.PublicKeyBytes = key
		}
		vals[i] = v
	}
	return vals
}

// ─── Demo senaryoları ─────────────────────────────────────────────────────────

func runScenario(title string, src, dest *fakeEVMClient, pchain *fakePChainClient,
	txHash common.Hash, pchainHeight uint64, outputFmt string) {

	fmt.Printf("\n%s\n%s\n", title, repeatChar('═', len(title)))

	engine := &diagnosis.Engine{
		SourceClient:      src,
		DestClient:        dest,
		PChain:            pchain,
		SubnetID:          ids.Empty,
		ExpectedNetworkID: pchain.networkID,
		PChainHeight:      pchainHeight,
		QuorumNum:         67,
		QuorumDen:         100,
	}

	report, err := engine.Diagnose(context.Background(), txHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Hata: %v\n", err)
		return
	}

	// Timestamp'i sabitleyerek deterministik çıktı
	report.Timestamp = time.Date(2026, 4, 13, 18, 0, 0, 0, time.UTC)

	if outputFmt == "json" {
		_ = output.WriteJSON(os.Stdout, report)
	} else {
		output.WriteTable(os.Stdout, report)
	}
}

func repeatChar(c rune, n int) string {
	s := make([]rune, n)
	for i := range s {
		s[i] = c
	}
	return string(s)
}

func main() {
	// Gerçekçi TX ve message ID'leri
	sourceTxHash := common.HexToHash("0x7a3f9b2c4d1e8f0a5b6c7d8e9f0a1b2c3d4e5f607182930a1b2c3d4e5f607182")
	msgID := common.HexToHash("0xdeadbeef00000000000000000000000000000000000000000000000000001234")
	destTxHash := common.HexToHash("0xaaaa1111bbbb2222cccc3333dddd4444eeee5555ffff666677778888999900ab")

	validators := buildValidators(5, true) // 5 validator, hepsi BLS key'e sahip

	fmt.Println("╔════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║         WarpScope — Cross-Chain Warp Message Diagnostic Engine     ║")
	fmt.Println("║         Fuji Testnet E2E Demonstration (Simulated)                 ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("\nSimülasyon zamanı: 2026-04-13 18:00:00 UTC\n")
	fmt.Printf("Kaynak TX Hash  : %s\n", sourceTxHash.Hex())

	// ─── Senaryo 1: Mesaj başarıyla iletildi (HEALTHY) ─────────────────────────
	runScenario(
		"SENARYO 1: Başarıyla İletilmiş Mesaj (HEALTHY)",
		&fakeEVMClient{
			sendEvent: &chain.WarpMessageEvent{
				TxHash:           sourceTxHash,
				BlockNumber:      38_421_001,
				MessageID:        msgID,
				UnsignedMsgBytes: []byte{},
			},
			blockNum: 38_421_001,
		},
		&fakeEVMClient{
			receiveEvent: &chain.ReceiveEvent{
				TxHash:      destTxHash,
				BlockNumber: 12_305_100,
				MessageID:   msgID,
			},
			blockNum: 12_305_200,
		},
		&fakePChainClient{
			networkID:     5,
			height:        2_014_500,
			currentHeight: 2_014_500,
			validatorsAt:  validators,
		},
		sourceTxHash, 2_014_500, "table",
	)

	// ─── Senaryo 2: Relayer mesajı hiç almadı (RELAYER_NEVER_PICKED_UP) ────────
	runScenario(
		"SENARYO 2: Relayer Mesajı Almadı (RELAYER_NEVER_PICKED_UP)",
		&fakeEVMClient{
			sendEvent: &chain.WarpMessageEvent{
				TxHash:           sourceTxHash,
				BlockNumber:      38_421_001,
				MessageID:        msgID,
				UnsignedMsgBytes: []byte{},
			},
			blockNum: 38_421_001,
		},
		&fakeEVMClient{
			receiveEvent: nil, // hiç iletilmedi
			blockNum:     12_400_000,
		},
		&fakePChainClient{
			networkID:     5,
			height:        2_014_500,
			currentHeight: 2_014_500,
			validatorsAt:  validators,
		},
		sourceTxHash, 2_014_500, "table",
	)

	// ─── Senaryo 3: İletildi ama kontrat revert etti (EXECUTION_FAILED) ─────────
	runScenario(
		"SENARYO 3: Hedef Kontrat Revert Etti (DESTINATION_EXECUTION_FAILED)",
		&fakeEVMClient{
			sendEvent: &chain.WarpMessageEvent{
				TxHash:           sourceTxHash,
				BlockNumber:      38_421_001,
				MessageID:        msgID,
				UnsignedMsgBytes: []byte{},
			},
			blockNum: 38_421_001,
		},
		&fakeEVMClient{
			receiveEvent: &chain.ReceiveEvent{
				TxHash:      destTxHash,
				BlockNumber: 12_305_100,
				MessageID:   msgID,
			},
			execFailedEvent: &chain.ExecutionFailedEvent{
				TxHash:      destTxHash,
				BlockNumber: 12_305_100,
				MessageID:   msgID,
			},
			blockNum: 12_305_200,
		},
		&fakePChainClient{
			networkID:     5,
			height:        2_014_500,
			currentHeight: 2_014_500,
			validatorsAt:  validators,
		},
		sourceTxHash, 2_014_500, "table",
	)

	// ─── Senaryo 4: Validator seti değişti (churn) ──────────────────────────────
	// Signing'de 5 validator vardı; şimdi 3'ü kaldı
	currentVals := buildValidators(3, true) // 2 validator çıktı
	runScenario(
		"SENARYO 4: Validator Seti Değişti (VALIDATOR_SET_CHANGED)",
		&fakeEVMClient{
			sendEvent: &chain.WarpMessageEvent{
				TxHash:           sourceTxHash,
				BlockNumber:      38_421_001,
				MessageID:        msgID,
				UnsignedMsgBytes: []byte{},
			},
			blockNum: 38_421_001,
		},
		&fakeEVMClient{
			receiveEvent: nil,
			blockNum:     12_500_000,
		},
		&fakePChainClient{
			networkID:     5,
			height:        2_014_500, // imzalama anındaki height
			currentHeight: 2_014_650, // şimdiki height (150 blok = 1 epoch)
			validatorsAt:  validators,
			currentVals:   currentVals,
		},
		sourceTxHash, 2_014_500, "table",
	)

	// ─── Senaryo 5: JSON formatında çıktı ───────────────────────────────────────
	fmt.Printf("\n%s\n%s\n", "SENARYO 5: JSON Çıktı Formatı", repeatChar('═', 35))
	runScenario(
		"",
		&fakeEVMClient{
			sendEvent: &chain.WarpMessageEvent{
				TxHash:           sourceTxHash,
				BlockNumber:      38_421_001,
				MessageID:        msgID,
				UnsignedMsgBytes: []byte{},
			},
			blockNum: 38_421_001,
		},
		&fakeEVMClient{
			receiveEvent: &chain.ReceiveEvent{
				TxHash:      destTxHash,
				BlockNumber: 12_305_100,
				MessageID:   msgID,
			},
			blockNum: 12_305_200,
		},
		&fakePChainClient{
			networkID:     5,
			height:        2_014_500,
			currentHeight: 2_014_500,
			validatorsAt:  validators,
		},
		sourceTxHash, 2_014_500, "json",
	)

	fmt.Println("\n✓ Demo tamamlandı.")
	fmt.Println("\nGerçek Fuji TX'i için:")
	fmt.Println("  CGO_ENABLED=1 go run ./cmd/fuji-finder/ \\")
	fmt.Println("    --source-rpc https://api.avax-test.network/ext/bc/C/rpc \\")
	fmt.Println("    --pchain-rpc https://api.avax-test.network")
	fmt.Println("")
	fmt.Println("Ardından:")
	fmt.Println("  ./warpscope diagnose <BULDUĞUNUZ_TX_HASH> \\")
	fmt.Println("    --source-rpc  https://api.avax-test.network/ext/bc/C/rpc \\")
	fmt.Println("    --dest-rpc    https://<HEDEF_ZİNCİR_RPC> \\")
	fmt.Println("    --pchain-rpc  https://api.avax-test.network \\")
	fmt.Println("    --subnet-id   11111111111111111111111111111111LpoYY \\")
	fmt.Println("    --network     fuji \\")
	fmt.Println("    --output      table")
}
