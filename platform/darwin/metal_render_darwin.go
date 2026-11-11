//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"image"
	"os"
	"time"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/ebitengine/purego/objc"
)

func (b *metalBackend) prepareSlot(slot *metalSlot, bytes, width, height int) error {
	if slot.command != 0 {
		status := msgSendUint0(slot.command, selStatus)
		if status < metalCommandComplete {
			start := time.Now()
			msgSend0(slot.command, selWaitUntilComp)
			b.stats.GPUWait = time.Since(start)
			status = msgSendUint0(slot.command, selStatus)
		}
		if status == metalCommandComplete {
			begin := msgSendFloat0(slot.command, selGPUStartTime)
			end := msgSendFloat0(slot.command, selGPUEndTime)
			if end >= begin && begin > 0 {
				b.stats.CompletedGPU = time.Duration((end - begin) * float64(time.Second))
			}
		} else if status == metalCommandError {
			failure := msgSend0(slot.command, selError)
			msgSend0(slot.command, selRelease)
			slot.command = 0
			return fmt.Errorf("%w: %v", render.ErrDeviceLost, metalError("command buffer", failure))
		}
		msgSend0(slot.command, selRelease)
		slot.command = 0
	}
	if bytes > slot.capacity {
		capacity := max(bytes, max(slot.capacity*2, 64<<10))
		if capacity > 64<<20 {
			return fmt.Errorf("darwin: vertex buffer exceeds 64 MiB: %w", render.ErrUnavailable)
		}
		buffer := b.device.Send(selector("newBufferWithLength:options:"), uint64(capacity), uint64(0))
		if buffer == 0 {
			return metalError("vertex buffer", 0)
		}
		contents := unsafe.Pointer(uintptr(msgSend0(buffer, selContents)))
		if contents == nil {
			buffer.Send(selRelease)
			return fmt.Errorf("darwin: Metal buffer is unmapped: %w", render.ErrDeviceLost)
		}
		if slot.buffer != 0 {
			slot.buffer.Send(selRelease) // only this slot was waited on
		}
		slot.buffer, slot.contents, slot.capacity = buffer, contents, capacity
	}
	if b.samples > 1 && (slot.color == 0 || slot.width != width || slot.height != height) {
		if slot.color != 0 {
			slot.color.Send(selRelease)
		}
		slot.color = 0
		desc := class("MTLTextureDescriptor").Send(selNew)
		desc.Send(selector("setTextureType:"), uint64(metalMultisampleType))
		desc.Send(selector("setPixelFormat:"), uint64(metalBGRA8))
		desc.Send(selector("setWidth:"), uint64(width))
		desc.Send(selector("setHeight:"), uint64(height))
		desc.Send(selector("setSampleCount:"), b.samples)
		desc.Send(selector("setStorageMode:"), uint64(metalStoragePrivate))
		desc.Send(selector("setUsage:"), uint64(metalRenderTarget))
		slot.color = b.device.Send(selector("newTextureWithDescriptor:"), desc)
		desc.Send(selRelease)
		if slot.color == 0 {
			return metalError("multisample target", 0)
		}
		slot.width, slot.height = width, height
	}
	return nil
}

func batchUsesBitmapFont(vertices []ui.Vertex, batch render.Batch) bool {
	const solidThreshold = float32(1.0 / metalAtlasSize)
	end := min(len(vertices), batch.First+batch.Count)
	for i := batch.First; i < end; i++ {
		if vertices[i].U > solidThreshold || vertices[i].V > solidThreshold {
			return true
		}
	}
	return false
}

