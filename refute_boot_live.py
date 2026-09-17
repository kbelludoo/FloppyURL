#!/usr/bin/env python3
"""Live refutation against the real bootloader (Node WebCrypto + Chrome)."""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import time

from refute_floppy import pack_deflate

ROOT = os.path.dirname(os.path.abspath(__file__))
MARKER = "PWNED-FORGED-PAYLOAD"
CHROME = shutil.which("google-chrome") or shutil.which("chromium")
BOOT = os.path.join(ROOT, "Website", "index.html")
NODE_PROBE = os.path.join(ROOT, "refute_boot_live.js")


def forged_disk() -> str:
    html = (
        "<!DOCTYPE html><html><head><meta charset='utf-8'></head><body>"
        f"<h1 id='pwn'>{MARKER}</h1>"
        "<p>Self-attested SHA-256 disk accepted by FloppyURL bootloader.</p>"
        "<script>try{parent.document.title='SANDBOX_ESCAPED';}"
        "catch(e){document.documentElement.setAttribute('data-held',e.message);}</script>"
        "</body></html>"
    )
    return pack_deflate(html.encode("utf-8"))[0]


def refute_with_node(disk: str) -> dict:
    res = subprocess.run(
        ["node", NODE_PROBE, BOOT, disk, MARKER],
        capture_output=True, text=True, timeout=20,
    )
    if res.returncode != 0:
        return {"ok": False, "error": (res.stderr or res.stdout)[-500:]}
    try:
        return json.loads(res.stdout.strip().splitlines()[-1])
    except json.JSONDecodeError:
        return {"ok": False, "error": res.stdout[-500:]}


def _port_open(port: int) -> bool:
    import socket
    s = socket.socket()
    s.settimeout(0.2)
    try:
        s.connect(("127.0.0.1", port))
        s.close()
        return True
    except OSError:
        return False


def chrome_dump(url: str, port: int, server: subprocess.Popen | None) -> dict:
    user_data = tempfile.mkdtemp(prefix="refute-chrome-")
    cmd = [
        CHROME, "--headless=old", "--no-sandbox", "--disable-gpu",
        "--disable-dev-shm-usage", f"--user-data-dir={user_data}",
        "--virtual-time-budget=4000", "--timeout=8000", "--dump-dom", url,
    ]
    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        stdout, stderr = proc.communicate(timeout=14)
        rc = proc.returncode
    except subprocess.TimeoutExpired:
        proc.kill()
        stdout, stderr = proc.communicate()
        rc = -9
    shutil.rmtree(user_data, ignore_errors=True)
    dom = stdout or ""
    return {
        "ok": MARKER in dom,
        "sandbox_escaped": "<title>SANDBOX_ESCAPED</title>" in dom,
        "fail_closed": "FAIL-CLOSED" in dom,
        "dom_bytes": len(dom),
        "rc": rc,
        "title": next((ln for ln in dom.splitlines() if "<title>" in ln.lower()), ""),
    }


def refute_with_chrome(disk: str) -> dict:
    if not CHROME:
        return {"skipped": True, "reason": "no chrome"}

    server = None
    if _port_open(8080):
        port = 8080
    else:
        port = 8767
        server = subprocess.Popen(
            [sys.executable, "-m", "http.server", str(port),
             "--directory", os.path.join(ROOT, "Website")],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        time.sleep(0.3)

    open_url = f"http://127.0.0.1:{port}/#" + disk
    pinned_url = f"http://127.0.0.1:{port}/?pin={'0'*64}#" + disk
    opened = chrome_dump(open_url, port, server)
    pinned = chrome_dump(pinned_url, port, server)
    if server:
        server.kill()
    return {"no_pin": opened, "wrong_pin": pinned}


def main() -> int:
    disk = forged_disk()
    node = refute_with_node(disk)
    chrome = refute_with_chrome(disk)
    print(json.dumps({"node": node, "chrome": chrome}, indent=2))
    rc = 0
    if node.get("ok") and node.get("rootHashCompared") and not node.get("sandboxHasSameOrigin"):
        print("[OK] node: checksum path still inflates; root compare present; no allow-same-origin")
    elif node.get("ok"):
        print("[OK] node: forged self-checksum still inflates (expected without pin)")
    else:
        print("[FAIL] node path:", node)
        rc = 1
    if chrome.get("skipped"):
        print("[SKIP] chrome not available")
        return rc
    opened, pinned = chrome.get("no_pin") or {}, chrome.get("wrong_pin") or {}
    if opened.get("ok") and not opened.get("sandbox_escaped"):
        print("[OK] chrome: no-pin forged disk still runs (checksum-only) AND sandbox held parent title")
    elif opened.get("sandbox_escaped"):
        print("[FAIL] chrome sandbox: parent title rewritten")
        rc = 1
    else:
        print("[WEAK] chrome no-pin:", opened)
    if pinned.get("fail_closed") and not pinned.get("ok"):
        print("[OK] chrome: wrong ?pin= fail-closed, forged HTML not injected")
    else:
        print("[FAIL] chrome pin:", pinned)
        rc = 1
    return rc


if __name__ == "__main__":
    sys.exit(main())
