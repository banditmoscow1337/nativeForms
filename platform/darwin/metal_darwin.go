//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"image"
	"os"
	"strconv"
	"structs"
	"time"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

const (
	metalBGRA8           = 80
	metalRGBA8           = 70
	metalTriangle        = 3
	metalBlendOne        = 1
	metalBlendOneMinusA  = 5
	metalAtlasSize       = 2048
	metalTextureBudget   = 64 << 20
	metalInFlight        = 3
	metalMultisampleType = 4
	metalRenderTarget    = 4
	metalStoragePrivate  = 2
	metalCommandComplete = 4
	metalCommandError    = 5
)

// MTLRegion and MTLClearColor are passed by value through objc_msgSend.
type metalRegion struct {
	_                    structs.HostLayout
	X, Y, Z, W, H, Depth uint64
}
type metalClearColor struct {
	_          structs.HostLayout
	R, G, B, A float64
}

const metalShaders = `#include <metal_stdlib>
using namespace metal;
struct Vertex { float2 position; float2 uv; float4 color; };
struct Varying { float4 position [[position]]; float2 uv; float4 color; };
vertex Varying nativeVertex(const device Vertex* vertices [[buffer(0)]],
                            constant float2& viewport [[buffer(1)]],
                            uint index [[vertex_id]]) {
    Vertex v = vertices[index];
    Varying out;
    out.position = float4(v.position.x * 2.0 / viewport.x - 1.0,
                          1.0 - v.position.y * 2.0 / viewport.y, 0.0, 1.0);
    out.uv = v.uv; out.color = v.color;
    return out;
}
fragment float4 nativeAtlas(Varying v [[stage_in]],
                             texture2d<float> tex [[texture(0)]],
                             sampler sampleState [[sampler(0)]]) {
    float alpha = tex.sample(sampleState, v.uv).a * v.color.a;
    return float4(v.color.rgb * alpha, alpha);
}
fragment float4 nativeImage(Varying v [[stage_in]],
                             texture2d<float> tex [[texture(0)]],
                             sampler sampleState [[sampler(0)]]) {
    return tex.sample(sampleState, v.uv) * v.color;
}`

type metalSlot struct {
	buffer, masks, color, command objc.ID
	capacity, width, height       int
	maskUpdates                   int
}

type metalImage struct {
	texture objc.ID
	bytes   int64
	last    uint64
}

type metalBackend struct {
	view, layer, device, queue    objc.ID
	atlas, nearest, linear        objc.ID
	atlasPipeline, imagePipeline  objc.ID
	slots                         [metalInFlight]metalSlot
	images                        map[render.TextureKey]metalImage
	batchTextures                 []objc.ID
	maskScratch                   []byte
	used                          int64
	mesh                          render.Mesh
	masks                         *render.GlyphAtlas
	serial                        uint64
	pixelWidth, pixelHeight       int
	drawableWidth, drawableHeight int
	drawableScale                 float32
	stats                         ui.RenderStats
	failAfter                     uint64
	samples                       uint64
}

