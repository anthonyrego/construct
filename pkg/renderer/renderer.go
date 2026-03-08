package renderer

import (
	"errors"
	"unsafe"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/window"
	"github.com/anthonyrego/construct/shaders"
)

const (
	offscreenWidth  = 320
	offscreenHeight = 180
)

type Vertex struct {
	X, Y, Z    float32
	R, G, B, A uint8
}

func NewVertex(x, y, z float32, r, g, b, a uint8) Vertex {
	return Vertex{X: x, Y: y, Z: z, R: r, G: g, B: b, A: a}
}

type LitVertex struct {
	X, Y, Z    float32 // location 0: FLOAT3
	NX, NY, NZ float32 // location 1: FLOAT3
	R, G, B, A uint8   // location 2: UBYTE4_NORM
}

type DrawCall struct {
	VertexBuffer *sdl.GPUBuffer
	IndexBuffer  *sdl.GPUBuffer
	IndexCount   uint32
	Transform    mgl32.Mat4
}

type LitDrawCall struct {
	VertexBuffer *sdl.GPUBuffer
	IndexBuffer  *sdl.GPUBuffer
	IndexCount   uint32
	MVP          mgl32.Mat4
	Model        mgl32.Mat4
}

type LitVertexUniforms struct {
	MVP   mgl32.Mat4
	Model mgl32.Mat4
}

type LightUniforms struct {
	LightPositions [4]mgl32.Vec4 // xyz=pos, w=unused
	LightColors    [4]mgl32.Vec4 // rgb=color, a=intensity
	AmbientColor   mgl32.Vec4
	CameraPos      mgl32.Vec4
	NumLights      mgl32.Vec4 // x=count
}

type PostProcessUniforms struct {
	Resolution mgl32.Vec4 // xy = offscreen size
	Dither     mgl32.Vec4 // x = strength (0=off, 1=full), y = color levels
	Tint       mgl32.Vec4 // rgb = per-channel multiplier
}

type Renderer struct {
	window   *window.Window
	pipeline *sdl.GPUGraphicsPipeline

	depthTexture *sdl.GPUTexture

	// Lit rendering
	litPipeline         *sdl.GPUGraphicsPipeline
	postProcessPipeline *sdl.GPUGraphicsPipeline
	offscreenTexture    *sdl.GPUTexture
	offscreenDepth      *sdl.GPUTexture
	nearestSampler      *sdl.GPUSampler
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

	r := &Renderer{
		window:       w,
		pipeline:     pipeline,
		depthTexture: depthTexture,
	}

	// Create lit rendering resources
	if err := r.initLitPipeline(); err != nil {
		r.Destroy()
		return nil, err
	}

	return r, nil
}

