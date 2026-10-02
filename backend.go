package nativeforms

import (
	"fmt"
	"log"
	"runtime"
	"time"
	"unsafe"

	binding "github.com/banditmoscow1337/miniVK"
)

const vertexSize = int(unsafe.Sizeof(Vertex{}))

func (manager *Manager) initSurface() {
	if manager.renderer == nil {
		panic("ui: nil renderer")
	}
	manager.device = manager.renderer.Device()
	manager.pipelineLayout = manager.renderer.QuadPipelineLayout()
	manager.frameStride = uint64(maxVertices * vertexSize)
	bufferSize := manager.frameStride * uint64(manager.renderer.FramesInFlight())
	manager.buffer, _ = manager.renderer.CreateBufferHelper(
		bufferSize,
		binding.VK_BUFFER_USAGE_VERTEX_BUFFER_BIT,
		binding.VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT|binding.VK_MEMORY_PROPERTY_HOST_COHERENT_BIT,
	)
	if manager.buffer == 0 {
		log.Fatal("Fatal: failed to create UI vertex buffer")
	}
	manager.mapped = manager.renderer.MapBufferHelper(manager.buffer, 0, bufferSize)
	if manager.mapped == nil {
		log.Fatal("Fatal: failed to map UI vertex buffer")
	}
	atlasID := manager.renderer.CreateTexture("__ui_font_atlas", buildFontAtlas())
	if atlasID <= 0 {
		log.Fatal("Fatal: failed to create UI font atlas")
	}
	manager.atlasTextureID = uint32(atlasID)
	manager.initPipeline()
	manager.initDescriptor()
}

func (manager *Manager) initPipeline() {
	if manager.pipeline != 0 {
		binding.VkDestroyPipeline(manager.device, manager.pipeline, 0)
		manager.pipeline = 0
	}
	manager.pipelineFormat = manager.renderer.SwapchainFormat()

	vertModule, err := binding.CompileShader(manager.device, "shaders/hud.vert.spv")
	if err != nil {
		log.Fatalf("Failed to load shaders/hud.vert.spv: %v", err)
	}
	fragModule, err := binding.CompileShader(manager.device, "shaders/hud.frag.spv")
	if err != nil {
		binding.VkDestroyShaderModule(manager.device, vertModule, 0)
		log.Fatalf("Failed to load shaders/hud.frag.spv: %v", err)
	}
	defer binding.VkDestroyShaderModule(manager.device, vertModule, 0)
	defer binding.VkDestroyShaderModule(manager.device, fragModule, 0)

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
		Layout: manager.pipelineLayout, RenderPass: manager.renderer.SwapRenderPass(),
	}
	result := binding.VkCreateGraphicsPipelines(manager.device, 0, 1, &info, 0, &manager.pipeline)
	runtime.KeepAlive(info)
	if result != binding.VK_SUCCESS || manager.pipeline == 0 {
		log.Fatalf("Fatal: failed to create UI pipeline: %d", result)
	}
}

func (manager *Manager) ensurePipeline() {
	if manager.pipeline == 0 || manager.pipelineFormat != manager.renderer.SwapchainFormat() {
		manager.initPipeline()
	}
}

func (manager *Manager) initDescriptor() {
	layout := manager.renderer.QuadSetLayout()
	allocate := binding.VkDescriptorSetAllocateInfo{
		SType:          binding.VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO,
		DescriptorPool: manager.renderer.DescriptorPool(), DescriptorSetCount: 1, PSetLayouts: &layout,
	}
	if result := binding.VkAllocateDescriptorSets(manager.device, &allocate, &manager.descriptorSet); result != binding.VK_SUCCESS || manager.descriptorSet == 0 {
		log.Fatalf("Fatal: failed to allocate UI descriptor set: %d", result)
	}
	vkImageView, ok := manager.renderer.TextureImageViewByID(manager.atlasTextureID)
	if !ok || vkImageView == 0 {
		log.Fatal("Fatal: UI font atlas is unavailable")
	}
	imageInfo := binding.VkDescriptorImageInfo{
		Sampler: manager.renderer.DefaultSampler(), ImageView: vkImageView,
		ImageLayout: binding.VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL,
	}
	write := binding.VkWriteDescriptorSet{
		SType:  binding.VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET,
		DstSet: manager.descriptorSet, DstBinding: 0, DescriptorCount: 1,
		DescriptorType: binding.VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER, PImageInfo: &imageInfo,
	}
	binding.VkUpdateDescriptorSets(manager.device, 1, &write, 0, nil)
}

