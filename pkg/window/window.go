package window

import (
	"errors"

	"github.com/Zyko0/go-sdl3/sdl"
)

type Window struct {
	handle     *sdl.Window
	device     *sdl.GPUDevice
	width      int
	height     int
	title      string
	fullscreen bool
}

type Config struct {
	Title  string
	Width  int
	Height int
}

func New(cfg Config) (*Window, error) {
	device, err := sdl.CreateGPUDevice(
		sdl.GPU_SHADERFORMAT_SPIRV|sdl.GPU_SHADERFORMAT_DXIL|sdl.GPU_SHADERFORMAT_MSL,
		true,
		"",
	)
	if err != nil {
		return nil, errors.New("failed to create GPU device: " + err.Error())
	}

	handle, err := sdl.CreateWindow(cfg.Title, cfg.Width, cfg.Height, sdl.WINDOW_RESIZABLE)
	if err != nil {
		device.Destroy()
		return nil, errors.New("failed to create window: " + err.Error())
	}

	err = device.ClaimWindow(handle)
	if err != nil {
		handle.Destroy()
		device.Destroy()
		return nil, errors.New("failed to claim window: " + err.Error())
	}

	return &Window{
		handle: handle,
		device: device,
		width:  cfg.Width,
		height: cfg.Height,
		title:  cfg.Title,
	}, nil
}

func (w *Window) Handle() *sdl.Window {
	return w.handle
}

func (w *Window) Device() *sdl.GPUDevice {
	return w.device
}

func (w *Window) Width() int {
	return w.width
}

func (w *Window) Height() int {
	return w.height
}

func (w *Window) IsFullscreen() bool {
	return w.fullscreen
}

func (w *Window) SetFullscreen(fullscreen bool) error {
	if fullscreen == w.fullscreen {
		return nil
	}
	if err := w.handle.SetFullscreen(fullscreen); err != nil {
		return err
	}
	w.fullscreen = fullscreen

	if fullscreen {
		// Use the display's native resolution
		displayID := sdl.GetDisplayForWindow(w.handle)
		mode, err := displayID.DesktopDisplayMode()
		if err == nil {
			w.width = int(mode.W)
			w.height = int(mode.H)
		}
	}
	return nil
}

func (w *Window) SetSize(width, height int) error {
	if w.fullscreen {
		return nil
	}
	if err := w.handle.SetSize(int32(width), int32(height)); err != nil {
		return err
	}
	w.width = width
	w.height = height
	return nil
}

func (w *Window) SetRelativeMouseMode(enabled bool) error {
	return w.handle.SetRelativeMouseMode(enabled)
}

func (w *Window) Destroy() {
	w.device.ReleaseWindow(w.handle)
	w.handle.Destroy()
	w.device.Destroy()
}
