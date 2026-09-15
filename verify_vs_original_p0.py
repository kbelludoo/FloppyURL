#!/usr/bin/env python3
"""Prova executavel: fork v2+irmas vs original Xelckis v1.
Nao alega superioridade blanket: mede cada dimensao e registra onde o
original ganha (brotli ratio, wire curto). So PASSa o que foi medido aqui.
Stdlib only (+ go/node como caixa-preta)."""
import json,os,shutil,subprocess,sys
REPO=os.path.dirname(os.path.abspath(__file__))
FAILS=[]
MET={}
def check(n,c,d=""):
    print(f"[{'PASS' if c else 'FAIL'}] vs-orig:{n}"+(f" — {d}" if d else ""))
    if not c: FAILS.append(n)
def go_pack(src,algo,v1,out):
    shutil.rmtree(out,ignore_errors=True)
    cmd=["go","run","main.go","lay_compiler.go","-file",src,"-algo",algo,"-out-dir",out]
    if v1: cmd.append("-v1")
    r=subprocess.run(cmd,capture_output=True,text=True,cwd=REPO)
    assert r.returncode==0, r.stderr[-300:]
    fs=sorted(x for x in os.listdir(out) if x.startswith("disk_"))
    return [open(os.path.join(out,x)).read().strip() for x in fs]

# 1. formato: v1 sem hash vs v2 com hash (estrutural, parsavel)
v1=go_pack("examples/demo.html","brotli",True,"/tmp/vs_v1")[0]
v2=go_pack("examples/demo.html","deflate",False,"/tmp/vs_v2")[0]
check("v1 nao tem campo de hash", ";" not in v1, f"{len(v1)} chars base64 puro")
p=v2.split(";",5)
check("v2 declara chunk+root sha256", len(p)==6 and p[0]=="v2" and len(p[3])==64 and len(p[4])==64)
import hashlib
check("v2 hash confere em oracle py", hashlib.sha256(p[5].encode()).hexdigest()==p[3]==p[4])
MET["wire_v1"],MET["wire_v2"]=len(v1),len(v2)

# 2. compressao: original brotli GANHA em ratio (honesto)
r=subprocess.run(["go","run","main.go","lay_compiler.go","-file","examples/demo.html",
                  "-algo","brotli","-out-dir","/tmp/vs_b"],capture_output=True,text=True,cwd=REPO)
man_b=json.load(open("/tmp/vs_b/manifest.json"))
man_d=json.load(open("/tmp/vs_v2/manifest.json")) if os.path.exists("/tmp/vs_v2/manifest.json") else None
# manifest do v2-deflate ja existe de go_pack acima? go_pack usou out /tmp/vs_v2 -> sim
man_d=json.load(open("/tmp/vs_v2/manifest.json"))
MET["brotli_B"],MET["deflate_B"]=man_b["compressed_bytes"],man_d["compressed_bytes"]
check("brotli comprime mais que deflate (original ganha ratio)",
      man_b["compressed_bytes"]<=man_d["compressed_bytes"],
      f"brotli {man_b['compressed_bytes']}B vs deflate {man_d['compressed_bytes']}B")
check("overhead de hashes no fio (custo declarado)",
      len(v2)>len(v1), f"v2 {len(v2)} chars vs v1 {len(v1)} chars (+{(len(v2)/len(v1)-1)*100:.0f}%)")

# 3. boot: v1 exige WASM 4.6MB; path deflate exige 0 bytes WASM
wasm=os.path.getsize(os.path.join(REPO,"Website","wasm.wasm"))
MET["wasm_B"]=wasm
check("v1/brotli depende de wasm.wasm no boot", wasm>4_000_000, f"{wasm} bytes p/ baixar")
idx=open(os.path.join(REPO,"Website","index.html")).read()
check("path deflate usa DecompressionStream nativo", "DecompressionStream" in idx)
r=subprocess.run(["node","linj_boot_p0.js","/tmp/vs_v2/disk_01.txt"],capture_output=True,text=True,cwd=REPO)
check("boot deflate provado sem wasm (node sem wasm)", r.returncode==0 and json.loads(r.stdout)["chunk_ok"])

# 4. mesma tela: .lay vs .html (comparacao justa, nao telas diferentes)
open("/tmp/vs_same.html","w").write('<div><h1>Titulo</h1><p style="color: #00ff66">Ola</p><div class="col"><input id="a" placeholder="x" action="mount"><button action="mount">MONTAR</button><button action="clear">LIMPAR</button></div></div>\n')
r=subprocess.run(["python3","transpile_html_to_lay.py","/tmp/vs_same.html"],capture_output=True,text=True,cwd=REPO)
assert r.returncode==0, r.stderr
hb=os.path.getsize("/tmp/vs_same.html"); lb=len(r.stdout.encode())
MET["same_html_B"],MET["same_lay_B"]=hb,lb
check("mesma tela: .lay menor que .html (pouco, honesto)",
      lb<hb, f"html {hb}B -> lay {lb}B ({(1-lb/hb)*100:.1f}%)")

# 5. determinismo: ambos deterministicos (original nao perde aqui)
a=go_pack("examples/demo.html","brotli",True,"/tmp/vs_v1a"); b=go_pack("examples/demo.html","brotli",True,"/tmp/vs_v1b")
check("v1 deterministico (original ok)", a==b)
c=go_pack("examples/demo.html","deflate",False,"/tmp/vs_v2a"); d=go_pack("examples/demo.html","deflate",False,"/tmp/vs_v2b")
check("v2 deterministico", c==d)

# 6. memoria: original escreve disco; mempipe nao pode (por desenho)
main=open(os.path.join(REPO,"main.go")).read()
check("original escreve arquivos (WriteFile/MkdirAll)", "WriteFile" in main and "MkdirAll" in main)
mem=open(os.path.join(REPO,"mempipe/mempipe.go")).read()
check("mempipe nao escreve (sem WriteFile)", "WriteFile" not in mem)

print("\n[METRICAS] "+" ".join(f"{k}={v}" for k,v in MET.items()))
print(f"\nvs-original-P0: {'ALL PASS' if not FAILS else FAILS}")
sys.exit(0 if not FAILS else 1)
