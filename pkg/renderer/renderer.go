package renderer

import (
	"errors"
	"unsafe"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/window"
	"github.com/anthonyrego/construct/shaders"
)

type Vertex struct {
	X, Y, Z    float32
	R, G, B, A uint8
}

func NewVertex(x, y, z float32, r, g, b, a uint8) Vertex {
	return Vertex{X: x, Y: y, Z: z, R: r, G: g, B: b, A: a}
}

type Renderer struct {
	window   *window.Window
	pipeline *sdl.GPUGraphicsPipeline

	depthTexture *sdl.GPUTexture
}

func New(w *window.Window) (*Renderer, error) {
	device := w.Device()

	// Create shaders
	vertexShader, err := shaders.LoadShader(device, "PositionColorTransform.vert", 0, 1, 0, 0)
	if err != nil {
		return nil, errors.New("failed to create vertex shader: " + err.Error())
	}
	defer device.ReleaseShader(vertexShader)

	fragmentShader, err := shaders.LoadShader(device, "SolidColor.frag", 0, 0, 0, 0)
	if err != nil {
		return nil, errors.New("failed to create fragment shader: " + err.Error())
	}
	defer device.ReleaseShader(fragmentShader)

	// Create pipeline
	colorTargetDescriptions := []sdl.GPUColorTargetDescription{
		{
			Format: device.SwapchainTextureFormat(w.Handle()),
		},
	}

	vertexBufferDescriptions := []sdl.GPUVertexBufferDescription{
		{
			Slot:             0,
			InputRate:        sdl.GPU_VERTEXINPUTRATE_VERTEX,
			InstanceStepRate: 0,
			Pitch:            uint32(unsafe.Sizeof(Vertex{})),
		},
	}

	vertexAttributes := []sdl.GPUVertexAttribute{
		{
			BufferSlot: 0,
			Format:     sdl.GPU_VERTEXELEMENTFORMAT_FLOAT3,
			Location:   0,
			Offset:     0,
		},
		{
			BufferSlot: 0,
			Format:     sdl.GPU_VERTEXELEMENTFORMAT_UBYTE4_NORM,
			Location:   1,
			Offset:     uint32(unsafe.Sizeof(float32(0)) * 3),
		},
	}

	pipelineCreateInfo := sdl.GPUGraphicsPipelineCreateInfo{
		TargetInfo: sdl.GPUGraphicsPipelineTargetInfo{
			ColorTargetDescriptions: colorTargetDescriptions,
			HasDepthStencilTarget:   true,
			DepthStencilFormat:      sdl.GPU_TEXTUREFORMAT_D32_FLOAT,
		},
		DepthStencilState: sdl.GPUDepthStencilState{
			EnableDepthTest:  true,
			EnableDepthWrite: true,
			CompareOp:        sdl.GPU_COMPAREOP_LESS,
		},
		VertexInputState: sdl.GPUVertexInputState{
			VertexBufferDescriptions: vertexBufferDescriptions,
			VertexAttributes:         vertexAttributes,
		},
		RasterizerState: sdl.GPURasterizerState{
			FillMode: sdl.GPU_FILLMODE_FILL,
			CullMode: sdl.GPU_CULLMODE_BACK,
		},
		PrimitiveType:  sdl.GPU_PRIMITIVETYPE_TRIANGLELIST,
		VertexShader:   vertexShader,
		FragmentShader: fragmentShader,
	}

	pipeline, err := device.CreateGraphicsPipeline(&pipelineCreateInfo)
	if err != nil {
		return nil, errors.New("failed to create pipeline: " + err.Error())
	}

	// Create depth texture
	depthTexture, err := device.CreateTexture(&sdl.GPUTextureCreateInfo{
		Type:              sdl.GPU_TEXTURETYPE_2D,
		Format:            sdl.GPU_TEXTUREFORMAT_D32_FLOAT,
		Width:             uint32(w.Width()),
		Height:            uint32(w.Height()),
		LayerCountOrDepth: 1,
		NumLevels:         1,
		Usage:             sdl.GPU_TEXTUREUSAGE_DEPTH_STENCIL_TARGET,
	})
	if err != nil {
		device.ReleaseGraphicsPipeline(pipeline)
		return nil, errors.New("failed to create depth texture: " + err.Error())
	}

	return &Renderer{
		window:       w,
		pipeline:     pipeline,
		depthTexture: depthTexture,
	}, nil
}

