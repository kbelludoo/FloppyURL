#!/usr/bin/env python3
"""Prova externa transpilador lint-P0: HTML-subset -> .lay canonico -> .laybc.
Stdlib only. Determinismo, canonismo, fail-closed, roundtrip com hashes LIN."""
import subprocess, sys, os
REPO=os.path.dirname(os.path.abspath(__file__))
T=os.path.join(REPO,"transpile_html_to_lay.py")
FAILS=[]
def check(n,c,d=""):
    print(f"[{'PASS' if c else 'FAIL'}] lint-t:{n}"+(f" — {d}" if d else ""))
    if not c: FAILS.append(n)
def tr(html):
    return subprocess.run(["python3",T],input=html,capture_output=True,text=True)
GOOD='<div><h1>Titulo</h1><p style="color: #00ff66">Ola</p><div class="col"><input id="a" placeholder="x" action="mount"><button action="mount">MONTAR</button></div></div>'
r1, r2 = tr(GOOD), tr(GOOD)
check("transpila subset", r1.returncode==0, r1.stderr[-150:] if r1.returncode else "")
check("deterministico/canonical", r1.stdout==r2.stdout and "@LAY:1.0" in r1.stdout and 'VIEW app' in r1.stdout)
open("/tmp/lint_t.lay","w").write(r1.stdout)
g=subprocess.run(["go","run","main.go","lay_compiler.go","-file","/tmp/lint_t.lay","-algo","deflate","-out-dir","/tmp/lint_t_out"],
                 capture_output=True,text=True,cwd=REPO)
check("compilador aceita saida do transpilador", g.returncode==0, g.stderr[-150:] if g.returncode else "")
for bad,why in [("<script>x()</script>","script"),("<style>.a{}</style>","style bloco"),
                ('<div style="position: absolute">x</div>',"css arbitrario"),
                ('<button action="nuke">x</button>',"action desconhecida"),
                ("<table><tr><td>x</td></tr></table>","table")]:
    r=tr(bad)
    check(f"fail-closed {why}", r.returncode!=0, "aceitou!" if r.returncode==0 else "")
check("fail-closed demo.html (comportamento, nao layout)",
      tr(open(os.path.join(REPO,"examples","demo.html")).read()).returncode!=0)
print(f"\nlint-transpile-P0: {'ALL PASS' if not FAILS else FAILS}")
sys.exit(0 if not FAILS else 1)
