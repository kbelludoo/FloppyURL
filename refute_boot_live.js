'use strict';
// Probe: boot.js regexV2 + WebCrypto + DecompressionStream.
const fs = require('fs');
const path = require('path');
const html = fs.readFileSync(process.argv[2], 'utf8');
const disk = process.argv[3];
const marker = process.argv[4];
const bootJsPath = path.join(path.dirname(process.argv[2]), 'boot.js');
const js = fs.readFileSync(bootJsPath, 'utf8');
const src = html + '\n' + js;

const extracted = js.match(/regexV2 = (\/\^v2;.*?\$\/);/);
if (!extracted) {
  console.log(JSON.stringify({ ok: false, error: 'regexV2 missing in boot.js' }));
  process.exit(1);
}
const regexV2 = eval(extracted[1]);

(async () => {
  const match = disk.match(regexV2);
  if (!match) {
    console.log(JSON.stringify({ ok: false, error: 'forged disk rejected by regexV2' }));
    return;
  }
  const payload = match[6];
  const declared = match[4];
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(payload));
  const hex = [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, '0')).join('');
  let b64 = payload.replace(/-/g, '+').replace(/_/g, '/');
  while (b64.length % 4) b64 += '=';
  const bytes = Buffer.from(b64, 'base64');
  const ds = new DecompressionStream('deflate-raw');
  const w = ds.writable.getWriter();
  w.write(bytes);
  w.close();
  const text = await new Response(ds.readable).text();
  console.log(JSON.stringify({
    ok: hex === declared && text.includes(marker),
    selfAttested: hex === declared,
    markerInPayload: text.includes(marker),
    sandboxHasSameOrigin: js.includes('allow-same-origin'),
    sandboxAllowScriptsForms: js.includes("sandbox', 'allow-scripts allow-forms'"),
    rootHashCompared: /assembledSha !== rootSha256Esperado/.test(js),
    pinSupported: js.includes("q.get('pin')"),
    linIsDisplayOnly: js.includes('NAO interpreta LIN'),
  }));
})().catch((e) => {
  console.log(JSON.stringify({ ok: false, error: String(e) }));
  process.exit(1);
});
