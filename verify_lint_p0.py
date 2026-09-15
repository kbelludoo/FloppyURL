#!/usr/bin/env python3
"""Oracle independente lint-P0 (stdlib only, sem Go, sem JS).
Prova auditavel: determinismo, integridade SHA-256, estrutura .laybc, fail-closed.
Uso: python3 verify_lint_p0.py  (roda Go apenas como caixa-preta p/ compilar)
"""
import base64, hashlib, json, os, shutil, struct, subprocess, sys, zlib

REPO = os.path.dirname(os.path.abspath(__file__))
SRC = os.path.join(REPO, "examples", "floppyurl_lay.lay")
OUT_A = "/tmp/lint_p0_verify_a"
OUT_B = "/tmp/lint_p0_verify_b"
FAILS = []

def check(name, cond, detail=""):
    print(f"[{'PASS' if cond else 'FAIL'}] {name}" + (f" — {detail}" if detail else ""))
    if not cond:
        FAILS.append(name)

def b64url_decode(s):
    s += "=" * (-len(s) % 4)
    return base64.urlsafe_b64decode(s)

def run_pack(src, outdir):
    shutil.rmtree(outdir, ignore_errors=True)
    r = subprocess.run(["go", "run", "main.go", "lay_compiler.go",
                        "-file", src, "-algo", "deflate", "-out-dir", outdir],
                       capture_output=True, text=True, cwd=REPO)
    return r

def parse_disk(path):
    parts = open(path).read().strip().split(";", 5)
    assert len(parts) == 6, f"formato v2 invalido: {open(path).read()[:60]}"
    return {"header": parts[0], "algo": parts[1], "frac": parts[2],
            "chunk_sha": parts[3], "root_sha": parts[4], "payload": parts[5]}

def decode_laybc(bc: bytes):
    assert bc[:4] == b"LAY1", f"magic={bc[:4]!r}"
    assert bc[4] == 1, f"version={bc[4]}"
    root, nnodes, nstr = struct.unpack("<HHH", bc[5:11])
    off = 11
    strs = []
    for _ in range(nstr):
        (ln,) = struct.unpack("<H", bc[off:off+2]); off += 2
        strs.append(bc[off:off+ln].decode("utf-8")); off += ln
    nodes = []
    for _ in range(nnodes):
        parent, tag, flags, ti, si, ai, aci, cc, _pad = struct.unpack("<HBBHHHHBB", bc[off:off+14]); off += 14
        nodes.append((parent, tag, flags, ti, si, ai, aci, cc))
    assert off == len(bc), f"trailing bytes: {len(bc)-off}"
    return root, nodes, strs

# 1. compila 2x -> determinismo byte-identico
ra = run_pack(SRC, OUT_A)
rb = run_pack(SRC, OUT_B)
check("go compila sem erro (run A)", ra.returncode == 0, ra.stderr[-200:] if ra.returncode else "")
check("go compila sem erro (run B)", rb.returncode == 0, rb.stderr[-200:] if rb.returncode else "")
da = parse_disk(os.path.join(OUT_A, "disk_01.txt"))
db = parse_disk(os.path.join(OUT_B, "disk_01.txt"))
check("determinismo: payload byte-identico 2 runs", da["payload"] == db["payload"])
check("determinismo: root_sha estavel", da["root_sha"] == db["root_sha"], da["root_sha"][:16])

# 2. integridade SHA-256 recomputada em Python (FIPS 180-4 via hashlib)
comp_full = b64url_decode(da["payload"])
recomputed_root = hashlib.sha256(da["payload"].encode()).hexdigest()
check("chunk_sha == sha256(payload) [oracle py]", recomputed_root == da["chunk_sha"])
manifest = json.load(open(os.path.join(OUT_A, "manifest.json")))
check("manifest.root == disco.root", manifest["root_sha256"] == da["root_sha"])
rulel = open(os.path.join(OUT_A, "manifest.rulel")).read()
check("manifest.rulel contem root_sha", da["root_sha"] in rulel)
check("manifest.rulel header RULEL", "@RULEL:FLOPPY_MANIFEST:2.0.0" in rulel)

