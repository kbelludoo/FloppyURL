#!/usr/bin/env python3
"""
Refutation suite for FloppyURL v2.0 marketing claims.

Each test PASSES when the claim is empirically shown false against this
tree. A later honest implementation should make the matching test FAIL.

Honest survivors (checksum of a self-declared chunk; deflate roundtrip)
are printed separately and do not count as refutations.
"""
from __future__ import annotations

import base64
import hashlib
import os
import re
import shutil
import subprocess
import sys
import zlib

ROOT = os.path.dirname(os.path.abspath(__file__))
BOOT = os.path.join(ROOT, "Website", "index.html")
BOOT_JS = os.path.join(ROOT, "Website", "boot.js")
README = os.path.join(ROOT, "README.md")
MAIN_GO = os.path.join(ROOT, "main.go")
LIN = os.path.join(ROOT, "examples", "cpmm_oracle.lin")
DEFI = os.path.join(ROOT, "examples", "defi_swap_lin.html")
WASM = os.path.join(ROOT, "Website", "wasm.wasm")
V2_RE = re.compile(
    r"^v2;([a-z0-9_-]+);\[(\d+)/(\d+)\];([a-f0-9]{64});([a-f0-9]{64});(.*)$"
)


def read(path: str) -> str:
    with open(path, "r", encoding="utf-8", errors="replace") as fh:
        return fh.read()


def boot_src() -> str:
    return read(BOOT) + "\n" + read(BOOT_JS)


