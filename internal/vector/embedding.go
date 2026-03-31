package vector

import (
	"encoding/binary"
	"math"
)

const (
	uint32Size  = 4
	float32Size = 4
)

func EncodeEmbedding(embedding []float32) []byte {
	if len(embedding) == 0 {
		return []byte{}
	}

	n := uint32(len(embedding))
	buf := make([]byte, uint32Size+n*float32Size)
	binary.LittleEndian.PutUint32(buf[0:], n)

	off := uint32Size
	for _, v := range embedding {
		binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(v))
		off += float32Size
	}

	return buf
}

func DecodeEmbedding(data []byte) ([]float32, error) {
	if len(data) == 0 {
		return []float32{}, nil
	}

	if len(data) < uint32Size {
		return nil, ErrInvalidBlob{Reason: "blob too short to contain length prefix"}
	}

	n := int(binary.LittleEndian.Uint32(data[0:]))
	expectedSize := uint32Size + n*float32Size
	if len(data) < expectedSize {
		return nil, ErrInvalidBlob{
			Reason:   "blob size does not match declared vector length",
			Declared: n,
			Actual:   len(data),
		}
	}

	vec := make([]float32, n)
	off := uint32Size
	for i := range n {
		bits := binary.LittleEndian.Uint32(data[off:])
		vec[i] = math.Float32frombits(bits)
		off += float32Size
	}

	return vec, nil
}
