// E2E headless do pipeline FloppyURL: simula exatamente o fluxo do
// bootloader (Website/index.html) em Node — parsing dos discos,
// verificação SHA-256 por disco e raiz, descriptografia PBKDF2+AES-GCM
// (WebCrypto) e descompressão (DecompressionStream nativo ou brotli
// vendorizado). Compara o resultado com manifest.minified_sha256.
//
// Uso: node scripts/e2e.mjs <dir-dos-discos> [--pass SENHA]
//      node scripts/e2e.mjs disks_e2e/brotli
//      node scripts/e2e.mjs disks_e2e/encrypted --pass minh senha

import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function fail(msg) {
	console.error(`E2E FALHOU: ${msg}`);
	process.exit(1);
}

function parseArgs(argv) {
	const args = { dir: null, pass: null };
	for (let i = 0; i < argv.length; i++) {
		if (argv[i] === '--pass') args.pass = argv[++i];
		else if (!args.dir) args.dir = argv[i];
	}
	if (!args.dir) {
		console.error('uso: node scripts/e2e.mjs <dir-discos> [--pass SENHA]');
		process.exit(2);
	}
	return args;
}

const toHex = (buf) => Array.from(new Uint8Array(buf)).map(b => b.toString(16).padStart(2, '0')).join('');

async function sha256Hex(str) {
	const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(str));
	return toHex(digest);
}

function base64UrlToUint8Array(base64Url) {
	let base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
	while (base64.length % 4) base64 += '=';
	return Uint8Array.from(atob(base64), c => c.charCodeAt(0));
}

async function decompressNative(bytes, format) {
	const ds = new DecompressionStream(format);
	const writer = ds.writable.getWriter();
	writer.write(bytes);
	writer.close();
	return new TextDecoder().decode(await new Response(ds.readable).arrayBuffer());
}

async function loadVendoredBrotli() {
	const loaderPath = path.join(repoRoot, 'Website', 'vendor', 'brotli-loader.mjs');
	const wasmPath = path.join(repoRoot, 'Website', 'vendor', 'brotli', 'brotli_dec_wasm_bg.wasm');
	const { loadBrotliDecoder } = await import(loaderPath);
	const wasmBytes = new Uint8Array(await readFile(wasmPath));
	return loadBrotliDecoder(wasmBytes);
}

const args = parseArgs(process.argv.slice(2));
const disksDir = path.resolve(args.dir);
const files = (await readdir(disksDir)).sort();
const diskFiles = files.filter(f => /^disk_\d+\.txt$/.test(f));

if (diskFiles.length === 0) fail(`nenhum disk_NN.txt em ${disksDir}`);

let manifest = null;
if (files.includes('manifest.json')) {
	manifest = JSON.parse(await readFile(path.join(disksDir, 'manifest.json'), 'utf8'));
	console.log(`[manifest] ${manifest.source_file} | algo=${manifest.algorithm} | formato=${manifest.format_version} | discos=${manifest.total_disks}${manifest.encrypted ? ' | criptografado' : ''}`);
}

const byIndex = new Map();
let algo = manifest?.algorithm ?? 'brotli';
let formato = manifest?.format_version ?? null;
let rootDeclarada = manifest?.root_sha256 ?? null;
const iteracoes = manifest?.pbkdf2_iterations ?? 200000;

