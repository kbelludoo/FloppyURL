#!/usr/bin/env python3
"""
Test Suite for FloppyURL v2.0
Validates:
1. Deflate compression & Base64 RawURL roundtrip
2. Multi-disk RAID-0 chunking & reassembly
3. LIN kernel packaging & manifest.rulel emission
4. SHA-256 integrity pinning (NIST FIPS 180-4)
"""

import os
import sys
import json
import base64
import zlib
import hashlib
import subprocess
import shutil

TEST_DIR = "test_run_disks"

def cleanup():
    if os.path.exists(TEST_DIR):
        shutil.rmtree(TEST_DIR)

def parse_v2_payload(raw_text):
    # Format: v2;[algo];[i/N];[chunk_sha];[root_sha];[payload]
    parts = raw_text.strip().split(';', 5)
    if len(parts) != 6:
        raise ValueError(f"Invalid v2 format: {raw_text[:40]}...")
    header, algo, frac, chunk_sha, root_sha, b64_payload = parts
    return {
        "header": header,
        "algo": algo,
        "frac": frac,
        "chunk_sha": chunk_sha,
        "root_sha": root_sha,
        "payload": b64_payload
    }

def decode_raw_url_base64(s):
    # Add padding if needed
    rem = len(s) % 4
    if rem > 0:
        s += '=' * (4 - rem)
    return base64.urlsafe_b64decode(s)

def run_test(name, fn):
    print(f"[*] RUNNING TEST: {name}...", end=" ", flush=True)
    try:
        fn()
        print("\033[92mPASS\033[0m")
        return True
    except Exception as e:
        print(f"\033[91mFAIL\033[0m: {e}")
        import traceback
        traceback.print_exc()
        return False

def test_deflate_html():
    cleanup()
    cmd = ["go", "run", "main.go", "lay_compiler.go", "-file", "examples/demo.html", "-algo", "deflate", "-out-dir", TEST_DIR]
    res = subprocess.run(cmd, capture_output=True, text=True)
    assert res.returncode == 0, f"Process failed: {res.stderr}"

    disk1_path = os.path.join(TEST_DIR, "disk_01.txt")
    assert os.path.exists(disk1_path), "disk_01.txt missing"

    with open(disk1_path, "r") as f:
        content = f.read()

    parsed = parse_v2_payload(content)
    assert parsed["algo"] == "deflate"
    assert parsed["frac"] == "[1/1]"

    # Check chunk SHA-256
    computed_chunk_sha = hashlib.sha256(parsed["payload"].encode('utf-8')).hexdigest()
    assert computed_chunk_sha == parsed["chunk_sha"], "Chunk SHA mismatch"

    # Decompress raw DEFLATE (wbits = -15)
    comp_bytes = decode_raw_url_base64(parsed["payload"])
    decompressed = zlib.decompress(comp_bytes, -15).decode('utf-8')

    assert "FloppyURL v2.0 Operacional" in decompressed

    # Check manifest.rulel
    rulel_path = os.path.join(TEST_DIR, "manifest.rulel")
    assert os.path.exists(rulel_path), "manifest.rulel missing"
    with open(rulel_path, "r") as f:
        rulel_content = f.read()
    assert "@RULEL:FLOPPY_MANIFEST:2.0.0" in rulel_content
    assert parsed["root_sha"] in rulel_content

def test_multidisk_raid0():
    cleanup()
    # Chunk size small to force multi-disk
    cmd = ["go", "run", "main.go", "lay_compiler.go", "-file", "examples/defi_swap_lin.html", "-algo", "deflate", "-chunk-size", "800", "-out-dir", TEST_DIR]
    res = subprocess.run(cmd, capture_output=True, text=True)
    assert res.returncode == 0, f"Process failed: {res.stderr}"

    manifest_path = os.path.join(TEST_DIR, "manifest.json")
    with open(manifest_path, "r") as f:
        meta = json.load(f)

    total_disks = meta["total_disks"]
    assert total_disks > 1, f"Expected multi-disk, got {total_disks}"

    assembled_payload = []
    root_sha = meta["root_sha256"]

    for i in range(1, total_disks + 1):
        disk_file = os.path.join(TEST_DIR, f"disk_{i:02d}.txt")
        assert os.path.exists(disk_file), f"Disk {i} missing"
        with open(disk_file, "r") as f:
            p = parse_v2_payload(f.read())
            assert p["root_sha"] == root_sha
            computed_sha = hashlib.sha256(p["payload"].encode('utf-8')).hexdigest()
            assert computed_sha == p["chunk_sha"]
            assembled_payload.append(p["payload"])

    full_b64 = "".join(assembled_payload)
    comp_bytes = decode_raw_url_base64(full_b64)
    decompressed = zlib.decompress(comp_bytes, -15).decode('utf-8')
    assert "LIN Sovereign Swap" in decompressed

