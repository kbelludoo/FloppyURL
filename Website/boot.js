'use strict';

let discos = [];
let totalDiscos = 0;
let algoritmo = 'brotli';
let rootSha256Esperado = null;
let wasmPronto = false;
let layPronto = false;

const terminal = document.getElementById('console-output');
const inputContainer = document.getElementById('input-container');
const inputCampo = document.getElementById('disco-input');
const prompt = document.getElementById('prompt');
const dropZone = document.getElementById('drop-zone');

function bootOpts() {
	const q = new URLSearchParams(window.location.search);
	const pin = (q.get('pin') || '').replace(/^sha256:/i, '').toLowerCase();
	return { pin, legacy: q.get('legacy') === '1', requirePin: q.get('requirePin') === '1' };
}

function printLog(msg, cssClass) {
	const div = document.createElement('div');
	div.className = 'linha ' + (cssClass || '');
	div.innerText = msg;
	terminal.appendChild(div);
	window.scrollTo(0, document.body.scrollHeight);
}

async function calcSha256(str) {
	const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(str));
	return Array.from(new Uint8Array(digest)).map((b) => b.toString(16).padStart(2, '0')).join('');
}

function base64UrlToUint8Array(base64Url) {
	let base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
	while (base64.length % 4) base64 += '=';
	const binary = atob(base64);
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
	return bytes;
}

async function decompressNative(compressedBytes, format) {
	const ds = new DecompressionStream(format);
	const writer = ds.writable.getWriter();
	writer.write(compressedBytes);
	writer.close();
	return await new Response(ds.readable).text();
}

function loadScript(src) {
	return new Promise((resolve, reject) => {
		const s = document.createElement('script');
		s.src = src;
		s.onload = resolve;
		s.onerror = () => reject(new Error('falha ao carregar ' + src));
		document.head.appendChild(s);
	});
}

async function bootWasm() {
	if (wasmPronto) return true;
	printLog('> CARREGANDO FALLBACK WASM (brotli)...');
	try {
		await loadScript('wasm_exec.js');
		const go = new Go();
		const result = await WebAssembly.instantiateStreaming(
			fetch('wasm.wasm'), go.importObject
		);
		go.run(result.instance);
		wasmPronto = true;
		printLog('> WASM BROTLI PRONTO.', 'sucesso');
		return true;
	} catch (err) {
		printLog('> ERRO WASM: ' + err, 'erro');
		return false;
	}
}

async function bootLay() {
	if (layPronto && typeof LAYToFloppy === 'function') return true;
	await loadScript('lay_runtime.js');
	layPronto = true;
	return typeof LAYToFloppy === 'function';
}

function renderProgresso() {
	let carregados = 0;
	for (let i = 0; i < totalDiscos; i++) if (discos[i]) carregados++;
	const pct = totalDiscos > 0 ? Math.round((carregados / totalDiscos) * 100) : 0;
	const blocos = Math.round((carregados / (totalDiscos || 1)) * 20);
	printLog('> STATUS: [' + '█'.repeat(blocos) + '░'.repeat(20 - blocos) + '] ' + pct + '% (' + carregados + '/' + totalDiscos + ')', 'progresso barra-progresso');
}

function failClosed(msg) {
	printLog('> [FAIL-CLOSED] ' + msg, 'erro');
}

