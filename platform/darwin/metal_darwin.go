//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	metalSelOnce       sync.Once
	selStatus          objc.SEL
	selWaitUntilComp   objc.SEL
	selGPUStartTime    objc.SEL
	selGPUEndTime      objc.SEL
	selError           objc.SEL
	selContents        objc.SEL
	selNextDrawable    objc.SEL
	selTexture         objc.SEL
	selSetTexture      objc.SEL
	selSetResolveTex   objc.SEL
	selSetStoreAction  objc.SEL
	selSetLoadAction   objc.SEL
	selCommandBuffer   objc.SEL
	selRenderEncoder   objc.SEL
	selSetVertexBuf    objc.SEL
	selSetVertexBytes  objc.SEL
	selSetPipeline     objc.SEL
	selSetFragTex      objc.SEL
	selSetFragSampler  objc.SEL
	selDrawPrimitives  objc.SEL
	selEndEncoding     objc.SEL
	selPresentDrawable objc.SEL
	selCommit          objc.SEL
)

func initMetalSelectors() {
	metalSelOnce.Do(func() {
		selStatus = selector("status")
		selWaitUntilComp = selector("waitUntilCompleted")
		selGPUStartTime = selector("GPUStartTime")
		selGPUEndTime = selector("GPUEndTime")
		selError = selector("error")
		selContents = selector("contents")
		selNextDrawable = selector("nextDrawable")
		selTexture = selector("texture")
		selSetTexture = selector("setTexture:")
		selSetResolveTex = selector("setResolveTexture:")
		selSetStoreAction = selector("setStoreAction:")
		selSetLoadAction = selector("setLoadAction:")
		selCommandBuffer = selector("commandBuffer")
		selRenderEncoder = selector("renderCommandEncoderWithDescriptor:")
		selSetVertexBuf = selector("setVertexBuffer:offset:atIndex:")
		selSetVertexBytes = selector("setVertexBytes:length:atIndex:")
		selSetPipeline = selector("setRenderPipelineState:")
		selSetFragTex = selector("setFragmentTexture:atIndex:")
		selSetFragSampler = selector("setFragmentSamplerState:atIndex:")
		selDrawPrimitives = selector("drawPrimitives:vertexStart:vertexCount:")
		selEndEncoding = selector("endEncoding")
		selPresentDrawable = selector("presentDrawable:")
		selCommit = selector("commit")
	})
}

type metalBackend struct {
	view, layer, device, queue    objc.ID
	atlas, maskTex                objc.ID
	nearest, linear               objc.ID
	atlasPipeline, imagePipeline  objc.ID
	passDesc, colorAttachment     objc.ID
	slots                         [metalInFlight]metalSlot
	images                        map[render.TextureKey]metalImage
	imagesDirty                   bool
	batchTextures                 []objc.ID
	used                          int64
	mesh                          render.Mesh
	masks                         *render.GlyphAtlas
	maskUpdates                   int
	atlasLoaded                   bool
	serial                        uint64
	pixelWidth, pixelHeight       int
	drawableWidth, drawableHeight int
	drawableScale                 float32
	stats                         ui.RenderStats
	failAfter                     uint64
	samples                       uint64
}

