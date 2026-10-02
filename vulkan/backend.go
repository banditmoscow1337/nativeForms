package vulkan

import (
	"fmt"
	"image"
	"log"
	"runtime"
	"unsafe"

	binding "github.com/banditmoscow1337/miniVK"
	nativeforms "github.com/banditmoscow1337/nativeForms"
)

const vertexSize = int(unsafe.Sizeof(nativeforms.Vertex{}))
const maxVertices = 64 * 1024

// Renderer is the game host's Vulkan resource provider.
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

type ShaderPaths struct { Vertex, Fragment string }

type Backend struct {
	renderer Renderer
	shaders ShaderPaths
	device uintptr
	pipeline, pipelineLayout, descriptorSet, buffer uint64
	pipelineFormat uint32
	mapped unsafe.Pointer
	frameStride uint64
	atlasTextureID uint32
	vertices []nativeforms.Vertex
}

// New uses shader files supplied by the game. The caller owns synchronization
// with in-flight GPU work before Destroy is called.
func New(renderer Renderer, shaders ShaderPaths) (*Backend, error) {
	if renderer == nil || shaders.Vertex == "" || shaders.Fragment == "" {
		return nil, fmt.Errorf("Vulkan renderer and shader paths are required")
	}
	if renderer.FramesInFlight() == 0 {
		return nil, fmt.Errorf("Vulkan renderer has no frames in flight")
	}
	backend := &Backend{renderer: renderer, shaders: shaders}
	if err := backend.initSurface(); err != nil {
		backend.Destroy()
		return nil, err
	}
	return backend, nil
}

func (backend *Backend) initSurface() error {
	backend.device = backend.renderer.Device()
	backend.pipelineLayout = backend.renderer.QuadPipelineLayout()
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
	if err := backend.initPipeline(); err != nil { return err }
	if err := backend.initDescriptor(); err != nil { return err }
	return nil
}

func (backend *Backend) initPipeline() error {
	if backend.pipeline != 0 {
		binding.VkDestroyPipeline(backend.device, backend.pipeline, 0)
		backend.pipeline = 0
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
	return nil
}

func (backend *Backend) ensurePipeline() error {
	if backend.pipeline == 0 || backend.pipelineFormat != backend.renderer.SwapchainFormat() {
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

func (backend *Backend) Render(commandBuffer uintptr, frame nativeforms.Frame) error {
	if frame.Width <= 0 || frame.Height <= 0 || len(frame.Commands) == 0 { return nil }
	if err := backend.ensurePipeline(); err != nil { return err }
	if backend.mapped == nil || !backend.renderer.TextureReady(backend.atlasTextureID) {
		return fmt.Errorf("Vulkan UI resources are unavailable")
	}
	backend.vertices = nativeforms.Tessellate(frame.Commands, backend.vertices)
	if len(backend.vertices) == 0 { return nil }
	if len(backend.vertices) > maxVertices {
		return fmt.Errorf("UI frame requires %d vertices; Vulkan capacity is %d", len(backend.vertices), maxVertices)
	}
	if backend.renderer.CurrentFrameIndex() >= backend.renderer.FramesInFlight() {
		return fmt.Errorf("Vulkan frame index out of range")
	}
	bytesToCopy := len(backend.vertices) * vertexSize
	bufferOffset := uint64(backend.renderer.CurrentFrameIndex()) * backend.frameStride
	destination := unsafe.Add(backend.mapped, uintptr(bufferOffset))
	copy(
		unsafe.Slice((*byte)(destination), bytesToCopy),
		unsafe.Slice((*byte)(unsafe.Pointer(&backend.vertices[0])), bytesToCopy),
	)

	binding.VkCmdBindPipeline(commandBuffer, binding.VK_PIPELINE_BIND_POINT_GRAPHICS, backend.pipeline)
	binding.VkCmdBindDescriptorSets(
		commandBuffer, binding.VK_PIPELINE_BIND_POINT_GRAPHICS, backend.pipelineLayout,
		0, 1, &backend.descriptorSet, 0, nil,
	)
	viewport := binding.VkViewport{Width: float32(frame.Width), Height: float32(frame.Height), MinDepth: 0, MaxDepth: 1}
	scissor := binding.VkRect2D{Extent: binding.VkExtent2D{Width: uint32(frame.Width), Height: uint32(frame.Height)}}
	binding.VkCmdSetViewport(commandBuffer, 0, 1, &viewport)
	binding.VkCmdSetScissor(commandBuffer, 0, 1, &scissor)
	binding.VkCmdBindVertexBuffers(commandBuffer, 0, 1, &backend.buffer, &bufferOffset)
	binding.VkCmdDraw(commandBuffer, uint32(len(backend.vertices)), 1, 0, 0)
	return nil
}

func (backend *Backend) Destroy() {
	if backend == nil || backend.renderer == nil {
		return
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
}