func (manager *Manager) Render(commandBuffer uintptr, width, height int) {
	if len(manager.vertices) == 0 || manager.mapped == nil || width <= 0 || height <= 0 ||
		!manager.renderer.TextureReady(manager.atlasTextureID) {
		return
	}
	bytesToCopy := len(manager.vertices) * vertexSize
	bufferOffset := uint64(manager.renderer.CurrentFrameIndex()) * manager.frameStride
	destination := unsafe.Add(manager.mapped, uintptr(bufferOffset))
	copy(
		unsafe.Slice((*byte)(destination), bytesToCopy),
		unsafe.Slice((*byte)(unsafe.Pointer(&manager.vertices[0])), bytesToCopy),
	)

	binding.VkCmdBindPipeline(commandBuffer, binding.VK_PIPELINE_BIND_POINT_GRAPHICS, manager.pipeline)
	binding.VkCmdBindDescriptorSets(
		commandBuffer, binding.VK_PIPELINE_BIND_POINT_GRAPHICS, manager.pipelineLayout,
		0, 1, &manager.descriptorSet, 0, nil,
	)
	viewport := binding.VkViewport{Width: float32(width), Height: float32(height), MinDepth: 0, MaxDepth: 1}
	scissor := binding.VkRect2D{Extent: binding.VkExtent2D{Width: uint32(width), Height: uint32(height)}}
	binding.VkCmdSetViewport(commandBuffer, 0, 1, &viewport)
	binding.VkCmdSetScissor(commandBuffer, 0, 1, &scissor)
	binding.VkCmdBindVertexBuffers(commandBuffer, 0, 1, &manager.buffer, &bufferOffset)
	binding.VkCmdDraw(commandBuffer, uint32(len(manager.vertices)), 1, 0, 0)
}

func (manager *Manager) Destroy() {
	if manager == nil {
		return
	}
	if manager.buffer != 0 {
		manager.renderer.DestroyBufferHelper(manager.buffer)
		manager.buffer = 0
		manager.mapped = nil
	}
	if manager.pipeline != 0 {
		binding.VkDestroyPipeline(manager.device, manager.pipeline, 0)
		manager.pipeline = 0
		manager.pipelineFormat = 0
	}
	if manager.descriptorSet != 0 {
		if result := binding.VkFreeDescriptorSets(
			manager.device, manager.renderer.DescriptorPool(), 1, &manager.descriptorSet,
		); result != binding.VK_SUCCESS {
			log.Printf("Warning: failed to free UI descriptor set: %d", result)
		}
		manager.descriptorSet = 0
	}
	if manager.atlasTextureID != 0 {
		manager.renderer.DestroyTexture(manager.atlasTextureID)
		manager.atlasTextureID = 0
	}
	manager.SetRoot(nil)
}

func (manager *Manager) ShowMessage(format string, args ...any) {
	if len(args) == 1 {
		var seconds float64
		hasDuration := true
		switch value := args[0].(type) {
		case float64:
			seconds = value
		case float32:
			seconds = float64(value)
		case int:
			seconds = float64(value)
		case int32:
			seconds = float64(value)
		default:
			hasDuration = false
		}
		if hasDuration {
			manager.Notify(format, time.Duration(seconds*float64(time.Second)))
			return
		}
	}
	if len(args) == 0 {
		manager.Notify(format)
		return
	}
	manager.Notify(fmt.Sprintf(format, args...))
}
