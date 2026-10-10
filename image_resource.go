package nativeforms

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sync/atomic"
)

var nextImageID atomic.Uint64

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
	copy := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(copy, copy.Bounds(), source, bounds.Min, draw.Src)
	return &ImageResource{width: width, height: height, pixels: copy.Pix}, nil
}

func (resource *ImageResource) ID() uint64      { return resource.id }
func (resource *ImageResource) Version() uint64 { return resource.version }
func (resource *ImageResource) Bounds() image.Rectangle {
	if resource == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, resource.width, resource.height)
}

func (resource *ImageResource) RGBAAt(x, y int) color.RGBA {
	if resource == nil || x < 0 || y < 0 || x >= resource.width || y >= resource.height {
		return color.RGBA{}
	}
	i := (y*resource.width + x) * 4
	return color.RGBA{R: resource.pixels[i], G: resource.pixels[i+1], B: resource.pixels[i+2], A: resource.pixels[i+3]}
}
