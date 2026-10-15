package vulkan

import (
	"fmt"
	"image"
	"log"
	"runtime"
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

func (backend *Backend) initSurface() error {
	backend.device = backend.renderer.Device()
	backend.pipelineLayout = backend.renderer.QuadPipelineLayout()
	backend.frameCapacity = maxVertices
	backend.frameStride = uint64(maxVertices * vertexSize)
	bufferSize := backend.frameStride * uint64(backend.renderer.FramesInFlight())
	backend.buffer, _ = backend.renderer.CreateBufferHelper(
		bufferSize,
		binding.VK_BUFFER_USAGE_VERTEX_BUFFER_BIT,
		binding.VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT|binding.VK_MEMORY_PROPERTY_HOST_COHERENT_BIT,
	)
	if backend.buffer == 0 {
		return fmt.Errorf("failed to create UI vertex buffer")
	}
	backend.mapped = backend.renderer.MapBufferHelper(backend.buffer, 0, bufferSize)
	if backend.mapped == nil {
		return fmt.Errorf("failed to map UI vertex buffer")
	}
	atlasID := backend.renderer.CreateTexture("__ui_font_atlas", nativeforms.FontAtlas())
	if atlasID <= 0 {
		return fmt.Errorf("failed to create UI font atlas")
	}
	backend.atlasTextureID = uint32(atlasID)
	if err := backend.initPipeline(); err != nil {
		return err
	}
	if err := backend.initDescriptor(); err != nil {
		return err
	}
	if err := backend.initSamplers(); err != nil {
		return err
	}
	return nil
}

func (backend *Backend) initPipeline() error {
	if backend.pipeline != 0 {
		binding.VkDestroyPipeline(backend.device, backend.pipeline, 0)
		backend.pipeline = 0
	}
	if backend.imagePipeline != 0 {
		binding.VkDestroyPipeline(backend.device, backend.imagePipeline, 0)
		backend.imagePipeline = 0
	}
	backend.pipelineFormat = backend.renderer.SwapchainFormat()

	vertModule, err := binding.CompileShader(backend.device, backend.shaders.Vertex)
	if err != nil {
		return fmt.Errorf("load hud.vert.spv: %w", err)
	}
	fragModule, err := binding.CompileShader(backend.device, backend.shaders.Fragment)
	if err != nil {
		binding.VkDestroyShaderModule(backend.device, vertModule, 0)
		return fmt.Errorf("load hud.frag.spv: %w", err)
	}
	defer binding.VkDestroyShaderModule(backend.device, vertModule, 0)
	defer binding.VkDestroyShaderModule(backend.device, fragModule, 0)

	entryPoint := append([]byte("main"), 0)
	stages := []binding.VkPipelineShaderStageCreateInfo{
		{
			SType:  binding.VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO,
			Stage:  binding.VK_SHADER_STAGE_VERTEX_BIT,
			Module: vertModule,
			PName:  &entryPoint[0],
		},
		{
			SType:  binding.VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO,
			Stage:  binding.VK_SHADER_STAGE_FRAGMENT_BIT,
			Module: fragModule,
			PName:  &entryPoint[0],
		},
	}
	bindingDescription := binding.VkVertexInputBindingDescription{
		Binding: 0, Stride: uint32(vertexSize), InputRate: 0,
	}
	attributes := []binding.VkVertexInputAttributeDescription{
		{Location: 0, Binding: 0, Format: binding.VK_FORMAT_R32G32_SFLOAT, Offset: 0},
		{Location: 1, Binding: 0, Format: binding.VK_FORMAT_R32G32_SFLOAT, Offset: 8},
		{Location: 2, Binding: 0, Format: binding.VK_FORMAT_R32G32B32A32_SFLOAT, Offset: 16},
	}
	vertexInput := binding.VkPipelineVertexInputStateCreateInfo{
		SType:                           binding.VK_STRUCTURE_TYPE_PIPELINE_VERTEX_INPUT_STATE_CREATE_INFO,
		VertexBindingDescriptionCount:   1,
		PVertexBindingDescriptions:      &bindingDescription,
		VertexAttributeDescriptionCount: uint32(len(attributes)),
		PVertexAttributeDescriptions:    &attributes[0],
	}
	inputAssembly := binding.VkPipelineInputAssemblyStateCreateInfo{
		SType: binding.VK_STRUCTURE_TYPE_PIPELINE_INPUT_ASSEMBLY_STATE_CREATE_INFO, Topology: binding.VK_PRIMITIVE_TOPOLOGY_TRIANGLE_LIST,
	}
	viewportState := binding.VkPipelineViewportStateCreateInfo{
		SType: binding.VK_STRUCTURE_TYPE_PIPELINE_VIEWPORT_STATE_CREATE_INFO, ViewportCount: 1, ScissorCount: 1,
	}
	rasterization := binding.VkPipelineRasterizationStateCreateInfo{
		SType:       binding.VK_STRUCTURE_TYPE_PIPELINE_RASTERIZATION_STATE_CREATE_INFO,
		PolygonMode: binding.VK_POLYGON_MODE_FILL, CullMode: binding.VK_CULL_MODE_NONE, LineWidth: 1,
	}
	multisample := binding.VkPipelineMultisampleStateCreateInfo{
		SType: binding.VK_STRUCTURE_TYPE_PIPELINE_MULTISAMPLE_STATE_CREATE_INFO, RasterizationSamples: binding.VK_SAMPLE_COUNT_1_BIT,
	}
	blendAttachment := binding.VkPipelineColorBlendAttachmentState{
		BlendEnable:         1,
		SrcColorBlendFactor: binding.VK_BLEND_FACTOR_SRC_ALPHA,
		DstColorBlendFactor: binding.VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA,
		ColorBlendOp:        binding.VK_BLEND_OP_ADD,
		SrcAlphaBlendFactor: binding.VK_BLEND_FACTOR_ONE,
		DstAlphaBlendFactor: binding.VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA,
		AlphaBlendOp:        binding.VK_BLEND_OP_ADD,
		ColorWriteMask:      0xF,
	}
	blend := binding.VkPipelineColorBlendStateCreateInfo{
		SType:           binding.VK_STRUCTURE_TYPE_PIPELINE_COLOR_BLEND_STATE_CREATE_INFO,
		AttachmentCount: 1, PAttachments: &blendAttachment,
	}
	dynamicStates := []uint32{binding.VK_DYNAMIC_STATE_VIEWPORT, binding.VK_DYNAMIC_STATE_SCISSOR}
	dynamic := binding.VkPipelineDynamicStateCreateInfo{
		SType:             binding.VK_STRUCTURE_TYPE_PIPELINE_DYNAMIC_STATE_CREATE_INFO,
		DynamicStateCount: uint32(len(dynamicStates)), PDynamicStates: &dynamicStates[0],
	}
	info := binding.VkGraphicsPipelineCreateInfo{
		SType:      binding.VK_STRUCTURE_TYPE_GRAPHICS_PIPELINE_CREATE_INFO,
		StageCount: uint32(len(stages)), PStages: &stages[0],
		PVertexInputState: &vertexInput, PInputAssemblyState: &inputAssembly,
		PViewportState: &viewportState, PRasterizationState: &rasterization,
		PMultisampleState: &multisample, PColorBlendState: &blend, PDynamicState: &dynamic,
		Layout: backend.pipelineLayout, RenderPass: backend.renderer.SwapRenderPass(),
	}
	result := binding.VkCreateGraphicsPipelines(backend.device, 0, 1, &info, 0, &backend.pipeline)
	runtime.KeepAlive(info)
	if result != binding.VK_SUCCESS || backend.pipeline == 0 {
		return fmt.Errorf("create UI pipeline: %d", result)
	}
	// CPU images store premultiplied RGBA. Their vertex tint is premultiplied
	// as well, so blending uses ONE for source color. Atlas glyph/solid draws
	// keep the original straight-alpha pipeline.
	blendAttachment.SrcColorBlendFactor = binding.VK_BLEND_FACTOR_ONE
	result = binding.VkCreateGraphicsPipelines(backend.device, 0, 1, &info, 0, &backend.imagePipeline)
	if result != binding.VK_SUCCESS || backend.imagePipeline == 0 {
		return fmt.Errorf("create UI image pipeline: %d", result)
	}
	return nil
}

func (backend *Backend) ensurePipeline() error {
	if backend.pipeline == 0 || backend.imagePipeline == 0 || backend.pipelineFormat != backend.renderer.SwapchainFormat() {
		return backend.initPipeline()
	}
	return nil
}

func (backend *Backend) initDescriptor() error {
	layout := backend.renderer.QuadSetLayout()
	allocate := binding.VkDescriptorSetAllocateInfo{
		SType:          binding.VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO,
		DescriptorPool: backend.renderer.DescriptorPool(), DescriptorSetCount: 1, PSetLayouts: &layout,
	}
	if result := binding.VkAllocateDescriptorSets(backend.device, &allocate, &backend.descriptorSet); result != binding.VK_SUCCESS || backend.descriptorSet == 0 {
		return fmt.Errorf("allocate UI descriptor set: %d", result)
	}
	vkImageView, ok := backend.renderer.TextureImageViewByID(backend.atlasTextureID)
	if !ok || vkImageView == 0 {
		return fmt.Errorf("UI font atlas is unavailable")
	}
	imageInfo := binding.VkDescriptorImageInfo{
		Sampler: backend.renderer.DefaultSampler(), ImageView: vkImageView,
		ImageLayout: binding.VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL,
	}
	write := binding.VkWriteDescriptorSet{
		SType:  binding.VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET,
		DstSet: backend.descriptorSet, DstBinding: 0, DescriptorCount: 1,
		DescriptorType: binding.VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER, PImageInfo: &imageInfo,
	}
	binding.VkUpdateDescriptorSets(backend.device, 1, &write, 0, nil)
	return nil
}

func (backend *Backend) initSamplers() error {
	create := func(filter uint32) (uint64, error) {
		info := binding.VkSamplerCreateInfo{
			SType:     binding.VK_STRUCTURE_TYPE_SAMPLER_CREATE_INFO,
			MinFilter: filter, MagFilter: filter,
			MipmapMode:   binding.VK_SAMPLER_MIPMAP_MODE_NEAREST,
			AddressModeU: binding.VK_SAMPLER_ADDRESS_MODE_CLAMP_TO_EDGE,
			AddressModeV: binding.VK_SAMPLER_ADDRESS_MODE_CLAMP_TO_EDGE,
			AddressModeW: binding.VK_SAMPLER_ADDRESS_MODE_CLAMP_TO_EDGE,
		}
		var sampler uint64
		if result := binding.VkCreateSampler(backend.device, &info, 0, &sampler); result != binding.VK_SUCCESS || sampler == 0 {
			return 0, fmt.Errorf("create UI sampler: %d", result)
		}
		return sampler, nil
	}
	var err error
	backend.nearestSampler, err = create(binding.VK_FILTER_NEAREST)
	if err != nil {
		return err
	}
	backend.linearSampler, err = create(binding.VK_FILTER_LINEAR)
	return err
}

func (backend *Backend) descriptorFor(texture uint32, filter nativeforms.ImageFilter) (uint64, error) {
	key := descriptorKey{texture, filter}
	if set := backend.descriptors[key]; set != 0 {
		return set, nil
	}
	view, ok := backend.renderer.TextureImageViewByID(texture)
	if !ok || view == 0 {
		return 0, fmt.Errorf("Vulkan UI texture %d has no image view", texture)
	}
	layout := backend.renderer.QuadSetLayout()
	allocate := binding.VkDescriptorSetAllocateInfo{
		SType:          binding.VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO,
		DescriptorPool: backend.renderer.DescriptorPool(), DescriptorSetCount: 1, PSetLayouts: &layout,
	}
	var set uint64
	if result := binding.VkAllocateDescriptorSets(backend.device, &allocate, &set); result != binding.VK_SUCCESS || set == 0 {
		return 0, fmt.Errorf("allocate UI image descriptor: %d", result)
	}
	sampler := backend.nearestSampler
	if filter == nativeforms.FilterBilinear {
		sampler = backend.linearSampler
	}
	imageInfo := binding.VkDescriptorImageInfo{Sampler: sampler, ImageView: view,
		ImageLayout: binding.VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL}
	write := binding.VkWriteDescriptorSet{SType: binding.VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET,
		DstSet: set, DstBinding: 0, DescriptorCount: 1,
		DescriptorType: binding.VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER, PImageInfo: &imageInfo}
	binding.VkUpdateDescriptorSets(backend.device, 1, &write, 0, nil)
	backend.descriptors[key] = set
	backend.setsByTexture[texture] = append(backend.setsByTexture[texture], set)
	return set, nil
}

func (backend *Backend) retireTexture(texture uint32) {
	for _, set := range backend.setsByTexture[texture] {
		if result := binding.VkFreeDescriptorSets(backend.device, backend.renderer.DescriptorPool(), 1, &set); result != binding.VK_SUCCESS {
			log.Printf("Warning: failed to free UI image descriptor: %d", result)
		}
	}
	delete(backend.setsByTexture, texture)
	for key := range backend.descriptors {
		if key.texture == texture {
			delete(backend.descriptors, key)
		}
	}
	backend.renderer.DestroyTexture(texture)
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
