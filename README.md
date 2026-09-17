# FloppyURL v2.0 — URL-hash packager + checksum bootloader

FloppyURL packs HTML (or a `.lin` / `.lay` wrapper) with minification, raw DEFLATE/Brotli/gzip, Base64url, and **SHA-256 checksums** into a `#` fragment the browser can boot.

It is **not** zero-byte hosting, **not** a signature/attestation scheme, **not** RAID-0, and **not** a LIN virtual machine. The Go packager and the browser checksum path are real; the rest of the old marketing was not.

## What it actually does

| Piece | Reality |
| :--- | :--- |
| **Go packager** | Minify, compress, split into sequential volumes, emit `manifest.rulel` with chunk + root SHA-256. |
| **Bootloader** | Parses `v2;algo;[i/N];chunk;root;payload`, checks **chunk** digest, checks **assembled root**, optionally checks **`?pin=`** in the query string (outside the fragment). Deflate/gzip use `DecompressionStream`; Brotli lazy-loads `wasm_exec.js` + `wasm.wasm`. |
| **HTML payload** | Injected into `<iframe sandbox="allow-scripts allow-forms">` (**no** `allow-same-origin`). |
| **`.lin` files** | Wrapped as JSON and **displayed** as text. No interpreter. |
| **`.lay` files** | Compiled to bytecode; `lay_runtime.js` is loaded only then. |

## Integrity model (honest)

```text
http://localhost:8080/?pin=<root_sha256>#v2;deflate;[1/1];<chunk_sha>;<root_sha>;<b64>
```

- **Chunk SHA-256** — detects a corrupted slice. Declared next to the slice (checksum, not a pin).
- **Root SHA-256 of the assembled Base64** — fail-closed if volumes disagree or the join does not match.
- **`?pin=`** — the same root, copied into the **query string**. Useful when a *trusted page* links to the bootloader with a pin it chose. A stranger who sends you a full URL can set both `pin` and `#` together; that is not a signature.
- **`?requirePin=1`** — refuse to execute if `pin` is missing.
- **`?legacy=1`** — opt-in for v1/raw volumes that have no checksum. Off by default.

The **bootloader origin** (`index.html` / `boot.js`) remains trusted computing base. Replacing those files bypasses every hash. There is no DNS-takeover immunity.

Brotli still needs the ~4.6 MB `Website/wasm.wasm`. Deflate does **not** download it; `wasm_exec.js` / `lay_runtime.js` load only when needed.

## Quick start

```bash
make test
make refute          # remaining overclaims vs this tree
go run main.go lay_compiler.go -file examples/demo.html -algo deflate
make serve           # http://localhost:8080
```

Open the `?pin=...#v2;...` URL printed by the packager (also in `disks/boot_url.txt`).

## Disk format (v2)

```text
#v2;<algorithm>;[<current_disk>/<total_disks>];<chunk_sha256>;<root_sha256>;<raw_url_base64_payload>
```

Volumes are **concatenated** slices of one Base64 string (not RAID-0). Default max slice: 1_800_000 chars (`-chunk-size`).

## Tests

```bash
make test            # packager + sister P0 oracles + refute suite
make refute-live     # Chrome: pin reject + sandbox (no parent title rewrite)
```

## Limits

- Browser fragment length (Safari is much smaller than Chromium).
- `crypto.subtle` needs a secure context (`https://` or `http://localhost`).
- Checksums ≠ signatures. Pin ≠ out-of-band identity unless the pin comes from a channel you already trust.
- `.lin` is not executed here.

Apache License 2.0.
