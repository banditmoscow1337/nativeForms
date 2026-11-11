package vulkan

import (
	"fmt"
	"log"
	"runtime"

	binding "github.com/banditmoscow1337/miniVK"
	nativeforms "github.com/banditmoscow1337/nativeForms"
)

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
	return backend.initSamplers()
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

func (backend *Backend) allocateSamplerSet(imageView, sampler uint64, label string) (uint64, error) {
	layout := backend.renderer.QuadSetLayout()
	allocate := binding.VkDescriptorSetAllocateInfo{
		SType:          binding.VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO,
		DescriptorPool: backend.renderer.DescriptorPool(), DescriptorSetCount: 1, PSetLayouts: &layout,
	}
	var set uint64
	if result := binding.VkAllocateDescriptorSets(backend.device, &allocate, &set); result != binding.VK_SUCCESS || set == 0 {
		return 0, fmt.Errorf("allocate %s: %d", label, result)
	}
	imageInfo := binding.VkDescriptorImageInfo{
		Sampler:     sampler,
		ImageView:   imageView,
		ImageLayout: binding.VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL,
	}
	write := binding.VkWriteDescriptorSet{
		SType:          binding.VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET,
		DstSet:         set,
		DstBinding:     0,
		DescriptorCount: 1,
		DescriptorType: binding.VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER,
		PImageInfo:     &imageInfo,
	}
	binding.VkUpdateDescriptorSets(backend.device, 1, &write, 0, nil)
	return set, nil
}

func (backend *Backend) initDescriptor() error {
	vkImageView, ok := backend.renderer.TextureImageViewByID(backend.atlasTextureID)
	if !ok || vkImageView == 0 {
		return fmt.Errorf("UI font atlas is unavailable")
	}
	var err error
	backend.descriptorSet, err = backend.allocateSamplerSet(vkImageView, backend.renderer.DefaultSampler(), "UI descriptor set")
	return err
}

func (backend *Backend) initSamplers() error {
	create := func(filter uint32) (uint64, error) {
		info := binding.VkSamplerCreateInfo{
			SType:        binding.VK_STRUCTURE_TYPE_SAMPLER_CREATE_INFO,
			MinFilter:    filter,
			MagFilter:    filter,
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
	sampler := backend.nearestSampler
	if filter == nativeforms.FilterBilinear {
		sampler = backend.linearSampler
	}
	set, err := backend.allocateSamplerSet(view, sampler, "UI image descriptor")
	if err != nil {
		return 0, err
	}
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
