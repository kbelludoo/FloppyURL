# FloppyURL v2.0 - Zero-Byte Attested Web & LIN Hosting via URL Hash

**FloppyURL** is a client-side, zero-byte hosting system and sovereign bootloader inspired by retro-computing and brutalist terminal aesthetics. It encodes entire web applications (HTML/CSS/JS) and deterministic **LIN kernels** into cryptographically attested Base64 URL fragments (`#`).

When loaded in any modern browser, FloppyURL verifies the cryptographic integrity of each disk chunk using **NIST FIPS 180-4 SHA-256** via the native `WebCrypto` API, decompresses the payload instantaneously via `DecompressionStream('deflate-raw')` (zero WASM download needed), and executes the application in an isolated sandbox or interactive LIN terminal.

---

## 🏛️ Specialized Multi-Language Architecture

Rather than forcing one language to do everything, FloppyURL v2.0 splits responsibilities so each language and runtime performs its optimal role:

| Component | Technology | Specialized Role |
| :--- | :--- | :--- |
| **CLI Packager & Chunker** | **Go** | AST minification (`tdewolff/minify`), raw DEFLATE / Brotli compression, RAID-0 multi-disk slicing, and RuleL manifest generation. |
| **Zero-WASM Bootloader** | **Web Standards / Browser C++** | `DecompressionStream('deflate-raw')` and `crypto.subtle` (SHA-256) eliminate the 4.6MB Go WASM download, achieving sub-2ms boot. |
| **Deterministic Kernel** | **LIN / RuleL** | Pure stack-only execution, zero heap, zero memory leaks, and verifiable `@RULEL` cryptographic provenance receipts. |
| **Retro Immersion** | **Web Audio API** | Real-time mathematical synthesis of 5.25" floppy disk head seek and stepper motor sounds without external audio assets. |

---

## ✨ Features

- **Zero-Byte Server Hosting:** The server only serves the static bootloader (`index.html`). All application code and state transitions live in the URL hash or user-mounted floppy disks.
- **NIST FIPS 180-4 SHA-256 Integrity Pinning:** Every disk chunk declares both its individual chunk hash and the dataset root hash (`#v2;[algo];[i/N];[chunk_sha];[root_sha];[payload]`). Tampered fragments fail closed immediately.
- **Instant Zero-WASM Boot:** Defaults to raw DEFLATE, decompressed by the browser's native C++ engine with zero dependencies downloaded.
- **Multi-Disk Virtual Floppy RAID-0:** Files exceeding URL limits (~1.8 MB) are automatically split into numbered virtual floppies. The terminal prompts the user to insert remaining disks sequentially.
- **LIN Sovereign Kernel Support:** Automatically detects `.lin` files, packaging them with `@RULEL:FLOPPY_MANIFEST` provenance receipts for deterministic, heapless smart contracts and fraud dispute resolution.
- **Synthesized Floppy Audio:** Vintage mechanical stepper motor audio synthesized via Web Audio API oscillators on disk insertion and mounting.

---

## 🚀 Quick Start

### 1. Requirements
- [Go](https://golang.org/dl/) (1.21+)
- Python 3 (for testing and local HTTP server)

### 2. Run Integration Tests
```bash
make test
# or: python3 test_floppy.py
```

### 3. Package an Application or LIN Kernel

**Package an HTML WebApp (Instant Zero-WASM Deflate):**
```bash
go run main.go -file examples/demo.html -algo deflate
```

**Package an Unstoppable DeFi Swapper:**
```bash
go run main.go -file examples/defi_swap_lin.html -algo deflate
```

**Package a Deterministic LIN Script:**
```bash
go run main.go -file examples/cpmm_oracle.lin -algo deflate
```

### 4. Boot Locally
Serve the bootloader:
```bash
make serve
# Serves Website/ on http://localhost:8080
```
Open your browser and navigate to the generated boot URL printed by `main.go`.

---

## 📜 Virtual Floppy Format Specification (v2)

Disks follow the canonical format:
```text
#v2;<algorithm>;[<current_disk>/<total_disks>];<chunk_sha256>;<root_sha256>;<raw_url_base64_payload>
```

- `v2`: Specification version identifier.
- `<algorithm>`: `deflate` (zero-WASM browser native), `brotli` (maximum ratio), or `gzip`.
- `[i/N]`: Virtual floppy disk index and total count (e.g., `[1/3]`, `[2/3]`).
- `<chunk_sha256>`: 64-character hex digest of the Base64 chunk string.
- `<root_sha256>`: 64-character hex digest of the fully assembled payload.
- `<raw_url_base64>`: URL-safe Base64 encoded compressed data without `=` padding.

Along with disk files, the packager generates a canonical **RuleL Manifest** (`manifest.rulel`):
```rulel
@RULEL:FLOPPY_MANIFEST:2.0.0
~R{.s=source .a=algo .o=orig_bytes .m=min_bytes .c=comp_bytes .e=enc_chars .t=total_disks .r=root_sha .f=format}
.source="defi_swap_lin.html"
.algo="deflate"
.format="v2"
.orig_bytes=8329
.min_bytes=5972
.comp_bytes=2490
.enc_chars=3320
.total_disks=1
.root_sha256="d549088d3242288b5133a86c90a5b427b86f7fdddb4a3ac4174c178faac9e79b"
.disk_hashes=[
  "d549088d3242288b5133a86c90a5b427b86f7fdddb4a3ac4174c178faac9e79b",
]
```

---

## 📦 Repository Structure

```text
├── main.go                     # Go CLI: Minifier, DEFLATE/Brotli compressor, RAID chunker, & RuleL generator
├── descompressor.go            # Optional Go WASM Kernel: Fallback Brotli decoder for older browsers
├── test_floppy.py              # Automated integration & attestation test suite
├── Makefile                    # Automation: build-wasm, demo-deflate, demo-defi, demo-lin, test, serve
├── examples/
│   ├── demo.html               # Minimal attested single-disk web application
│   ├── defi_swap_lin.html      # Unstoppable AMM swapper dApp (zero backend, CPMM x*y=k)
│   └── cpmm_oracle.lin         # Deterministic LIN contract oracle
└── Website/
    ├── index.html              # Sovereign CRT bootloader, WebCrypto validator, & Web Audio synth
    ├── wasm_exec.js            # Go WASM JS bridge (only loaded if Brotli selected)
    └── wasm.wasm               # Compiled fallback WASM binary
```

---

## ⚠️ Operational Boundaries

- **Browser URL Length Limit:** Most modern browsers support fragment lengths of up to ~2 MB. For larger applications (games, complex SPAs), multi-disk chunking automatically splits the payload into virtual floppy volumes that can be inserted sequentially into the terminal prompt.
- **WebCrypto Secure Context:** `crypto.subtle` requires a secure context (`https://` or `http://localhost`). When testing locally, always use `http://localhost` rather than raw IP addresses.

---

## 📄 License

This project is licensed under the **Apache License 2.0**.