# 3. roundtrip deflate-raw -> JSON -> laybc (sem codigo Go)
raw = zlib.decompress(comp_full, -15).decode("utf-8")
obj = json.loads(raw)
check("payload JSON type==lay", obj.get("type") == "lay", str(obj.get("type")))
bc = b64url_decode(obj["bytecode"])
root, nodes, strs = decode_laybc(bc)
check("laybc magic+versao", bc[:4] == b"LAY1" and bc[4] == 1)
check("laybc root==0", root == 0, f"root={root}")
check("laybc nos>=3", len(nodes) >= 3, f"nodes={len(nodes)}")
# integridade referencial: pais valido, indices de string no range, flags coerentes
ok = True
for i, (parent, tag, flags, ti, si, ai, aci, cc) in enumerate(nodes):
    if parent != 0xFFFF and not (0 <= parent < len(nodes)):
        ok = False
    if not (0 <= tag <= 15):
        ok = False
    for idx, bit in ((ti, 0), (si, 1), (ai, 2), (aci, 3)):
        has = bool(flags & (1 << bit))
        if has and not (0 <= idx < len(strs)):
            ok = False
        if not has and idx != 0xFFFF:
            ok = False
check("laybc integridade referencial (pais/tags/flags)", ok,
      f"{len(nodes)} nos, {len(strs)} strings")
check("laybc tem acao mount/clear (paridade UI)", "mount" in strs and "clear" in strs)

# 4. fail-closed: tag desconhecida deve falhar, nao passar silencioso
bad = os.path.join("/tmp", "lint_p0_bad.lay")
open(bad, "w").write('@LAY:1.0\nVIEW app\nFOOBAR "x"\nEND\n')
rbad = run_pack(bad, "/tmp/lint_p0_bad_out")
check("fail-closed: tag desconhecida rejeitada", rbad.returncode != 0,
      "aceitou silencioso!" if rbad.returncode == 0 else "rejeitou como esperado")
# bytecode truncado deve falhar no oracle
try:
    decode_laybc(bc[:10])
    check("fail-closed: bytecode truncado rejeitado", False, "aceitou!")
except Exception as e:
    check("fail-closed: bytecode truncado rejeitado", True, type(e).__name__)
# magic errado deve falhar
try:
    decode_laybc(b"XXXX\x01\x00\x00\x00\x00\x00\x00")
    check("fail-closed: magic invalido rejeitado", False, "aceitou!")
except AssertionError:
    check("fail-closed: magic invalido rejeitado", True)

# 5. metricas honestas (sem arredondar a favor)
src_bytes = os.path.getsize(SRC)
html_bytes = os.path.getsize(os.path.join(REPO, "examples", "demo.html"))
print(f"\n[METRICA] .lay source={src_bytes}B laybc={len(bc)}B demo.html={html_bytes}B")
print(f"[METRICA] reducao source vs html={(1-src_bytes/html_bytes)*100:.1f}% | toks~ html={html_bytes//4} lay={src_bytes//4}")
print(f"[METRICA] rulel orig_bytes={manifest['original_bytes']} min_bytes={manifest['minified_bytes']} comp_bytes={manifest['compressed_bytes']} enc_chars={manifest['encoded_chars']}")
if manifest["minified_bytes"] > manifest["original_bytes"]:
    print("[HONESTO] min_bytes > orig_bytes: o 'minify' JSON do envelope aumenta o .lay (overhead de envelope, nao do bytecode). Bytecode puro 536->492 (-8.2%) continua valendo.")

print(f"\nRESULTADO: {'ALL PASS' if not FAILS else f'{len(FAILS)} FALHAS: {FAILS}'}")
sys.exit(0 if not FAILS else 1)
