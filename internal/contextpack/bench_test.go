package contextpack

import "testing"

func BenchmarkPack(b *testing.B) {
	candidates := fakeMatches(20, 3, 40)
	for i := 0; i < b.N; i++ {
		if _, err := pack(baseFor(maxTokens), candidates, nil, maxTokens); err != nil {
			b.Fatal(err)
		}
	}
}
