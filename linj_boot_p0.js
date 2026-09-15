'use strict';
// linj boot P0 — node stdlib only (crypto, zlib, fs). Sem DOM, sem wasm, sem deps.
const crypto=require('crypto'), zlib=require('zlib'), fs=require('fs');
function b64url(s){ s=s.replace(/-/g,'+').replace(/_/g,'/'); while(s.length%4)s+='='; return Buffer.from(s,'base64'); }
function main(){
  const f=process.argv[2];
  const raw=fs.readFileSync(f,'utf8').trim();
  const p=raw.split(';'); if(p.length<6) throw new Error('formato v2 invalido');
  const [h,algo,frac,chunk_sha,root_sha]=p; const payload=p.slice(5).join(';');
  if(h!=='v2') throw new Error('header!=v2');
  const cc=crypto.createHash('sha256').update(payload,'utf8').digest('hex');
  if(cc!==chunk_sha) throw new Error('chunk_sha mismatch');
  const comp=b64url(payload);
  let inflated;
  if(algo==='deflate') inflated=zlib.inflateRawSync(comp).toString('utf8');
  else if(algo==='gzip') inflated=zlib.gunzipSync(comp).toString('utf8');
  else throw new Error('algo sem path zero-wasm nesta P0: '+algo);
  let dispatch='html', marker='';
  try{ const o=JSON.parse(inflated);
    if(o.type==='lin'){dispatch='lin'; marker=o.source.slice(0,40);}
    else if(o.type==='lay'){dispatch='lay'; marker='LAY1:'+Buffer.from(o.bytecode.replace(/-/g,'+').replace(/_/g,'/'),'base64').slice(0,4).toString();}
    else {dispatch='json'; marker=inflated.slice(0,40);}
  }catch(e){ marker=inflated.slice(0,60).replace(/\n/g,' '); }
  console.log(JSON.stringify({algo,frac,dispatch,chunk_ok:true,bytes:inflated.length,marker}));
}
main();
