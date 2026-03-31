package vector

import (
	"math"
	"testing"
)

func TestCosineSimilarity_IdenticalVectors(t *testing.T) {
	vec := []float32{1.0, 2.0, 3.0, 4.0, 5.0}
	got := CosineSimilarity(vec, vec)

	if got != 1.0 {
		t.Errorf("identical vectors: got %v, want 1.0", got)
	}
}

func TestCosineSimilarity_OrthogonalVectors(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	got := CosineSimilarity(a, b)

	if got != 0.0 {
		t.Errorf("orthogonal vectors: got %v, want 0.0", got)
	}
}

func TestCosineSimilarity_OppositeVectors(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{-1.0, -2.0, -3.0}
	got := CosineSimilarity(a, b)

	if got != -1.0 {
		t.Errorf("opposite vectors: got %v, want -1.0", got)
	}
}

func TestCosineSimilarity_DimensionMismatch(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for dimension mismatch, got none")
		}
	}()

	CosineSimilarity([]float32{1.0, 2.0}, []float32{1.0, 2.0, 3.0})
}

func TestCosineSimilarity_ZeroNormVectors(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
	}{
		{name: "both zero", a: []float32{0, 0, 0}, b: []float32{0, 0, 0}},
		{name: "a zero, b nonzero", a: []float32{0, 0, 0}, b: []float32{1, 2, 3}},
		{name: "a nonzero, b zero", a: []float32{1, 2, 3}, b: []float32{0, 0, 0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CosineSimilarity(tc.a, tc.b)
			if got != 0.0 {
				t.Errorf("zero-norm vectors: got %v, want 0.0", got)
			}
		})
	}
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	got := CosineSimilarity([]float32{}, []float32{})
	if got != 0.0 {
		t.Errorf("empty vectors: got %v, want 0.0", got)
	}
}

func TestCosineSimilarity_ScaledVectors(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{10.0, 20.0, 30.0}

	got := CosineSimilarity(a, b)
	if got != 1.0 {
		t.Errorf("scaled vectors: got %v, want 1.0", got)
	}
}

func TestCosineSimilarity_SmallValues(t *testing.T) {
	a := []float32{1e-7, 2e-7, 3e-7}
	b := []float32{1e-7, 2e-7, 3e-7}

	got := CosineSimilarity(a, b)
	if math.Abs(float64(got)-1.0) > 1e-6 {
		t.Errorf("small identical values: got %v, want ~1.0", got)
	}
}

func TestCosineSimilarity_LargeValues(t *testing.T) {
	a := []float32{1e7, 2e7, 3e7}
	b := []float32{1e7, 2e7, 3e7}

	got := CosineSimilarity(a, b)
	if math.Abs(float64(got)-1.0) > 1e-3 {
		t.Errorf("large identical values: got %v, want ~1.0", got)
	}
}

func TestCosineSimilarity_NaN(t *testing.T) {
	a := []float32{float32(math.NaN()), 1.0, 2.0}
	b := []float32{1.0, 2.0, 3.0}

	got := CosineSimilarity(a, b)
	if !math.IsNaN(float64(got)) {
		t.Errorf("NaN input: got %v, want NaN", got)
	}
}

func TestTopK_BasicOrdering(t *testing.T) {
	embeddings := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.7071068, 0.7071068, 0.0},
		{1.0, 0.0, 0.0},
	}
	query := []float32{1.0, 0.0, 0.0}

	indices, scores := TopK(embeddings, query, 2)

	if len(indices) != 2 {
		t.Fatalf("expected 2 results, got %d", len(indices))
	}

	if indices[0] != 0 && indices[0] != 3 {
		t.Errorf("expected first result to be index 0 or 3 (identical to query), got %d", indices[0])
	}

	if scores[0] < scores[1] {
		t.Errorf("scores not in descending order: %v", scores)
	}
}

func TestTopK_KGreaterThanLen(t *testing.T) {
	embeddings := [][]float32{
		{1.0, 0.0},
		{0.0, 1.0},
	}
	query := []float32{1.0, 0.0}

	indices, scores := TopK(embeddings, query, 10)

	if len(indices) != 2 {
		t.Errorf("expected 2 results (clamped to len), got %d", len(indices))
	}
	if len(scores) != 2 {
		t.Errorf("expected 2 scores, got %d", len(scores))
	}
}

