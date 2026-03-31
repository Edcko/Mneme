package vector

import (
	"math"
	"testing"
)

func TestEncodeDecodeEmbedding_Roundtrip(t *testing.T) {
	tests := []struct {
		name string
		vec  []float32
	}{
		{name: "empty", vec: []float32{}},
		{name: "single dimension", vec: []float32{3.14}},
		{name: "768 dimensions (OpenAI ada-002)", vec: makeTestVec(768, 42)},
		{name: "1536 dimensions (OpenAI text-embedding-3-small)", vec: makeTestVec(1536, 99)},
		{name: "negative values", vec: []float32{-1.0, -0.5, -0.001}},
		{name: "mixed values", vec: []float32{0.0, 1.0, -1.0, 0.5, -0.5, 1e-6, -1e-6}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded := EncodeEmbedding(tc.vec)
			decoded, err := DecodeEmbedding(encoded)
			if err != nil {
				t.Fatalf("DecodeEmbedding error: %v", err)
			}

			if len(decoded) != len(tc.vec) {
				t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(tc.vec))
			}

			for i := range tc.vec {
				if decoded[i] != tc.vec[i] {
					t.Errorf("index %d: got %v, want %v", i, decoded[i], tc.vec[i])
				}
			}
		})
	}
}

func TestEncodeEmbedding_Nil(t *testing.T) {
	encoded := EncodeEmbedding(nil)
	if len(encoded) != 0 {
		t.Errorf("expected empty BLOB for nil input, got %d bytes", len(encoded))
	}
}

func TestDecodeEmbedding_Nil(t *testing.T) {
	vec, err := DecodeEmbedding(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 0 {
		t.Errorf("expected empty vector, got %d elements", len(vec))
	}
}

func TestDecodeEmbedding_EmptyBlob(t *testing.T) {
	vec, err := DecodeEmbedding([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 0 {
		t.Errorf("expected empty vector, got %d elements", len(vec))
	}
}

func TestDecodeEmbedding_TooShort(t *testing.T) {
	short := []byte{0x01, 0x00}
	_, err := DecodeEmbedding(short)
	if err == nil {
		t.Fatal("expected error for blob too short to contain length prefix")
	}
}

func TestDecodeEmbedding_LengthMismatch(t *testing.T) {
	header := make([]byte, 4)
	header[0] = 0x05
	data := make([]byte, 4+3*4)
	copy(data, header)
	_, err := DecodeEmbedding(data)
	if err == nil {
		t.Fatal("expected error for length mismatch")
	}
	var blobErr ErrInvalidBlob
	if ok := isErrorType(err, &blobErr); !ok {
		t.Errorf("expected ErrInvalidBlob, got %T: %v", err, err)
	}
}

func TestEncodeEmbedding_BlobSize(t *testing.T) {
	vec := makeTestVec(768, 0)
	encoded := EncodeEmbedding(vec)

	expectedSize := 4 + 768*4
	if len(encoded) != expectedSize {
		t.Errorf("blob size = %d, want %d (4 header + 768*4 payload)", len(encoded), expectedSize)
	}
}

func TestEncodeEmbedding_PreservesSpecialValues(t *testing.T) {
	vec := []float32{
		0.0,
		math.Float32frombits(0x7F800000),
		math.Float32frombits(0xFF800000),
		math.Float32frombits(0x7FC00000),
	}

	encoded := EncodeEmbedding(vec)
	decoded, err := DecodeEmbedding(encoded)
	if err != nil {
		t.Fatalf("DecodeEmbedding error: %v", err)
	}

	for i := range vec {
		if math.Float32bits(decoded[i]) != math.Float32bits(vec[i]) {
			t.Errorf("index %d: bits got 0x%08X, want 0x%08X", i, math.Float32bits(decoded[i]), math.Float32bits(vec[i]))
		}
	}
}

func makeTestVec(dim, seed int) []float32 {
	vec := make([]float32, dim)
	for i := range dim {
		vec[i] = float32(float64(i+seed) * 0.001)
	}
	return vec
}

func isErrorType(err error, target any) bool {
	switch target.(type) {
	case *ErrInvalidBlob:
		_, ok := err.(ErrInvalidBlob)
		return ok
	}
	return false
}
