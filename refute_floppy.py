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
    """Mirror of Website/index.html processarHash + verificarProgresso."""

    def __init__(self) -> None:
        self.disks: list[str | None] = []
        self.total = 0
        self.root: str | None = None
        self.algo = "brotli"
        self.rejected = False
        self.executed: str | None = None

    def process(self, raw: str) -> None:
        raw = raw[1:] if raw.startswith("#") else raw
        raw = raw.strip()
        m = V2_RE.match(raw)
        if m:
            self.algo, a, t, decl, root, payload = (
                m.group(1), int(m.group(2)), int(m.group(3)),
                m.group(4), m.group(5), m.group(6),
            )
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
        if self.total and all(self.disks):
            assembled = "".join(self.disks)  # type: ignore[arg-type]
            # Bootloader never hashes assembled vs self.root.
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
    boot = os.path.getsize(BOOT)
    js = os.path.getsize(os.path.join(ROOT, "Website", "wasm_exec.js"))
    lay = os.path.getsize(os.path.join(ROOT, "Website", "lay_runtime.js"))
    refute(
        "zero-byte server hosting",
        wasm > 4_000_000 and (boot + js + lay) > 30_000,
        f"Website/ serves wasm.wasm={wasm}B plus bootloader {boot+js+lay}B",
    )


def test_unconditional_wasm_js() -> None:
    html = read(BOOT)
    before = html.split("processarHash")[0]
    refute(
        "instant zero-WASM / zero dependencies downloaded",
        '<script src="wasm_exec.js"></script>' in before
        and '<script src="lay_runtime.js"></script>' in before,
        "wasm_exec.js + lay_runtime.js load before any algo check; "
        f"wasm.wasm still {os.path.getsize(WASM)} bytes on the server",
    )


def test_self_attestation() -> None:
    evil = b"<html><body><h1>PWNED-FORGED-PAYLOAD</h1></body></html>"
    disk = pack_deflate(evil)[0]
    b = Boot()
    b.process(disk)
    body = inflate(b.executed or "")
    refute(
        "NIST FIPS 180-4 integrity pinning / fail-closed attestation",
        b.executed is not None and b"PWNED-FORGED-PAYLOAD" in body,
        "attacker who rewrites payload AND declared SHA-256 is accepted; "
        "hashes live inside the untrusted fragment, not a trusted pin",
    )


def test_chunk_checksum_survives() -> None:
    disk = pack_deflate(b"<html>ok</html>")[0]
    parts = disk.split(";", 5)
    parts[5] = parts[5][:-1] + ("A" if parts[5][-1] != "A" else "B")
    b = Boot()
    b.process(";".join(parts))
    if b.rejected and b.executed is None:
        survive(
            "chunk SHA-256 as a checksum (not a pin)",
            "tamper without updating declared digest is rejected",
        )
    else:
        refute(
            "chunk checksum actually rejects stale digest",
            False,
            f"rejected={b.rejected} executed={b.executed is not None}",
        )


def test_root_hash_never_checked() -> None:
    html = read(BOOT)
    assigns = len(re.findall(r"rootSha256Esperado\s*=", html))
    compares = len(re.findall(r"rootSha256Esperado\s*[!=]==", html))
    disks = pack_deflate(b"<html>root-mismatch</html>", chunk_size=8)
    assert len(disks) > 1
    parts = disks[1].split(";", 5)
    parts[4] = "f" * 64
    disks[1] = ";".join(parts)
    b = Boot()
    for d in disks:
        b.process(d)
    refute(
        "tampered fragments fail closed on dataset root hash",
        assigns >= 1 and compares == 0 and b.executed is not None,
        f"rootSha256Esperado assigned {assigns}x, compared {compares}x; "
        "mismatched per-disk root hashes still execute",
    )


def test_raid0_is_concat() -> None:
    go = read(MAIN_GO)
    refute(
        "multi-disk virtual floppy RAID-0",
        "chunkPayload := encodedData[start:end]" in go
        and "RAID" in go
        and "stripe" not in go.lower(),
        "volumes are sequential slices of one base64 string (concat), "
        "not RAID-0 striping, parity, or parallel disks",
    )


def test_lin_kernel_is_a_dump() -> None:
    html = read(BOOT)
    src = read(LIN)
    refute(
        "LIN sovereign kernel / heapless stack-only execution",
        "renderLinPayload" in html
        and "payloadObj.source" in html
        and "LinVM" in html
        and "0_BYTES" in html
        and "function parseLin" not in html
        and "@LIN:L1c" in src
        and "eval(" not in html,
        "bootloader prints .lin source in <pre>, waits 300ms, then forges "
        "@RULEL:LIN_JIT_RUN receipts; no LIN parser or stack machine exists",
    )


def test_lin_oracle_ignores_division() -> None:
    src = read(LIN)
    refute(
        "deterministic CPMM oracle (x*y=k)",
        "_lia_ushr(numerator, 0)" in src
        and "denominator" in src
        and re.search(r"\^\s*_lia_ushr\(numerator,\s*0\)", src) is not None
        and "numerator / denominator" not in src
        and "numerator / denominator" not in src.replace(" ", ""),
        "oracle computes denominator then returns USHR(numerator,0), i.e. numerator",
    )


