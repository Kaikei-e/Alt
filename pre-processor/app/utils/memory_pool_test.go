package utils

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"pre-processor/models"
)

func TestObjectPool(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create and use object pool",
			test: func(t *testing.T) {
				pool := NewObjectPool(
					func() *models.Article {
						return &models.Article{}
					},
					func(a *models.Article) {
						*a = models.Article{}
					},
				)
				
				assert.NotNil(t, pool)
				
				// Get object from pool
				article := pool.Get()
				assert.NotNil(t, article)
				
				// Use the object
				article.Title = "Test Article"
				article.URL = "https://example.com"
				
				// Return to pool (should be reset)
				pool.Put(article)
				
				// Get another object (might be the same instance, but reset)
				article2 := pool.Get()
				assert.NotNil(t, article2)
				assert.Empty(t, article2.Title) // Should be reset
				assert.Empty(t, article2.URL)   // Should be reset
				
				pool.Put(article2)
			},
		},
		{
			name: "should handle concurrent access safely",
			test: func(t *testing.T) {
				pool := NewObjectPool(
					func() *models.Article {
						return &models.Article{}
					},
					func(a *models.Article) {
						*a = models.Article{}
					},
				)
				
				const numGoroutines = 100
				const opsPerGoroutine = 10
				
				var wg sync.WaitGroup
				wg.Add(numGoroutines)
				
				for i := 0; i < numGoroutines; i++ {
					go func(id int) {
						defer wg.Done()
						
						for j := 0; j < opsPerGoroutine; j++ {
							article := pool.Get()
							article.Title = "Test"
							pool.Put(article)
						}
					}(i)
				}
				
				wg.Wait()
				
				// Pool should still be functional
				article := pool.Get()
				assert.NotNil(t, article)
				assert.Empty(t, article.Title) // Should be reset
			},
		},
		{
			name: "should track pool metrics",
			test: func(t *testing.T) {
				pool := NewObjectPool(
					func() *models.Article {
						return &models.Article{}
					},
					func(a *models.Article) {
						*a = models.Article{}
					},
				)
				
				// Initial metrics should be zero
				metrics := pool.GetMetrics()
				assert.Equal(t, int64(0), metrics.Gets)
				assert.Equal(t, int64(0), metrics.Puts)
				
				// Get and put should increment metrics
				article := pool.Get()
				metrics = pool.GetMetrics()
				assert.Equal(t, int64(1), metrics.Gets)
				assert.Equal(t, int64(0), metrics.Puts)
				
				pool.Put(article)
				metrics = pool.GetMetrics()
				assert.Equal(t, int64(1), metrics.Gets)
				assert.Equal(t, int64(1), metrics.Puts)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestBufferPool(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create and use buffer pool",
			test: func(t *testing.T) {
				pool := NewBufferPool()
				assert.NotNil(t, pool)
				
				// Get buffer from pool
				buffer := pool.Get()
				assert.NotNil(t, buffer)
				assert.Equal(t, 0, buffer.Len())
				
				// Use the buffer
				buffer.WriteString("Hello, World!")
				assert.Equal(t, 13, buffer.Len())
				
				// Return to pool (should be reset)
				pool.Put(buffer)
				
				// Get another buffer (might be same instance, but reset)
				buffer2 := pool.Get()
				assert.NotNil(t, buffer2)
				assert.Equal(t, 0, buffer2.Len()) // Should be reset
				
				pool.Put(buffer2)
			},
		},
		{
			name: "should handle concurrent buffer operations",
			test: func(t *testing.T) {
				pool := NewBufferPool()
				
				const numGoroutines = 50
				const opsPerGoroutine = 20
				
				var wg sync.WaitGroup
				wg.Add(numGoroutines)
				
				for i := 0; i < numGoroutines; i++ {
					go func(id int) {
						defer wg.Done()
						
						for j := 0; j < opsPerGoroutine; j++ {
							buffer := pool.Get()
							buffer.WriteString("test data")
							pool.Put(buffer)
						}
					}(i)
				}
				
				wg.Wait()
				
				// Pool should still be functional
				buffer := pool.Get()
				assert.NotNil(t, buffer)
				assert.Equal(t, 0, buffer.Len()) // Should be reset
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestStringBuilderPool(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create and use string builder pool",
			test: func(t *testing.T) {
				pool := NewStringBuilderPool()
				assert.NotNil(t, pool)
				
				// Get string builder from pool
				sb := pool.Get()
				assert.NotNil(t, sb)
				assert.Equal(t, 0, sb.Len())
				
				// Use the string builder
				sb.WriteString("Hello")
				sb.WriteString(", ")
				sb.WriteString("World!")
				
				result := sb.String()
				assert.Equal(t, "Hello, World!", result)
				assert.Equal(t, 13, sb.Len())
				
				// Return to pool (should be reset)
				pool.Put(sb)
				
				// Get another string builder (might be same instance, but reset)
				sb2 := pool.Get()
				assert.NotNil(t, sb2)
				assert.Equal(t, 0, sb2.Len()) // Should be reset
				assert.Equal(t, "", sb2.String()) // Should be reset
				
				pool.Put(sb2)
			},
		},
		{
			name: "should handle concurrent string builder operations",
			test: func(t *testing.T) {
				pool := NewStringBuilderPool()
				
				const numGoroutines = 50
				const opsPerGoroutine = 20
				
				var wg sync.WaitGroup
				wg.Add(numGoroutines)
				
				for i := 0; i < numGoroutines; i++ {
					go func(id int) {
						defer wg.Done()
						
						for j := 0; j < opsPerGoroutine; j++ {
							sb := pool.Get()
							sb.WriteString("test string")
							pool.Put(sb)
						}
					}(i)
				}
				
				wg.Wait()
				
				// Pool should still be functional
				sb := pool.Get()
				assert.NotNil(t, sb)
				assert.Equal(t, 0, sb.Len()) // Should be reset
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestSlicePool(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create and use slice pool",
			test: func(t *testing.T) {
				pool := NewSlicePool()
				assert.NotNil(t, pool)
				
				// Get slice from pool
				slice := pool.GetSlice(1024)
				assert.NotNil(t, slice)
				assert.Equal(t, 1024, len(slice))
				assert.GreaterOrEqual(t, cap(slice), 1024)
				
				// Use the slice
				copy(slice, []byte("Hello, World!"))
				
				// Return to pool
				pool.PutSlice(slice)
				
				// Get another slice (might be larger due to power-of-2 rounding)
				slice2 := pool.GetSlice(500)
				assert.NotNil(t, slice2)
				assert.Equal(t, 500, len(slice2))
				
				pool.PutSlice(slice2)
			},
		},
		{
			name: "should round up to power of 2",
			test: func(t *testing.T) {
				pool := NewSlicePool()
				
				// Test various sizes
				testSizes := []struct {
					requested int
					expected  int
				}{
					{100, 128},   // Rounds up to 128
					{128, 128},   // Exact power of 2
					{300, 512},   // Rounds up to 512
					{1000, 1024}, // Rounds up to 1024
					{2048, 2048}, // Exact power of 2
				}
				
				for _, test := range testSizes {
					slice := pool.GetSlice(test.requested)
					assert.Equal(t, test.requested, len(slice))
					assert.GreaterOrEqual(t, cap(slice), test.expected)
					pool.PutSlice(slice)
				}
			},
		},
		{
			name: "should handle concurrent slice operations",
			test: func(t *testing.T) {
				pool := NewSlicePool()
				
				const numGoroutines = 50
				const opsPerGoroutine = 10
				
				var wg sync.WaitGroup
				wg.Add(numGoroutines)
				
				for i := 0; i < numGoroutines; i++ {
					go func(id int) {
						defer wg.Done()
						
						for j := 0; j < opsPerGoroutine; j++ {
							size := 1024 + (id*opsPerGoroutine+j)*10 // Varying sizes
							slice := pool.GetSlice(size)
							assert.Equal(t, size, len(slice))
							pool.PutSlice(slice)
						}
					}(i)
				}
				
				wg.Wait()
				
				// Pool should still be functional
				slice := pool.GetSlice(1024)
				assert.NotNil(t, slice)
				assert.Equal(t, 1024, len(slice))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestMemoryOptimizer(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create memory optimizer with batch sizes",
			test: func(t *testing.T) {
				optimizer := NewMemoryOptimizer(100, 50)
				assert.NotNil(t, optimizer)
				assert.Equal(t, 100, optimizer.articleBatchSize)
				assert.Equal(t, 50, optimizer.summaryBatchSize)
			},
		},
		{
			name: "should preallocate article batch with correct capacity",
			test: func(t *testing.T) {
				optimizer := NewMemoryOptimizer(100, 50)
				
				articles := optimizer.PreallocateArticleBatch()
				assert.NotNil(t, articles)
				assert.Equal(t, 0, len(articles))
				assert.Equal(t, 100, cap(articles))
			},
		},
		{
			name: "should preallocate summary batch with correct capacity",
			test: func(t *testing.T) {
				optimizer := NewMemoryOptimizer(100, 50)
				
				summaries := optimizer.PreallocateSummaryBatch()
				assert.NotNil(t, summaries)
				assert.Equal(t, 0, len(summaries))
				assert.Equal(t, 50, cap(summaries))
			},
		},
		{
			name: "should preallocate without reallocations during append",
			test: func(t *testing.T) {
				optimizer := NewMemoryOptimizer(5, 3)
				
				articles := optimizer.PreallocateArticleBatch()
				
				// Append up to capacity without reallocations
				for i := 0; i < 5; i++ {
					articles = append(articles, models.Article{
						Title: "Article " + string(rune(i+'0')),
					})
				}
				
				assert.Equal(t, 5, len(articles))
				assert.Equal(t, 5, cap(articles)) // Should not have grown
				
				// Verify content
				assert.Equal(t, "Article 0", articles[0].Title)
				assert.Equal(t, "Article 4", articles[4].Title)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func BenchmarkMemoryPools(b *testing.B) {
	b.Run("ObjectPool_ArticleGet", func(b *testing.B) {
		pool := NewObjectPool(
			func() *models.Article {
				return &models.Article{}
			},
			func(a *models.Article) {
				*a = models.Article{}
			},
		)
		
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				article := pool.Get()
				pool.Put(article)
			}
		})
	})
	
	b.Run("BufferPool_Operations", func(b *testing.B) {
		pool := NewBufferPool()
		
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				buffer := pool.Get()
				buffer.WriteString("benchmark test")
				pool.Put(buffer)
			}
		})
	})
	
	b.Run("StringBuilderPool_Operations", func(b *testing.B) {
		pool := NewStringBuilderPool()
		
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				sb := pool.Get()
				sb.WriteString("benchmark test")
				pool.Put(sb)
			}
		})
	})
	
	b.Run("SlicePool_Operations", func(b *testing.B) {
		pool := NewSlicePool()
		
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				slice := pool.GetSlice(1024)
				pool.PutSlice(slice)
			}
		})
	})
	
	b.Run("DirectAllocation_Comparison", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				article := &models.Article{}
				_ = article // Use the variable
			}
		})
	})
}