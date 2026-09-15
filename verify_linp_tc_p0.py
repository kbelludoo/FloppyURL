#!/usr/bin/env python3
"""Prova externa linp transpilador+compilador P0. Stdlib only.
Transpile py-subset->.linp; compile .linp->.linpbc (Go); exec -> discos v2 == oracle py."""
import base64,hashlib,json,os,shutil,struct,subprocess,sys,zlib
REPO=os.path.dirname(os.path.abspath(__file__))
T=os.path.join(REPO,"transpile_py_to_linp.py")
CLI=["go","run","./linp/cmd/linp"]
FAILS=[]
def check(n,c,d=""):
    print(f"[{'PASS' if c else 'FAIL'}] linp-tc:{n}"+(f" — {d}" if d else ""))
    if not c: FAILS.append(n)
def tr(src):
    return subprocess.run(["python3",T],input=src,capture_output=True,text=True)
def run_cli(linp,out):
    shutil.rmtree(out,ignore_errors=True)
    return subprocess.run(CLI+["-file",linp,"-base",REPO,"-out-dir",out],capture_output=True,text=True,cwd=REPO)
GOOD='job("demo")\npack("examples/demo.html", algo="deflate", chunk=1800000)\n'
r1,tr2=tr(GOOD),tr(GOOD)
check("transpila subset",r1.returncode==0,r1.stderr[-150:] if r1.returncode else "")
check("transpile deterministico",r1.stdout==tr2.stdout and r1.stdout.startswith("@LINP:1.0"))
open("/tmp/tc.linp","w").write(r1.stdout)
a=run_cli("/tmp/tc.linp","/tmp/tc_a"); b=run_cli("/tmp/tc.linp","/tmp/tc_b")
check("compila+executa",a.returncode==0 and b.returncode==0,(a.stderr[-200:] if a.returncode else ""))
da=open("/tmp/tc_a/disk_01.txt").read().strip(); db=open("/tmp/tc_b/disk_01.txt").read().strip()
check("execucao deterministica",da==db)
# oracle py independente: bytes crus -> deflate -> b64 -> sha
raw=open(os.path.join(REPO,"examples/demo.html"),"rb").read()
import io
co=io.BytesIO(); 
import zlib as Z
compobj=Z.compressobj(9,Z.DEFLATED,-15); comp=compobj.compress(raw)+compobj.flush()
enc=base64.urlsafe_b64encode(comp).decode().rstrip("=")
h,a,f,c,r,p=da.split(";",5)
dec=zlib.decompress(base64.urlsafe_b64decode(p+"="*(-len(p)%4)),-15)
check("roundtrip: inflate(go)==bytes originais",dec==raw,f"{len(dec)} vs {len(raw)}")
check("chunk_sha == oracle py",c==hashlib.sha256(p.encode()).hexdigest())
check("root_sha == oracle py",r==hashlib.sha256(p.encode()).hexdigest())
bc=open("/tmp/tc_a/job.linpbc","rb").read()
check("magic LNP1",bc[:4]==b"LNP1" and bc[4]==1)
rec=json.load(open("/tmp/tc_a/receipt.json"))
check("receipt amarra source",rec["source_sha256"]==hashlib.sha256(r1.stdout.encode()).hexdigest())
check("receipt amarra bytecode",rec["bytecode_sha256"]==hashlib.sha256(bc).hexdigest())
check("receipt amarra root",rec["root_sha256"]==r)
for bad in ['import os\njob("x")\npack("a")\n','job("x")\nfor i in r:\n pack("a")\n',
            'job("x")\npack("/etc/passwd")\n','job("x")\npack("a", algo="lzma")\n',
            'pack("a")\n','job("x")\npack("a")\npack("b")\n']:
    check(f"transpile fail-closed {bad.splitlines()[0][:20]!r}",tr(bad).returncode!=0)
open("/tmp/bad.linp","w").write("@LINP:1.0\nJOB x\nFILE a\nALGO lzma\nCHUNK 100\nPACK\nEND\n")
check("compile fail-closed algo",run_cli("/tmp/bad.linp","/tmp/tc_bad").returncode!=0)
print(f"\nlinp-tc-P0: {'ALL PASS' if not FAILS else FAILS}")
sys.exit(0 if not FAILS else 1)
