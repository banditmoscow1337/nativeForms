package software

import (
	"image"
	"testing"

	"github.com/banditmoscow1337/nativeForms/internal/renderparity"
)

func TestRenderParityGoldenScenes(t *testing.T) {
	for _, scene := range renderparity.Scenes() {
		t.Run(scene.Name, func(t *testing.T) {
			dst := image.NewRGBA(image.Rect(0, 0, scene.Frame.Width, scene.Frame.Height))
			if err := New().Render(scene.Frame, dst); err != nil {
				t.Fatal(err)
			}
			assertPixels(t, dst, scene.Expected...)
		})
	}
}
