#!/usr/bin/env python3
"""Live refutation: the real bootloader executes a self-attested forged disk."""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

from refute_floppy import pack_deflate

ROOT = os.path.dirname(os.path.abspath(__file__))
MARKER = "PWNED-FORGED-PAYLOAD"
CHROME = shutil.which("google-chrome") or shutil.which("chromium") or shutil.which("chromium-browser")


class WebsiteHandler(SimpleHTTPRequestHandler):
    def log_message(self, fmt: str, *args: object) -> None:
        return


def main() -> int:
    if not CHROME:
        print("SKIP live refutation: no chrome/chromium in PATH")
        return 0

    evil = (
        "<!DOCTYPE html><html><head><meta charset='utf-8'></head><body>"
        f"<h1 id='pwn'>{MARKER}</h1>"
        "<script>try{parent.document.title='SANDBOX_ESCAPED';}"
        "catch(e){document.documentElement.setAttribute('data-held', e.message);}</script>"
        "</body></html>"
    ).encode("utf-8")
    disk = pack_deflate(evil)[0]
    url = "http://127.0.0.1:8765/#" + disk

    os.chdir(os.path.join(ROOT, "Website"))
    httpd = ThreadingHTTPServer(("127.0.0.1", 8765), WebsiteHandler)
    t = threading.Thread(target=httpd.serve_forever, daemon=True)
    t.start()
    time.sleep(0.2)

    user_data = tempfile.mkdtemp(prefix="refute-chrome-")
    cmd = [
        CHROME, "--headless=new", "--no-sandbox", "--disable-gpu",
        "--disable-dev-shm-usage", f"--user-data-dir={user_data}",
        "--virtual-time-budget=8000", "--timeout=15000",
        "--dump-dom", url,
    ]
    try:
        res = subprocess.run(cmd, capture_output=True, text=True, timeout=40)
    finally:
        httpd.shutdown()
        shutil.rmtree(user_data, ignore_errors=True)

    dom = res.stdout
    err = res.stderr
    forged = MARKER in dom
    escaped = "SANDBOX_ESCAPED" in dom
    print(json.dumps({
        "chrome_rc": res.returncode,
        "forged_payload_executed": forged,
        "sandbox_escaped_via_parent_title": escaped,
        "dom_bytes": len(dom),
        "stderr_tail": err[-400:],
    }, indent=2))
    if "PWNED" in dom or MARKER in dom:
        print("[REFUTED] live bootloader executed self-attested forged HTML")
    else:
        print("[NOT-REFUTED] dump-dom did not contain forged marker")
        print(dom[:1500])
        return 1
    if escaped:
        print("[REFUTED] live sandbox: payload set parent.document.title")
    else:
        print("[WEAK] parent title not rewritten in dump-dom (srcdoc still executed)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