func (r *Renderer) CreateVertexBuffer(vertices []Vertex) (*sdl.GPUBuffer, error) {
	device := r.window.Device()

	bufferSize := uint32(len(vertices)) * uint32(unsafe.Sizeof(Vertex{}))

	buffer, err := device.CreateBuffer(&sdl.GPUBufferCreateInfo{
		Usage: sdl.GPU_BUFFERUSAGE_VERTEX,
		Size:  bufferSize,
	})
	if err != nil {
		return nil, errors.New("failed to create vertex buffer: " + err.Error())
	}

	// Create transfer buffer and upload data
	transferBuffer, err := device.CreateTransferBuffer(&sdl.GPUTransferBufferCreateInfo{
		Usage: sdl.GPU_TRANSFERBUFFERUSAGE_UPLOAD,
		Size:  bufferSize,
	})
	if err != nil {
		device.ReleaseBuffer(buffer)
		return nil, errors.New("failed to create transfer buffer: " + err.Error())
	}

	transferDataPtr, err := device.MapTransferBuffer(transferBuffer, false)
	if err != nil {
		device.ReleaseBuffer(buffer)
		device.ReleaseTransferBuffer(transferBuffer)
		return nil, errors.New("failed to map transfer buffer: " + err.Error())
	}

	vertexData := unsafe.Slice((*Vertex)(unsafe.Pointer(transferDataPtr)), len(vertices))
	copy(vertexData, vertices)

	device.UnmapTransferBuffer(transferBuffer)

	// Upload to GPU
	cmdBuf, err := device.AcquireCommandBuffer()
	if err != nil {
		device.ReleaseBuffer(buffer)
		device.ReleaseTransferBuffer(transferBuffer)
		return nil, errors.New("failed to acquire command buffer: " + err.Error())
	}

	copyPass := cmdBuf.BeginCopyPass()
	copyPass.UploadToGPUBuffer(
		&sdl.GPUTransferBufferLocation{
			TransferBuffer: transferBuffer,
			Offset:         0,
		},
		&sdl.GPUBufferRegion{
			Buffer: buffer,
			Offset: 0,
			Size:   bufferSize,
		},
		false,
	)
	copyPass.End()
	cmdBuf.Submit()

	device.ReleaseTransferBuffer(transferBuffer)

	return buffer, nil
}

func (r *Renderer) CreateIndexBuffer(indices []uint16) (*sdl.GPUBuffer, error) {
	device := r.window.Device()

	bufferSize := uint32(len(indices)) * uint32(unsafe.Sizeof(uint16(0)))

	buffer, err := device.CreateBuffer(&sdl.GPUBufferCreateInfo{
		Usage: sdl.GPU_BUFFERUSAGE_INDEX,
		Size:  bufferSize,
	})
	if err != nil {
		return nil, errors.New("failed to create index buffer: " + err.Error())
	}

	transferBuffer, err := device.CreateTransferBuffer(&sdl.GPUTransferBufferCreateInfo{
		Usage: sdl.GPU_TRANSFERBUFFERUSAGE_UPLOAD,
		Size:  bufferSize,
	})
	if err != nil {
		device.ReleaseBuffer(buffer)
		return nil, errors.New("failed to create transfer buffer: " + err.Error())
	}

	transferDataPtr, err := device.MapTransferBuffer(transferBuffer, false)
	if err != nil {
		device.ReleaseBuffer(buffer)
		device.ReleaseTransferBuffer(transferBuffer)
		return nil, errors.New("failed to map transfer buffer: " + err.Error())
	}

	indexData := unsafe.Slice((*uint16)(unsafe.Pointer(transferDataPtr)), len(indices))
	copy(indexData, indices)

	device.UnmapTransferBuffer(transferBuffer)

	cmdBuf, err := device.AcquireCommandBuffer()
	if err != nil {
		device.ReleaseBuffer(buffer)
		device.ReleaseTransferBuffer(transferBuffer)
		return nil, errors.New("failed to acquire command buffer: " + err.Error())
	}

	copyPass := cmdBuf.BeginCopyPass()
	copyPass.UploadToGPUBuffer(
		&sdl.GPUTransferBufferLocation{
			TransferBuffer: transferBuffer,
			Offset:         0,
		},
		&sdl.GPUBufferRegion{
			Buffer: buffer,
			Offset: 0,
			Size:   bufferSize,
		},
		false,
	)
	copyPass.End()
	cmdBuf.Submit()

	device.ReleaseTransferBuffer(transferBuffer)

	return buffer, nil
}