func metalError(prefix string, native objc.ID) error {
	if native != 0 {
		return fmt.Errorf("darwin: Metal %s: %s", prefix, stringValue(native.Send(selector("localizedDescription"))))
	}
	return fmt.Errorf("darwin: Metal %s: %w", prefix, render.ErrUnavailable)
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
	b.queue = b.device.Send(selector("newCommandQueue"))
	if b.queue == 0 {
		return nil, metalError("command queue", 0)
	}
	b.samples = 1
	if objc.Send[bool](b.device, selector("supportsTextureSampleCount:"), uint64(4)) {
		b.samples = 4
	}
	var compileError objc.ID
	library := b.device.Send(selector("newLibraryWithSource:options:error:"), nsString(metalShaders), objc.ID(0), &compileError)
	if library == 0 {
		return nil, metalError("shader compilation", compileError)
	}
	defer library.Send(selector("release"))
	vertex := library.Send(selector("newFunctionWithName:"), nsString("nativeVertex"))
	if vertex == 0 {
		return nil, metalError("vertex shader", 0)
	}
	defer vertex.Send(selector("release"))
	for _, p := range []struct {
		name string
		dest *objc.ID
	}{{"nativeAtlas", &b.atlasPipeline}, {"nativeImage", &b.imagePipeline}} {
		fragment := library.Send(selector("newFunctionWithName:"), nsString(p.name))
		if fragment == 0 {
			return nil, metalError("fragment shader "+p.name, 0)
		}
		desc := class("MTLRenderPipelineDescriptor").Send(selector("new"))
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
		desc.Send(selector("release"))
		fragment.Send(selector("release"))
		if *p.dest == 0 {
			return nil, metalError("render pipeline "+p.name, compileError)
		}
	}
	for _, s := range []struct {
		filter uint64
		dest   *objc.ID
	}{{0, &b.nearest}, {1, &b.linear}} {
		desc := class("MTLSamplerDescriptor").Send(selector("new"))
		desc.Send(selector("setMinFilter:"), s.filter)
		desc.Send(selector("setMagFilter:"), s.filter)
		// Clamp to zero prevents a UV outside an image from leaking its edge.
		desc.Send(selector("setSAddressMode:"), uint64(4))
		desc.Send(selector("setTAddressMode:"), uint64(4))
		*s.dest = b.device.Send(selector("newSamplerStateWithDescriptor:"), desc)
		desc.Send(selector("release"))
		if *s.dest == 0 {
			return nil, metalError("sampler", 0)
		}
	}
	// The static font atlas is immutable. Dynamic masks are updated per slot.
	font := ui.FontAtlas()
	b.atlas, err = b.newTexture(font.Bounds().Dx(), font.Bounds().Dy())
	if err != nil {
		return nil, err
	}
	if err = uploadMetalImage(b.atlas, font); err != nil {
		return nil, err
	}
	for i := range b.slots {
		b.slots[i].masks, err = b.newTexture(metalAtlasSize, metalAtlasSize)
		if err != nil {
			return nil, err
		}
	}
	b.layer = class("CAMetalLayer").Send(selector("new"))
	if b.layer == 0 {
		return nil, metalError("CAMetalLayer", 0)
	}
	b.layer.Send(selector("setDevice:"), b.device)
	b.layer.Send(selector("setPixelFormat:"), uint64(metalBGRA8))
	b.layer.Send(selector("setFramebufferOnly:"), true)
	b.layer.Send(selector("setOpaque:"), false)
	view.Send(selector("setWantsLayer:"), true)
	view.Send(selector("setLayer:"), b.layer)
	return b, nil
}

func (b *metalBackend) Kind() render.Kind           { return render.Hardware }
func (b *metalBackend) RenderStats() ui.RenderStats { return b.stats }
func (b *metalBackend) SetDrawableSize(width, height int) {
	b.pixelWidth, b.pixelHeight = width, height
}

func (b *metalBackend) newTexture(width, height int) (objc.ID, error) {
	desc := class("MTLTextureDescriptor").Send(selector("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"),
		uint64(metalRGBA8), uint64(width), uint64(height), false)
	if desc == 0 {
		return 0, metalError("texture descriptor", 0)
	}
	texture := b.device.Send(selector("newTextureWithDescriptor:"), desc)
	if texture == 0 {
		return 0, metalError("texture allocation", 0)
	}
	return texture, nil
}

// replaceRegion copies its source synchronously. Keep the slice alive across
// the native call; Metal's private texture remains valid after it returns.
func uploadMetalPixels(texture objc.ID, region metalRegion, pixels []byte, rowBytes int) {
	texture.Send(selector("replaceRegion:mipmapLevel:withBytes:bytesPerRow:"),
		region, uint64(0), unsafe.Pointer(&pixels[0]), uint64(rowBytes))
}

