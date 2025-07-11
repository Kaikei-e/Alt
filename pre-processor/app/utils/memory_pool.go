package utils

import (
	"bytes"
	"strings"
	"sync"
	"sync/atomic"

	"pre-processor/models"
)

// PoolMetrics tracks pool usage statistics
type PoolMetrics struct {
	Gets int64 `json:"gets"`
	Puts int64 `json:"puts"`
}

// ObjectPool provides a generic object pool implementation
type ObjectPool[T any] struct {
	pool    sync.Pool
	reset   func(*T)
	metrics *PoolMetrics
}

// NewObjectPool creates a new generic object pool
func NewObjectPool[T any](new func() *T, reset func(*T)) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() interface{} {
				return new()
			},
		},
		reset:   reset,
		metrics: &PoolMetrics{},
	}
}

// Get retrieves an object from the pool
func (p *ObjectPool[T]) Get() *T {
	atomic.AddInt64(&p.metrics.Gets, 1)
	return p.pool.Get().(*T)
}

// Put returns an object to the pool after resetting it
func (p *ObjectPool[T]) Put(obj *T) {
	if p.reset != nil {
		p.reset(obj)
	}
	atomic.AddInt64(&p.metrics.Puts, 1)
	p.pool.Put(obj)
}

// GetMetrics returns current pool usage metrics
func (p *ObjectPool[T]) GetMetrics() PoolMetrics {
	return PoolMetrics{
		Gets: atomic.LoadInt64(&p.metrics.Gets),
		Puts: atomic.LoadInt64(&p.metrics.Puts),
	}
}

// BufferPool manages a pool of byte buffers
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a new buffer pool
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 4096))
			},
		},
	}
}

// Get retrieves a buffer from the pool
func (p *BufferPool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}

// Put returns a buffer to the pool after resetting it
func (p *BufferPool) Put(b *bytes.Buffer) {
	b.Reset()
	p.pool.Put(b)
}

// StringBuilderPool manages a pool of string builders
type StringBuilderPool struct {
	pool sync.Pool
}

// NewStringBuilderPool creates a new string builder pool
func NewStringBuilderPool() *StringBuilderPool {
	return &StringBuilderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &strings.Builder{}
			},
		},
	}
}

// Get retrieves a string builder from the pool
func (p *StringBuilderPool) Get() *strings.Builder {
	return p.pool.Get().(*strings.Builder)
}

// Put returns a string builder to the pool after resetting it
func (p *StringBuilderPool) Put(sb *strings.Builder) {
	sb.Reset()
	p.pool.Put(sb)
}

// SlicePool manages pools of byte slices with different sizes
type SlicePool struct {
	pools map[int]*sync.Pool
	mu    sync.RWMutex
}

// NewSlicePool creates a new slice pool
func NewSlicePool() *SlicePool {
	return &SlicePool{
		pools: make(map[int]*sync.Pool),
	}
}

// GetSlice retrieves a byte slice with at least the specified size
func (s *SlicePool) GetSlice(size int) []byte {
	// Round up to the nearest power of 2
	poolSize := nextPowerOfTwo(size)

	s.mu.RLock()
	pool, exists := s.pools[poolSize]
	s.mu.RUnlock()

	if !exists {
		s.mu.Lock()
		// Double-check after acquiring write lock
		if pool, exists = s.pools[poolSize]; !exists {
			pool = &sync.Pool{
				New: func() interface{} {
					return make([]byte, poolSize)
				},
			}
			s.pools[poolSize] = pool
		}
		s.mu.Unlock()
	}

	slice := pool.Get().([]byte)
	return slice[:size] // Return slice with requested length
}

// PutSlice returns a byte slice to the appropriate pool
func (s *SlicePool) PutSlice(slice []byte) {
	if slice == nil || cap(slice) == 0 {
		return
	}

	poolSize := cap(slice)

	s.mu.RLock()
	pool, exists := s.pools[poolSize]
	s.mu.RUnlock()

	if exists {
		// Reset slice to full capacity before returning to pool
		slice = slice[:cap(slice)]
		pool.Put(slice)
	}
	// If pool doesn't exist, just let the slice be garbage collected
}

