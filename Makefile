# FloppyURL v2.1 - Makefile

SHELL := /bin/bash
GO ?= go
E2E_PASS ?= senha@forte2026

.PHONY: all build-wasm test test-go test-e2e demos demo-brotli demo-deflate demo-raid demo-v1 demo-encrypted serve clean

all: build-wasm test demo-brotli

# Compilar o kernel WebAssembly (fallback Brotli) + runtime JS correspondente
build-wasm:
	@echo "==> Compilando kernel WebAssembly (cmd/wasmdecoder)..."
	GOOS=js GOARCH=wasm $(GO) build -o Website/wasm.wasm ./cmd/wasmdecoder
	@cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" Website/wasm_exec.js 2>/dev/null \
		|| cp "$$($(GO) env GOROOT)/misc/wasm/wasm_exec.js" Website/wasm_exec.js
	@echo "==> Kernel WASM em Website/wasm.wasm (wasm_exec.js atualizado)"

# Testes
test: test-go test-e2e

test-go:
	@echo "==> Testes unitários Go..."
	$(GO) vet ./...
	$(GO) test ./...

test-e2e: demos
	@echo "==> E2E headless (Node) em todos os cenários..."
	node scripts/e2e.mjs disks_e2e/brotli
	node scripts/e2e.mjs disks_e2e/deflate
	node scripts/e2e.mjs disks_e2e/gzip
	node scripts/e2e.mjs disks_e2e/raid
	node scripts/e2e.mjs disks_e2e/v1
	node scripts/e2e.mjs disks_e2e/encrypted --pass "$(E2E_PASS)"
	@echo "==> E2E completo: OK"

# Gerar payloads de demonstração para os testes E2E
demos: demo-brotli demo-deflate demo-gzip demo-raid demo-v1 demo-encrypted

demo-brotli:
	$(GO) run . -file examples/demo.html -algo brotli -out-dir disks_e2e/brotli

demo-deflate:
	$(GO) run . -file examples/demo.html -algo deflate -out-dir disks_e2e/deflate

demo-gzip:
	$(GO) run . -file examples/demo.html -algo gzip -out-dir disks_e2e/gzip

# Simulação RAID-0 multi-disco (chunks pequenos)
demo-raid:
	$(GO) run . -file examples/demo.html -algo deflate -chunk-size 400 -out-dir disks_e2e/raid

demo-v1:
	$(GO) run . -file examples/demo.html -algo brotli -v1 -out-dir disks_e2e/v1

# Payload criptografado (AES-256-GCM)
demo-encrypted:
	$(GO) run . -file examples/demo.html -algo deflate -pass "$(E2E_PASS)" -out-dir disks_e2e/encrypted

# Servidor local (WASM exige HTTP)
serve:
	@echo "==> Servindo FloppyURL em http://localhost:8080 (Ctrl+C para parar)..."
	cd Website && python3 -m http.server 8080

clean:
	rm -rf disks disks_e2e disks_brotli disks_deflate disks_multidisk base64