func uploadMetalImage(texture objc.ID, source image.Image) error {
	b := source.Bounds()
	if b.Empty() {
		return fmt.Errorf("darwin: empty Metal texture")
	}
	if rgba, ok := source.(*image.RGBA); ok && b.Min == (image.Point{}) && rgba.Stride == b.Dx()*4 {
		uploadMetalPixels(texture, metalRegion{W: uint64(b.Dx()), H: uint64(b.Dy()), Depth: 1}, rgba.Pix, rgba.Stride)
		return nil
	}
	pixels := make([]byte, b.Dx()*b.Dy()*4)
	switch src := source.(type) {
	case *image.NRGBA:
		w, h := b.Dx(), b.Dy()
		for y := 0; y < h; y++ {
			so := src.PixOffset(b.Min.X, b.Min.Y+y)
			do := y * w * 4
			row := src.Pix[so : so+w*4]
			dst := pixels[do : do+w*4]
			for i := 0; i < len(row); i += 4 {
				a := uint32(row[i+3])
				dst[i] = byte((uint32(row[i])*a + 127) / 255)
				dst[i+1] = byte((uint32(row[i+1])*a + 127) / 255)
				dst[i+2] = byte((uint32(row[i+2])*a + 127) / 255)
				dst[i+3] = byte(a)
			}
		}
	case *ui.ImageResource:
		w, h := b.Dx(), b.Dy()
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := src.RGBAAt(b.Min.X+x, b.Min.Y+y)
				i := (y*w + x) * 4
				pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = c.R, c.G, c.B, c.A
			}
		}
	default:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, blue, a := source.At(x, y).RGBA()
				i := ((y-b.Min.Y)*b.Dx() + x - b.Min.X) * 4
				pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = byte(r>>8), byte(g>>8), byte(blue>>8), byte(a>>8)
			}
		}
	}
	uploadMetalPixels(texture, metalRegion{W: uint64(b.Dx()), H: uint64(b.Dy()), Depth: 1}, pixels, b.Dx()*4)
	return nil
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
			s.command.Send(selector("waitUntilCompleted"))
			s.command.Send(selector("release"))
		}
		for _, object := range []objc.ID{s.buffer, s.masks, s.color} {
			if object != 0 {
				object.Send(selector("release"))
			}
		}
		*s = metalSlot{}
	}
	for key, resource := range b.images {
		resource.texture.Send(selector("release"))
		delete(b.images, key)
	}
	for _, object := range []objc.ID{b.layer, b.atlas, b.atlasPipeline, b.imagePipeline, b.nearest, b.linear, b.queue, b.device} {
		if object != 0 {
			object.Send(selector("release"))
		}
	}
	b.layer, b.device, b.view = 0, 0, 0
	return nil
}

func (b *metalBackend) prepareSlot(slot *metalSlot, bytes, width, height int) error {
	if slot.command != 0 {
		start := time.Now()
		if msgSendUint0(slot.command, selector("status")) < metalCommandComplete {
			msgSend0(slot.command, selector("waitUntilCompleted"))
		}
		b.stats.GPUWait = time.Since(start)
		status := msgSendUint0(slot.command, selector("status"))
		if status == metalCommandComplete {
			begin := objc.Send[float64](slot.command, selector("GPUStartTime"))
			end := objc.Send[float64](slot.command, selector("GPUEndTime"))
			if end >= begin && begin > 0 {
				b.stats.CompletedGPU = time.Duration((end - begin) * float64(time.Second))
			}
		}
		failure := msgSend0(slot.command, selector("error"))
		msgSend0(slot.command, selector("release"))
		slot.command = 0
		if status == metalCommandError {
			return fmt.Errorf("%w: %v", render.ErrDeviceLost, metalError("command buffer", failure))
		}
	}
	if bytes > slot.capacity {
		capacity := max(bytes, max(slot.capacity*2, 256<<10))
		if capacity > 64<<20 {
			return fmt.Errorf("darwin: vertex buffer exceeds 64 MiB: %w", render.ErrUnavailable)
		}
		buffer := b.device.Send(selector("newBufferWithLength:options:"), uint64(capacity), uint64(0))
		if buffer == 0 {
			return metalError("vertex buffer", 0)
		}
		if slot.buffer != 0 {
			slot.buffer.Send(selector("release")) // only this slot was waited on
		}
		slot.buffer, slot.capacity = buffer, capacity
	}
	if b.samples > 1 && (slot.color == 0 || slot.width != width || slot.height != height) {
		if slot.color != 0 {
			slot.color.Send(selector("release"))
		}
		slot.color = 0
		desc := class("MTLTextureDescriptor").Send(selector("new"))
		desc.Send(selector("setTextureType:"), uint64(metalMultisampleType))
		desc.Send(selector("setPixelFormat:"), uint64(metalBGRA8))
		desc.Send(selector("setWidth:"), uint64(width))
		desc.Send(selector("setHeight:"), uint64(height))
		desc.Send(selector("setSampleCount:"), b.samples)
		desc.Send(selector("setStorageMode:"), uint64(metalStoragePrivate))
		desc.Send(selector("setUsage:"), uint64(metalRenderTarget))
		slot.color = b.device.Send(selector("newTextureWithDescriptor:"), desc)
		desc.Send(selector("release"))
		if slot.color == 0 {
			return metalError("multisample target", 0)
		}
		slot.width, slot.height = width, height
	}
	return nil
}