func (r *Renderer) initLitPipeline() error {
	device := r.window.Device()

	// --- Lit pipeline ---
	litVert, err := shaders.LoadShader(device, "Lit.vert", 0, 1, 0, 0)
	if err != nil {
		return errors.New("failed to create lit vertex shader: " + err.Error())
	}
	defer device.ReleaseShader(litVert)

	litFrag, err := shaders.LoadShader(device, "Lit.frag", 0, 1, 0, 0)
	if err != nil {
		return errors.New("failed to create lit fragment shader: " + err.Error())
	}
	defer device.ReleaseShader(litFrag)

	litPipeline, err := device.CreateGraphicsPipeline(&sdl.GPUGraphicsPipelineCreateInfo{
		TargetInfo: sdl.GPUGraphicsPipelineTargetInfo{
			ColorTargetDescriptions: []sdl.GPUColorTargetDescription{
				{Format: sdl.GPU_TEXTUREFORMAT_R8G8B8A8_UNORM},
			},
			HasDepthStencilTarget: true,
			DepthStencilFormat:    sdl.GPU_TEXTUREFORMAT_D32_FLOAT,
		},
		DepthStencilState: sdl.GPUDepthStencilState{
			EnableDepthTest:  true,
			EnableDepthWrite: true,
			CompareOp:        sdl.GPU_COMPAREOP_LESS,
		},
		VertexInputState: sdl.GPUVertexInputState{
			VertexBufferDescriptions: []sdl.GPUVertexBufferDescription{
				{
					Slot:      0,
					InputRate: sdl.GPU_VERTEXINPUTRATE_VERTEX,
					Pitch:     uint32(unsafe.Sizeof(LitVertex{})),
				},
			},
			VertexAttributes: []sdl.GPUVertexAttribute{
				{BufferSlot: 0, Format: sdl.GPU_VERTEXELEMENTFORMAT_FLOAT3, Location: 0, Offset: 0},
				{BufferSlot: 0, Format: sdl.GPU_VERTEXELEMENTFORMAT_FLOAT3, Location: 1, Offset: 12},
				{BufferSlot: 0, Format: sdl.GPU_VERTEXELEMENTFORMAT_UBYTE4_NORM, Location: 2, Offset: 24},
			},
		},
		RasterizerState: sdl.GPURasterizerState{
			FillMode: sdl.GPU_FILLMODE_FILL,
			CullMode: sdl.GPU_CULLMODE_BACK,
		},
		PrimitiveType:  sdl.GPU_PRIMITIVETYPE_TRIANGLELIST,
		VertexShader:   litVert,
		FragmentShader: litFrag,
	})
	if err != nil {
		return errors.New("failed to create lit pipeline: " + err.Error())
	}
	r.litPipeline = litPipeline

	// --- Post-process pipeline ---
	ppVert, err := shaders.LoadShader(device, "PostProcess.vert", 0, 0, 0, 0)
	if err != nil {
		return errors.New("failed to create post-process vertex shader: " + err.Error())
	}
	defer device.ReleaseShader(ppVert)

	ppFrag, err := shaders.LoadShader(device, "PostProcess.frag", 1, 1, 0, 0)
	if err != nil {
		return errors.New("failed to create post-process fragment shader: " + err.Error())
	}
	defer device.ReleaseShader(ppFrag)

	postProcessPipeline, err := device.CreateGraphicsPipeline(&sdl.GPUGraphicsPipelineCreateInfo{
		TargetInfo: sdl.GPUGraphicsPipelineTargetInfo{
			ColorTargetDescriptions: []sdl.GPUColorTargetDescription{
				{Format: device.SwapchainTextureFormat(r.window.Handle())},
			},
		},
		RasterizerState: sdl.GPURasterizerState{
			FillMode: sdl.GPU_FILLMODE_FILL,
			CullMode: sdl.GPU_CULLMODE_NONE,
		},
		PrimitiveType:  sdl.GPU_PRIMITIVETYPE_TRIANGLELIST,
		VertexShader:   ppVert,
		FragmentShader: ppFrag,
	})
	if err != nil {
		return errors.New("failed to create post-process pipeline: " + err.Error())
	}
	r.postProcessPipeline = postProcessPipeline

	// --- Offscreen texture (320x180) ---
	offscreenTexture, err := device.CreateTexture(&sdl.GPUTextureCreateInfo{
		Type:              sdl.GPU_TEXTURETYPE_2D,
		Format:            sdl.GPU_TEXTUREFORMAT_R8G8B8A8_UNORM,
		Width:             offscreenWidth,
		Height:            offscreenHeight,
		LayerCountOrDepth: 1,
		NumLevels:         1,
		Usage:             sdl.GPU_TEXTUREUSAGE_SAMPLER | sdl.GPU_TEXTUREUSAGE_COLOR_TARGET,
	})
	if err != nil {
		return errors.New("failed to create offscreen texture: " + err.Error())
	}
	r.offscreenTexture = offscreenTexture

	// --- Offscreen depth (320x180) ---
	offscreenDepth, err := device.CreateTexture(&sdl.GPUTextureCreateInfo{
		Type:              sdl.GPU_TEXTURETYPE_2D,
		Format:            sdl.GPU_TEXTUREFORMAT_D32_FLOAT,
		Width:             offscreenWidth,
		Height:            offscreenHeight,
		LayerCountOrDepth: 1,
		NumLevels:         1,
		Usage:             sdl.GPU_TEXTUREUSAGE_DEPTH_STENCIL_TARGET,
	})
	if err != nil {
		return errors.New("failed to create offscreen depth texture: " + err.Error())
	}
	r.offscreenDepth = offscreenDepth

	// --- Nearest sampler ---
	nearestSampler, err := device.CreateSampler(&sdl.GPUSamplerCreateInfo{
		MinFilter:    sdl.GPU_FILTER_NEAREST,
		MagFilter:    sdl.GPU_FILTER_NEAREST,
		MipmapMode:   sdl.GPU_SAMPLERMIPMAPMODE_NEAREST,
		AddressModeU: sdl.GPU_SAMPLERADDRESSMODE_CLAMP_TO_EDGE,
		AddressModeV: sdl.GPU_SAMPLERADDRESSMODE_CLAMP_TO_EDGE,
		AddressModeW: sdl.GPU_SAMPLERADDRESSMODE_CLAMP_TO_EDGE,
	})
	if err != nil {
		return errors.New("failed to create nearest sampler: " + err.Error())
	}
	r.nearestSampler = nearestSampler

	return nil
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

func (r *Renderer) CreateLitVertexBuffer(vertices []LitVertex) (*sdl.GPUBuffer, error) {
	device := r.window.Device()

	bufferSize := uint32(len(vertices)) * uint32(unsafe.Sizeof(LitVertex{}))

	buffer, err := device.CreateBuffer(&sdl.GPUBufferCreateInfo{
		Usage: sdl.GPU_BUFFERUSAGE_VERTEX,
		Size:  bufferSize,
	})
	if err != nil {
		return nil, errors.New("failed to create lit vertex buffer: " + err.Error())
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

	vertexData := unsafe.Slice((*LitVertex)(unsafe.Pointer(transferDataPtr)), len(vertices))
	copy(vertexData, vertices)

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

// --- Original rendering methods ---

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

// --- Lit rendering methods ---

func (r *Renderer) BeginLitFrame() (*sdl.GPUCommandBuffer, error) {
	device := r.window.Device()

	cmdBuf, err := device.AcquireCommandBuffer()
	if err != nil {
		return nil, errors.New("failed to acquire command buffer: " + err.Error())
	}

	return cmdBuf, nil
}

func (r *Renderer) BeginScenePass(cmdBuf *sdl.GPUCommandBuffer) *sdl.GPURenderPass {
	colorTargetInfo := sdl.GPUColorTargetInfo{
		Texture:    r.offscreenTexture,
		ClearColor: sdl.FColor{R: 0.02, G: 0.01, B: 0.02, A: 1.0},
		LoadOp:     sdl.GPU_LOADOP_CLEAR,
		StoreOp:    sdl.GPU_STOREOP_STORE,
	}

	depthStencilTargetInfo := sdl.GPUDepthStencilTargetInfo{
		Texture:        r.offscreenDepth,
		ClearDepth:     1.0,
		LoadOp:         sdl.GPU_LOADOP_CLEAR,
		StoreOp:        sdl.GPU_STOREOP_DONT_CARE,
		StencilLoadOp:  sdl.GPU_LOADOP_DONT_CARE,
		StencilStoreOp: sdl.GPU_STOREOP_DONT_CARE,
		Cycle:          true,
	}

	renderPass := cmdBuf.BeginRenderPass(
		[]sdl.GPUColorTargetInfo{colorTargetInfo},
		&depthStencilTargetInfo,
	)

	renderPass.BindGraphicsPipeline(r.litPipeline)

	return renderPass
}

func (r *Renderer) PushLightUniforms(cmdBuf *sdl.GPUCommandBuffer, lights LightUniforms) {
	cmdBuf.PushFragmentUniformData(0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&lights)), unsafe.Sizeof(lights),
	))
}

