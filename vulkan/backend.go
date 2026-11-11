package vulkan

import (
	"fmt"
	"image"
	"log"
	"strconv"
	"unsafe"

	binding "github.com/banditmoscow1337/miniVK"
	nativeforms "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/render"
)

const vertexSize = int(unsafe.Sizeof(nativeforms.Vertex{}))
const maxVertices = 64 * 1024

type Renderer interface {
	Device() uintptr
	QuadPipelineLayout() uint64
	FramesInFlight() uint32
	CreateBufferHelper(size uint64, usage, props uint32) (uint64, uint64)
	MapBufferHelper(buffer, offset, size uint64) unsafe.Pointer
	CreateTexture(name string, img image.Image) int
	SwapchainFormat() uint32
	SwapRenderPass() uint64
	QuadSetLayout() uint64
	DefaultSampler() uint64
	TextureImageViewByID(id uint32) (uint64, bool)
	DestroyTexture(uint32)
	TextureReady(id uint32) bool
	CurrentFrameIndex() uint32
	DestroyBufferHelper(uint64)
	DescriptorPool() uint64
}

type ShaderPaths struct{ Vertex, Fragment string }

type Backend struct {
	renderer                                        Renderer
	shaders                                         ShaderPaths
	device                                          uintptr
	pipeline, pipelineLayout, descriptorSet, buffer uint64
	imagePipeline, nearestSampler, linearSampler    uint64
	textures                                        *render.TextureCache
	descriptors                                     map[descriptorKey]uint64
	setsByTexture                                   map[uint32][]uint64
	serial                                          uint64
	pipelineFormat                                  uint32
	mapped                                          unsafe.Pointer
	frameStride                                     uint64
	frameCapacity                                   int
	atlasTextureID                                  uint32
	mesh                                            render.Mesh
}

type descriptorKey struct {
	texture uint32
	filter  nativeforms.ImageFilter
}

// SubmittedSerial is the sequence assigned to the latest RenderList call.
// An engine implementing CompletedUISerial must report completion in this
// sequence, after the submission fence signals.
func (backend *Backend) SubmittedSerial() uint64 {
	if backend == nil {
		return 0
	}
	return backend.serial
}

func New(renderer Renderer, shaders ShaderPaths) (*Backend, error) {
	if renderer == nil || shaders.Vertex == "" || shaders.Fragment == "" {
		return nil, fmt.Errorf("Vulkan renderer and shader paths are required")
	}
	if renderer.FramesInFlight() == 0 {
		return nil, fmt.Errorf("Vulkan renderer has no frames in flight")
	}
	backend := &Backend{renderer: renderer, shaders: shaders,
		textures: render.NewTextureCache(64 << 20), descriptors: make(map[descriptorKey]uint64),
		setsByTexture: make(map[uint32][]uint64)}
	if err := backend.initSurface(); err != nil {
		backend.Destroy()
		return nil, err
	}
	return backend, nil
}

func (backend *Backend) Render(commandBuffer uintptr, frame nativeforms.Frame) error {
	if frame.Width <= 0 || frame.Height <= 0 || len(frame.Commands) == 0 {
		return nil
	}
	list, err := render.Freeze(frame, nil)
	if err != nil {
		return err
	}
	return backend.RenderList(commandBuffer, list)
}

