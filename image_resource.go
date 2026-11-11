package nativeforms

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"sync/atomic"
)

var nextImageID atomic.Uint64

// DecodeImageResource reads a PNG, JPEG or GIF image into an immutable RGBA
// snapshot. For animated GIFs it uses the first frame.
func DecodeImageResource(reader io.Reader) (*ImageResource, error) {
	if reader == nil {
		return nil, fmt.Errorf("image reader is nil")
	}
	source, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}
	return NewImageResource(source)
}

type ImageResource struct {
	id, version   uint64
	width, height int
	pixels        []byte
	versions      *atomic.Uint64
}

func NewImageResource(source image.Image) (*ImageResource, error) {
	resource, err := copyImageResource(source)
	if err != nil {
		return nil, err
	}
	resource.id = nextImageID.Add(1)
	resource.version = 1
	resource.versions = new(atomic.Uint64)
	resource.versions.Store(1)
	return resource, nil
}

func (resource *ImageResource) Replace(source image.Image) (*ImageResource, error) {
	if resource == nil || resource.versions == nil {
		return nil, fmt.Errorf("cannot replace an uninitialized image resource")
	}
	replacement, err := copyImageResource(source)
	if err != nil {
		return nil, err
	}
	replacement.id, replacement.version = resource.id, resource.versions.Add(1)
	replacement.versions = resource.versions
	return replacement, nil
}

func copyImageResource(source image.Image) (*ImageResource, error) {
	if source == nil {
		return nil, fmt.Errorf("image source is nil")
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || width > 16384 || height > 16384 ||
		int64(width)*int64(height) > 64*1024*1024 {
		return nil, fmt.Errorf("invalid image dimensions: %d x %d", width, height)
	}
	pixels := make([]byte, width*height*4)
	switch src := source.(type) {
	case *image.RGBA:
		for y := 0; y < height; y++ {
			so := src.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			do := y * width * 4
			copy(pixels[do:do+width*4], src.Pix[so:so+width*4])
		}
	case *image.NRGBA:
		for y := 0; y < height; y++ {
			so := src.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			do := y * width * 4
			row := src.Pix[so : so+width*4]
			dst := pixels[do : do+width*4]
			for i := 0; i < len(row); i += 4 {
				a := uint32(row[i+3])
				if a == 255 {
					dst[i], dst[i+1], dst[i+2], dst[i+3] = row[i], row[i+1], row[i+2], 255
				} else if a != 0 {
					dst[i] = byte((uint32(row[i])*a + 127) / 255)
					dst[i+1] = byte((uint32(row[i+1])*a + 127) / 255)
					dst[i+2] = byte((uint32(row[i+2])*a + 127) / 255)
					dst[i+3] = byte(a)
				}
			}
		}
	case *ImageResource:
		if bounds.Min == (image.Point{}) && src.width == width && src.height == height {
			copy(pixels, src.pixels)
		} else {
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					c := src.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
					i := (y*width + x) * 4
					pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = c.R, c.G, c.B, c.A
				}
			}
		}
	default:
		dstImg := &image.RGBA{Pix: pixels, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
		draw.Draw(dstImg, dstImg.Bounds(), source, bounds.Min, draw.Src)
	}
	return &ImageResource{width: width, height: height, pixels: pixels}, nil
}

func (resource *ImageResource) ID() uint64      { return resource.id }
func (resource *ImageResource) Version() uint64 { return resource.version }
func (resource *ImageResource) Bounds() image.Rectangle {
	if resource == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, resource.width, resource.height)
}

// Pixels returns the underlying premultiplied RGBA pixel buffer for zero-copy
// GPU texture upload. Callers must not mutate the returned slice.
func (resource *ImageResource) Pixels() []byte {
	if resource == nil {
		return nil
	}
	return resource.pixels
}

func (resource *ImageResource) RGBAAt(x, y int) color.RGBA {
	if resource == nil || x < 0 || y < 0 || x >= resource.width || y >= resource.height {
		return color.RGBA{}
	}
	i := (y*resource.width + x) * 4
	return color.RGBA{R: resource.pixels[i], G: resource.pixels[i+1], B: resource.pixels[i+2], A: resource.pixels[i+3]}
}

// ImageResource implements image.Image so backends can upload its immutable
// premultiplied pixels without allocating a copy on every frame.
func (resource *ImageResource) ColorModel() color.Model { return color.RGBAModel }

func (resource *ImageResource) At(x, y int) color.Color { return resource.RGBAAt(x, y) }
