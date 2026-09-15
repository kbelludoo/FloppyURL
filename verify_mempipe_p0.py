#!/usr/bin/env python3
"""Prova externa mempipe-P0: pipeline 100% em memoria == pipeline em disco, byte-identico.
Mais: prova de ZERO escritas (cwd read-only) e fail-closed. Stdlib only."""
import json,os,shutil,stat,subprocess,sys,hashlib
REPO=os.path.dirname(os.path.abspath(__file__))
CLI=["go","run","./mempipe/cmd/mempipe"]
FAILS=[]
def check(n,c,d=""):
    print(f"[{'PASS' if c else 'FAIL'}] mem:{n}"+(f" — {d}" if d else ""))
    if not c: FAILS.append(n)
def mem(f,algo="deflate",chunk=1800000):
    return subprocess.run(CLI+["-file",f,"-algo",algo,"-chunk-size",str(chunk)],
                         capture_output=True,text=True,cwd=REPO)
def disk_go(f,algo="deflate",chunk=1800000):
    out="/tmp/mem_diskcmp"
    shutil.rmtree(out,ignore_errors=True)
    r=subprocess.run(["go","run","main.go","lay_compiler.go","-file",f,"-algo",algo,
                      "-chunk-size",str(chunk),"-out-dir",out],capture_output=True,text=True,cwd=REPO)
    assert r.returncode==0, r.stderr[-300:]
    fs=sorted(x for x in os.listdir(out) if x.startswith("disk_"))
    return [open(os.path.join(out,x)).read().strip() for x in fs]
for src in ["examples/floppyurl_lay.lay","examples/demo.html","examples/cpmm_oracle.lin"]:
    r=mem(src)
    check(f"mempipe {os.path.basename(src)}",r.returncode==0,r.stderr[-200:] if r.returncode else "")
    if r.returncode==0:
        b=json.loads(r.stdout)
        d=disk_go(src)
        check(f"in-mem==on-disk {os.path.basename(src)}",b["disks"]==d,
              f"root {b['root_sha256'][:16]}")
r=mem("examples/floppyurl_lay.lay"); b=json.loads(r.stdout)
# invariante de ponta a ponta: wrapper em-mem inflado tem bytecode LAY1.
# (receipt source/bytecode/root provado nos mesmos nucleos em linp-tc/linz-tc)
import base64,zlib
pay=b["disks"][0].split(";",5)[5]
raw_js=zlib.decompress(base64.urlsafe_b64decode(pay+"="*(-len(pay)%4)),-15).decode()
obj=json.loads(raw_js)
check("wrapper em-mem tem bytecode LAY1",
      base64.urlsafe_b64decode(obj["bytecode"]+"="*(-len(obj["bytecode"])%4))[:4]==b"LAY1")
# determinismo
check("deterministico 2 runs", mem("examples/demo.html").stdout==mem("examples/demo.html").stdout)
# ZERO escritas: usa o BINARIO compilado (go run exigiria go.mod no cwd, o que
# mascararia o teste) com cwd read-only; qualquer escrita local falha alto.
subprocess.run(["go","build","-o","/tmp/mempipe_bin","./mempipe/cmd/mempipe"],
               check=True,cwd=REPO,capture_output=True)
ro="/tmp/mem_ro"; shutil.rmtree(ro,ignore_errors=True); os.makedirs(ro); os.chmod(ro,0o555)
before=set(os.listdir(ro))
r=subprocess.run(["/tmp/mempipe_bin","-file",os.path.join(REPO,"examples/demo.html")],
                 capture_output=True,text=True,cwd=ro,env={**os.environ,"HOME":ro})
check("binario roda com cwd read-only (nada a escrever)",r.returncode==0,r.stderr[-200:] if r.returncode else "")
check("cwd continua vazio (zero writes)",set(os.listdir(ro))==before, str(set(os.listdir(ro))-before))
check("saida do binario == saida do go run",
      json.loads(r.stdout)["root_sha256"]==json.loads(mem("examples/demo.html").stdout)["root_sha256"])
src_cli=open(os.path.join(REPO,"mempipe/cmd/mempipe/main.go")).read()
check("CLI nao define flag -out-dir (por desenho)",
      '"out-dir",' not in src_cli and "WriteFile" not in src_cli and "MkdirAll" not in src_cli)
check("mempipe.go nao escreve em disco", "WriteFile" not in open(os.path.join(REPO,"mempipe/mempipe.go")).read())
# fail-closed
open("/tmp/mem_bad.lay","w").write("VIEW x\nFOOBAR oi\nEND\n")
check("fail-closed .lay invalido",mem("/tmp/mem_bad.lay").returncode!=0)
check("fail-closed algo",mem("examples/demo.html",algo="lzma").returncode!=0)
check("fail-closed chunk",mem("examples/demo.html",chunk=10).returncode!=0)
check("fail-closed inexistente",mem("examples/NAO.html").returncode!=0)
print(f"\nmempipe-P0: {'ALL PASS' if not FAILS else FAILS}")
sys.exit(0 if not FAILS else 1)
