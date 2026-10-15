package render

import (
	"errors"
	"fmt"
	"image"

	ui "github.com/banditmoscow1337/nativeForms"
)

type Kind string

const (
	Auto     Kind = "auto"
	Software Kind = "software"
	Hardware Kind = "hardware"
)

var ErrUnavailable = errors.New("render: backend unavailable")
var ErrDeviceLost = errors.New("render: device lost")

// Backend owns its rendering state. RenderAtScale consumes a synchronous
// Frame; asynchronous backends should Freeze it before returning to the UI.
type Backend interface {
	Kind() Kind
	RenderAtScale(ui.Frame, *image.RGBA, float32) error
	Close() error
}

type Constructor func() (Backend, error)

type Selection struct {
	Backend  Backend
	Fallback error
}

func (selection Selection) FallbackReason() string {
	if selection.Fallback == nil {
		return ""
	}
	return selection.Fallback.Error()
}

// Factory negotiates a desktop renderer after a surface exists.
type Factory struct{ Software, Hardware Constructor }

// Failover switches a failed hardware backend to software. The caller renders
// the same frame again on the returned backend, then keeps it for future frames.
func Failover(current Backend, failure error, software Constructor) (Backend, error) {
	if failure == nil || current == nil || current.Kind() != Hardware {
		return nil, failure
	}
	_ = current.Close()
	if software == nil {
		return nil, errors.Join(failure, ErrUnavailable)
	}
	fallback, err := software()
	if err != nil {
		return nil, errors.Join(failure, err)
	}
	if fallback == nil || fallback.Kind() != Software {
		if fallback != nil {
			_ = fallback.Close()
		}
		return nil, errors.Join(failure, ErrUnavailable)
	}
	return fallback, nil
}

func (factory Factory) Open(preference Kind) (Selection, error) {
	try := func(create Constructor) (Backend, error) {
		if create == nil {
			return nil, ErrUnavailable
		}
		backend, err := create()
		if err != nil {
			return nil, err
		}
		if backend == nil {
			return nil, ErrUnavailable
		}
		return backend, nil
	}
	switch preference {
	case Software:
		backend, err := try(factory.Software)
		return Selection{Backend: backend}, err
	case Hardware, Auto, "":
		backend, err := try(factory.Hardware)
		if err == nil {
			return Selection{Backend: backend}, nil
		}
		if preference == Hardware {
			return Selection{}, err
		}
		fallback, fallbackErr := try(factory.Software)
		if fallbackErr != nil {
			return Selection{}, errors.Join(err, fallbackErr)
		}
		return Selection{Backend: fallback, Fallback: err}, nil
	default:
		return Selection{}, fmt.Errorf("render: unknown backend %q", preference)
	}
}