func newMetalBackend(view objc.ID) (backend render.Backend, err error) {
	if os.Getenv("NATIVEFORMS_METAL_FAULT") == "init" {
		return nil, fmt.Errorf("darwin: injected Metal initialization failure: %w", render.ErrUnavailable)
	}
	lib, err := purego.Dlopen("/System/Library/Frameworks/Metal.framework/Metal", purego.RTLD_LOCAL|purego.RTLD_LAZY)
	if err != nil {
		return nil, fmt.Errorf("darwin: Metal framework: %w", err)
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/QuartzCore.framework/QuartzCore", purego.RTLD_GLOBAL|purego.RTLD_LAZY); err != nil {
		return nil, fmt.Errorf("darwin: QuartzCore framework: %w", err)
	}
	initMetalSelectors()
	var createDevice func() objc.ID
	purego.RegisterLibFunc(&createDevice, lib, "MTLCreateSystemDefaultDevice")
	device := createDevice()
	if device == 0 {
		return nil, render.ErrUnavailable
	}
	b := &metalBackend{view: view, device: device,
		images: make(map[render.TextureKey]metalImage), masks: render.NewGlyphAtlas(metalAtlasSize)}
	defer func() {
		if err != nil {
			_ = b.Close()
		}
	}()
	if n, e := strconv.ParseUint(os.Getenv("NATIVEFORMS_METAL_FAIL_AFTER"), 10, 64); e == nil {
		b.failAfter = n
	}
	if objc.Send[bool](b.device, selector("respondsToSelector:"), selector("newCommandQueueWithMaxCommandBufferCount:")) {
		b.queue = b.device.Send(selector("newCommandQueueWithMaxCommandBufferCount:"), uint64(8))
	} else {
		b.queue = b.device.Send(selector("newCommandQueue"))
	}
	if b.queue == 0 {
		return nil, metalError("command queue", 0)
	}
	// Default to 1x rasterization because UI rounded rects, lines, and glyph masks
	// are already analytically anti-aliased; 4x MSAA fullscreen targets consume
	// 67-200 MiB of unified RAM on Retina displays. Allow opt-in via env var.
	b.samples = 1
	if os.Getenv("NATIVEFORMS_METAL_MSAA") == "4" && objc.Send[bool](b.device, selector("supportsTextureSampleCount:"), uint64(4)) {
		b.samples = 4
	}
	var compileError objc.ID
	library := b.device.Send(selector("newLibraryWithSource:options:error:"), nsString(metalShaders), objc.ID(0), &compileError)
	if library == 0 {
		return nil, metalError("shader compilation", compileError)
	}
	defer library.Send(selRelease)
	vertex := library.Send(selector("newFunctionWithName:"), nsString("nativeVertex"))
	if vertex == 0 {
		return nil, metalError("vertex shader", 0)
	}
	defer vertex.Send(selRelease)
	for _, p := range []struct {
		name string
		dest *objc.ID
	}{{"nativeAtlas", &b.atlasPipeline}, {"nativeImage", &b.imagePipeline}} {
		fragment := library.Send(selector("newFunctionWithName:"), nsString(p.name))
		if fragment == 0 {
			return nil, metalError("fragment shader "+p.name, 0)
		}
		desc := class("MTLRenderPipelineDescriptor").Send(selNew)
		desc.Send(selector("setVertexFunction:"), vertex)
		desc.Send(selector("setFragmentFunction:"), fragment)
		desc.Send(selector("setRasterSampleCount:"), b.samples)
		attachment := desc.Send(selector("colorAttachments")).Send(selector("objectAtIndexedSubscript:"), uint64(0))
		attachment.Send(selector("setPixelFormat:"), uint64(metalBGRA8))
		attachment.Send(selector("setBlendingEnabled:"), true)
		attachment.Send(selector("setSourceRGBBlendFactor:"), uint64(metalBlendOne))
		attachment.Send(selector("setDestinationRGBBlendFactor:"), uint64(metalBlendOneMinusA))
		attachment.Send(selector("setSourceAlphaBlendFactor:"), uint64(metalBlendOne))
		attachment.Send(selector("setDestinationAlphaBlendFactor:"), uint64(metalBlendOneMinusA))
		compileError = 0
		*p.dest = b.device.Send(selector("newRenderPipelineStateWithDescriptor:error:"), desc, &compileError)
		desc.Send(selRelease)
		fragment.Send(selRelease)
		if *p.dest == 0 {
			return nil, metalError("render pipeline "+p.name, compileError)
		}
	}
	for _, s := range []struct {
		filter uint64
		dest   *objc.ID
	}{{0, &b.nearest}, {1, &b.linear}} {
		desc := class("MTLSamplerDescriptor").Send(selNew)
		desc.Send(selector("setMinFilter:"), s.filter)
		desc.Send(selector("setMagFilter:"), s.filter)
		// Clamp to zero prevents a UV outside an image from leaking its edge.
		desc.Send(selector("setSAddressMode:"), uint64(4))
		desc.Send(selector("setTAddressMode:"), uint64(4))
		*s.dest = b.device.Send(selector("newSamplerStateWithDescriptor:"), desc)
		desc.Send(selRelease)
		if *s.dest == 0 {
			return nil, metalError("sampler", 0)
		}
	}
	// Pre-allocate and configure a reusable MTLRenderPassDescriptor so per-frame
	// rendering performs zero Objective-C allocations or struct-by-value reflection calls.
	b.passDesc = class("MTLRenderPassDescriptor").Send(selNew)
	if b.passDesc == 0 {
		return nil, metalError("render pass descriptor", 0)
	}
	b.colorAttachment = msgSend1(msgSend0(b.passDesc, selector("colorAttachments")), selector("objectAtIndexedSubscript:"), 0)
	msgSend1(b.colorAttachment, selSetLoadAction, 2) // clear
	b.colorAttachment.Send(selector("setClearColor:"), metalClearColor{})

	// Start with a tiny 4x4 opaque white texture so we do not eagerly allocate
	// the 16 MiB fallback bitmap font atlas unless a frame actually uses it.
	b.atlas, err = b.newTextureWithFormat(metalRGBA8, 4, 4)
	if err != nil {
		return nil, err
	}
	white := make([]byte, 4*4*4)
	for i := range white {
		white[i] = 255
	}
	uploadMetalPixels(b.atlas, metalRegion{W: 4, H: 4, Depth: 1}, white, 16)

	// Single shared single-channel A8Unorm glyph mask atlas (256 KiB at 512x512).
	// Initialize top-left 2x2 texels to 255 so solid-color quads (UV = 0.5/2048) can share
	// b.maskTex and coalesce with glyph mask batches into a single draw call.
	b.maskTex, err = b.newTextureWithFormat(metalA8, metalAtlasSize, metalAtlasSize)
	if err != nil {
		return nil, err
	}
	opaqueTexels := [4]byte{255, 255, 255, 255}
	uploadMetalPixels(b.maskTex, metalRegion{X: 0, Y: 0, W: 2, H: 2, Depth: 1}, opaqueTexels[:], 2)

	b.layer = class("CAMetalLayer").Send(selNew)
	if b.layer == 0 {
		return nil, metalError("CAMetalLayer", 0)
	}
	b.layer.Send(selector("setDevice:"), b.device)
	b.layer.Send(selector("setPixelFormat:"), uint64(metalBGRA8))
	b.layer.Send(selector("setFramebufferOnly:"), true)
	b.layer.Send(selector("setOpaque:"), true)
	if objc.Send[bool](b.layer, selector("respondsToSelector:"), selector("setMaximumDrawableCount:")) {
		b.layer.Send(selector("setMaximumDrawableCount:"), uint64(2))
	}
	if objc.Send[bool](b.layer, selector("respondsToSelector:"), selector("setAllowsNextDrawableTimeout:")) {
		b.layer.Send(selector("setAllowsNextDrawableTimeout:"), true)
	}
	// Assign layer BEFORE setWantsLayer:true so AppKit creates a layer-hosting
	// view without allocating an unused 16.8 MiB Retina CABackingStore.
	view.Send(selector("setLayer:"), b.layer)
	view.Send(selector("setWantsLayer:"), true)
	return b, nil
}

