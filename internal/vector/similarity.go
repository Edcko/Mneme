package vector

import (
	"math"
	"slices"
	"sort"
)

func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("vector: dimension mismatch: len(a)=" + itoa(len(a)) + " len(b)=" + itoa(len(b)))
	}
	if len(a) == 0 {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

type scored struct {
	index int
	score float32
}

func TopK(embeddings [][]float32, query []float32, k int) ([]int, []float32) {
	if k <= 0 || len(embeddings) == 0 || len(query) == 0 {
		return nil, nil
	}

	candidates := make([]scored, 0, len(embeddings))
	for i, emb := range embeddings {
		s := CosineSimilarity(query, emb)
		candidates = append(candidates, scored{index: i, score: s})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index < candidates[j].index
	})

	if k > len(candidates) {
		k = len(candidates)
	}

	top := candidates[:k]
	indices := make([]int, k)
	scores := make([]float32, k)
	for i, c := range top {
		indices[i] = c.index
		scores[i] = c.score
	}

	return indices, scores
}

func itoa(n int) string {
	return formatInt(n)
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	slices.Reverse(buf)
	return string(buf)
}