func TestTopK_KZero(t *testing.T) {
	embeddings := [][]float32{{1.0, 0.0}, {0.0, 1.0}}
	query := []float32{1.0, 0.0}

	indices, scores := TopK(embeddings, query, 0)

	if indices != nil {
		t.Errorf("expected nil indices for k=0, got %v", indices)
	}
	if scores != nil {
		t.Errorf("expected nil scores for k=0, got %v", scores)
	}
}

func TestTopK_EmptyEmbeddings(t *testing.T) {
	query := []float32{1.0, 0.0}

	indices, scores := TopK(nil, query, 3)

	if indices != nil {
		t.Errorf("expected nil for empty embeddings, got %v", indices)
	}
	if scores != nil {
		t.Errorf("expected nil for empty embeddings, got %v", scores)
	}
}

func TestTopK_EmptyQuery(t *testing.T) {
	embeddings := [][]float32{{1.0, 0.0}, {0.0, 1.0}}

	indices, scores := TopK(embeddings, []float32{}, 3)

	if indices != nil {
		t.Errorf("expected nil for empty query, got %v", indices)
	}
	if scores != nil {
		t.Errorf("expected nil for empty query, got %v", scores)
	}
}

func TestTopK_SingleEmbedding(t *testing.T) {
	embeddings := [][]float32{{1.0, 0.0}}
	query := []float32{1.0, 0.0}

	indices, scores := TopK(embeddings, query, 1)

	if len(indices) != 1 || indices[0] != 0 {
		t.Errorf("expected [0], got %v", indices)
	}
	if len(scores) != 1 || scores[0] != 1.0 {
		t.Errorf("expected [1.0], got %v", scores)
	}
}

func TestTopK_TieBreaking(t *testing.T) {
	embeddings := [][]float32{
		{1.0, 0.0},
		{1.0, 0.0},
		{0.0, 1.0},
	}
	query := []float32{1.0, 0.0}

	indices, _ := TopK(embeddings, query, 2)

	if indices[0] != 0 || indices[1] != 1 {
		t.Errorf("ties should be broken by index ascending, got %v", indices)
	}
}

func TestTopK_NegativeK(t *testing.T) {
	embeddings := [][]float32{{1.0, 0.0}}
	query := []float32{1.0, 0.0}

	indices, scores := TopK(embeddings, query, -1)

	if indices != nil {
		t.Errorf("expected nil for negative k, got %v", indices)
	}
	if scores != nil {
		t.Errorf("expected nil for negative k, got %v", scores)
	}
}

func TestTopK_DifferentDimensions(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for dimension mismatch in TopK")
		}
	}()

	embeddings := [][]float32{{1.0, 0.0, 0.0}}
	query := []float32{1.0, 0.0}

	TopK(embeddings, query, 1)
}

func TestCosineSimilarity_RandomVectorsBounded(t *testing.T) {
	a := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	b := []float32{0.5, 0.4, 0.3, 0.2, 0.1}

	got := CosineSimilarity(a, b)

	if got <= -1.0 || got >= 1.0 {
		t.Errorf("cosine similarity out of [-1,1]: got %v", got)
	}
}

func TestCosineSimilarity_HighDimensionalIdentity(t *testing.T) {
	vec := makeTestVec(1536, 0)
	got := CosineSimilarity(vec, vec)

	if got != 1.0 {
		t.Errorf("high-dim identical: got %v, want 1.0", got)
	}
}

func TestTopK_FullPipeline(t *testing.T) {
	query := []float32{1.0, 0.0, 0.0}

	embeddings := [][]float32{
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
		{0.577, 0.577, 0.577},
		{0.707, 0.707, 0.0},
		{1.0, 0.0, 0.0},
	}

	indices, scores := TopK(embeddings, query, 3)

	if len(indices) != 3 {
		t.Fatalf("expected 3 results, got %d", len(indices))
	}

	if scores[0] < scores[1] || scores[1] < scores[2] {
		t.Errorf("scores not in descending order: %v", scores)
	}

	if indices[0] != 4 {
		t.Errorf("expected exact match at index 0 of results, got index %d (score=%v)", indices[0], scores[0])
	}
}