type DrawCall struct {
	VertexBuffer *sdl.GPUBuffer
	IndexBuffer  *sdl.GPUBuffer
	IndexCount   uint32
	Transform    mgl32.Mat4
}

func (r *Renderer) BeginFrame() (*sdl.GPUCommandBuffer, *sdl.GPURenderPass, error) {
	device := r.window.Device()

	cmdBuf, err := device.AcquireCommandBuffer()
	if err != nil {
		return nil, nil, errors.New("failed to acquire command buffer: " + err.Error())
	}

	swapchainTexture, err := cmdBuf.WaitAndAcquireGPUSwapchainTexture(r.window.Handle())
	if err != nil {
		return nil, nil, errors.New("failed to acquire swapchain texture: " + err.Error())
	}

	if swapchainTexture == nil {
		cmdBuf.Submit()
		return nil, nil, nil
	}

	colorTargetInfo := sdl.GPUColorTargetInfo{
		Texture:    swapchainTexture.Texture,
		ClearColor: sdl.FColor{R: 0.1, G: 0.1, B: 0.15, A: 1.0},
		LoadOp:     sdl.GPU_LOADOP_CLEAR,
		StoreOp:    sdl.GPU_STOREOP_STORE,
	}

	depthStencilTargetInfo := sdl.GPUDepthStencilTargetInfo{
		Texture:        r.depthTexture,
		ClearDepth:     1.0,
		LoadOp:         sdl.GPU_LOADOP_CLEAR,
		StoreOp:        sdl.GPU_STOREOP_DONT_CARE,
		StencilLoadOp:  sdl.GPU_LOADOP_DONT_CARE,
		StencilStoreOp: sdl.GPU_STOREOP_DONT_CARE,
		Cycle:          true,
		ClearStencil:   0,
	}

	renderPass := cmdBuf.BeginRenderPass(
		[]sdl.GPUColorTargetInfo{colorTargetInfo},
		&depthStencilTargetInfo,
	)

	renderPass.BindGraphicsPipeline(r.pipeline)

	return cmdBuf, renderPass, nil
}

func (r *Renderer) Draw(cmdBuf *sdl.GPUCommandBuffer, renderPass *sdl.GPURenderPass, call DrawCall) {
	// Push MVP matrix
	cmdBuf.PushVertexUniformData(0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&call.Transform)), unsafe.Sizeof(call.Transform),
	))

	renderPass.BindVertexBuffers([]sdl.GPUBufferBinding{
		{Buffer: call.VertexBuffer, Offset: 0},
	})

	renderPass.BindIndexBuffer(&sdl.GPUBufferBinding{
		Buffer: call.IndexBuffer, Offset: 0,
	}, sdl.GPU_INDEXELEMENTSIZE_16BIT)

	renderPass.DrawIndexedPrimitives(call.IndexCount, 1, 0, 0, 0)
}

func (r *Renderer) EndFrame(cmdBuf *sdl.GPUCommandBuffer, renderPass *sdl.GPURenderPass) {
	renderPass.End()
	cmdBuf.Submit()
}

func (r *Renderer) ReleaseBuffer(buffer *sdl.GPUBuffer) {
	r.window.Device().ReleaseBuffer(buffer)
}

func (r *Renderer) Destroy() {
	device := r.window.Device()
	device.ReleaseTexture(r.depthTexture)
	device.ReleaseGraphicsPipeline(r.pipeline)
}
