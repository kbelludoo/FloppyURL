#!/usr/bin/env python3
"""Prova externa linj transpilador+compilador P0 + amarracao plano->artefato real."""
import json,os,shutil,subprocess,sys
REPO=os.path.dirname(os.path.abspath(__file__))
T=os.path.join(REPO,"transpile_js_to_linj.py")
FAILS=[]
def check(n,c,d=""):
    print(f"[{'PASS' if c else 'FAIL'}] linj-tc:{n}"+(f" — {d}" if d else ""))
    if not c: FAILS.append(n)
def tr(s): return subprocess.run(["python3",T],input=s,capture_output=True,text=True)
def comp(linp,out):
    return subprocess.run(["go","run","./linj/cmd/linj","-file",linp,"-out",out],capture_output=True,text=True,cwd=REPO)
GOOD='boot("b");\nverify("sha256");\ninflate("deflate");\ndispatch("lay");\n'
r1,r2=tr(GOOD),tr(GOOD)
check("transpila subset",r1.returncode==0)
check("deterministico",r1.stdout==r2.stdout and r1.stdout.startswith("@LINJ:1.0"))
open("/tmp/j.linj","w").write(r1.stdout)
c1=comp("/tmp/j.linj","/tmp/j1.linjbc"); c2=comp("/tmp/j.linj","/tmp/j2.linjbc")
check("compila",c1.returncode==0 and c2.returncode==0)
check("bytecode deterministico",open("/tmp/j1.linjbc","rb").read()==open("/tmp/j2.linjbc","rb").read())
check("magic LNJ1",open("/tmp/j1.linjbc","rb").read()[:4]==b"LNJ1")
# amarracao: plano (deflate,lay) x artefato real floppyurl_lay.lay
subprocess.run(["go","run","main.go","lay_compiler.go","-file","examples/floppyurl_lay.lay",
                "-algo","deflate","-out-dir","/tmp/j_bind"],capture_output=True,cwd=REPO)
r=subprocess.run(["node","linj_boot_p0.js","/tmp/j_bind/disk_01.txt"],capture_output=True,text=True,cwd=REPO)
check("plano executa artefato real",r.returncode==0 and json.loads(r.stdout)["dispatch"]=="lay")
for bad in ['boot("b");\ninflate("lzma");\ndispatch("html");\n','boot("b");\nfetch(url);\ndispatch("html");\n',
            'inflate("deflate");\ndispatch("html");\n','boot("b");\ninflate("deflate");\ndispatch("exe");\n']:
    check(f"transpile fail-closed {bad.splitlines()[-1][:24]!r}",tr(bad).returncode!=0)
open("/tmp/jbad.linj","w").write("@LINJ:1.0\nBOOT b\nEXPECT deflate\nEXPECT html\nSTEP INFLATE\nSTEP VERIFY\nSTEP DISPATCH\nEND\n")
check("compile fail-closed ordem",comp("/tmp/jbad.linj","/tmp/jbad.linjbc").returncode!=0)
print(f"\nlinj-tc-P0: {'ALL PASS' if not FAILS else FAILS}")
sys.exit(0 if not FAILS else 1)
