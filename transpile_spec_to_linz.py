#!/usr/bin/env python3
"""linz transpilador P0: spec textual -> .linz canonico. Stdlib only, fail-closed.
Subset: envelope("v2"), payload("nome"), algo("deflate"|"gzip"|"brotli"), chunk(N)."""
import re,sys
def transpile(src:str)->str:
    env=None; pay=None; algo=None; chunk=None
    for i,raw in enumerate(src.splitlines(),1):
        l=raw.strip()
        if not l or l.startswith("#"): continue
        m=re.fullmatch(r'envelope\(\s*"([^"]+)"\s*\)',l)
        if m:
            if m.group(1)!="v2": raise ValueError(f"linha {i}: envelope so v2 no P0")
            env="v2"; continue
        m=re.fullmatch(r'payload\(\s*"([^"]+)"\s*\)',l)
        if m:
            if ".." in m.group(1) or m.group(1).startswith("/"): raise ValueError(f"linha {i}: path fora do repo")
            pay=m.group(1); continue
        m=re.fullmatch(r'algo\(\s*"(deflate|gzip|brotli)"\s*\)',l)
        if m: algo=m.group(1); continue
        m=re.fullmatch(r'chunk\(\s*(\d+)\s*\)',l)
        if m:
            chunk=int(m.group(1))
            if not (64<=chunk<=1800000): raise ValueError(f"linha {i}: chunk fora de [64,1800000]")
            continue
        raise ValueError(f"linha {i}: fora do subset linz-P0: {l[:60]!r}")
    if not (env and pay and algo and chunk): raise ValueError("spec incompleta (envelope/payload/algo/chunk)")
    return f"@LINZ:1.0\nENVELOPE {env}\nPAYLOAD {pay}\nALGO {algo}\nCHUNK {chunk}\nASSEMBLE\nEND\n"
if __name__=="__main__":
    src=open(sys.argv[1]).read() if len(sys.argv)>1 else sys.stdin.read()
    try: sys.stdout.write(transpile(src))
    except ValueError as e: sys.stderr.write(f"linz-transpile FAIL-CLOSED: {e}\n"); sys.exit(1)
