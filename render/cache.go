package render

import (
	"image"
	"sort"
)

// Upload owns its source until a renderer commits it or a device reset. Image
// resources and glyph masks must be immutable for that interval.
type Upload struct {
	Key        TextureKey
	Image      image.Image
	Generation uint64
}

type resident struct {
	source image.Image
	handle uint32
	bytes  int64
	used   uint64
	serial uint64
	queued bool
}

type retired struct {
	handle uint32
	serial uint64
}

// TextureCache tracks device resources separately from immutable CPU sources.
// All methods run on the render thread. Frame serials must increase; callers
// release retired handles only after the matching GPU fence has completed.
type TextureCache struct {
	budget, used int64
	generation   uint64
	tick         uint64
	entries      map[TextureKey]*resident
	queue        []TextureKey
	retired      []retired
	current      uint64
}

func NewTextureCache(budgetBytes int64) *TextureCache {
	if budgetBytes < 0 {
		budgetBytes = 0
	}
	return &TextureCache{budget: budgetBytes, generation: 1, entries: make(map[TextureKey]*resident)}
}

func (cache *TextureCache) Generation() uint64 { return cache.generation }

func (cache *TextureCache) PendingRetirements() int { return len(cache.retired) }

// Trim evicts old resident entries after every resource needed by this frame
// has been requested. It also reduces a temporary over-budget frame once its
// textures are no longer in use.
func (cache *TextureCache) Trim(serial uint64) {
	cache.current = max(cache.current, serial)
	cache.evict()
}

func (cache *TextureCache) Request(key TextureKey, source image.Image, serial uint64) (uint32, bool) {
	if source == nil {
		return 0, false
	}
	cache.tick++
	cache.current = max(cache.current, serial)
	entry := cache.entries[key]
	if entry == nil {
		entry = &resident{source: source, bytes: int64(source.Bounds().Dx()) * int64(source.Bounds().Dy()) * 4}
		cache.entries[key] = entry
	}
	entry.used, entry.serial = cache.tick, max(entry.serial, serial)
	if entry.handle != 0 {
		return entry.handle, true
	}
	if !entry.queued {
		entry.queued = true
		cache.queue = append(cache.queue, key)
	}
	return 0, false
}

// DrainUploads takes pending requests in painter order. A failed upload can
// be retried via Request on the next frame.
func (cache *TextureCache) DrainUploads() []Upload {
	uploads := make([]Upload, 0, len(cache.queue))
	for _, key := range cache.queue {
		if entry := cache.entries[key]; entry != nil && entry.queued {
			entry.queued = false
			uploads = append(uploads, Upload{Key: key, Image: entry.source, Generation: cache.generation})
		}
	}
	cache.queue = cache.queue[:0]
	return uploads
}

// Commit rejects stale uploads after device loss. A cache may temporarily
// exceed its budget while in-flight textures await their completion fence.
func (cache *TextureCache) Commit(upload Upload, handle uint32) bool {
	entry := cache.entries[upload.Key]
	if upload.Generation != cache.generation || entry == nil || handle == 0 || entry.handle != 0 {
		return false
	}
	entry.handle = handle
	cache.used += entry.bytes
	cache.evict()
	return true
}

func (cache *TextureCache) evict() {
	if cache.used <= cache.budget {
		return
	}
	keys := make([]TextureKey, 0, len(cache.entries))
	for key, entry := range cache.entries {
		if entry.handle != 0 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return cache.entries[keys[i]].used < cache.entries[keys[j]].used })
	for _, key := range keys {
		if cache.used <= cache.budget {
			break
		}
		entry := cache.entries[key]
		if entry.serial == cache.current {
			continue
		}
		// Keep one oversized texture so a single-image frame can make progress.
		if len(keys) == 1 {
			break
		}
		cache.retired = append(cache.retired, retired{entry.handle, entry.serial})
		cache.used -= entry.bytes
		delete(cache.entries, key)
	}
}

// Collect returns textures safe to destroy after the completed frame serial.
func (cache *TextureCache) Collect(completed uint64) []uint32 {
	var handles []uint32
	remaining := cache.retired[:0]
	for _, r := range cache.retired {
		if r.serial <= completed {
			handles = append(handles, r.handle)
		} else {
			remaining = append(remaining, r)
		}
	}
	cache.retired = remaining
	return handles
}

// DeviceLost drops GPU handles and invalidates queued uploads. The next
// Request repopulates resources from their CPU snapshots on the new device.
func (cache *TextureCache) DeviceLost() {
	cache.generation++
	cache.used = 0
	cache.current = 0
	cache.queue = nil
	cache.retired = nil
	for _, entry := range cache.entries {
		entry.handle, entry.queued = 0, false
	}
}

// Close returns all live handles for cleanup after the caller waits for the
// device. On device loss, call DeviceLost instead (handles are invalid).
func (cache *TextureCache) Close() []uint32 {
	var handles []uint32
	for _, entry := range cache.entries {
		if entry.handle != 0 {
			handles = append(handles, entry.handle)
		}
	}
	for _, r := range cache.retired {
		handles = append(handles, r.handle)
	}
	cache.entries = make(map[TextureKey]*resident)
	cache.queue, cache.retired = nil, nil
	cache.used = 0
	return handles
}
