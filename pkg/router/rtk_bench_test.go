package router

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var benchSample = `
 \x1b[32m[INFO] \x1b[0m Starting service compilation on worker node-4...
 \x1b[33m[WARN] \x1b[0m High memory utilization detected (82.4%):
index 4b825dc..6a19f02 100644
--- a/pkg/service/worker.go
+++ b/pkg/service/worker.go
@@ -10,6 +10,7 @@ func RunWorker() {
+    log.Println("Worker initialized")


   
 \x1b[34m[DEBUG] \x1b[0m Garbage collection cycle finished: 1.2ms.


Process complete with exit code 0.
`

func BenchmarkRTKLegacy(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CompressToolOutput(benchSample)
	}
}

func BenchmarkRTKHybridNim(b *testing.B) {
	nimPath, err := filepath.Abs("../../floppy-rtk-nim/rtk.exe")
	if err != nil {
		b.Skip("Nim binary not found")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(nimPath)
		cmd.Stdin = strings.NewReader(benchSample)
		_, _ = cmd.Output()
	}
}