func (r *Renderer) DrawLit(cmdBuf *sdl.GPUCommandBuffer, renderPass *sdl.GPURenderPass, call LitDrawCall) {
	uniforms := LitVertexUniforms{
		MVP:   call.MVP,
		Model: call.Model,
	}
	cmdBuf.PushVertexUniformData(0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&uniforms)), unsafe.Sizeof(uniforms),
	))

	renderPass.BindVertexBuffers([]sdl.GPUBufferBinding{
		{Buffer: call.VertexBuffer, Offset: 0},
	})

	renderPass.BindIndexBuffer(&sdl.GPUBufferBinding{
		Buffer: call.IndexBuffer, Offset: 0,
	}, sdl.GPU_INDEXELEMENTSIZE_16BIT)

	renderPass.DrawIndexedPrimitives(call.IndexCount, 1, 0, 0, 0)
}

func (r *Renderer) EndScenePass(renderPass *sdl.GPURenderPass) {
	renderPass.End()
}

func (r *Renderer) RunPostProcess(cmdBuf *sdl.GPUCommandBuffer, swapchainTexture *sdl.GPUTexture, uniforms PostProcessUniforms) {
	uniforms.Resolution = mgl32.Vec4{offscreenWidth, offscreenHeight, 0, 0}

	colorTargetInfo := sdl.GPUColorTargetInfo{
		Texture: swapchainTexture,
		LoadOp:  sdl.GPU_LOADOP_DONT_CARE,
		StoreOp: sdl.GPU_STOREOP_STORE,
	}

	renderPass := cmdBuf.BeginRenderPass(
		[]sdl.GPUColorTargetInfo{colorTargetInfo},
		nil,
	)

	renderPass.BindGraphicsPipeline(r.postProcessPipeline)

	renderPass.BindFragmentSamplers([]sdl.GPUTextureSamplerBinding{
		{Texture: r.offscreenTexture, Sampler: r.nearestSampler},
	})

	cmdBuf.PushFragmentUniformData(0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&uniforms)), unsafe.Sizeof(uniforms),
	))

	renderPass.DrawPrimitives(3, 1, 0, 0)
	renderPass.End()
}

func (r *Renderer) EndLitFrame(cmdBuf *sdl.GPUCommandBuffer) {
	cmdBuf.Submit()
}

func (r *Renderer) Window() *window.Window {
	return r.window
}

func (r *Renderer) ReleaseBuffer(buffer *sdl.GPUBuffer) {
	r.window.Device().ReleaseBuffer(buffer)
}

func (r *Renderer) Destroy() {
	device := r.window.Device()

	if r.nearestSampler != nil {
		device.ReleaseSampler(r.nearestSampler)
	}
	if r.offscreenDepth != nil {
		device.ReleaseTexture(r.offscreenDepth)
	}
	if r.offscreenTexture != nil {
		device.ReleaseTexture(r.offscreenTexture)
	}
	if r.postProcessPipeline != nil {
		device.ReleaseGraphicsPipeline(r.postProcessPipeline)
	}
	if r.litPipeline != nil {
		device.ReleaseGraphicsPipeline(r.litPipeline)
	}
	if r.depthTexture != nil {
		device.ReleaseTexture(r.depthTexture)
	}
	if r.pipeline != nil {
		device.ReleaseGraphicsPipeline(r.pipeline)
	}
}