func (b *metalBackend) Kind() render.Kind           { return render.Hardware }
func (b *metalBackend) RenderStats() ui.RenderStats { return b.stats }
func (b *metalBackend) SetDrawableSize(width, height int) {
	b.pixelWidth, b.pixelHeight = width, height
}

func (b *metalBackend) waitAllSlots() {
	for i := range b.slots {
		s := &b.slots[i]
		if s.command != 0 {
			if msgSendUint0(s.command, selStatus) < metalCommandComplete {
				msgSend0(s.command, selWaitUntilComp)
			}
			msgSend0(s.command, selRelease)
			s.command = 0
		}
	}
}

func (b *metalBackend) Close() error {
	if b == nil {
		return nil
	}
	if b.view != 0 && b.layer != 0 && b.view.Send(selector("layer")) == b.layer {
		b.view.Send(selector("setLayer:"), objc.ID(0))
		b.view.Send(selector("setWantsLayer:"), false)
	}
	for i := range b.slots {
		s := &b.slots[i]
		if s.command != 0 {
			s.command.Send(selWaitUntilComp)
			s.command.Send(selRelease)
		}
		for _, object := range []objc.ID{s.buffer, s.color} {
			if object != 0 {
				object.Send(selRelease)
			}
		}
		*s = metalSlot{}
	}
	for key, resource := range b.images {
		resource.texture.Send(selRelease)
		delete(b.images, key)
	}
	for _, object := range []objc.ID{b.passDesc, b.layer, b.atlas, b.maskTex, b.atlasPipeline, b.imagePipeline, b.nearest, b.linear, b.queue, b.device} {
		if object != 0 {
			object.Send(selRelease)
		}
	}
	b.passDesc, b.colorAttachment = 0, 0
	b.layer, b.device, b.view = 0, 0, 0
	return nil
}
