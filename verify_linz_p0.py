#!/usr/bin/env python3
"""linz-P0 — irma formato/bootstrap. Validador v2 + RuleL, fail-closed.
Prova: fixtures reais do packer Go; tamper de 1 char, truncamento, v1 legado e manifest quebrado sao rejeitados.
"""
import base64, hashlib, json, os, re, shutil, subprocess, sys, zlib
REPO=os.path.dirname(os.path.abspath(__file__))
FAILS=[]
def check(n,c,d=""):
    print(f"[{'PASS' if c else 'FAIL'}] linz:{n}" + (f" — {d}" if d else ""))
    if not c: FAILS.append(n)

def parse_v2(txt):
    parts=txt.strip().split(";",5)
    if len(parts)!=6: raise ValueError("v2 precisa de 6 campos")
    h,a,f,c,r,p=parts
    if h!="v2": raise ValueError(f"header={h}")
    if a not in ("deflate","brotli","gzip"): raise ValueError(f"algo={a}")
    if not re.fullmatch(r"\[\d+/\d+\]",f): raise ValueError(f"frac={f}")
    if not re.fullmatch(r"[0-9a-f]{64}",c): raise ValueError("chunk_sha nao-hex64")
    if not re.fullmatch(r"[0-9a-f]{64}",r): raise ValueError("root_sha nao-hex64")
    if not p: raise ValueError("payload vazio")
    return {"algo":a,"frac":f,"chunk_sha":c,"root_sha":r,"payload":p}

def validate_disk(txt):
    d=parse_v2(txt)
    if hashlib.sha256(d["payload"].encode()).hexdigest()!=d["chunk_sha"]:
        raise ValueError("chunk_sha mismatch (tamper)")
    return d

def validate_set(disk_txts, expect_root):
    """Valida conjunto: cada chunk + root do conjunto == manifest."""
    ds=[validate_disk(d) for d in disk_txts]
    full="".join(d["payload"] for d in ds)
    if hashlib.sha256(full.encode()).hexdigest()!=expect_root:
        raise ValueError("root mismatch (manifest)")
    if any(d["root_sha"]!=expect_root for d in ds):
        raise ValueError("disco declara root divergente do manifest")
    return ds

def validate_manifest_rulel(txt, expect_root):
    if "@RULEL:FLOPPY_MANIFEST:2.0.0" not in txt: raise ValueError("header RULEL ausente")
    m=re.search(r'\.root_sha256="([0-9a-f]{64})"',txt)
    if not m: raise ValueError("root_sha ausente no rulel")
    if m.group(1)!=expect_root: raise ValueError("root rulel != disco")
    hashes=re.findall(r'"([0-9a-f]{64})"',txt)
    if expect_root not in hashes: raise ValueError("root fora de disk_hashes")
    return True

def pack(src,out,extra=()):
    shutil.rmtree(out,ignore_errors=True)
    r=subprocess.run(["go","run","main.go","lay_compiler.go","-file",src,"-algo","deflate","-out-dir",out,*extra],
                     capture_output=True,text=True,cwd=REPO)
    assert r.returncode==0, r.stderr[-300:]

# fixtures reais
pack("examples/demo.html","/tmp/linz_single")
single=open("/tmp/linz_single/disk_01.txt").read().strip()
pack("examples/defi_swap_lin.html","/tmp/linz_raid",("-chunk-size","800"))
raid=[open(f"/tmp/linz_raid/{f}").read().strip() for f in sorted(os.listdir("/tmp/linz_raid")) if f.startswith("disk_")]
man=json.load(open("/tmp/linz_raid/manifest.json"))
rulel=open("/tmp/linz_raid/manifest.rulel").read()

try: validate_disk(single); check("single valido aceito",True)
except Exception as e: check("single valido aceito",False,str(e))
try:
    ds=[validate_disk(d) for d in raid]
    roots={d["root_sha"] for d in ds}
    fracs=[d["frac"] for d in ds]
    check("raid todos validos + root unico", len(roots)==1, str(roots)[:20])
    check("raid frac cobre 1..N", fracs==[f"[{i}/{len(raid)}]" for i in range(1,len(raid)+1)])
    full="".join(d["payload"] for d in ds)
    check("raid root==sha256(full)", hashlib.sha256(full.encode()).hexdigest()==man["root_sha256"])
    validate_manifest_rulel(rulel, man["root_sha256"])
    check("rulel raid valido",True)
except Exception as e: check("raid/rulel validos",False,str(e))

# ataques / quebras — todos DEVEM falhar
h,a,f,c,r,p = single.split(";",5)
bad_p=("A" if p[0]!="A" else "B")+p[1:]
try: validate_disk(";".join([h,a,f,c,r,bad_p])); check("rejeita tamper 1 char",False,"aceitou!")
except ValueError: check("rejeita tamper 1 char",True)
# HONESTO: root trocado nao e detectavel no disco isolado (chunk continua valido);
# a deteccao correta e no conjunto vs manifest — e e isso que o boot faz.
try: validate_disk(";".join([h,a,f,c,"0"*64,p])); check("chunk isolado ignora root (esperado)",True,"chunk ok, root pego no set")
except ValueError: check("chunk isolado ignora root (esperado)",False,"nao devia falhar no chunk")
try: validate_set([";".join([h,a,f,c,"0"*64,p])], r); check("rejeita root trocado no set",False,"aceitou!")
except ValueError: check("rejeita root trocado no set",True)
try: parse_v2("v2;deflate;[1/1];abc;def"); check("rejeita truncado",False,"aceitou!")
except ValueError: check("rejeita truncado",True)
try: parse_v2("v1;[1/1]"+p); check("rejeita legado v1 (Xelckis, sem hash)",False,"aceitou v1!")
except ValueError as e: check("rejeita legado v1 (Xelckis, sem hash)",True,str(e)[:40])
try: validate_manifest_rulel(rulel.replace(man["root_sha256"],"f"*64), man["root_sha256"]); check("rejeita rulel adulterado",False,"aceitou!")
except ValueError: check("rejeita rulel adulterado",True)
try: validate_manifest_rulel("lixo sem header", man["root_sha256"]); check("rejeita rulel sem header",False,"aceitou!")
except ValueError: check("rejeita rulel sem header",True)
# disco faltando no raid
try:
    if len(raid)>=2:
        short="".join(d.split(";",5)[5] for d in raid[:-1])
        if hashlib.sha256(short.encode()).hexdigest()==man["root_sha256"]:
            check("rejeita raid incompleto",False,"root bateu sem 1 disco!")
        else: check("rejeita raid incompleto",True,"root nao bate sem todos os discos")
    else: check("rejeita raid incompleto",False,"raid tem 1 disco so")
except Exception as e: check("rejeita raid incompleto",False,str(e))

print(f"\nlinz-P0: {'ALL PASS' if not FAILS else f'FALHAS {FAILS}'}")
sys.exit(0 if not FAILS else 1)
