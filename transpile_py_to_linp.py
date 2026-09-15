#!/usr/bin/env python3
"""linp transpilador P0: python-subset -> .linp canonico. Stdlib only, fail-closed.
Subset: apenas chamadas pack("arquivo", algo="deflate|gzip|brotli", chunk=N) e job("nome"), uma por linha.
Qualquer outra sintaxe (import, for, if, f-string, metodo, etc) -> REJEITA.
Saida canonica ordenada -> bytes identicos."""
import re, sys
ALGOS={"deflate","gzip","brotli"}
def transpile(src:str)->str:
    jobs=[]; packs=[]
    for i,raw in enumerate(src.splitlines(),1):
        line=raw.strip()
        if not line or line.startswith("#"): continue
        m=re.fullmatch(r'job\(\s*"([^"]+)"\s*\)',line)
        if m:
            if jobs: raise ValueError(f"linha {i}: multiplos jobs fora do P0")
            jobs.append(m.group(1)); continue
        m=re.fullmatch(r'pack\(\s*"([^"]+)"\s*(?:,\s*algo\s*=\s*"([^"]+)"\s*)?(?:,\s*chunk\s*=\s*(\d+)\s*)?\)',line)
        if m:
            f,algo,chunk=m.group(1),m.group(2) or "deflate",int(m.group(3) or 1800000)
            if algo not in ALGOS: raise ValueError(f"linha {i}: algo {algo!r} desconhecido")
            if not (64<=chunk<=1800000): raise ValueError(f"linha {i}: chunk fora de [64,1800000]")
            if ".." in f or f.startswith("/"): raise ValueError(f"linha {i}: path fora do repo")
            packs.append((f,algo,chunk)); continue
        raise ValueError(f"linha {i}: fora do subset linp-P0: {raw.strip()[:60]!r}")
    if not jobs: raise ValueError("job() ausente")
    if not packs: raise ValueError("pack() ausente")
    if len(packs)>1: raise ValueError("P0: 1 PACK por job (multi-pack no roadmap)")
    out=[f"@LINP:1.0",f"JOB {jobs[0]}"]
    for f,a,c in packs: out+=["FILE "+f,f"ALGO {a}",f"CHUNK {c}","PACK"]
    out+=["END"]
    return "\n".join(out)+"\n"
if __name__=="__main__":
    src=open(sys.argv[1]).read() if len(sys.argv)>1 else sys.stdin.read()
    try: sys.stdout.write(transpile(src))
    except ValueError as e: sys.stderr.write(f"linp-transpile FAIL-CLOSED: {e}\n"); sys.exit(1)
