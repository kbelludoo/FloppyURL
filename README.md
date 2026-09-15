# FloppyURL - Zero-Byte Hosting System via URL Hash & WebAssembly

**FloppyURL** is a client-side, zero-byte web hosting system and bootloader inspired by brutalist terminal aesthetics and retro computing. It allows you to encode an entire web application (HTML, CSS, JS, SVG, etc.) into heavily compressed Base64 chunks embedded directly in URL hashes (`#`). 

When loaded, a custom **Go-based WebAssembly (WASM) kernel** decompresses the payload inside the browser and dynamically boots the application inside an isolated DOM iframe—without a backend server hosting the site files!

---

## ✨ Features

- **Zero-Byte Server Hosting:** The server only hosts the bootloader (`index.html`, `wasm_exec.js`, and `wasm.wasm`). The actual website lives entirely inside the URL hash or user-provided "floppy disks".
- **Multi-Disk RAID-0 Emulation:** Payloads exceeding typical browser URL limits (~1.8MB chunks) are automatically split into sequentially numbered "floppy disk" volumes (`v1;[1/2]`, `v1;[2/2]`, etc.). The terminal prompts the user to insert missing disks sequentially.
- **Go + Brotli Kernel:** Utilizes `andybalholm/brotli` at maximum compression level combined with `tdewolff/minify` to shrink web assets to their absolute minimum size before encoding.
- **WASM Client-Side Execution:** The browser loads the compressed Base64 string and calls the compiled Go WebAssembly kernel (`window.decoder`) to decompress and inject the payload safely.
- **Retro Terminal Interface:** Brutalist CRT/Terminal UI featuring green-on-black typography, dynamic disk prompt alerts, and bootloader status logs.

---

## 🏗️ Architecture & How It Works

1. **Minification & Compression (`main.go`):**
   - Takes any input HTML file (with embedded CSS/JS) and strips unnecessary whitespace/tokens using `tdewolff/minify`.
   - Compresses the minified payload using **Brotli** (`BestCompression`).
   - Encodes the binary stream into **Base64 RawURL** format.
   - Automatically chunks the output into **1.8MB segments** prefixed with signature headers (e.g., `v1;[1/2]`, `v1;[2/2]`) and outputs to a `base64` file.

2. **The WASM Kernel (`descompressor.go`):**
   - Compiles to a lightweight WebAssembly binary using **TinyGo** / standard Go WASM toolchain.
   - Exposes a global Javascript function `window.decoder(base64Str)` that decodes and decompresses the Brotli stream back into raw HTML.

3. **The Bootloader (`index.html`):**
   - Initializes the WASM runtime (`Go()`) and loads `wasm.wasm`.
   - Checks `window.location.hash` on startup. If a valid payload is found, it immediately boots.
   - If a multi-part volume is missing disks, it drops into an interactive CLI prompt (`> INSIRA O DISCO X:`) allowing the user to paste remaining parts.
   - Once all parts are combined, it injects the rendered HTML into a full-screen `<iframe>`.

---

## 🚀 Getting Started

### Prerequisites
- [Go](https://golang.org/dl/) (1.21+ recommended) or [TinyGo](https://tinygo.org/) for optimized `.wasm` builds.
- A basic static web server (or Python's `http.server`) to serve the bootloader locally.

### 1. Build the WASM Decompressor Kernel
Compile `descompressor.go` to WebAssembly and place it in the website directory:

```bash
# Using standard Go compiler:
GOOS=js GOARCH=wasm go build -o Website/wasm.wasm descompressor.go

# (Optional) If using standard Go, ensure you have the correct wasm_exec.js:
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" Website/

```

### 2. Compress Your Website / Payload

Run the compressor CLI tool against your target HTML file:

```bash
go run main.go -file my_website.html

```

*This will generate a `base64` file containing your chunked payload strings.*

### 3. Run the Bootloader

Serve the `Website/` folder locally (WASM requires HTTP/HTTPS to load via `fetch`):

```bash
cd Website
python3 -m http.server 8080

```

Open your browser at `http://localhost:8080`.

* **Single-Disk Boot:** Append `#<your-base64-string>` directly to the URL.
* **Multi-Disk Boot:** Paste each `v1;[x/y]...` chunk directly into the terminal prompt when requested.

---

## 📦 Repository Structure

```text
├── main.go               # CLI tool: HTML minifier, Brotli compressor, & RAID chunker
├── descompressor.go      # Go WASM Kernel: Exposes window.decoder to JS
├── go.mod / go.sum       # Go module dependencies
└── Website/
    ├── index.html        # Brutalist terminal bootloader & RAID-0 disk manager
    ├── wasm_exec.js      # Go/TinyGo WebAssembly JavaScript runtime bridge
    └── wasm.wasm         # Compiled WebAssembly decompressor kernel

```

---

## ⚠️ Known Limitations

* **URL Length Limits:** While modern browsers handle large fragments, sharing 2MB+ URLs over standard messaging platforms may truncate the link. The RAID-0 multi-disk prompt mitigates this by allowing manual pasting.
* **Single-File Scope:** The input HTML should preferably have CSS and JavaScript inlined for a seamless single-payload deployment.

---

## 📄 License

This project is licensed under the **Apache License 2.0** 
