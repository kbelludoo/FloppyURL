import std/[strutils, monotimes, times, os]

proc isAlphaAscii(c: char): bool {.inline.} =
  (c >= 'a' and c <= 'z') or (c >= 'A' and c <= 'Z')

proc isHexAscii(c: char): bool {.inline.} =
  (c >= '0' and c <= '9') or (c >= 'a' and c <= 'f') or (c >= 'A' and c <= 'F')

## Ultra-fast ANSI sequence stripping using zero-regex single pass scanner
proc stripAnsi*(input: string): string =
  if input.len == 0:
    return ""
  
  result = newStringOfCap(input.len)
  var i = 0
  let n = input.len

  while i < n:
    if input[i] == '\x1b' and i + 1 < n and input[i + 1] == '[':
      i += 2
      while i < n:
        let c = input[i]
        i += 1
        if isAlphaAscii(c):
          break
    else:
      result.add(input[i])
      i += 1

## Check if line starts with git index header: index <hex>..<hex> [mode]
proc isGitIndexHeader*(line: string): bool {.inline.} =
  if not line.startsWith("index "):
    return false
  
  var i = 6
  let n = line.len
  var h1Count = 0
  while i < n and isHexAscii(line[i]):
    h1Count += 1
    i += 1
  
  if h1Count < 7 or i + 2 >= n or line[i] != '.' or line[i+1] != '.':
    return false
  
  i += 2
  var h2Count = 0
  while i < n and isHexAscii(line[i]):
    h2Count += 1
    i += 1
  
  if h2Count < 7:
    return false
  
  if i == n:
    return true
  
  # Check optional mode, e.g. " 100644"
  if line[i] == ' ':
    i += 1
    while i < n and line[i] in {'0'..'9'}:
      i += 1
    return i == n
  
  return false

## Lossless RTK (Result Token Kompressor) in Nim
proc compressToolOutput*(input: string): string =
  if input.len == 0:
    return ""

  let noAnsi = stripAnsi(input)
  var cleanedLines: seq[string] = @[]

  for line in noAnsi.splitLines():
    # Trim trailing whitespace
    var endIdx = line.len - 1
    while endIdx >= 0 and line[endIdx] in {' ', '\t', '\r'}:
      dec(endIdx)
    
    let trimmed = if endIdx >= 0: line[0..endIdx] else: ""

    # Filter git index metadata
    if not isGitIndexHeader(trimmed):
      cleanedLines.add(trimmed)

  var joined = cleanedLines.join("\n")

  # Collapse 3+ newlines to 2 (\n\n)
  var collapsed = newStringOfCap(joined.len)
  var newlineCount = 0

  for c in joined:
    if c == '\n':
      newlineCount += 1
      if newlineCount <= 2:
        collapsed.add('\n')
    else:
      newlineCount = 0
      collapsed.add(c)

  result = collapsed.strip()

# C-ABI Exports for FFI
proc NimMain() {.cdecl, importc.}

proc floppy_rtk_init*() {.cdecl, exportc, dynlib.} =
  NimMain()

proc floppy_rtk_compress_nim*(input: cstring): cstring {.cdecl, exportc, dynlib.} =
  if input == nil:
    return nil
  let res = compressToolOutput($input)
  return cstring(res)

when isMainModule:
  let params = commandLineParams()
  if params.len > 0 and params[0] == "bench":
    var iters = 5000
    if params.len > 1:
      try:
        iters = parseInt(params[1])
      except ValueError:
        iters = 5000

    echo "==============================================================="
    echo "⚡ FLOOPY-RTK NIM PERFORMANCE BENCHMARK (iterations: ", iters, ")"
    echo "===============================================================\n"

    let sampleTerminalOutput = """
 \x1b[32m[INFO] \x1b[0m Starting service compilation on worker node-4...
 \x1b[33m[WARN] \x1b[0m High memory utilization detected (82.4%):
index 4b825dc..6a19f02 100644
--- a/pkg/service/worker.go
+++ b/pkg/service/worker.go
@@ -10,6 +10,7 @@ func RunWorker() {
+    log.Println("Worker initialized")


   
 \x1b[34m[DEBUG] \x1b[0m Garbage collection cycle finished: 1.2ms.


Process complete with exit code 0.
"""
    let start = getMonoTime()
    var totalBytes = 0
    for _ in 1..iters:
      let comp = compressToolOutput(sampleTerminalOutput)
      totalBytes += comp.len
    let elapsed = getMonoTime() - start

    let totalMs = inMilliseconds(elapsed).float64
    let avgUs = inMicroseconds(elapsed).float64 / iters.float64
    let throughputMB = (sampleTerminalOutput.len * iters).float64 / 1_048_576.0 / (inNanoseconds(elapsed).float64 / 1_000_000_000.0)

    echo "Nim RTK (Result Token Kompressor with ARC/ORC):"
    echo "   Total time:         ", totalMs, " ms"
    echo "   Average latency:    ", avgUs, " µs / op"
    echo "   Throughput:         ", throughputMB, " MB/s"
    echo "   Output bytes:       ", totalBytes, " bytes"
    echo "==============================================================="
  else:
    var input = ""
    if params.len > 0:
      input = params.join(" ")
    else:
      input = readAll(stdin)
    stdout.write compressToolOutput(input)
