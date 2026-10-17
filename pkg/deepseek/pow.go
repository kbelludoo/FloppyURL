package deepseek

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

//go:embed sha3_wasm_bg.wasm
var wasmModule []byte

// Challenge is the proof-of-work challenge returned by DeepSeek's
// /api/v0/chat/create_pow_challenge endpoint.
type Challenge struct {
	Algorithm  string  `json:"algorithm"`
	Challenge  string  `json:"challenge"`
	Salt       string  `json:"salt"`
	Difficulty float64 `json:"difficulty"`
	ExpireAt   int64   `json:"expire_at"`
	Signature  string  `json:"signature"`
	TargetPath string  `json:"target_path"`
}

type powPayload struct {
	Algorithm  string `json:"algorithm"`
	Challenge  string `json:"challenge"`
	Salt       string `json:"salt"`
	Answer     int64  `json:"answer"`
	Signature  string `json:"signature"`
	TargetPath string `json:"target_path"`
}

// Pow runs DeepSeek's own DeepSeekHashV1 WebAssembly module in a sandbox to
// solve PoW challenges, exactly like the website does.
type Pow struct {
	rt         wazero.Runtime
	mod        api.Module
	mem        api.Memory
	solveFn    api.Function
	mallocFn   api.Function
	addStackFn api.Function

	// The wasm module is not reentrant; serialise Solve calls.
	mu sync.Mutex
}

// NewPow compiles and instantiates the embedded wasm module.
func NewPow(ctx context.Context) (*Pow, error) {
	rt := wazero.NewRuntime(ctx)

	compiled, err := rt.CompileModule(ctx, wasmModule)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("deepseek: compile pow wasm: %w", err)
	}

	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("deepseek: instantiate pow wasm: %w", err)
	}

	p := &Pow{
		rt:         rt,
		mod:        mod,
		mem:        mod.Memory(),
		solveFn:    mod.ExportedFunction("wasm_solve"),
		mallocFn:   mod.ExportedFunction("__wbindgen_export_0"),
		addStackFn: mod.ExportedFunction("__wbindgen_add_to_stack_pointer"),
	}
	if p.mem == nil || p.solveFn == nil || p.mallocFn == nil || p.addStackFn == nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("deepseek: pow wasm is missing expected exports")
	}
	return p, nil
}

// Close releases the wasm runtime.
func (p *Pow) Close(ctx context.Context) error {
	if p == nil || p.rt == nil {
		return nil
	}
	return p.rt.Close(ctx)
}

func (p *Pow) writeString(ctx context.Context, text string) (uint32, uint32, error) {
	data := []byte(text)
	res, err := p.mallocFn.Call(ctx, uint64(len(data)), 1)
	if err != nil {
		return 0, 0, fmt.Errorf("deepseek: pow malloc: %w", err)
	}
	ptr := uint32(res[0])
	if len(data) > 0 && !p.mem.Write(ptr, data) {
		return 0, 0, fmt.Errorf("deepseek: pow out-of-bounds write")
	}
	return ptr, uint32(len(data)), nil
}

// Solve returns the integer PoW answer, or ok=false if the module reports
// failure (e.g. the challenge expired).
func (p *Pow) Solve(ctx context.Context, challenge, prefix string, difficulty float64) (int64, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Reserve a 16-byte return slot on the shadow stack (i32 status at +0,
	// f64 answer at +8).
	res, err := p.addStackFn.Call(ctx, api.EncodeI32(-16))
	if err != nil {
		return 0, false, fmt.Errorf("deepseek: pow stack alloc: %w", err)
	}
	retptr := uint32(res[0])
	defer func() { _, _ = p.addStackFn.Call(ctx, api.EncodeI32(16)) }()

	cPtr, cLen, err := p.writeString(ctx, challenge)
	if err != nil {
		return 0, false, err
	}
	pPtr, pLen, err := p.writeString(ctx, prefix)
	if err != nil {
		return 0, false, err
	}

	if _, err := p.solveFn.Call(ctx, uint64(retptr), uint64(cPtr), uint64(cLen),
		uint64(pPtr), uint64(pLen), api.EncodeF64(difficulty)); err != nil {
		return 0, false, fmt.Errorf("deepseek: pow solve: %w", err)
	}

	statusBytes, ok := p.mem.Read(retptr, 4)
	if !ok {
		return 0, false, fmt.Errorf("deepseek: pow status read failed")
	}
	valueBytes, ok := p.mem.Read(retptr+8, 8)
	if !ok {
		return 0, false, fmt.Errorf("deepseek: pow answer read failed")
	}
	status := int32(uint32(statusBytes[0]) | uint32(statusBytes[1])<<8 | uint32(statusBytes[2])<<16 | uint32(statusBytes[3])<<24)
	value := math.Float64frombits(uint64(valueBytes[0]) | uint64(valueBytes[1])<<8 | uint64(valueBytes[2])<<16 | uint64(valueBytes[3])<<24 |
		uint64(valueBytes[4])<<32 | uint64(valueBytes[5])<<40 | uint64(valueBytes[6])<<48 | uint64(valueBytes[7])<<56)

	if status == 0 {
		return 0, false, nil
	}
	return int64(value), true, nil
}

// MakeHeader builds the base64 x-ds-pow-response header value for a challenge.
func (p *Pow) MakeHeader(ctx context.Context, ch Challenge) (string, error) {
	prefix := fmt.Sprintf("%s_%d_", ch.Salt, ch.ExpireAt)
	answer, ok, err := p.Solve(ctx, ch.Challenge, prefix, ch.Difficulty)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("deepseek: PoW solver returned no answer (challenge expired?)")
	}
	payload := powPayload{
		Algorithm:  ch.Algorithm,
		Challenge:  ch.Challenge,
		Salt:       ch.Salt,
		Answer:     answer,
		Signature:  ch.Signature,
		TargetPath: ch.TargetPath,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}