// RenderAtScale submits triangles to a CAMetalLayer without a CPU framebuffer.
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
	if b.masks.Overflows() > previousOverflows {
		// Reclaim full glyph mask atlas once in-flight GPU work completes.
		b.waitAllSlots()
		b.masks.Reset()
		b.maskUpdates = 0
		opaqueTexels := [4]byte{255, 255, 255, 255}
		uploadMetalPixels(b.maskTex, metalRegion{X: 0, Y: 0, W: 2, H: 2, Depth: 1}, opaqueTexels[:], 2)
		previousOverflows = 0
		mesh, err = render.BuildFrame(frame, mesh, b.masks)
		if err != nil {
			return err
		}
	}
	b.mesh = mesh
	b.stats.Build = time.Since(start)
	b.stats.Vertices = len(mesh.Vertices)
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
		if slot.contents == nil {
			return fmt.Errorf("darwin: Metal buffer is unmapped: %w", render.ErrDeviceLost)
		}
		copy(unsafe.Slice((*byte)(slot.contents), bytes), unsafe.Slice((*byte)(unsafe.Pointer(&mesh.Vertices[0])), bytes))
		b.stats.UploadBytes += int64(bytes)
	}
	b.uploadMasks()
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
			if batchUsesBitmapFont(mesh.Vertices, batch) {
				if err := b.ensureFullAtlas(); err != nil {
					return err
				}
				textures[i] = b.atlas
			} else {
				// Solid-color geometry samples texel (0,0), which is 255 in b.maskTex.
				// Binding b.maskTex lets solid quads and glyph masks coalesce into one draw call.
				textures[i] = b.maskTex
			}
		case render.MaskTexture:
			textures[i] = b.maskTex
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
	drawable := msgSend0(b.layer, selNextDrawable)
	if drawable == 0 {
		return fmt.Errorf("darwin: Metal drawable unavailable: %w", render.ErrDeviceLost)
	}
	drawableTex := msgSend0(drawable, selTexture)
	if b.samples > 1 {
		msgSend1(b.colorAttachment, selSetTexture, uintptr(slot.color))
		msgSend1(b.colorAttachment, selSetResolveTex, uintptr(drawableTex))
		msgSend1(b.colorAttachment, selSetStoreAction, 2) // resolve
	} else {
		msgSend1(b.colorAttachment, selSetTexture, uintptr(drawableTex))
		msgSend1(b.colorAttachment, selSetStoreAction, 1) // store
	}
	command := msgSend0(b.queue, selCommandBuffer)
	if command == 0 {
		return fmt.Errorf("darwin: Metal command buffer unavailable: %w", render.ErrDeviceLost)
	}
	encoder := msgSend1(command, selRenderEncoder, uintptr(b.passDesc))
	if encoder == 0 {
		return fmt.Errorf("darwin: Metal render encoder unavailable: %w", render.ErrDeviceLost)
	}
	viewport := [2]float32{float32(frame.Width), float32(frame.Height)}
	if bytes > 0 {
		msgSend3(encoder, selSetVertexBuf, uintptr(slot.buffer), 0, 0)
		msgSend3(encoder, selSetVertexBytes, uintptr(unsafe.Pointer(&viewport[0])), 8, 1)
	}
	var lastPipeline, lastTexture, lastSampler objc.ID
	drawCalls := 0
	for i := 0; i < len(mesh.Batches); {
		batch := mesh.Batches[i]
		pipeline, sampler := b.atlasPipeline, b.linear
		if batch.Image != nil {
			pipeline = b.imagePipeline
			if batch.Filter != ui.FilterBilinear {
				sampler = b.nearest
			}
		}
		tex := textures[i]
		first := batch.First
		count := batch.Count
		i++
		for i < len(mesh.Batches) {
			next := mesh.Batches[i]
			nextPipeline, nextSampler := b.atlasPipeline, b.linear
			if next.Image != nil {
				nextPipeline = b.imagePipeline
				if next.Filter != ui.FilterBilinear {
					nextSampler = b.nearest
				}
			}
			if nextPipeline != pipeline || textures[i] != tex || nextSampler != sampler || next.First != first+count {
				break
			}
			count += next.Count
			i++
		}
		if pipeline != lastPipeline {
			msgSend1(encoder, selSetPipeline, uintptr(pipeline))
			lastPipeline = pipeline
		}
		if tex != lastTexture {
			msgSend2(encoder, selSetFragTex, uintptr(tex), 0)
			lastTexture = tex
		}
		if sampler != lastSampler {
			msgSend2(encoder, selSetFragSampler, uintptr(sampler), 0)
			lastSampler = sampler
		}
		msgSend3(encoder, selDrawPrimitives, metalTriangle, uintptr(first), uintptr(count))
		drawCalls++
	}
	b.stats.Batches = drawCalls
	msgSend0(encoder, selEndEncoding)
	msgSend1(command, selPresentDrawable, uintptr(drawable))
	slot.command = msgSend0(command, selRetain)
	msgSend0(command, selCommit)
	b.trimImages()
	b.stats.Submit = time.Since(start)
	return nil
}

var _ render.Backend = (*metalBackend)(nil)