def test_defi_is_js_floats() -> None:
    src = read(DEFI)
    refute(
        "unstoppable LIN VM DeFi swapper",
        "let reserveIn = 10000.0" in src
        and "Date.now()" in src
        and "numerator / denominator" in src
        and "LIN VM Stack-Only" in src
        and "Zero backend, zero DNS takeover risk" in src,
        "swap is in-page JS floats + Date.now(); receipts are innerHTML strings, "
        "not a kernel; bootloader origin remains a DNS/hosting trust root",
    )


def test_sandbox_not_isolated() -> None:
    html = read(BOOT)
    refute(
        "executes the application in an isolated sandbox",
        'sandbox", "allow-scripts allow-forms allow-same-origin allow-popups"' in html
        and "iframe.srcdoc = htmlPuro" in html,
        "srcdoc + allow-scripts + allow-same-origin lets payload script the parent "
        "(HTML standard: combination can remove sandbox)",
    )


def test_v1_and_raw_have_no_integrity() -> None:
    b1, b2 = Boot(), Boot()
    b1.process("v1;[1/1]AAAA")
    b2.process("not-even-a-header")
    refute(
        "every disk chunk is attested",
        b1.executed == "AAAA" and b2.executed == "not-even-a-header",
        "v1 and raw volumes skip SHA-256 entirely (default algo becomes brotli)",
    )


def test_floppy_audio_is_a_chirp() -> None:
    html = read(BOOT)
    refute(
        "5.25\" floppy head-seek / stepper synthesis",
        "sawtooth" in html
        and "setValueAtTime(160" in html
        and "exponentialRampToValueAtTime(30" in html
        and "stepper" not in html.lower(),
        "one 90ms 160→30 Hz sawtooth beep; no stepper pulse train or 5.25\" model",
    )


def test_readme_overclaims_vs_code() -> None:
    rm, html = read(README), read(BOOT)
    refute(
        "README vs bootloader contract",
        "Tampered fragments fail closed immediately" in rm
        and "Zero-Byte Server Hosting" in rm
        and "rootSha256Esperado" in html
        and "calcSha256(payloadCompleto)" not in html
        and "calcSha256(payloadString)" not in html,
        "README promises fail-closed root pinning; bootloader never hashes the assembly",
    )


def test_deflate_roundtrip_survives() -> None:
    marker = b"FloppyURL-roundtrip-marker-7fb9"
    disk = pack_deflate(marker)[0]
    body = inflate(V2_RE.match(disk).group(6))
    survive("raw DEFLATE + Base64url roundtrip", f"recovered {body!r}")
    if body != marker:
        refute("deflate roundtrip", False, "roundtrip broke")


def test_go_packager_forge() -> None:
    """Real packager output can be rewritten into a still-valid v2 disk."""
    out = os.path.join(ROOT, "test_run_refute")
    cmd = [
        "go", "run", "main.go", "lay_compiler.go",
        "-file", "examples/demo.html", "-algo", "deflate", "-out-dir", out,
    ]
    res = subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True)
    if res.returncode != 0:
        refute("packager-forged attested disk", False, res.stderr[-400:])
        return
    original = read(os.path.join(out, "disk_01.txt"))
    forged_html = read(os.path.join(ROOT, "examples", "demo.html")).replace(
        "FloppyURL v2.0 Operacional", "PWNED via recomputed hashes"
    )
    forged = pack_deflate(forged_html.encode("utf-8"))[0]
    b_ok, b_evil = Boot(), Boot()
    b_ok.process(original)
    b_evil.process(forged)
    refute(
        "attested packager disks cannot be replaced",
        b_ok.executed is not None and b_evil.executed is not None
        and b"PWNED via recomputed hashes" in inflate(b_evil.executed),
        "honest disk and rewritten disk both satisfy the v2 regex + chunk digest",
    )
    shutil.rmtree(out, ignore_errors=True)


def main() -> int:
    print("=== FloppyURL claim refutation ===")
    tests = [
        test_zero_byte,
        test_unconditional_wasm_js,
        test_self_attestation,
        test_chunk_checksum_survives,
        test_root_hash_never_checked,
        test_raid0_is_concat,
        test_lin_kernel_is_a_dump,
        test_lin_oracle_ignores_division,
        test_defi_is_js_floats,
        test_sandbox_not_isolated,
        test_v1_and_raw_have_no_integrity,
        test_floppy_audio_is_a_chirp,
        test_readme_overclaims_vs_code,
        test_deflate_roundtrip_survives,
        test_go_packager_forge,
    ]
    for fn in tests:
        fn()
    print("--------------------------------------------------")
    print(f"Refuted: {passed}  |  Not refuted: {failed}  |  Survived: {len(survived)}")
    for s in survived:
        print(f"  survived: {s}")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