// MemoryOptimizer provides memory optimization utilities
type MemoryOptimizer struct {
	articleBatchSize int
	summaryBatchSize int
}

// NewMemoryOptimizer creates a new memory optimizer
func NewMemoryOptimizer(articleBatchSize, summaryBatchSize int) *MemoryOptimizer {
	return &MemoryOptimizer{
		articleBatchSize: articleBatchSize,
		summaryBatchSize: summaryBatchSize,
	}
}

// PreallocateArticleBatch creates a pre-allocated slice for articles
func (m *MemoryOptimizer) PreallocateArticleBatch() []models.Article {
	return make([]models.Article, 0, m.articleBatchSize)
}

// PreallocateSummaryBatch creates a pre-allocated slice for summaries
func (m *MemoryOptimizer) PreallocateSummaryBatch() []models.ArticleSummary {
	return make([]models.ArticleSummary, 0, m.summaryBatchSize)
}

// nextPowerOfTwo returns the next power of 2 greater than or equal to n
func nextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}

	// If n is already a power of 2, return it
	if n&(n-1) == 0 {
		return n
	}

	// Find the next power of 2
	power := 1
	for power < n {
		power <<= 1
	}
	return power
}

// Global pool instances for common use cases
var (
	globalArticlePool       *ObjectPool[models.Article]
	globalBufferPool        *BufferPool
	globalStringBuilderPool *StringBuilderPool
	globalSlicePool         *SlicePool

	poolInitOnce sync.Once
)

// InitGlobalPools initializes global pool instances
func InitGlobalPools() {
	poolInitOnce.Do(func() {
		globalArticlePool = NewObjectPool(
			func() *models.Article {
				return &models.Article{}
			},
			func(a *models.Article) {
				*a = models.Article{} // Reset all fields
			},
		)

		globalBufferPool = NewBufferPool()
		globalStringBuilderPool = NewStringBuilderPool()
		globalSlicePool = NewSlicePool()
	})
}

// GetGlobalArticlePool returns the global article pool
func GetGlobalArticlePool() *ObjectPool[models.Article] {
	InitGlobalPools()
	return globalArticlePool
}

// GetGlobalBufferPool returns the global buffer pool
func GetGlobalBufferPool() *BufferPool {
	InitGlobalPools()
	return globalBufferPool
}

// GetGlobalStringBuilderPool returns the global string builder pool
func GetGlobalStringBuilderPool() *StringBuilderPool {
	InitGlobalPools()
	return globalStringBuilderPool
}

// GetGlobalSlicePool returns the global slice pool
func GetGlobalSlicePool() *SlicePool {
	InitGlobalPools()
	return globalSlicePool
}

// Helper functions for easy access to global pools

// GetArticle gets an article from the global pool
func GetArticle() *models.Article {
	return GetGlobalArticlePool().Get()
}

// PutArticle returns an article to the global pool
func PutArticle(article *models.Article) {
	GetGlobalArticlePool().Put(article)
}

// GetBuffer gets a buffer from the global pool
func GetBuffer() *bytes.Buffer {
	return GetGlobalBufferPool().Get()
}

// PutBuffer returns a buffer to the global pool
func PutBuffer(buffer *bytes.Buffer) {
	GetGlobalBufferPool().Put(buffer)
}

// GetStringBuilder gets a string builder from the global pool
func GetStringBuilder() *strings.Builder {
	return GetGlobalStringBuilderPool().Get()
}

// PutStringBuilder returns a string builder to the global pool
func PutStringBuilder(sb *strings.Builder) {
	GetGlobalStringBuilderPool().Put(sb)
}

// GetSlice gets a byte slice from the global pool
func GetSlice(size int) []byte {
	return GetGlobalSlicePool().GetSlice(size)
}

// PutSlice returns a byte slice to the global pool
func PutSlice(slice []byte) {
	GetGlobalSlicePool().PutSlice(slice)
}