func (b *metalBackend) uploadMasks(slot *metalSlot) {
	for _, update := range b.masks.UpdatesSince(slot.maskUpdates) {
		r, mask := update.Rect, update.Mask
		needed := r.Dx() * r.Dy() * 4
		if cap(b.maskScratch) < needed {
			b.maskScratch = make([]byte, needed)
		}
		pixels := b.maskScratch[:needed]
		for y := 0; y < r.Dy(); y++ {
			for x := 0; x < r.Dx(); x++ {
				i := (y*r.Dx() + x) * 4
				pixels[i], pixels[i+1], pixels[i+2] = 255, 255, 255
				pixels[i+3] = mask.Pix[mask.PixOffset(mask.Rect.Min.X+x, mask.Rect.Min.Y+y)]
			}
		}
		uploadMetalPixels(slot.masks, metalRegion{X: uint64(r.Min.X), Y: uint64(r.Min.Y),
			W: uint64(r.Dx()), H: uint64(r.Dy()), Depth: 1}, pixels, r.Dx()*4)
		b.stats.TextureUploads++
		b.stats.UploadBytes += int64(len(pixels))
	}
	slot.maskUpdates = b.masks.UpdateCount()
}

func (b *metalBackend) imageTexture(batch render.Batch) (objc.ID, error) {
	if batch.Image == nil {
		return 0, fmt.Errorf("darwin: image batch has no resource")
	}
	if entry, ok := b.images[batch.Texture]; ok {
		entry.last = b.serial
		b.images[batch.Texture] = entry
		return entry.texture, nil
	}
	bounds := batch.Image.Bounds()
	texture, err := b.newTexture(bounds.Dx(), bounds.Dy())
	if err != nil {
		return 0, err
	}
	if err := uploadMetalImage(texture, batch.Image); err != nil {
		texture.Send(selector("release"))
		return 0, err
	}
	bytes := int64(bounds.Dx()) * int64(bounds.Dy()) * 4
	b.images[batch.Texture] = metalImage{texture: texture, bytes: bytes, last: b.serial}
	b.used += bytes
	b.stats.TextureUploads++
	b.stats.UploadBytes += bytes
	return texture, nil
}

func (b *metalBackend) trimImages() {
	for b.used > metalTextureBudget {
		var oldest render.TextureKey
		var found bool
		var timestamp uint64
		for key, item := range b.images {
			if item.last < b.serial && (!found || item.last < timestamp) {
				oldest, timestamp, found = key, item.last, true
			}
		}
		if !found {
			return // all textures in use this frame
		}
		item := b.images[oldest]
		item.texture.Send(selector("release"))
		b.used -= item.bytes
		delete(b.images, oldest)
	}
}

