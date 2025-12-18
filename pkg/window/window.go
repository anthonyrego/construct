package window

import (
	"errors"

	"github.com/Zyko0/go-sdl3/sdl"
)

type Window struct {
	handle *sdl.Window
	device *sdl.GPUDevice
	width  int
	height int
	title  string
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

func (w *Window) SetRelativeMouseMode(enabled bool) error {
	return w.handle.SetRelativeMouseMode(enabled)
}

func (w *Window) Destroy() {
	w.device.ReleaseWindow(w.handle)
	w.handle.Destroy()
	w.device.Destroy()
}
