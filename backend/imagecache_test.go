package backend

import (
	"context"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"
)

// createTestImage creates a simple test image
func createTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 255, 255})
		}
	}
	return img
}

func TestImageCache_SetAndGet(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	img := createTestImage(10, 10)

	// Test Set and Get
	cache.Set("test1", img)
	retrieved, err := cache.Get("test1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if retrieved != img {
		t.Error("Retrieved image does not match stored image")
	}

	// Test Get on non-existent key
	_, err = cache.Get("nonexistent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestImageCache_Has(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	img := createTestImage(10, 10)

	if cache.Has("test1") {
		t.Error("Expected Has to return false for non-existent key")
	}

	cache.Set("test1", img)
	if !cache.Has("test1") {
		t.Error("Expected Has to return true for existing key")
	}
}

func TestImageCache_SetWithTTL(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	img := createTestImage(10, 10)

	// Set with short TTL
	cache.SetWithTTL("test1", img, 50*time.Millisecond)

	// Should exist immediately
	if !cache.Has("test1") {
		t.Error("Expected item to exist immediately after setting")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Item should still be retrievable (expired but not yet evicted)
	retrieved, err := cache.Get("test1")
	if err != nil {
		t.Errorf("Expected item to still be retrievable, got error: %v", err)
	}
	if retrieved != img {
		t.Error("Retrieved image does not match")
	}
}

func TestImageCache_Eviction(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    5,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	// Fill cache to max
	for i := 0; i < 5; i++ {
		img := createTestImage(10, 10)
		cache.Set(string(rune('a'+i)), img)
	}

	// All 5 should exist
	for i := 0; i < 5; i++ {
		if !cache.Has(string(rune('a' + i))) {
			t.Errorf("Expected key %c to exist", 'a'+i)
		}
	}

	// Add one more - should evict LRU
	cache.Set("f", createTestImage(10, 10))

	// Should still have 5 items (one was evicted)
	cache.mu.RLock()
	count := len(cache.cache)
	cache.mu.RUnlock()
	if count != 5 {
		t.Errorf("Expected 5 items, got %d", count)
	}
}

func TestImageCache_EvictExpired(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Second,
	}
	// Initialize cache with mutex but don't start periodic eviction
	cache.cache = make(map[string]CacheItem)
	cache.mu = sync.RWMutex{}

	// Add items with short TTL (1 second for Unix timestamp precision)
	for i := 0; i < 5; i++ {
		img := createTestImage(10, 10)
		cache.SetWithTTL(string(rune('a'+i)), img, time.Second)
	}

	// Wait for expiration (2 seconds to ensure all items expire)
	time.Sleep(2 * time.Second)

	// Trigger manual eviction
	cache.EvictExpired()

	// Should have at most MinSize items remaining
	cache.mu.RLock()
	count := len(cache.cache)
	cache.mu.RUnlock()

	if count > cache.MinSize {
		t.Errorf("Expected at most %d items after eviction, got %d", cache.MinSize, count)
	}
}

func TestImageCache_GetResetTTL(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	img := createTestImage(10, 10)
	ttl := 10 * time.Second
	cache.SetWithTTL("test1", img, ttl)

	// Get original expiry
	cache.mu.RLock()
	origExpiry := cache.cache["test1"].expiresAt
	cache.mu.RUnlock()

	// Wait long enough to make a measurable difference (Unix timestamps are in seconds)
	time.Sleep(2 * time.Second)

	// Get with reset TTL - should reset expiry to now + original TTL
	_, err := cache.GetResetTTL("test1", true)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check expiry was extended
	cache.mu.RLock()
	newExpiry := cache.cache["test1"].expiresAt
	cache.mu.RUnlock()

	// New expiry should be at least 1 second later (we slept 2s, reset adds 10s back)
	// So new expiry should be roughly origExpiry + 2 seconds
	diff := newExpiry - origExpiry
	if diff < 1 {
		t.Errorf("Expected expiry to be extended by at least 1 second: orig=%d, new=%d, diff=%d",
			origExpiry, newExpiry, diff)
	}
}

func TestImageCache_GetExtendTTL(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	img := createTestImage(10, 10)
	cache.SetWithTTL("test1", img, 100*time.Millisecond)

	// Get original expiry
	cache.mu.RLock()
	origExpiry := cache.cache["test1"].expiresAt
	cache.mu.RUnlock()

	// Extend with longer TTL
	_, err := cache.GetExtendTTL("test1", time.Hour)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check expiry was extended
	cache.mu.RLock()
	newExpiry := cache.cache["test1"].expiresAt
	cache.mu.RUnlock()

	if newExpiry <= origExpiry {
		t.Error("Expected expiry to be extended")
	}

	// Try to extend with shorter TTL (should not change)
	origExpiry = newExpiry
	_, err = cache.GetExtendTTL("test1", 50*time.Millisecond)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	cache.mu.RLock()
	newExpiry = cache.cache["test1"].expiresAt
	cache.mu.RUnlock()

	// Should not have shortened
	if newExpiry < origExpiry {
		t.Error("Expected expiry not to be shortened")
	}
}

func TestImageCache_GetWithNewTTL(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	img := createTestImage(10, 10)
	cache.SetWithTTL("test1", img, time.Hour)

	// Change TTL
	newTTL := 200 * time.Millisecond
	_, err := cache.GetWithNewTTL("test1", newTTL)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify TTL was changed
	cache.mu.RLock()
	actualTTL := cache.cache["test1"].ttl
	cache.mu.RUnlock()

	if actualTTL != newTTL {
		t.Errorf("Expected TTL to be %v, got %v", newTTL, actualTTL)
	}
}

func TestImageCache_Clear(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	// Add items
	for i := 0; i < 5; i++ {
		img := createTestImage(10, 10)
		cache.Set(string(rune('a'+i)), img)
	}

	cache.Clear()

	cache.mu.RLock()
	count := len(cache.cache)
	cache.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 items after clear, got %d", count)
	}
}

func TestImageCache_ConcurrentAccess(t *testing.T) {
	cache := &ImageCache{
		MinSize:    5,
		MaxSize:    20,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 50

	// Concurrent writes
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				img := createTestImage(5, 5)
				key := string(rune('a' + (id*iterations+j)%26))
				cache.Set(key, img)
			}
		}(i)
	}

	// Concurrent reads
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := string(rune('a' + (id*iterations+j)%26))
				_, _ = cache.Get(key)
			}
		}(i)
	}

	// Concurrent Has checks
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := string(rune('a' + (id*iterations+j)%26))
				_ = cache.Has(key)
			}
		}(i)
	}

	wg.Wait()

	// If we got here without data races, test passes
	// Run with: go test -race
}

func TestImageCache_UpdateExisting(t *testing.T) {
	cache := &ImageCache{
		MinSize:    2,
		MaxSize:    10,
		DefaultTTL: time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.Init(ctx, time.Minute)

	img1 := createTestImage(10, 10)
	img2 := createTestImage(20, 20)

	// Set initial
	cache.Set("test1", img1)
	retrieved, _ := cache.Get("test1")
	if retrieved != img1 {
		t.Error("Expected first image")
	}

	// Update with new image
	cache.Set("test1", img2)
	retrieved, _ = cache.Get("test1")
	if retrieved != img2 {
		t.Error("Expected second image after update")
	}

	// Should still have only 1 item
	cache.mu.RLock()
	count := len(cache.cache)
	cache.mu.RUnlock()
	if count != 1 {
		t.Errorf("Expected 1 item, got %d", count)
	}
}
