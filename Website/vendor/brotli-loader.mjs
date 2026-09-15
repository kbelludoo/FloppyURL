// Loader do decodificador Brotli dedicado (brotli-dec-wasm 2.3.2, ~200 KB).
// Usado como caminho rápido pelo bootloader; o kernel WASM Go
// (window.decoder) continua existindo como fallback.
//
// Uso:
//   const { loadBrotliDecoder } = await import('./vendor/brotli-loader.mjs');
//   const decompress = await loadBrotliDecoder();          // browser (fetch)
//   const decompress = await loadBrotliDecoder(wasmBytes); // Node/sem fetch
//
// Integridade dos arquivos vendorizados: sha256sum -c vendor/brotli/CHECKSUMS.txt

let cachedPromise = null;

export function loadBrotliDecoder(wasmBytes) {
	if (cachedPromise) return cachedPromise;

	cachedPromise = (async () => {
		const mod = await import('./brotli/brotli_dec_wasm.mjs');

		if (wasmBytes) {
			// Compila o módulo antes: não depende de fetch (funciona em Node
			// também) e usa a API nova da lib ({module}), sem warnings.
			mod.initSync({ module: new WebAssembly.Module(wasmBytes) });
		} else {
			try {
				await mod.default();
			} catch (err) {
				// Fallback: busca manual dos bytes (MIME de .wasm incorreto, etc).
				const url = new URL('./brotli/brotli_dec_wasm_bg.wasm', import.meta.url);
				const resp = await fetch(url);
				if (!resp.ok) throw new Error('falha ao buscar brotli_dec_wasm_bg.wasm: ' + resp.status);
				mod.initSync({ module: new WebAssembly.Module(await resp.arrayBuffer()) });
			}
		}

		return (bytes) => mod.decompress(bytes);
	})();

	return cachedPromise;
}
