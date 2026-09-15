#!/usr/bin/env python3
"""linj transpilador P0: js-subset -> .linj canonico. Stdlib only, fail-closed.
Subset: verify("sha256"), inflate("deflate"|"gzip"), dispatch("html"|"lin"|"lay"), boot("nome") — uma por linha.
Ordem canonica de saida: BOOT, EXPECT algo, EXPECT type, STEP VERIFY, STEP INFLATE, STEP DISPATCH, END."""
import re,sys
def transpile(src:str)->str:
    boot=None; inf=None; disp=None
    for i,raw in enumerate(src.splitlines(),1):
        l=raw.strip()
        if not l or l.startswith("//"): continue
        m=re.fullmatch(r'boot\(\s*"([^"]+)"\s*\)\s*;?',l)
        if m:
            if boot: raise ValueError(f"linha {i}: boot duplicado")
            boot=m.group(1); continue
        m=re.fullmatch(r'verify\(\s*"sha256"\s*\)\s*;?',l)
        if m: continue  # implicito no STEP
        m=re.fullmatch(r'inflate\(\s*"(deflate|gzip)"\s*\)\s*;?',l)
        if m:
            if inf: raise ValueError(f"linha {i}: inflate duplicado")
            inf=m.group(1); continue
        m=re.fullmatch(r'dispatch\(\s*"(html|lin|lay)"\s*\)\s*;?',l)
        if m:
            if disp: raise ValueError(f"linha {i}: dispatch duplicado")
            disp=m.group(1); continue
        raise ValueError(f"linha {i}: fora do subset linj-P0: {l[:60]!r}")
    if not boot: raise ValueError("boot() ausente")
    if not inf: raise ValueError("inflate() ausente")
    if not disp: raise ValueError("dispatch() ausente")
    return f"@LINJ:1.0\nBOOT {boot}\nEXPECT {inf}\nEXPECT {disp}\nSTEP VERIFY\nSTEP INFLATE\nSTEP DISPATCH\nEND\n"
if __name__=="__main__":
    src=open(sys.argv[1]).read() if len(sys.argv)>1 else sys.stdin.read()
    try: sys.stdout.write(transpile(src))
    except ValueError as e: sys.stderr.write(f"linj-transpile FAIL-CLOSED: {e}\n"); sys.exit(1)
