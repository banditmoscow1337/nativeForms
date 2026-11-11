//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"image"
	"structs"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/ebitengine/purego/objc"
)

const (
	metalA8              = 1
	metalBGRA8           = 80
	metalRGBA8           = 70
	metalTriangle        = 3
	metalBlendOne        = 1
	metalBlendOneMinusA  = 5
	metalAtlasSize       = 512
	metalTextureBudget   = 4 << 20
	metalInFlight        = 2
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
    float texAlpha = (v.uv.x < (1.0 / 512.0) && v.uv.y < (1.0 / 512.0))
                     ? 1.0
                     : tex.sample(sampleState, v.uv).a;
    float alpha = texAlpha * v.color.a;
    return float4(v.color.rgb * alpha, alpha);
}
fragment float4 nativeImage(Varying v [[stage_in]],
                             texture2d<float> tex [[texture(0)]],
                             sampler sampleState [[sampler(0)]]) {
    return tex.sample(sampleState, v.uv) * v.color;
}`

type metalSlot struct {
	buffer, color, command  objc.ID
	contents                unsafe.Pointer
	capacity, width, height int
}

type metalImage struct {
	texture       objc.ID
	width, height int
	bytes         int64
	last          uint64
}

func metalError(prefix string, native objc.ID) error {
	if native != 0 {
		return fmt.Errorf("darwin: Metal %s: %s", prefix, stringValue(native.Send(selector("localizedDescription"))))
	}
	return fmt.Errorf("darwin: Metal %s: %w", prefix, render.ErrUnavailable)
}

func (b *metalBackend) newTexture(width, height int) (objc.ID, error) {
	return b.newTextureWithFormat(metalRGBA8, width, height)
}

func (b *metalBackend) newTextureWithFormat(format, width, height int) (objc.ID, error) {
	desc := class("MTLTextureDescriptor").Send(selector("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"),
		uint64(format), uint64(width), uint64(height), false)
	if desc == 0 {
		return 0, metalError("texture descriptor", 0)
	}
	texture := b.device.Send(selector("newTextureWithDescriptor:"), desc)
	if texture == 0 {
		return 0, metalError("texture allocation", 0)
	}
	return texture, nil
}

func (b *metalBackend) ensureFullAtlas() error {
	if b.atlasLoaded {
		return nil
	}
	font := ui.FontAtlas()
	tex, err := b.newTexture(font.Bounds().Dx(), font.Bounds().Dy())
	if err != nil {
		return err
	}
	if err := uploadMetalImage(tex, font); err != nil {
		tex.Send(selRelease)
		return err
	}
	if b.atlas != 0 {
		b.atlas.Send(selRelease)
	}
	b.atlas = tex
	b.atlasLoaded = true
	return nil
}

// replaceRegion copies its source synchronously. Keep the slice alive across
// the native call; Metal's private texture remains valid after it returns.
func uploadMetalPixels(texture objc.ID, region metalRegion, pixels []byte, rowBytes int) {
	if texture == 0 || len(pixels) == 0 {
		return
	}
	ensureObjcMsgSend()
	if selReplaceRegion == 0 {
		selReplaceRegion = selector("replaceRegion:mipmapLevel:withBytes:bytesPerRow:")
	}
	if objcMsgSendRegion != nil {
		objcMsgSendRegion(texture, selReplaceRegion, region, 0, unsafe.Pointer(&pixels[0]), uint64(rowBytes))
		return
	}
	texture.Send(selReplaceRegion,
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
	if res, ok := source.(*ui.ImageResource); ok {
		if raw := res.Pixels(); len(raw) == b.Dx()*b.Dy()*4 {
			uploadMetalPixels(texture, metalRegion{W: uint64(b.Dx()), H: uint64(b.Dy()), Depth: 1}, raw, b.Dx()*4)
			return nil
		}
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

func (b *metalBackend) uploadMasks() {
	updates := b.masks.UpdatesSince(b.maskUpdates)
	if len(updates) == 0 {
		return
	}
	if dirty, sheet, stride := b.masks.TakeDirtySheet(); !dirty.Empty() && len(sheet) > 0 {
		offset := dirty.Min.Y*stride + dirty.Min.X
		uploadMetalPixels(b.maskTex, metalRegion{
			X: uint64(dirty.Min.X), Y: uint64(dirty.Min.Y),
			W: uint64(dirty.Dx()), H: uint64(dirty.Dy()), Depth: 1,
		}, sheet[offset:], stride)
		b.stats.TextureUploads++
		b.stats.UploadBytes += int64(dirty.Dx() * dirty.Dy())
		b.maskUpdates = b.masks.UpdateCount()
		return
	}
	for i := range updates {
		r, mask := updates[i].Rect, updates[i].Mask
		if r.Empty() || mask == nil {
			continue
		}
		offset := mask.PixOffset(mask.Rect.Min.X, mask.Rect.Min.Y)
		uploadMetalPixels(b.maskTex, metalRegion{
			X: uint64(r.Min.X), Y: uint64(r.Min.Y),
			W: uint64(r.Dx()), H: uint64(r.Dy()), Depth: 1,
		}, mask.Pix[offset:], mask.Stride)
		b.stats.TextureUploads++
		b.stats.UploadBytes += int64(r.Dx() * r.Dy())
	}
	b.maskUpdates = b.masks.UpdateCount()
}

func (b *metalBackend) imageTexture(batch render.Batch) (objc.ID, error) {
	if batch.Image == nil {
		return 0, fmt.Errorf("darwin: image batch has no resource")
	}
	if entry, ok := b.images[batch.Texture]; ok {
		if entry.last != b.serial {
			entry.last = b.serial
			b.images[batch.Texture] = entry
		}
		return entry.texture, nil
	}
	bounds := batch.Image.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	// Reuse an older retired version of the same ImageResource ID in-place when
	// dimensions match so dynamic widgets (like ColorPicker) do not allocate new
	// GPU textures on every hue change.
	for oldKey, oldItem := range b.images {
		if oldKey.ID == batch.Texture.ID && oldKey.Version < batch.Texture.Version &&
			oldItem.last+metalInFlight <= b.serial && oldItem.width == w && oldItem.height == h {
			if err := uploadMetalImage(oldItem.texture, batch.Image); err == nil {
				delete(b.images, oldKey)
				oldItem.last = b.serial
				b.images[batch.Texture] = oldItem
				b.stats.TextureUploads++
				b.stats.UploadBytes += int64(w) * int64(h) * 4
				return oldItem.texture, nil
			}
		}
	}
	texture, err := b.newTexture(w, h)
	if err != nil {
		return 0, err
	}
	if err := uploadMetalImage(texture, batch.Image); err != nil {
		texture.Send(selRelease)
		return 0, err
	}
	// Account for 16 KiB Apple Silicon GPU page alignment so tiny dynamic
	// textures cannot accumulate hundreds of uncollected Metal allocations.
	bytes := max(int64(16<<10), int64(w)*int64(h)*4)
	b.images[batch.Texture] = metalImage{texture: texture, width: w, height: h, bytes: bytes, last: b.serial}
	b.used += bytes
	b.imagesDirty = true
	b.stats.TextureUploads++
	b.stats.UploadBytes += int64(w) * int64(h) * 4
	return texture, nil
}

func (b *metalBackend) trimImages() {
	if !b.imagesDirty && b.used <= metalTextureBudget {
		return
	}
	b.imagesDirty = false
	// Immediately evict superseded ImageResource versions that are no longer in flight.
	for key, item := range b.images {
		if item.last+metalInFlight > b.serial {
			continue
		}
		for otherKey := range b.images {
			if otherKey.ID == key.ID && otherKey.Version > key.Version {
				item.texture.Send(selRelease)
				b.used -= item.bytes
				delete(b.images, key)
				break
			}
		}
	}
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
		item.texture.Send(selRelease)
		b.used -= item.bytes
		delete(b.images, oldest)
	}
}