// RenderList consumes a stable frame independently of Manager's reused buffer.
func (backend *Backend) RenderList(commandBuffer uintptr, list *render.DisplayList) error {
	if list == nil || list.Size().X <= 0 || list.Size().Y <= 0 {
		return nil
	}
	if backend.renderer == nil || backend.textures == nil {
		return fmt.Errorf("Vulkan UI backend is closed")
	}
	if err := backend.ensurePipeline(); err != nil {
		return err
	}
	if backend.mapped == nil || !backend.renderer.TextureReady(backend.atlasTextureID) {
		return fmt.Errorf("Vulkan UI resources are unavailable")
	}
	var err error
	backend.mesh, err = render.Build(list, backend.mesh)
	if err != nil {
		return err
	}
	if len(backend.mesh.Vertices) == 0 {
		return nil
	}
	if err := backend.ensureVertexCapacity(len(backend.mesh.Vertices)); err != nil {
		return err
	}
	if backend.renderer.CurrentFrameIndex() >= backend.renderer.FramesInFlight() {
		return fmt.Errorf("Vulkan frame index out of range")
	}
	backend.serial++
	if fence, ok := backend.renderer.(interface{ CompletedUISerial() uint64 }); ok {
		for _, texture := range backend.textures.Collect(fence.CompletedUISerial()) {
			backend.retireTexture(texture)
		}
	}
	for _, batch := range backend.mesh.Batches {
		if batch.Image != nil {
			backend.textures.Request(batch.Texture, batch.Image, backend.serial)
		}
	}
	backend.textures.Trim(backend.serial)
	for _, upload := range backend.textures.DrainUploads() {
		name := "__ui_image_" + strconv.FormatUint(upload.Key.ID, 10) + "_" + strconv.FormatUint(upload.Key.Version, 10)
		id := backend.renderer.CreateTexture(name, upload.Image)
		if id <= 0 {
			return fmt.Errorf("upload UI image %s failed", name)
		}
		if !backend.textures.Commit(upload, uint32(id)) {
			backend.renderer.DestroyTexture(uint32(id))
			return fmt.Errorf("stale UI image upload %s", name)
		}
	}
	// Older engine integrations have no UI completion serial. In that case,
	// wait only when an eviction needs to retire a live GPU texture.
	if _, ok := backend.renderer.(interface{ CompletedUISerial() uint64 }); !ok && backend.textures.PendingRetirements() != 0 {
		if result := binding.VkDeviceWaitIdle(backend.device); result != binding.VK_SUCCESS {
			return fmt.Errorf("wait to evict UI textures: %d", result)
		}
		for _, texture := range backend.textures.Collect(^uint64(0)) {
			backend.retireTexture(texture)
		}
	}
	sets := make([]uint64, len(backend.mesh.Batches))
	for i, batch := range backend.mesh.Batches {
		sets[i] = backend.descriptorSet
		if batch.Image == nil {
			continue
		}
		id, resident := backend.textures.Request(batch.Texture, batch.Image, backend.serial)
		if !resident || !backend.renderer.TextureReady(id) {
			return fmt.Errorf("Vulkan UI image %d/%d is not ready", batch.Texture.ID, batch.Texture.Version)
		}
		sets[i], err = backend.descriptorFor(id, batch.Filter)
		if err != nil {
			return err
		}
	}
	bytesToCopy := len(backend.mesh.Vertices) * vertexSize
	bufferOffset := uint64(backend.renderer.CurrentFrameIndex()) * backend.frameStride
	destination := unsafe.Add(backend.mapped, uintptr(bufferOffset))
	copy(
		unsafe.Slice((*byte)(destination), bytesToCopy),
		unsafe.Slice((*byte)(unsafe.Pointer(&backend.mesh.Vertices[0])), bytesToCopy),
	)

	viewport := binding.VkViewport{Width: float32(list.Size().X), Height: float32(list.Size().Y), MinDepth: 0, MaxDepth: 1}
	scissor := binding.VkRect2D{Extent: binding.VkExtent2D{Width: uint32(list.Size().X), Height: uint32(list.Size().Y)}}
	binding.VkCmdSetViewport(commandBuffer, 0, 1, &viewport)
	binding.VkCmdSetScissor(commandBuffer, 0, 1, &scissor)
	binding.VkCmdBindVertexBuffers(commandBuffer, 0, 1, &backend.buffer, &bufferOffset)
	for i, batch := range backend.mesh.Batches {
		pipeline := backend.pipeline
		if batch.Image != nil {
			pipeline = backend.imagePipeline
		}
		binding.VkCmdBindPipeline(commandBuffer, binding.VK_PIPELINE_BIND_POINT_GRAPHICS, pipeline)
		binding.VkCmdBindDescriptorSets(commandBuffer, binding.VK_PIPELINE_BIND_POINT_GRAPHICS,
			backend.pipelineLayout, 0, 1, &sets[i], 0, nil)
		binding.VkCmdDraw(commandBuffer, uint32(batch.Count), 1, uint32(batch.First), 0)
	}
	return nil
}

