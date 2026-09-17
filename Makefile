# FloppyURL v2.0 - Makefile

.PHONY: all build-wasm demo-brotli demo-deflate demo-defi demo-lin demo-lay demo-raid test refute refute-live serve clean

all: test demo-deflate

# Executar bateria completa de testes de integridade e atestação (irmãs P0)
test:
	@echo "==> Suite legada + oracles externos das irmãs..."
	python3 test_floppy.py
	python3 verify_lint_p0.py
	python3 verify_lint_transpile_p0.py
	python3 verify_linp_p0.py
	python3 verify_linp_tc_p0.py
	python3 verify_linj_p0.py
	python3 verify_linj_tc_p0.py
	python3 verify_linz_p0.py
	python3 verify_linz_tc_p0.py
	python3 verify_mempipe_p0.py
	python3 verify_vs_original_p0.py
	python3 refute_floppy.py

# Tentar refutar as alegacoes do README contra o codigo real
refute:
	python3 refute_floppy.py

refute-live:
	python3 refute_boot_live.py

# Compilar o kernel WebAssembly
build-wasm:
	@echo "==> Compilando kernel WebAssembly (descompressor.go)..."
	GOOS=js GOARCH=wasm go build -o Website/wasm.wasm descompressor.go
	@echo "==> Kernel WASM gerado com sucesso em Website/wasm.wasm"

# Gerar exemplo com compressão Brotli (máxima compressão)
demo-brotli:
	@echo "==> Gerando payload Brotli..."
	go run main.go lay_compiler.go -file examples/demo.html -algo brotli -out-dir disks_brotli

# Gerar exemplo com Deflate (boot instantâneo zero-WASM via DecompressionStream)
demo-deflate:
	@echo "==> Gerando payload Deflate (Zero-WASM)..."
	go run main.go lay_compiler.go -file examples/demo.html -algo deflate -out-dir disks_deflate

# Gerar dApp DeFi Swap Unstoppable
demo-defi:
	@echo "==> Gerando payload DeFi AMM Swapper..."
	go run main.go lay_compiler.go -file examples/defi_swap_lin.html -algo deflate -out-dir disks_defi

# Gerar Kernel LIN determinístico com atestação RuleL
demo-lin:
	@echo "==> Gerando payload LIN Determinístico..."
	go run main.go lay_compiler.go -file examples/cpmm_oracle.lin -algo deflate -out-dir disks_lin

# Gerar UI LAY DSL (bytecode declarativo nativo)
demo-lay:
	@echo "==> Gerando payload LAY DSL (Bytecode UI)..."
	go run main.go lay_compiler.go -file examples/floppyurl_lay.lay -algo deflate -out-dir disks_lay

# Gerar simulacao multi-volume (chunks pequenos)
demo-raid:
	@echo "==> Gerando payload multi-volume concatenado..."
	go run main.go lay_compiler.go -file examples/demo.html -algo deflate -chunk-size 400 -out-dir disks_multidisk

# Iniciar servidor local para testes
serve:
	@echo "==> Servindo FloppyURL em http://localhost:8080 (Ctrl+C para parar)..."
	cd Website && python3 -m http.server 8080

clean:
	rm -rf disks disks_brotli disks_deflate disks_defi disks_lin disks_multidisk test_run_disks test_run_refute base64
