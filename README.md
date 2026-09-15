# FloppyURL - Zero-Byte Attested Web Packager via URL Hash

**FloppyURL** é um sistema client-side de hospedagem zero-byte: ele comprime um
web app inteiro (HTML/CSS/JS/SVG), o fatia em "disquetes" Base64URL atestados
por SHA-256 e o distribui dentro de fragments de URL (`#`) — sem servidor
hospedando os arquivos do site.

No boot, um **bootloader** com estética de terminal CRT verifica a integridade
criptográfica de cada disco e do conjunto completo, decripta (opcional,
AES-256-GCM) e descomprime o payload — via **`DecompressionStream` nativo**
(deflate/gzip, zero-WASM) ou **Brotli** (~200 KB dedicado, com fallback ao
kernel WASM em Go) — e injeta o site num `<iframe sandboxed>`.

Inspirado no projeto original [Xelckis/FloppyURL](https://github.com/Xelckis/FloppyURL).

---

## ✨ Destaques (v2.1)

- **Atestação criptográfica de ponta a ponta:** SHA-256 de cada disco **e**
  raiz do conjunto completo, calculados no empacotador (Go) e verificados no
  browser (WebCrypto) antes de qualquer execução.
- **Criptografia opcional:** `-pass` empacota com AES-256-GCM
  (chave derivada por PBKDF2-HMAC-SHA256, 200.000 iterações, WebCrypto no
  cliente). Sem a senha, o payload é indecifrável.
- **Boot híbrido:** `deflate`/`gzip` usam `DecompressionStream` nativo
  (instantâneo, zero-WASM); Brotli usa decodificador dedicado de ~200 KB
  ([brotli-dec-wasm](https://github.com/torao/brotli-dec-wasm), vendorizado)
  com fallback para o kernel WASM Go.
- **RAID-0 de disquetes:** payloads acima de `-chunk-size` (padrão ~1,8 MB)
  são fatiados em volumes `v2;algo;[i/n];sha_disco;sha_raiz;payload`.
  O terminal pede os discos faltantes em ordem, ou você arrasta todos de uma vez.
- **Bundler single-file:** `<link>`, `<script src>` e `<img src>` locais são
  inlinados automaticamente antes da minificação (`-no-inline` para desligar).
- **Isolamento real:** o site hóspede roda em `<iframe sandbox>` **sem**
  `allow-same-origin` (origem opaca — sem acesso ao bootloader, ao hash ou ao
  storage do host).
- **Retrocompatível:** lê discos `v1;[i/n]` do projeto original.

## 📀 Formatos de disco

| Formato | Estrutura | Integridade | Criptografia |
|---|---|---|---|
| `v2` | `v2;<algo>;[i/n];<sha256_disco>;<sha256_raiz>;<payload>` | ✅ por disco + raiz | — |
| `v2e` | idem ao `v2`, payload = `salt‖iv‖AES-GCM(ct)` em Base64URL | ✅ por disco + raiz | ✅ AES-256-GCM |
| `v1` | `v1;[i/n]<payload>` (legado) | ❌ | — |
| bruto | `<payload>` (disco único v1-era) | ❌ | — |

`<algo>` ∈ `brotli` | `deflate` | `gzip` · `<payload>` = Base64URL (sem padding)

---

## 🏗️ Arquitetura

```text
├── main.go                    # CLI do empacotador (fina; lógica em internal/)
├── internal/floppy/           # Pipeline: inlining → minify → compress → (AES-GCM)
│   │                          #          → Base64URL → chunking → atestação SHA-256
│   └── floppy_test.go         # Testes: roundtrip, PBKDF2 (vetor RFC), chunking, inlining
├── cmd/wasmdecoder/main.go    # Kernel WASM Go: window.decoder(b64Brotli) [fallback]
├── scripts/e2e.mjs            # E2E headless (Node): simula o bootloader inteiro
├── Website/
│   ├── index.html             # Bootloader terminal CRT (v2.1)
│   ├── wasm_exec.js           # Runtime Go WASM (regenerado no build)
│   ├── wasm.wasm              # (gerado) `make build-wasm` / CI — fora do Git
│   └── vendor/brotli/         # brotli-dec-wasm 2.3.2 + CHECKSUMS.txt
├── examples/demo.html         # Site de demonstração
├── .github/workflows/
│   ├── ci.yml                 # gofmt, vet, test, build WASM, E2E (positivo+negative)
│   └── deploy-pages.yml       # Deploy do bootloader no GitHub Pages
└── Makefile
```

## 🚀 Como usar

### Empacotar um site

```bash
# Brotli (máxima compressão) com atestação
go run . -file meu_site.html

# Boot instantâneo (zero-WASM) via DecompressionStream
go run . -file meu_site.html -algo deflate

# Criptografado com senha
go run . -file meu_site.html -algo brotli -pass "minha senha"

# Multi-disco (chunks de 400 chars, p/ testar o fluxo RAID-0)
go run . -file meu_site.html -algo deflate -chunk-size 400 -out-dir disks_multidisk

# Legado v1 (sem hashes)
go run . -file meu_site.html -v1
```

Flags: `-file` (obrigatória), `-algo`, `-chunk-size`, `-out-dir`, `-pass`,
`-v1`, `-no-inline`, `-version`.
Saída em `<out-dir>/`: `disk_NN.txt`, `manifest.json` (+ `boot_url.txt` em disco único).

### Dar boot

Sirva a pasta `Website/` via HTTP e:

- **Disco único:** abra `http://localhost:8080/#<payload>` (veja `boot_url.txt`);
- **Multi-disco:** abra o bootloader e insira/arraste os `disk_NN.txt` na ordem
  (o terminal pede o disco faltante; `manifest.json` pode ser arrastado junto);
- **Criptografado:** o terminal pede a senha (`v2e`).

```bash
make build-wasm   # gera Website/wasm.wasm (obrigatório p/ payloads brotli)
make serve        # python3 -m http.server 8080 em Website/
```

### Desenvolvimento

```bash
make test         # go vet + go test + E2E headless completo
make demos        # gera payloads de todos os cenários em disks_e2e/
make clean
```

O E2E (`scripts/e2e.mjs`) roda o mesmo fluxo do bootloader em Node: parsing,
SHA-256 por disco, atestação raiz, PBKDF2+AES-GCM (WebCrypto) e descompressão
nativa/vendorizada, validando o resultado contra `manifest.minified_sha256`.

---

## 🔒 Modelo de segurança

- **Integridade:** impossível trocar/corromper um disco sem quebrar o SHA-256
  do disco ou a raiz do conjunto (verificado antes do boot).
- **Confidencialidade (opcional):** AES-256-GCM + PBKDF2-SHA256 (200k iterações).
  O `manifest.json` registra apenas metadados — nunca a senha.
- **Execução isolada:** iframe `sandbox="allow-scripts allow-forms allow-popups"`
  **sem** `allow-same-origin` — o site hóspede não acessa o bootloader, o hash
  da URL nem o `localStorage` do host. Trade-off: apps hóspedes que usam
  `localStorage`/`document.cookie` próprios não terão persistência.
- **Cadeia de suprimento:** o decoder Brotli vendorizado é pinado por SHA-256
  (`Website/vendor/brotli/CHECKSUMS.txt`, verificado no CI).

## ⚠️ Limitações conhecidas

- URLs de 2 MB+ podem ser truncadas por apps de mensagem — use multi-disco.
- ES modules (`<script type="module" src=...>`) não são inlinados.
- A criptografia `v2e` não é suportada no formato legado `-v1`.

## 📄 Licença

Apache-2.0. O decoder vendorizado é dual MIT/Apache-2.0 (ver
`Website/vendor/brotli/LICENSE-*.txt`). Créditos ao projeto original
[Xelckis/FloppyURL](https://github.com/Xelckis/FloppyURL).
