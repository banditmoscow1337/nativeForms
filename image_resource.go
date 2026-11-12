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
	"sync"
	"sync/atomic"
)

var (
	nextImageID        atomic.Uint64
	sharedDarkChkOnce  sync.Once
	sharedDarkChkImage *ImageResource
)

// SharedCheckerboardResource returns a shared 16x16 dark transparency
// checkerboard texture suitable for pixel-art and sprite editors.
func SharedCheckerboardResource() *ImageResource {
	sharedDarkChkOnce.Do(func() {
		const size = 16
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		c1 := color.RGBA{R: 0x1b, G: 0x24, B: 0x30, A: 0xff}
		c2 := color.RGBA{R: 0x25, G: 0x30, B: 0x3d, A: 0xff}
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				if ((x/8)+(y/8))%2 == 0 {
					img.SetRGBA(x, y, c2)
				} else {
					img.SetRGBA(x, y, c1)
				}
			}
		}
		sharedDarkChkImage, _ = NewImageResource(img)
	})
	return sharedDarkChkImage
}

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

// NewBlankImageResource allocates a transparent ImageResource of width x height
// ready for zero-allocation in-place pixel updates via UpdateNRGBA or UpdateRGBA.
func NewBlankImageResource(width, height int) (*ImageResource, error) {
	if err := validateImageDimensions(width, height); err != nil {
		return nil, err
	}
	resource := &ImageResource{
		id:       nextImageID.Add(1),
		version:  1,
		width:    width,
		height:   height,
		pixels:   make([]byte, width*height*4),
		versions: new(atomic.Uint64),
	}
	resource.versions.Store(1)
	return resource, nil
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

// UpdateRGBA updates the ImageResource in-place from premultiplied RGBA pixels
// without allocating when dimensions match, and increments Version().
func (resource *ImageResource) UpdateRGBA(width, height int, rgba []byte) error {
	if resource == nil || resource.versions == nil {
		return fmt.Errorf("cannot update an uninitialized image resource")
	}
	if err := validateImageDimensions(width, height); err != nil {
		return err
	}
	needed := width * height * 4
	if len(rgba) < needed {
		return fmt.Errorf("pixel buffer too short: got %d bytes, need %d", len(rgba), needed)
	}
	if cap(resource.pixels) < needed {
		resource.pixels = make([]byte, needed)
	} else {
		resource.pixels = resource.pixels[:needed]
	}
	copy(resource.pixels, rgba[:needed])
	resource.width = width
	resource.height = height
	resource.version = resource.versions.Add(1)
	return nil
}

// UpdateNRGBA updates the ImageResource in-place from straight-alpha NRGBA
// pixels, converting to premultiplied RGBA without allocating when dimensions
// match, and increments Version().
func (resource *ImageResource) UpdateNRGBA(width, height int, nrgba []byte) error {
	if resource == nil || resource.versions == nil {
		return fmt.Errorf("cannot update an uninitialized image resource")
	}
	if err := validateImageDimensions(width, height); err != nil {
		return err
	}
	needed := width * height * 4
	if len(nrgba) < needed {
		return fmt.Errorf("pixel buffer too short: got %d bytes, need %d", len(nrgba), needed)
	}
	if cap(resource.pixels) < needed {
		resource.pixels = make([]byte, needed)
	} else {
		resource.pixels = resource.pixels[:needed]
	}
	dst := resource.pixels
	src := nrgba[:needed]
	for i := 0; i < needed; i += 4 {
		a := uint32(src[i+3])
		switch a {
		case 255:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i], src[i+1], src[i+2], 255
		case 0:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = 0, 0, 0, 0
		default:
			dst[i] = byte((uint32(src[i])*a + 127) / 255)
			dst[i+1] = byte((uint32(src[i+1])*a + 127) / 255)
			dst[i+2] = byte((uint32(src[i+2])*a + 127) / 255)
			dst[i+3] = byte(a)
		}
	}
	resource.width = width
	resource.height = height
	resource.version = resource.versions.Add(1)
	return nil
}

// UpdateImage updates the ImageResource in-place from an image.Image and bumps
// its Version() without allocating a new pixel buffer when dimensions match.
func (resource *ImageResource) UpdateImage(source image.Image) error {
	if resource == nil || resource.versions == nil {
		return fmt.Errorf("cannot update an uninitialized image resource")
	}
	if source == nil {
		return fmt.Errorf("image source is nil")
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if err := validateImageDimensions(width, height); err != nil {
		return err
	}
	needed := width * height * 4
	if cap(resource.pixels) < needed {
		resource.pixels = make([]byte, needed)
	} else {
		resource.pixels = resource.pixels[:needed]
	}
	copyPixelsFromImage(resource.pixels, width, height, source)
	resource.width = width
	resource.height = height
	resource.version = resource.versions.Add(1)
	return nil
}

func validateImageDimensions(width, height int) error {
	if width <= 0 || height <= 0 || width > 16384 || height > 16384 ||
		int64(width)*int64(height) > 64*1024*1024 {
		return fmt.Errorf("invalid image dimensions: %d x %d", width, height)
	}
	return nil
}

func copyPixelsFromImage(pixels []byte, width, height int, source image.Image) {
	bounds := source.Bounds()
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
				switch a {
				case 255:
					dst[i], dst[i+1], dst[i+2], dst[i+3] = row[i], row[i+1], row[i+2], 255
				case 0:
					dst[i], dst[i+1], dst[i+2], dst[i+3] = 0, 0, 0, 0
				default:
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
		clear(pixels)
		dstImg := &image.RGBA{Pix: pixels, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
		draw.Draw(dstImg, dstImg.Bounds(), source, bounds.Min, draw.Src)
	}
}

func copyImageResource(source image.Image) (*ImageResource, error) {
	if source == nil {
		return nil, fmt.Errorf("image source is nil")
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if err := validateImageDimensions(width, height); err != nil {
		return nil, err
	}
	pixels := make([]byte, width*height*4)
	copyPixelsFromImage(pixels, width, height, source)
	return &ImageResource{width: width, height: height, pixels: pixels}, nil
}

func (resource *ImageResource) ID() uint64      { return resource.id }
func (resource *ImageResource) Version() uint64 { return resource.version }
func (resource *ImageResource) Width() int {
	if resource == nil {
		return 0
	}
	return resource.width
}
func (resource *ImageResource) Height() int {
	if resource == nil {
		return 0
	}
	return resource.height
}
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