// RenderAtScale submits triangles to a CAMetalLayer without a CPU framebuffer.
// Three persistent buffers and separate mask textures let us update the atlas
// and resize a buffer after waiting for only the slot about to be reused.
func (b *metalBackend) RenderAtScale(frame ui.Frame, _ *image.RGBA, scale float32) error {
	b.stats = ui.RenderStats{}
	if b.device == 0 || frame.Width <= 0 || frame.Height <= 0 || !(scale > 0 && scale <= 8) {
		return fmt.Errorf("darwin: invalid Metal frame: %w", render.ErrUnavailable)
	}
	if os.Getenv("NATIVEFORMS_METAL_FAULT") == "render" ||
		b.failAfter > 0 && b.serial+1 >= b.failAfter {
		return fmt.Errorf("darwin: injected Metal render failure: %w", render.ErrDeviceLost)
	}
	start := time.Now()
	previousOverflows := b.masks.Overflows()
	mesh, err := render.BuildFrame(frame, b.mesh, b.masks)
	if err != nil {
		return err
	}
	b.mesh = mesh
	b.stats.Build = time.Since(start)
	b.stats.Batches, b.stats.Vertices = len(mesh.Batches), len(mesh.Vertices)
	b.stats.MaskAtlasFallbacks = b.masks.Overflows() - previousOverflows
	b.serial++
	slot := &b.slots[(b.serial-1)%metalInFlight]
	start = time.Now()
	bytes := len(mesh.Vertices) * int(unsafe.Sizeof(ui.Vertex{}))
	width, height := b.pixelWidth, b.pixelHeight
	if width <= 0 || height <= 0 {
		width, height = int(float32(frame.Width)*scale), int(float32(frame.Height)*scale)
	}
	if err := b.prepareSlot(slot, bytes, width, height); err != nil {
		return err
	}
	if bytes > 0 {
		pointer := unsafe.Pointer(uintptr(msgSend0(slot.buffer, selector("contents"))))
		if pointer == nil {
			return fmt.Errorf("darwin: Metal buffer is unmapped: %w", render.ErrDeviceLost)
		}
		copy(unsafe.Slice((*byte)(pointer), bytes), unsafe.Slice((*byte)(unsafe.Pointer(&mesh.Vertices[0])), bytes))
		b.stats.UploadBytes += int64(bytes)
	}
	b.uploadMasks(slot)
	// Load all new images before encoding. Present commands only refer to
	// immutable textures; the Metal command buffer retains their references.
	if cap(b.batchTextures) < len(mesh.Batches) {
		b.batchTextures = make([]objc.ID, len(mesh.Batches))
	} else {
		b.batchTextures = b.batchTextures[:len(mesh.Batches)]
	}
	textures := b.batchTextures
	for i, batch := range mesh.Batches {
		switch batch.Texture {
		case (render.TextureKey{}):
			textures[i] = b.atlas
		case render.MaskTexture:
			textures[i] = slot.masks
		default:
			textures[i], err = b.imageTexture(batch)
			if err != nil {
				return err
			}
		}
	}
	b.stats.Upload = time.Since(start)
	start = time.Now()
	if width != b.drawableWidth || height != b.drawableHeight || scale != b.drawableScale {
		b.layer.Send(selector("setContentsScale:"), float64(scale))
		b.layer.Send(selector("setDrawableSize:"), size{W: float64(width), H: float64(height)})
		b.drawableWidth, b.drawableHeight, b.drawableScale = width, height, scale
	}
	drawable := msgSend0(b.layer, selector("nextDrawable"))
	if drawable == 0 {
		return fmt.Errorf("darwin: Metal drawable unavailable: %w", render.ErrDeviceLost)
	}
	pass := msgSend0(class("MTLRenderPassDescriptor"), selector("renderPassDescriptor"))
	attachment := msgSend1(msgSend0(pass, selector("colorAttachments")), selector("objectAtIndexedSubscript:"), 0)
	drawableTex := msgSend0(drawable, selector("texture"))
	if b.samples > 1 {
		msgSend1(attachment, selector("setTexture:"), uintptr(slot.color))
		msgSend1(attachment, selector("setResolveTexture:"), uintptr(drawableTex))
		msgSend1(attachment, selector("setStoreAction:"), 2) // resolve
	} else {
		msgSend1(attachment, selector("setTexture:"), uintptr(drawableTex))
		msgSend1(attachment, selector("setStoreAction:"), 1) // store
	}
	msgSend1(attachment, selector("setLoadAction:"), 2) // clear
	attachment.Send(selector("setClearColor:"), metalClearColor{})
	command := msgSend0(b.queue, selector("commandBuffer"))
	if command == 0 {
		return fmt.Errorf("darwin: Metal command buffer unavailable: %w", render.ErrDeviceLost)
	}
	encoder := msgSend1(command, selector("renderCommandEncoderWithDescriptor:"), uintptr(pass))
	if encoder == 0 {
		return fmt.Errorf("darwin: Metal render encoder unavailable: %w", render.ErrDeviceLost)
	}
	viewport := [2]float32{float32(frame.Width), float32(frame.Height)}
	if bytes > 0 {
		msgSend3(encoder, selector("setVertexBuffer:offset:atIndex:"), uintptr(slot.buffer), 0, 0)
		msgSend3(encoder, selector("setVertexBytes:length:atIndex:"), uintptr(unsafe.Pointer(&viewport[0])), 8, 1)
	}
	selSetPipeline := selector("setRenderPipelineState:")
	selSetTexture := selector("setFragmentTexture:atIndex:")
	selSetSampler := selector("setFragmentSamplerState:atIndex:")
	selDraw := selector("drawPrimitives:vertexStart:vertexCount:")
	for i, batch := range mesh.Batches {
		pipeline, sampler := b.atlasPipeline, b.nearest
		if batch.Image != nil {
			pipeline = b.imagePipeline
			if batch.Filter == ui.FilterBilinear {
				sampler = b.linear
			}
		}
		msgSend1(encoder, selSetPipeline, uintptr(pipeline))
		msgSend2(encoder, selSetTexture, uintptr(textures[i]), 0)
		msgSend2(encoder, selSetSampler, uintptr(sampler), 0)
		msgSend3(encoder, selDraw, metalTriangle, uintptr(batch.First), uintptr(batch.Count))
	}
	msgSend0(encoder, selector("endEncoding"))
	msgSend1(command, selector("presentDrawable:"), uintptr(drawable))
	slot.command = msgSend0(command, selector("retain"))
	msgSend0(command, selector("commit"))
	b.trimImages()
	b.stats.Submit = time.Since(start)
	return nil
}

var _ render.Backend = (*metalBackend)(nil)
