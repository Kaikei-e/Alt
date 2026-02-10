package domain_test

import (
	"strings"
	"testing"

	"rag-orchestrator/internal/domain"
)

func BenchmarkChunker_Short(b *testing.B) {
	chunker := domain.NewChunker()
	text := "This is a short article about AI. It has a few sentences. Machine learning is powerful."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.Chunk(text)
	}
}

func BenchmarkChunker_Medium(b *testing.B) {
	chunker := domain.NewChunker()
	// ~1000 words
	text := strings.Repeat("This is a paragraph about artificial intelligence and machine learning. It discusses various applications of AI in modern technology. ", 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.Chunk(text)
	}
}

func BenchmarkChunker_Long(b *testing.B) {
	chunker := domain.NewChunker()
	// ~5000 words
	text := strings.Repeat("This is a paragraph about artificial intelligence and machine learning. It discusses various applications of AI in modern technology, including natural language processing and computer vision. ", 200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.Chunk(text)
	}
}

func BenchmarkChunker_Japanese(b *testing.B) {
	chunker := domain.NewChunker()
	text := strings.Repeat("人工知能の研究開発が加速しています。機械学習やディープラーニングの技術は、自然言語処理、画像認識、音声認識などの分野で大きな進歩を遂げています。", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.Chunk(text)
	}
}
