package vulkan

import (
	"image"
	"testing"
	"unsafe"

	binding "github.com/banditmoscow1337/miniVK"
	"github.com/banditmoscow1337/nativeForms/internal/renderparity"
	"github.com/banditmoscow1337/nativeForms/render"
)

type fakeRenderer struct {
	nextID  int
	uploads []image.Image
}

func (*fakeRenderer) Device() uintptr                                            { return 1 }
func (*fakeRenderer) QuadPipelineLayout() uint64                                 { return 1 }
func (*fakeRenderer) FramesInFlight() uint32                                     { return 1 }
func (*fakeRenderer) CreateBufferHelper(uint64, uint32, uint32) (uint64, uint64) { return 1, 1 }
func (*fakeRenderer) MapBufferHelper(uint64, uint64, uint64) unsafe.Pointer      { return nil }
func (f *fakeRenderer) CreateTexture(_ string, img image.Image) int {
	f.nextID++
	f.uploads = append(f.uploads, img)
	return f.nextID
}
func (*fakeRenderer) SwapchainFormat() uint32                       { return 44 }
func (*fakeRenderer) SwapRenderPass() uint64                        { return 1 }
func (*fakeRenderer) QuadSetLayout() uint64                         { return 1 }
func (*fakeRenderer) DefaultSampler() uint64                        { return 1 }
func (*fakeRenderer) TextureImageViewByID(id uint32) (uint64, bool) { return uint64(id) + 100, true }
func (*fakeRenderer) DestroyTexture(uint32)                         {}
func (*fakeRenderer) TextureReady(uint32) bool                      { return true }
func (*fakeRenderer) CurrentFrameIndex() uint32                     { return 0 }
func (*fakeRenderer) DestroyBufferHelper(uint64)                    {}
func (*fakeRenderer) DescriptorPool() uint64                        { return 1 }

// This checks the actual Vulkan command/descriptor submission path without a
// GPU. Pixel readback parity still needs an engine device and swapchain.
func TestGoldenScenesSubmitAllVulkanBatches(t *testing.T) {
	oldBindPipeline := binding.VkCmdBindPipeline
	oldBindSets := binding.VkCmdBindDescriptorSets
	oldBindBuffer := binding.VkCmdBindVertexBuffers
	oldViewport := binding.VkCmdSetViewport
	oldScissor := binding.VkCmdSetScissor
	oldDraw := binding.VkCmdDraw
	oldAllocate := binding.VkAllocateDescriptorSets
	oldUpdate := binding.VkUpdateDescriptorSets
	defer func() {
		binding.VkCmdBindPipeline = oldBindPipeline
		binding.VkCmdBindDescriptorSets = oldBindSets
		binding.VkCmdBindVertexBuffers = oldBindBuffer
		binding.VkCmdSetViewport = oldViewport
		binding.VkCmdSetScissor = oldScissor
		binding.VkCmdDraw = oldDraw
		binding.VkAllocateDescriptorSets = oldAllocate
		binding.VkUpdateDescriptorSets = oldUpdate
	}()
	type draw struct {
		pipeline, set uint64
		first, count  uint32
	}
	var draws []draw
	var pipeline, set uint64
	samplers := make(map[uint64]uint64)
	binding.VkCmdBindPipeline = func(_ uintptr, _ uint32, value uint64) { pipeline = value }
	binding.VkCmdBindDescriptorSets = func(_ uintptr, _ uint32, _ uint64, _ uint32, _ uint32, value *uint64, _ uint32, _ *uint32) {
		set = *value
	}
	binding.VkCmdBindVertexBuffers = func(uintptr, uint32, uint32, *uint64, *uint64) {}
	binding.VkCmdSetViewport = func(uintptr, uint32, uint32, *binding.VkViewport) {}
	binding.VkCmdSetScissor = func(uintptr, uint32, uint32, *binding.VkRect2D) {}
	binding.VkCmdDraw = func(_ uintptr, count, _ uint32, first, _ uint32) {
		draws = append(draws, draw{pipeline, set, first, count})
	}
	var nextSet uint64 = 40
	binding.VkAllocateDescriptorSets = func(_ uintptr, _ *binding.VkDescriptorSetAllocateInfo, result *uint64) int32 {
		nextSet++
		*result = nextSet
		return binding.VK_SUCCESS
	}
	binding.VkUpdateDescriptorSets = func(_ uintptr, _ uint32, write *binding.VkWriteDescriptorSet, _ uint32, _ unsafe.Pointer) {
		samplers[write.DstSet] = write.PImageInfo.Sampler
	}

	fake := &fakeRenderer{nextID: 1} // Atlas is already texture 1.
	storage := make([]byte, 1<<20)
	backend := &Backend{renderer: fake, device: 1, pipeline: 11, imagePipeline: 12,
		pipelineLayout: 1, pipelineFormat: 44, descriptorSet: 21, atlasTextureID: 1,
		nearestSampler: 31, linearSampler: 32, textures: render.NewTextureCache(64 << 20),
		descriptors: make(map[descriptorKey]uint64), setsByTexture: make(map[uint32][]uint64),
		mapped: unsafe.Pointer(&storage[0]), frameStride: uint64(len(storage)), frameCapacity: len(storage) / vertexSize}
	for _, scene := range renderparity.Scenes() {
		t.Run(scene.Name, func(t *testing.T) {
			draws = nil
			list, err := render.Freeze(scene.Frame, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := backend.RenderList(1, list); err != nil {
				t.Fatal(err)
			}
			if len(draws) != len(backend.mesh.Batches) || len(draws) == 0 {
				t.Fatalf("Vulkan dropped a golden-scene batch: %v", draws)
			}
			for i, batch := range backend.mesh.Batches {
				got := draws[i]
				wantPipeline := uint64(11)
				if batch.Image != nil {
					wantPipeline = 12
					if got.set == 21 {
						t.Fatal("image was bound to the font atlas descriptor")
					}
					wantSampler := uint64(31)
					if batch.Filter == 1 {
						wantSampler = 32
					}
					if samplers[got.set] != wantSampler {
						t.Fatalf("incorrect image sampler: set=%d sampler=%d", got.set, samplers[got.set])
					}
				}
				if got.pipeline != wantPipeline || got.first != uint32(batch.First) || got.count != uint32(batch.Count) {
					t.Fatalf("draw %d: %+v, batch %+v", i, got, batch)
				}
			}
			uploaded := len(fake.uploads)
			if err := backend.RenderList(1, list); err != nil || len(fake.uploads) != uploaded {
				t.Fatalf("repeated frame reuploaded an unchanged image: uploads %d -> %d, %v", uploaded, len(fake.uploads), err)
			}
		})
	}
	if len(fake.uploads) == 0 {
		t.Fatal("CommandImage did not enqueue Vulkan textures")
	}
}
