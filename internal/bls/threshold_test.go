package bls

import (
	"testing"
)

func TestCheckThreshold(t *testing.T) {
	tests := []struct {
		name         string
		signedWeight uint64
		totalWeight  uint64
		quorumNum    uint64
		quorumDen    uint64
		wantErr      bool
	}{
		{"exact 67%",          67, 100, 67, 100, false},
		{"above 67%",          68, 100, 67, 100, false},
		{"100%",               100, 100, 67, 100, false},
		{"just below 67%",     66, 100, 67, 100, true},
		{"zero signed",        0,  100, 67, 100, true},
		{"50% below quorum",   50, 100, 67, 100, true},
		{"custom quorum 33%",  33, 100, 33, 100, false},
		{"custom quorum 33% miss", 32, 100, 33, 100, true},
		{"all weight signed",  1000, 1000, 67, 100, false},
		{"zero denominator",   67, 100, 67, 0, true},
		{"zero total",         0, 0, 67, 100, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckThreshold(tt.signedWeight, tt.totalWeight, tt.quorumNum, tt.quorumDen)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckThreshold(%d, %d, %d, %d) error=%v, wantErr=%v",
					tt.signedWeight, tt.totalWeight, tt.quorumNum, tt.quorumDen, err, tt.wantErr)
			}
		})
	}
}

func TestAccumulateWeight(t *testing.T) {
	weights := []uint64{100, 200, 300, 400}
	tests := []struct {
		signers []byte
		want    uint64
	}{
		{[]byte{0x0F}, 1000}, // all 4 bits set: 100+200+300+400
		{[]byte{0x01}, 100},  // bit 0: 100
		{[]byte{0x06}, 500},  // bits 1,2: 200+300
		{[]byte{0x00}, 0},    // no bits
	}
	for _, tt := range tests {
		got := AccumulateWeight(tt.signers, weights)
		if got != tt.want {
			t.Errorf("AccumulateWeight(%x, %v) = %d, want %d", tt.signers, weights, got, tt.want)
		}
	}
}