async function processarHash(hashStr) {
	if (hashStr.startsWith('#')) hashStr = hashStr.substring(1);
	hashStr = hashStr.trim();
	if (!hashStr) return;

	if (hashStr.toLowerCase() === 'reboot' || hashStr.toLowerCase() === 'clear') {
		discos = [];
		totalDiscos = 0;
		rootSha256Esperado = null;
		terminal.innerHTML = '';
		printLog('> SISTEMA REINICIALIZADO.');
		await verificarProgresso();
		return;
	}

	const regexV2 = /^v2;([a-z0-9_-]+);\[(\d+)\/(\d+)\];([a-f0-9]{64});([a-f0-9]{64});(.*)$/;
	const matchV2 = hashStr.match(regexV2);
	if (matchV2) {
		const algo = matchV2[1];
		const parteAtual = parseInt(matchV2[2], 10);
		const total = parseInt(matchV2[3], 10);
		const chunkShaDeclarado = matchV2[4];
		const rootShaDeclarado = matchV2[5];
		const payloadBruto = matchV2[6];

		if (totalDiscos && total !== totalDiscos && discos.some(Boolean)) {
			failClosed('disco declara total ' + total + ', volume atual e ' + totalDiscos);
			return;
		}
		if (rootSha256Esperado && rootShaDeclarado !== rootSha256Esperado) {
			failClosed('root_sha256 diverge entre discos');
			printLog('   esperado: ' + rootSha256Esperado, 'erro');
			printLog('   disco:    ' + rootShaDeclarado, 'erro');
			return;
		}

		algoritmo = algo;
		totalDiscos = total;
		if (!rootSha256Esperado) rootSha256Esperado = rootShaDeclarado;

		const chunkShaCalculado = await calcSha256(payloadBruto);
		if (chunkShaCalculado !== chunkShaDeclarado) {
			failClosed('chunk SHA-256 invalido no disco ' + parteAtual + '/' + total);
			printLog('   declarado: ' + chunkShaDeclarado, 'erro');
			printLog('   calculado: ' + chunkShaCalculado, 'erro');
			await verificarProgresso();
			return;
		}

		if (discos.length !== totalDiscos) discos = new Array(totalDiscos).fill(null);
		discos[parteAtual - 1] = payloadBruto;
		playFloppySeekSound();
		printLog('> [OK] CHECKSUM DO CHUNK [' + parteAtual + '/' + totalDiscos + '] ' + chunkShaDeclarado.slice(0, 16) + '...', 'sucesso');
		renderProgresso();
		await verificarProgresso();
		return;
	}

	const opts = bootOpts();
	if (!opts.legacy) {
		failClosed('formato v1/raw recusado (sem SHA-256). Use v2 ou ?legacy=1');
		return;
	}
	const matchV1 = hashStr.match(/^v1;\[(\d+)\/(\d+)\](.*)/);
	if (matchV1) {
		const parteAtual = parseInt(matchV1[1], 10);
		totalDiscos = parseInt(matchV1[2], 10);
		algoritmo = 'brotli';
		if (discos.length !== totalDiscos) discos = new Array(totalDiscos).fill(null);
		discos[parteAtual - 1] = matchV1[3];
		playFloppySeekSound();
		printLog('> LIDO SETOR V1 LEGADO [' + parteAtual + '/' + totalDiscos + '] — sem checksum', 'alerta');
		renderProgresso();
		await verificarProgresso();
		return;
	}

	discos = [hashStr];
	totalDiscos = 1;
	algoritmo = 'brotli';
	playFloppySeekSound();
	printLog('> VOLUME RAW LEGADO — sem checksum.', 'alerta');
	await verificarProgresso();
}

async function verificarProgresso() {
	let discoFaltando = -1;
	for (let i = 0; i < totalDiscos; i++) {
		if (!discos[i]) { discoFaltando = i + 1; break; }
	}

	if (totalDiscos === 0 || discoFaltando !== -1) {
		inputContainer.style.display = 'block';
		prompt.innerText = totalDiscos === 0 ? '> INSIRA O DISCO 1: ' : '> INSIRA O DISCO ' + discoFaltando + ': ';
		inputCampo.value = '';
		inputCampo.focus();
		return;
	}

	inputContainer.style.display = 'none';
	const payloadCompleto = discos.join('');
	const assembledSha = await calcSha256(payloadCompleto);
	if (rootSha256Esperado && assembledSha !== rootSha256Esperado) {
		failClosed('SHA-256 da montagem != root declarado');
		printLog('   root:      ' + rootSha256Esperado, 'erro');
		printLog('   montagem:  ' + assembledSha, 'erro');
		return;
	}
	if (rootSha256Esperado) {
		printLog('> [OK] ROOT SHA-256 DA MONTAGEM CONFERE.', 'sucesso');
	}

	const opts = bootOpts();
	if (opts.requirePin && !opts.pin) {
		failClosed('?requirePin=1 exige ?pin=<root_sha256> fora do fragmento');
		return;
	}
	if (opts.pin) {
		if (opts.pin !== assembledSha) {
			failClosed('pin de confianca != SHA-256 montado (payload reescrito?)');
			printLog('   pin:       ' + opts.pin, 'erro');
			printLog('   montagem:  ' + assembledSha, 'erro');
			return;
		}
		printLog('> [OK] PIN DE CONFIANCA CONFERIDO (query ?pin=).', 'sucesso');
	} else {
		printLog('> AVISO: sem ?pin= — checksums detectam bitrot, nao um autor. Quem controla a URL controla os hashes.', 'alerta');
	}

	playFloppySeekSound();
	await executarPayload(payloadCompleto);
}

function playFloppySeekSound() {
	try {
		const ctx = new (window.AudioContext || window.webkitAudioContext)();
		const osc = ctx.createOscillator();
		const gain = ctx.createGain();
		osc.type = 'square';
		osc.frequency.setValueAtTime(420, ctx.currentTime);
		osc.frequency.setValueAtTime(180, ctx.currentTime + 0.04);
		gain.gain.setValueAtTime(0.04, ctx.currentTime);
		gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.08);
		osc.connect(gain);
		gain.connect(ctx.destination);
		osc.start();
		osc.stop(ctx.currentTime + 0.09);
	} catch (e) { /* audio optional */ }
}