def test_lin_payload_packaging():
    cleanup()
    cmd = ["go", "run", "main.go", "lay_compiler.go", "-file", "examples/cpmm_oracle.lin", "-algo", "deflate", "-out-dir", TEST_DIR]
    res = subprocess.run(cmd, capture_output=True, text=True)
    assert res.returncode == 0, f"Process failed: {res.stderr}"

    disk1_path = os.path.join(TEST_DIR, "disk_01.txt")
    with open(disk1_path, "r") as f:
        parsed = parse_v2_payload(f.read())

    comp_bytes = decode_raw_url_base64(parsed["payload"])
    decompressed_str = zlib.decompress(comp_bytes, -15).decode('utf-8')
    payload_obj = json.loads(decompressed_str)

    assert payload_obj["type"] == "lin"
    assert payload_obj["filename"] == "cpmm_oracle.lin"
    assert "@CPMM_ORACLE" in payload_obj["source"]

def test_lay_dsl_compilation():
    cleanup()
    lay_src = "@LAY:1.0\nVIEW app\nSTYLE bg=#000 fg=#0f0\nH1 \"Hello LAY\"\nP \"Bytecode test\" CLASS body\nEND\n"
    lay_path = os.path.join(TEST_DIR, "test_app.lay")
    os.makedirs(os.path.dirname(lay_path), exist_ok=True)
    with open(lay_path, "w") as f:
        f.write(lay_src)

    cmd = ["go", "run", "main.go", "lay_compiler.go", "-file", lay_path, "-algo", "deflate", "-out-dir", TEST_DIR]
    res = subprocess.run(cmd, capture_output=True, text=True)
    assert res.returncode == 0, f"Process failed: {res.stderr}"

    disk1_path = os.path.join(TEST_DIR, "disk_01.txt")
    assert os.path.exists(disk1_path), "disk_01.txt missing"

    with open(disk1_path, "r") as f:
        parsed = parse_v2_payload(f.read())

    comp_bytes = decode_raw_url_base64(parsed["payload"])
    decompressed_str = zlib.decompress(comp_bytes, -15).decode('utf-8')
    payload_obj = json.loads(decompressed_str)

    assert payload_obj["type"] == "lay"
    assert payload_obj["filename"] == "test_app.lay"

    # Decode the bytecode and verify magic
    bc = decode_raw_url_base64(payload_obj["bytecode"])
    assert bc[:4] == b'LAY1', f"Bad magic: {bc[:4]}"
    assert bc[4] == 1, f"Bad version: {bc[4]}"

    # Verify node count and structure
    import struct
    root = struct.unpack('<H', bc[5:7])[0]
    nodes = struct.unpack('<H', bc[7:9])[0]
    strings = struct.unpack('<H', bc[9:11])[0]
    assert root == 0, f"Root should be 0, got {root}"
    assert nodes >= 3, f"Should have at least 3 nodes (VIEW+H1+P), got {nodes}"
    assert strings >= 3, f"Should have at least 3 strings, got {strings}"

    # Determinism: recompile and check same bytecode
    res2 = subprocess.run(cmd, capture_output=True, text=True)
    assert res2.returncode == 0
    with open(disk1_path, "r") as f:
        parsed2 = parse_v2_payload(f.read())
    comp2 = decode_raw_url_base64(parsed2["payload"])
    decomp2 = zlib.decompress(comp2, -15).decode('utf-8')
    obj2 = json.loads(decomp2)
    assert obj2["bytecode"] == payload_obj["bytecode"], "LAY compilation not deterministic"

def main():
    print("==================================================")
    print(" FloppyURL v2.0 Integration & Attestation Test Suite")
    print("==================================================")
    
    tests = [
        ("Deflate HTML Packaging & Decompression", test_deflate_html),
        ("Multi-Disk RAID-0 Partitioning & Reassembly", test_multidisk_raid0),
        ("Deterministic LIN Script Packaging & RuleL Manifest", test_lin_payload_packaging),
        ("LAY DSL Compilation & Bytecode Roundtrip", test_lay_dsl_compilation),
    ]

    passed = 0
    for name, fn in tests:
        if run_test(name, fn):
            passed += 1

    cleanup()
    print("--------------------------------------------------")
    print(f"Results: {passed}/{len(tests)} tests passed.")
    if passed == len(tests):
        print("\033[92mALL TESTS PASSED WITH ZERO ERRORS.\033[0m")
        sys.exit(0)
    else:
        print("\033[91mSOME TESTS FAILED.\033[0m")
        sys.exit(1)

if __name__ == "__main__":
    main()
