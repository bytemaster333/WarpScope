package bls

import (
	"testing"
)

func TestIsSet(t *testing.T) {
	tests := []struct {
		name    string
		signers []byte
		bit     int
		want    bool
	}{
		{"bit 0 set",       []byte{0x01}, 0, true},
		{"bit 1 set",       []byte{0x02}, 1, true},
		{"bit 7 set",       []byte{0x80}, 7, true},
		{"bit 8 set",       []byte{0x00, 0x01}, 8, true},
		{"bit 0 not set",   []byte{0xFE}, 0, false},
		{"empty signers",   []byte{}, 0, false},
		{"out of range",    []byte{0xFF}, 8, false},
		{"all bits set",    []byte{0xFF}, 3, true},
		{"no bits set",     []byte{0x00}, 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSet(tt.signers, tt.bit)
			if got != tt.want {
				t.Errorf("IsSet(%x, %d) = %v, want %v", tt.signers, tt.bit, got, tt.want)
			}
		})
	}
}

func TestSignerIndices(t *testing.T) {
	tests := []struct {
		name           string
		signers        []byte
		validatorCount int
		want           []int
	}{
		{"no bits",       []byte{0x00}, 8, nil},
		{"bit 0 only",    []byte{0x01}, 8, []int{0}},
		{"bits 0 and 2",  []byte{0x05}, 8, []int{0, 2}},
		{"all 8 bits",    []byte{0xFF}, 8, []int{0, 1, 2, 3, 4, 5, 6, 7}},
		{"count limits",  []byte{0xFF}, 3, []int{0, 1, 2}},
		{"cross byte",    []byte{0x80, 0x01}, 16, []int{7, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SignerIndices(tt.signers, tt.validatorCount)
			if len(got) != len(tt.want) {
				t.Fatalf("SignerIndices(%x, %d) = %v, want %v", tt.signers, tt.validatorCount, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSignerCount(t *testing.T) {
	if n := SignerCount([]byte{0xFF}, 8); n != 8 {
		t.Errorf("got %d, want 8", n)
	}
	if n := SignerCount([]byte{0x00}, 8); n != 0 {
		t.Errorf("got %d, want 0", n)
	}
	if n := SignerCount([]byte{0x07}, 3); n != 3 {
		t.Errorf("got %d, want 3", n)
	}
}