def sha(s: str) -> str:
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def pack_deflate(data: bytes, chunk_size: int | None = None) -> list[str]:
    c = zlib.compressobj(9, zlib.DEFLATED, -15)
    comp = c.compress(data) + c.flush()
    b64 = base64.urlsafe_b64encode(comp).rstrip(b"=").decode("ascii")
    root = sha(b64)
    n = chunk_size or len(b64) or 1
    disks = []
    total = max(1, (len(b64) + n - 1) // n)
    for i in range(total):
        payload = b64[i * n : (i + 1) * n]
        disks.append(
            f"v2;deflate;[{i+1}/{total}];{sha(payload)};{root};{payload}"
        )
    return disks


class Boot:
    """Mirror of Website/boot.js processarHash + verificarProgresso."""

    def __init__(self, pin: str | None = None, legacy: bool = False, require_pin: bool = False) -> None:
        self.disks: list[str | None] = []
        self.total = 0
        self.root: str | None = None
        self.algo = "brotli"
        self.rejected = False
        self.executed: str | None = None
        self.pin = (pin or "").replace("sha256:", "").lower()
        self.legacy = legacy
        self.require_pin = require_pin

    def process(self, raw: str) -> None:
        raw = raw[1:] if raw.startswith("#") else raw
        raw = raw.strip()
        m = V2_RE.match(raw)
        if m:
            self.algo, a, t, decl, root, payload = (
                m.group(1), int(m.group(2)), int(m.group(3)),
                m.group(4), m.group(5), m.group(6),
            )
            if self.root and root != self.root:
                self.rejected = True
                return
            self.total = t
            if self.root is None:
                self.root = root
            if sha(payload) != decl:
                self.rejected = True
                return
            if len(self.disks) != self.total:
                self.disks = [None] * self.total
            self.disks[a - 1] = payload
            self._maybe_exec()
            return
        if not self.legacy:
            self.rejected = True
            return
        if re.match(r"^v1;\[(\d+)/(\d+)\]", raw):
            mm = re.match(r"^v1;\[(\d+)/(\d+)\](.*)", raw)
            assert mm
            self.algo = "brotli"
            self.total = int(mm.group(2))
            if len(self.disks) != self.total:
                self.disks = [None] * self.total
            self.disks[int(mm.group(1)) - 1] = mm.group(3)
            self._maybe_exec()
            return
        self.disks = [raw]
        self.total = 1
        self.algo = "brotli"
        self._maybe_exec()

    def _maybe_exec(self) -> None:
        if not (self.total and all(self.disks)):
            return
        assembled = "".join(self.disks)  # type: ignore[arg-type]
        got = sha(assembled)
        if self.root and got != self.root:
            self.rejected = True
            return
        if self.require_pin and not self.pin:
            self.rejected = True
            return
        if self.pin and self.pin != got:
            self.rejected = True
            return
        self.executed = assembled


def inflate(b64: str) -> bytes:
    pad = "=" * ((4 - len(b64) % 4) % 4)
    return zlib.decompress(base64.urlsafe_b64decode(b64 + pad), -15)


passed = 0
failed = 0
survived: list[str] = []


def refute(name: str, ok: bool, detail: str) -> None:
    global passed, failed
    status = "REFUTED" if ok else "NOT-REFUTED"
    print(f"[{status}] {name}: {detail}")
    if ok:
        passed += 1
    else:
        failed += 1


def survive(name: str, detail: str) -> None:
    survived.append(name)
    print(f"[SURVIVED] {name}: {detail}")


def test_zero_byte() -> None:
    wasm = os.path.getsize(WASM) if os.path.exists(WASM) else 0
    if wasm > 4_000_000:
        survive(
            "brotli fallback wasm still on disk (~4.6MB)",
            f"wasm.wasm={wasm}B — deflate path must not fetch it; README no longer claims zero-byte",
        )
    else:
        refute("wasm.wasm still shipped", False, "file missing or tiny")


def test_lazy_scripts() -> None:
    html = read(BOOT)
    js = read(BOOT_JS)
    if (
        "wasm_exec.js" not in html
        and "lay_runtime.js" not in html
        and 'src="boot.js"' in html
        and "loadScript('wasm_exec.js')" in js
        and "loadScript('lay_runtime.js')" in js
    ):
        survive(
            "deflate boot does not fetch WASM/LAY up front",
            "index.html only loads boot.js; wasm_exec/lay_runtime are lazy",
        )
    else:
        refute("lazy script loading", False, "unconditional wasm/lay still in index.html")


def test_self_attestation() -> None:
    evil = b"<html><body><h1>PWNED-FORGED-PAYLOAD</h1></body></html>"
    disk = pack_deflate(evil)[0]
    b = Boot()
    b.process(disk)
    body = inflate(b.executed or "")
    refute(
        "hashes inside the fragment are a signature / trusted pin",
        b.executed is not None and b"PWNED-FORGED-PAYLOAD" in body,
        "without ?pin= from a trusted channel, rewrite+rehash still boots (checksum only)",
    )


def test_query_pin_rejects_rewrite() -> None:
    honest = pack_deflate(b"<html>honest</html>")[0]
    evil = pack_deflate(b"<html>PWNED</html>")[0]
    root = V2_RE.match(honest).group(5)
    b = Boot(pin=root)
    b.process(evil)
    if b.rejected and b.executed is None:
        survive("?pin=<root> rejects a rewritten self-consistent disk", f"pin={root[:16]}...")
    else:
        refute("query pin", False, f"rejected={b.rejected} executed={b.executed is not None}")


def test_chunk_checksum_survives() -> None:
    disk = pack_deflate(b"<html>ok</html>")[0]
    parts = disk.split(";", 5)
    parts[5] = parts[5][:-1] + ("A" if parts[5][-1] != "A" else "B")
    b = Boot()
    b.process(";".join(parts))
    if b.rejected and b.executed is None:
        survive("chunk SHA-256 checksum", "stale digest rejected")
    else:
        refute("chunk checksum", False, f"rejected={b.rejected}")


def test_root_hash_checked() -> None:
    js = read(BOOT_JS)
    disks = pack_deflate(b"<html>root-mismatch</html>", chunk_size=8)
    assert len(disks) > 1
    parts = disks[1].split(";", 5)
    parts[4] = "f" * 64
    disks[1] = ";".join(parts)
    b = Boot()
    for d in disks:
        b.process(d)
    if "root_sha256 diverge" in js and b.rejected and b.executed is None:
        survive("assembled/per-disk root SHA-256 fail-closed", "mismatched roots rejected")
    else:
        refute("root hash check", False, f"rejected={b.rejected} executed={b.executed is not None}")


def test_raid0_is_concat() -> None:
    go = read(MAIN_GO)
    rm = read(README)
    if "chunkPayload := encodedData[start:end]" in go and "not RAID-0" in rm and "RAID-0" not in go:
        survive("multi-volume concat", "packager slices Base64 sequentially; README says not RAID-0")
    else:
        refute("RAID-0 wording", False, "code or README still claims RAID-0")


def test_lin_kernel_is_a_dump() -> None:
    js, src = read(BOOT_JS), read(LIN)
    refute(
        "LIN sovereign kernel / heapless VM",
        "NAO interpreta LIN" in js
        and "function parseLin" not in js
        and "0_BYTES" not in js
        and "@LIN:L1c" in src,
        "bootloader is honest now, but still has no LIN interpreter",
    )


def test_lin_oracle_divides() -> None:
    src = read(LIN)
    if "^ (numerator / denominator);" in src and "_lia_ushr(numerator, 0)" not in src:
        survive("CPMM oracle formula", "source returns numerator/denominator (still not executed)")
    else:
        refute("oracle math", False, "division still missing")


def test_defi_is_js_demo() -> None:
    src = read(DEFI)
    refute(
        "on-chain / LIN-VM DeFi",
        "let reserveIn = 10000.0" in src and "Date.now()" in src and "not a LIN VM" in src,
        "still in-page JS floats; labels are honest, it is not a kernel",
    )


def test_sandbox_no_same_origin() -> None:
    js = read(BOOT_JS)
    has = "setAttribute('sandbox', 'allow-scripts allow-forms')" in js
    if has and "allow-same-origin" not in js:
        survive("iframe sandbox", "allow-scripts allow-forms only; opaque origin")
    else:
        refute("sandbox isolation", False, "allow-same-origin still present or sandbox missing")


def test_v1_raw_rejected() -> None:
    b1, b2 = Boot(), Boot()
    b1.process("v1;[1/1]AAAA")
    b2.process("not-even-a-header")
    if b1.rejected and b2.rejected and b1.executed is None and b2.executed is None:
        survive("v1/raw default-deny", "need ?legacy=1 to boot unsigned volumes")
    else:
        refute("v1/raw deny", False, "legacy formats still execute")


def test_floppy_audio() -> None:
    js, rm = read(BOOT_JS), read(README)
    if "square" in js and "5.25" not in rm and "sawtooth" not in js:
        survive("disk insert click", "short square click; README does not claim 5.25\" synthesis")
    else:
        refute("audio marketing", False, "5.25\" claim or sawtooth still present")


def test_readme_matches_boot() -> None:
    rm, js = read(README), read(BOOT_JS)
    ok = (
        "?pin=" in rm
        and "not a signature" in rm.lower()
        and "assembledSha !== rootSha256Esperado" in js
        and "Zero-Byte Server Hosting" not in rm
        and "Tampered fragments fail closed immediately" not in rm
    )
    if ok:
        survive("README vs bootloader", "checksum + optional pin; no zero-byte/attestation marketing")
    else:
        refute("README honesty", False, "marketing leftover or pin/root missing")


def test_deflate_roundtrip_survives() -> None:
    marker = b"FloppyURL-roundtrip-marker-7fb9"
    disk = pack_deflate(marker)[0]
    body = inflate(V2_RE.match(disk).group(6))
    survive("raw DEFLATE + Base64url roundtrip", f"recovered {body!r}")
    if body != marker:
        refute("deflate roundtrip", False, "roundtrip broke")


def test_go_packager_forge() -> None:
    out = os.path.join(ROOT, "test_run_refute")
    cmd = [
        "go", "run", "main.go", "lay_compiler.go",
        "-file", "examples/demo.html", "-algo", "deflate", "-out-dir", out,
    ]
    res = subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True)
    if res.returncode != 0:
        refute("packager", False, res.stderr[-400:])
        return
    original = read(os.path.join(out, "disk_01.txt"))
    forged_html = read(os.path.join(ROOT, "examples", "demo.html")).replace(
        "FloppyURL v2.0 Operacional", "PWNED via recomputed hashes"
    )
    forged = pack_deflate(forged_html.encode("utf-8"))[0]
    honest_root = V2_RE.match(original).group(5)
    b_open, b_pinned = Boot(), Boot(pin=honest_root)
    b_open.process(forged)
    b_pinned.process(forged)
    refute(
        "self-checksum without pin is authorship",
        b_open.executed is not None and b"PWNED via recomputed hashes" in inflate(b_open.executed),
        "rewritten packager-shaped disk still boots when the URL has no trusted pin",
    )
    if b_pinned.rejected and b_pinned.executed is None:
        survive("packager pin in ?pin= binds boot to that root", honest_root[:16] + "...")
    else:
        refute("packager pin", False, "forged disk executed against honest pin")
    shutil.rmtree(out, ignore_errors=True)


def main() -> int:
    print("=== FloppyURL claim refutation (post-hardening) ===")
    tests = [
        test_zero_byte,
        test_lazy_scripts,
        test_self_attestation,
        test_query_pin_rejects_rewrite,
        test_chunk_checksum_survives,
        test_root_hash_checked,
        test_raid0_is_concat,
        test_lin_kernel_is_a_dump,
        test_lin_oracle_divides,
        test_defi_is_js_demo,
        test_sandbox_no_same_origin,
        test_v1_raw_rejected,
        test_floppy_audio,
        test_readme_matches_boot,
        test_deflate_roundtrip_survives,
        test_go_packager_forge,
    ]
    for fn in tests:
        fn()
    print("--------------------------------------------------")
    print(f"Still overclaimed: {passed}  |  Broken checks: {failed}  |  Fixed/honest: {len(survived)}")
    for s in survived:
        print(f"  honest: {s}")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