async function renderLinPayload(payloadObj) {
	printLog('> [LIN SOURCE] ' + (payloadObj.filename || 'script.lin') + ' — exibicao apenas', 'alerta');
	printLog('> este bootloader NAO interpreta LIN (sem VM, sem heapless, sem recibo JIT).', 'alerta');
	const pre = document.createElement('pre');
	pre.style.cssText = 'color:#00ff66;background:#000;padding:12px;border:1px solid #00ff66;margin:10px 0;max-height:260px;overflow-y:auto;font-size:12px;';
	pre.innerText = payloadObj.source || JSON.stringify(payloadObj, null, 2);
	terminal.appendChild(pre);
	const digest = await calcSha256(payloadObj.source || '');
	printLog('> checksum do texto exibido sha256:' + digest, 'sucesso');
	inputContainer.style.display = 'block';
	prompt.innerText = "> LIN VIEW (digite 'reboot'): ";
	inputCampo.focus();
}

async function executarPayload(payloadString) {
	printLog('> DESCOMPRIMINDO [' + algoritmo.toUpperCase() + ']...');
	try {
		let htmlPuro = '';
		if (algoritmo === 'deflate' || algoritmo === 'gzip') {
			const format = algoritmo === 'deflate' ? 'deflate-raw' : 'gzip';
			htmlPuro = await decompressNative(base64UrlToUint8Array(payloadString), format);
			printLog('> DESCOMPRESSAO NATIVA OK.', 'sucesso');
		} else {
			if (!(await bootWasm())) throw new Error('WASM brotli indisponivel');
			htmlPuro = window.decoder(payloadString);
			if (htmlPuro.startsWith('Error')) throw new Error(htmlPuro);
			printLog('> DESCOMPRESSAO BROTLI WASM OK.', 'sucesso');
		}

		if (htmlPuro.trim().startsWith('{')) {
			try {
				const parsed = JSON.parse(htmlPuro);
				if (parsed.type === 'lin' || parsed.type === 'dispute') {
					await renderLinPayload(parsed);
					return;
				}
				if (parsed.type === 'lay') {
					if (!(await bootLay())) throw new Error('lay_runtime.js indisponivel');
					LAYToFloppy(parsed);
					return;
				}
			} catch (e) {
				if (e && e.message && e.message.indexOf('lay_runtime') !== -1) throw e;
			}
		}

		printLog('> INJETANDO PAYLOAD EM IFRAME sandbox=scripts+forms (origem opaca)...');
		const iframe = document.createElement('iframe');
		iframe.style.cssText = 'position:absolute;top:0;left:0;width:100vw;height:100vh;border:none;z-index:9999;background:white;';
		iframe.setAttribute('sandbox', 'allow-scripts allow-forms');
		iframe.srcdoc = htmlPuro;
		document.body.innerHTML = '';
		document.body.appendChild(iframe);
	} catch (e) {
		printLog('> FALHA NA DESCOMPRESSAO: ' + e.message, 'erro');
		inputContainer.style.display = 'block';
		prompt.innerText = "> DIGITE 'reboot' PARA RECOMECAR: ";
		inputCampo.focus();
	}
}

inputCampo.addEventListener('keypress', function (e) {
	if (e.key === 'Enter') {
		const val = inputCampo.value.trim();
		inputCampo.value = '';
		processarHash(val);
	}
});

window.addEventListener('dragover', (e) => { e.preventDefault(); dropZone.style.display = 'flex'; });
window.addEventListener('dragleave', (e) => {
	if (e.clientX <= 0 || e.clientY <= 0) dropZone.style.display = 'none';
});
window.addEventListener('drop', (e) => {
	e.preventDefault();
	dropZone.style.display = 'none';
	const files = e.dataTransfer.files;
	if (files.length > 0) {
		const reader = new FileReader();
		reader.onload = function (evt) {
			printLog('> ARQUIVO: ' + files[0].name + ' (' + files[0].size + ' bytes)');
			processarHash(evt.target.result);
		};
		reader.readAsText(files[0]);
	}
});

window.onload = async () => {
	const hashAtual = window.location.hash;
	if (hashAtual.length > 1) await processarHash(hashAtual);
	else {
		inputContainer.style.display = 'block';
		prompt.innerText = '> INSIRA O DISCO 1 (ou arraste o arquivo aqui): ';
		inputCampo.focus();
		if (!bootOpts().pin) {
			printLog('> Dica: o packager emite ?pin=<root> na query. Sem pin, so ha checksum.', 'alerta');
		}
	}
};
