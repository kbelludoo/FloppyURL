#!/usr/bin/env python3
"""linp-P0 — irma packer/manifest. Oracle externo stdlib-only.
Prova que o empacotador Go e deterministico e que manifest.json <-> manifest.rulel <-> discos batem.
Escopo P0 congelado: deflate single + multidisk + lin/lay/html dispatch. Sem brotli mandatory.
"""
import base64, hashlib, json, os, shutil, subprocess, sys, zlib
REPO = os.path.dirname(os.path.abspath(__file__))
FAILS = []
def check(n, c, d=""):
    print(f"[{'PASS' if c else 'FAIL'}] linp:{n}" + (f" — {d}" if d else ""))
    if not c: FAILS.append(n)
def b64d(s): return base64.urlsafe_b64decode(s + "=" * (-len(s) % 4))
def pack(src, out, extra=()):
    shutil.rmtree(out, ignore_errors=True)
    r = subprocess.run(["go","run","main.go","lay_compiler.go","-file",src,
                        "-algo","deflate","-out-dir",out,*extra],
                       capture_output=True, text=True, cwd=REPO)
    return r
def disks(out):
    fs = sorted(f for f in os.listdir(out) if f.startswith("disk_") and f.endswith(".txt"))
    return [open(os.path.join(out,f)).read().strip() for f in fs]

# 1. determinismo por tipo
for src in ["examples/demo.html","examples/cpmm_oracle.lin","examples/floppyurl_lay.lay"]:
    a, b = "/tmp/linp_a", "/tmp/linp_b"
    ra, rb = pack(src,a), pack(src,b)
    check(f"compila {os.path.basename(src)}", ra.returncode==0 and rb.returncode==0,
          (ra.stderr[-150:] if ra.returncode else ""))
    if ra.returncode==0:
        check(f"deterministico {os.path.basename(src)}", disks(a)==disks(b))

# 2. hashes recomputados em python p/ cada disco do demo.html
pack("examples/demo.html","/tmp/linp_demo")
man = json.load(open("/tmp/linp_demo/manifest.json"))
rulel = open("/tmp/linp_demo/manifest.rulel").read()
ds = disks("/tmp/linp_demo")
# parse real:
ok=True
assembled=""
for d in ds:
    h,a,f,c,r,p = d.split(";",5)
    if h!="v2" or a!="deflate": ok=False
    if hashlib.sha256(p.encode()).hexdigest()!=c: ok=False
    if r!=man["root_sha256"]: ok=False
    assembled+=p
check("v2 hashes por-disco conferem (py oracle)", ok)
check("root == sha256(payload completo)", hashlib.sha256(assembled.encode()).hexdigest()==man["root_sha256"])
check("rulel contem root + discos", man["root_sha256"] in rulel and all(
    hashlib.sha256(dict(zip(["h","a","f","c","r","p"],d.split(";",5)))["p"].encode()).hexdigest() in rulel for d in ds))
check("manifest.json<->rulel bytes consistentes",
      man["original_bytes"]==int([l.split("=")[1] for l in rulel.splitlines() if l.startswith(".orig_bytes")][0]) and
      man["compressed_bytes"]==int([l.split("=")[1] for l in rulel.splitlines() if l.startswith(".comp_bytes")][0]))

# 3. roundtrip conteudo por tipo (inflate real, nao so hash)
def inflate_payload(disk_txt):
    p = disk_txt.split(";",5)[5]
    return zlib.decompress(b64d(p),-15)
html_raw = inflate_payload(disks("/tmp/linp_demo")[0]).decode("utf-8", "replace")
check("html roundtrip contem marcador", "FloppyURL v2.0 Operacional" in html_raw)
pack("examples/cpmm_oracle.lin","/tmp/linp_lin")
lin_raw = json.loads(inflate_payload(disks("/tmp/linp_lin")[0]).decode())
check("lin dispatch type==lin + source", lin_raw.get("type")=="lin" and "@CPMM_ORACLE" in lin_raw.get("source",""))
pack("examples/floppyurl_lay.lay","/tmp/linp_lay")
lay_raw = json.loads(inflate_payload(disks("/tmp/linp_lay")[0]).decode())
check("lay dispatch type==lay + bytecode LAY1", lay_raw.get("type")=="lay" and b64d(lay_raw["bytecode"])[:4]==b"LAY1")

# 4. multidisk: chunk-size pequeno forca RAID-0, remontagem byte-identica
r = pack("examples/defi_swap_lin.html","/tmp/linp_raid",("-chunk-size","800"))
man2 = json.load(open("/tmp/linp_raid/manifest.json"))
ds2 = disks("/tmp/linp_raid")
check("raid-0 multi-disco forcado", man2["total_disks"]>1, f"N={man2['total_disks']}")
assembled2="".join(d.split(";",5)[5] for d in ds2)
check("raid root estavel", hashlib.sha256(assembled2.encode()).hexdigest()==man2["root_sha256"])
dec2 = zlib.decompress(b64d(assembled2),-15).decode("utf-8","replace")
check("raid conteudo remontado", "LIN Sovereign Swap" in dec2)
# frac sequencial [i/N]
fracs=[d.split(";",5)[2] for d in ds2]
check("frac sequencial [i/N]", fracs==[f"[{i}/{len(ds2)}]" for i in range(1,len(ds2)+1)], ",".join(fracs[:4]))

# 5. fail-closed: arquivo inexistente deve falhar, nao emitir disco vazio
r = pack("examples/NAO_EXISTE.html","/tmp/linp_missing")
check("fail-closed arquivo inexistente", r.returncode!=0)

print(f"\nlinp-P0: {'ALL PASS' if not FAILS else f'FALHAS {FAILS}'}")
sys.exit(0 if not FAILS else 1)
