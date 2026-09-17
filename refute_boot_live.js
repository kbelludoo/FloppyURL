'use strict';
// Live probe: run the bootloader's own regex + WebCrypto + DecompressionStream.
const fs = require('fs');
const html = fs.readFileSync(process.argv[2], 'utf8');
const disk = process.argv[3];
const marker = process.argv[4];

const extracted = html.match(/regexV2 = (\/\^v2;.*?\$\/);/);
if (!extracted) {
  console.log(JSON.stringify({ ok: false, error: 'regexV2 missing in index.html' }));
  process.exit(1);
}
const regexV2 = eval(extracted[1]); // the literal copied from Website/index.html

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
    sandboxCombo: html.includes('allow-scripts allow-forms allow-same-origin allow-popups'),
    rootHashCompared: /rootSha256Esperado\s*[!=]==/.test(html),
    linExecutesSource: html.includes('payloadObj.source') && !html.includes('function parseLin'),
  }));
})().catch((e) => {
  console.log(JSON.stringify({ ok: false, error: String(e) }));
  process.exit(1);
});
