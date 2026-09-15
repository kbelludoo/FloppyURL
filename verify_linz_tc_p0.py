#!/usr/bin/env python3
"""Prova externa linz transpilador+compilador+assembler P0. Stdlib only.
Amarracao: assemble(payload do linp, mesmo chunk) == discos do linp byte-identicos."""
import hashlib,json,os,shutil,subprocess,sys
REPO=os.path.dirname(os.path.abspath(__file__))
T=os.path.join(REPO,"transpile_spec_to_linz.py")
FAILS=[]
def check(n,c,d=""):
    print(f"[{'PASS' if c else 'FAIL'}] linz-tc:{n}"+(f" — {d}" if d else ""))
    if not c: FAILS.append(n)
def tr(s): return subprocess.run(["python3",T],input=s,capture_output=True,text=True)
def comp(f,o): return subprocess.run(["go","run","./linz/cmd/linz","-file",f,"-payload",o,"-out-dir","/tmp/linz_tmp"],
                                     capture_output=True,text=True,cwd=REPO)
GOOD='envelope("v2")\npayload("demo")\nalgo("deflate")\nchunk(1800000)\n'
r1,r2=tr(GOOD),tr(GOOD)
check("transpila subset",r1.returncode==0,r1.stderr[-150:] if r1.returncode else "")
check("deterministico",r1.stdout==r2.stdout and r1.stdout.startswith("@LINZ:1.0"))
open("/tmp/z.linz","w").write(r1.stdout)
# fixture: pipeline linp real
subprocess.run(["go","run","./linp/cmd/linp","-file","/tmp/demo.linp","-base",REPO,"-out-dir","/tmp/linz_linp"],
               capture_output=True,cwd=REPO)
pay=open("/tmp/linz_linp/disk_01.txt").read().strip().split(";",5)[5]
open("/tmp/z.pay","w").write(pay)
c=comp("/tmp/z.linz","/tmp/z.pay")
check("compila+monta",c.returncode==0,c.stderr[-200:] if c.returncode else "")
lz=open("/tmp/linz_tmp/disk_01.txt").read().strip()
lp=open("/tmp/linz_linp/disk_01.txt").read().strip()
check("assemble==linp byte-identico (mesmo chunk)",lz==lp)
check("root igual",lz.split(";",5)[4]==lp.split(";",5)[4],lz.split(";",5)[4][:16])
# mesma payload, chunk diferente -> mesma raiz, fatiamento diferente (propriedade)
open("/tmp/z800.linz","w").write(r1.stdout.replace("1800000","800"))
c2=subprocess.run(["go","run","./linz/cmd/linz","-file","/tmp/z800.linz","-payload","/tmp/z.pay","-out-dir","/tmp/linz_800"],
                  capture_output=True,text=True,cwd=REPO)
d800=open("/tmp/linz_800/disk_01.txt").read().strip()
check("raiz independe do chunk",d800.split(";",5)[4]==lp.split(";",5)[4])
check("chunk 800 fatia em 2",d800.split(";",5)[2]=="[1/2]")
bc=open("/tmp/linz_tmp/job.linzbc","rb").read()
check("magic LNZ1",bc[:4]==b"LNZ1" and bc[4]==1)
rec=json.load(open("/tmp/linz_tmp/receipt.json"))
check("receipt amarra source+bytecode+root",
      rec["source_sha256"]==hashlib.sha256(r1.stdout.encode()).hexdigest() and
      rec["bytecode_sha256"]==hashlib.sha256(bc).hexdigest() and rec["root_sha256"]==lp.split(";",5)[4])
for bad in ['envelope("v1")\npayload("a")\nalgo("deflate")\nchunk(800)\n',
            'envelope("v2")\npayload("/etc/x")\nalgo("deflate")\nchunk(800)\n',
            'envelope("v2")\npayload("a")\nalgo("lzma")\nchunk(800)\n',
            'envelope("v2")\npayload("a")\nalgo("deflate")\nchunk(10)\n',
            'envelope("v2")\nalgo("deflate")\nchunk(800)\n']:
    check(f"transpile fail-closed {bad.splitlines()[0][:26]!r}",tr(bad).returncode!=0)
open("/tmp/zbad.linz","w").write("@LINZ:1.0\nENVELOPE v2\nPAYLOAD a\nALGO lzma\nCHUNK 800\nASSEMBLE\nEND\n")
check("compile fail-closed algo",comp("/tmp/zbad.linz","/tmp/z.pay").returncode!=0)
open("/tmp/zempty.pay","w").write("")
check("assemble fail-closed vazio",comp("/tmp/z.linz","/tmp/zempty.pay").returncode!=0)
print(f"\nlinz-tc-P0: {'ALL PASS' if not FAILS else FAILS}")
sys.exit(0 if not FAILS else 1)
