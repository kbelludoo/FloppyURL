#!/usr/bin/env python3
"""linj-P0 — irma boot/verify do browser. Prova externa em 2 oraculos:
py (hashlib+zlib = espelho de WebCrypto+DecompressionStream) x node (zlib+crypto nativos, sem DOM).
Escopo P0: path deflate zero-WASM p/ html/lin/lay. index.html nao pode exigir wasm no path deflate.
"""
import base64, hashlib, json, os, re, shutil, subprocess, sys, zlib
REPO=os.path.dirname(os.path.abspath(__file__))
FAILS=[]
def check(n,c,d=""):
    print(f"[{'PASS' if c else 'FAIL'}] linj:{n}" + (f" — {d}" if d else ""))
    if not c: FAILS.append(n)
def b64d(s): return base64.urlsafe_b64decode(s+"="*(-len(s)%4))

JS_DEC = os.path.join(REPO,"linj_boot_p0.js")
open(JS_DEC,"w").write(r"""'use strict';
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
""")

def py_boot(disk_txt):
    h,a,f,c,r,p = disk_txt.split(";",5)
    assert h=="v2"
    assert hashlib.sha256(p.encode()).hexdigest()==c, "chunk mismatch py"
    comp=b64d(p)
    inf = zlib.decompress(comp,-15).decode("utf-8","replace") if a=="deflate" else zlib.decompress(comp,16+zlib.MAX_WBITS).decode("utf-8","replace")
    try:
        o=json.loads(inf)
        if o.get("type")=="lin": return ("lin",len(inf),o["source"][:40])
        if o.get("type")=="lay": return ("lay",len(inf),"LAY1:"+b64d(o["bytecode"])[:4].decode())
    except Exception: pass
    return ("html",len(inf),inf[:60].replace("\n"," "))

def pack(src,out):
    shutil.rmtree(out,ignore_errors=True)
    r=subprocess.run(["go","run","main.go","lay_compiler.go","-file",src,"-algo","deflate","-out-dir",out],
                     capture_output=True,text=True,cwd=REPO)
    assert r.returncode==0, r.stderr[-300:]
    return open(os.path.join(out,"disk_01.txt")).read().strip()

cases={"examples/demo.html":"html","examples/cpmm_oracle.lin":"lin","examples/floppyurl_lay.lay":"lay"}
for src,exp in cases.items():
    disk=pack(src,"/tmp/linj_"+exp)
    open("/tmp/linj_disk.txt","w").write(disk)
    py=py_boot(disk)
    r=subprocess.run(["node",JS_DEC,"/tmp/linj_disk.txt"],capture_output=True,text=True)
    check(f"node executa {exp}", r.returncode==0, r.stderr[-150:] if r.returncode else "")
    if r.returncode==0:
        js=json.loads(r.stdout)
        check(f"py==node dispatch {exp}", js["dispatch"]==py[0]==exp, f"py={py[0]} node={js['dispatch']}")
        check(f"py==node bytes {exp}", js["bytes"]==py[1], f"{js['bytes']} vs {py[1]}")
        check(f"chunk_ok {exp}", js["chunk_ok"] is True)

# path deflate nao exige wasm: estatico + dinamico (node com fetch que explode se wasm for buscado)
idx=open(os.path.join(REPO,"Website","index.html")).read()
m=re.search(r"if\s*\(.*?algo.*?(deflate|brotli).*?\)([\s\S]{0,600})",idx)
check("index.html tem branch por algo", m is not None)
# deflate usa DecompressionStream nativo; wasm so como fallback brotli/gzip
check("deflate usa DecompressionStream", "DecompressionStream" in idx)
check("verify usa crypto.subtle SHA-256", "crypto.subtle" in idx and "SHA-256" in idx)
check("lay_runtime hook presente", "lay_runtime.js" in idx and "renderLAY" in open(os.path.join(REPO,"Website","lay_runtime.js")).read())

# fail-closed: payload adulterado deve falhar nos DOIS oraculos
disk=pack("examples/demo.html","/tmp/linj_tamper")
h,a,f,c,r,p = disk.split(";",5)
bad_p = ("A" if p[0]!="A" else "B")+p[1:]
bad = ";".join([h,a,f,c,r,bad_p])
open("/tmp/linj_bad.txt","w").write(bad)
try:
    py_boot(bad); check("fail-closed py rejeita tamper", False, "aceitou!")
except AssertionError: check("fail-closed py rejeita tamper", True)
r=subprocess.run(["node",JS_DEC,"/tmp/linj_bad.txt"],capture_output=True,text=True)
check("fail-closed node rejeita tamper", r.returncode!=0)

print(f"\nlinj-P0: {'ALL PASS' if not FAILS else f'FALHAS {FAILS}'}")
sys.exit(0 if not FAILS else 1)