// A text mask can produce many small triangles. Grow the buffer when needed,
// waiting for existing submissions before retiring the old per-frame storage.
func (backend *Backend) ensureVertexCapacity(required int) error {
	if required <= backend.frameCapacity {
		return nil
	}
	capacity := required
	if backend.frameCapacity <= int(^uint(0)>>1)/2 {
		capacity = max(capacity, backend.frameCapacity*2)
	}
	frames := uint64(backend.renderer.FramesInFlight())
	if uint64(capacity) > ^uint64(0)/uint64(vertexSize)/frames {
		return fmt.Errorf("UI vertex buffer size overflow: %d vertices", required)
	}
	stride := uint64(capacity) * uint64(vertexSize)
	buffer, _ := backend.renderer.CreateBufferHelper(stride*frames,
		binding.VK_BUFFER_USAGE_VERTEX_BUFFER_BIT,
		binding.VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT|binding.VK_MEMORY_PROPERTY_HOST_COHERENT_BIT)
	if buffer == 0 {
		return fmt.Errorf("failed to grow UI vertex buffer for %d vertices", required)
	}
	mapped := backend.renderer.MapBufferHelper(buffer, 0, stride*frames)
	if mapped == nil {
		backend.renderer.DestroyBufferHelper(buffer)
		return fmt.Errorf("failed to map grown UI vertex buffer")
	}
	if result := binding.VkDeviceWaitIdle(backend.device); result != binding.VK_SUCCESS {
		backend.renderer.DestroyBufferHelper(buffer)
		return fmt.Errorf("wait for UI vertex buffer resize: %d", result)
	}
	backend.renderer.DestroyBufferHelper(backend.buffer)
	backend.buffer, backend.mapped = buffer, mapped
	backend.frameStride, backend.frameCapacity = stride, capacity
	return nil
}

func (backend *Backend) Destroy() {
	if backend == nil || backend.renderer == nil {
		return
	}
	if backend.device != 0 {
		if result := binding.VkDeviceWaitIdle(backend.device); result != binding.VK_SUCCESS {
			log.Printf("Warning: failed to wait for UI cleanup: %d", result)
		}
	}
	if backend.textures != nil {
		for _, texture := range backend.textures.Close() {
			backend.retireTexture(texture)
		}
		backend.textures = nil
	}
	if backend.buffer != 0 {
		backend.renderer.DestroyBufferHelper(backend.buffer)
		backend.buffer = 0
		backend.mapped = nil
	}
	if backend.pipeline != 0 {
		binding.VkDestroyPipeline(backend.device, backend.pipeline, 0)
		backend.pipeline = 0
		backend.pipelineFormat = 0
	}
	if backend.imagePipeline != 0 {
		binding.VkDestroyPipeline(backend.device, backend.imagePipeline, 0)
		backend.imagePipeline = 0
	}
	if backend.descriptorSet != 0 {
		if result := binding.VkFreeDescriptorSets(
			backend.device, backend.renderer.DescriptorPool(), 1, &backend.descriptorSet,
		); result != binding.VK_SUCCESS {
			log.Printf("Warning: failed to free UI descriptor set: %d", result)
		}
		backend.descriptorSet = 0
	}
	if backend.atlasTextureID != 0 {
		backend.renderer.DestroyTexture(backend.atlasTextureID)
		backend.atlasTextureID = 0
	}
	if backend.nearestSampler != 0 {
		binding.VkDestroySampler(backend.device, backend.nearestSampler, 0)
		backend.nearestSampler = 0
	}
	if backend.linearSampler != 0 {
		binding.VkDestroySampler(backend.device, backend.linearSampler, 0)
		backend.linearSampler = 0
	}
}

// RecoverDevice is called after the owner recreates a lost Vulkan device.
// Old Vulkan handles are invalid and must not be destroyed on the new device;
// immutable CPU images are reuploaded when next referenced by RenderList.
func (backend *Backend) RecoverDevice(renderer Renderer) error {
	if backend == nil || renderer == nil || renderer.FramesInFlight() == 0 {
		return fmt.Errorf("Vulkan UI recovery needs a live renderer")
	}
	backend.renderer = renderer
	backend.device = 0
	backend.pipeline, backend.imagePipeline, backend.descriptorSet = 0, 0, 0
	backend.buffer, backend.mapped, backend.atlasTextureID = 0, nil, 0
	backend.nearestSampler, backend.linearSampler = 0, 0
	backend.pipelineFormat = 0
	backend.descriptors = make(map[descriptorKey]uint64)
	backend.setsByTexture = make(map[uint32][]uint64)
	if backend.textures == nil {
		backend.textures = render.NewTextureCache(64 << 20)
	} else {
		backend.textures.DeviceLost()
	}
	if err := backend.initSurface(); err != nil {
		backend.Destroy()
		return err
	}
	return nil
}