for (const f of diskFiles) {
	const content = (await readFile(path.join(disksDir, f), 'utf8')).trim();

	let m = content.match(/^v2e;([a-z0-9_-]+);\[(\d+)\/(\d+)\];([a-f0-9]{64});([a-f0-9]{64});(.*)$/s);
	let cript = false;
	if (!m) m = content.match(/^v2;([a-z0-9_-]+);\[(\d+)\/(\d+)\];([a-f0-9]{64});([a-f0-9]{64});(.*)$/s);
	else cript = true;
	if (!m) m = content.match(/^v1;\[(\d+)\/(\d+)\](.*)$/s);

	if (m && m.length >= 4 && m[0].startsWith('v')) {
		if (m.length >= 6) {
			// v2/v2e
			const [, a, idx, total, chunkSha, rootSha, payload] = m;
			algo = a;
			formato = cript ? 'v2e' : 'v2';
			const calc = await sha256Hex(payload);
			if (calc !== chunkSha) fail(`${f}: SHA-256 do chunk inválido (esperado ${chunkSha}, calculado ${calc})`);
			if (rootDeclarada && rootDeclarada !== rootSha) fail(`${f}: raiz divergente do conjunto`);
			rootDeclarada = rootSha;
			byIndex.set(parseInt(idx), { total: parseInt(total), payload });
		} else {
			// v1
			const [, idx, total, payload] = m;
			formato = 'v1';
			byIndex.set(parseInt(idx), { total: parseInt(total), payload });
		}
	} else {
		// volume único bruto
		formato = formato ?? 'raw';
		byIndex.set(1, { total: 1, payload: content });
	}
}

const total = byIndex.get(1)?.total;
if (!total || byIndex.size !== total) fail(`discos incompletos: ${byIndex.size}/${total}`);

const ordenado = [...byIndex.entries()].sort((a, b) => a[0] - b[0]);
const payloadCompleto = ordenado.map(([, d]) => d.payload).join('');
console.log(`[discos] ${total} montado(s) | payload=${payloadCompleto.length} chars | formato=${formato}`);

// Atestação raiz (v2/v2e)
if (formato === 'v2' || formato === 'v2e') {
	const raizCalc = await sha256Hex(payloadCompleto);
	if (raizCalc !== rootDeclarada) fail(`atestação raiz falhou (esperada ${rootDeclarada}, calculada ${raizCalc})`);
	console.log(`[attest] raiz OK: ${raizCalc.slice(0, 24)}...`);
} else {
	console.log('[attest] formato legado — sem atestação (esperado)');
}

let compressed;
if (formato === 'v2e') {
	if (!args.pass) fail('payload v2e requer --pass');
	const wire = base64UrlToUint8Array(payloadCompleto);
	const salt = wire.slice(0, 16);
	const iv = wire.slice(16, 28);
	const ct = wire.slice(28);
	const baseKey = await crypto.subtle.importKey('raw', new TextEncoder().encode(args.pass), 'PBKDF2', false, ['deriveKey']);
	const key = await crypto.subtle.deriveKey(
		{ name: 'PBKDF2', salt, iterations: iteracoes, hash: 'SHA-256' },
		baseKey, { name: 'AES-GCM', length: 256 }, false, ['decrypt']
	);
	try {
		compressed = new Uint8Array(await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ct));
		console.log(`[crypto] AES-256-GCM decriptado (PBKDF2-SHA256 x${iteracoes})`);
	} catch (e) {
		fail('senha inválida ou dados corrompidos (AES-GCM rejeitou)');
	}
} else {
	compressed = base64UrlToUint8Array(payloadCompleto);
}

let html;
if (algo === 'deflate' || algo === 'gzip') {
	html = await decompressNative(compressed, algo === 'deflate' ? 'deflate-raw' : 'gzip');
	console.log('[decompress] DecompressionStream nativo');
} else {
	const decompress = await loadVendoredBrotli();
	html = new TextDecoder().decode(decompress(compressed));
	console.log('[decompress] brotli-dec-wasm vendorizado');
}

if (manifest?.minified_sha256) {
	const got = await sha256Hex(html);
	if (got !== manifest.minified_sha256) fail(`HTML divergente do manifest (esperado ${manifest.minified_sha256}, obtido ${got})`);
	console.log(`[attest] HTML bate com minified_sha256: ${got.slice(0, 24)}...`);
}

if (!html.includes("<") || /^Error/.test(html)) fail('saída não parece HTML');

console.log(`\nE2E OK: ${disksDir} (formato=${formato}, algo=${algo}, discos=${total}, html=${html.length} chars)`);
