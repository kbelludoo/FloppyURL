// CLI linz-P0: .linz + payload b64 -> discos v2. Uso:
// go run ./linz/cmd/linz -file job.linz -payload payload.b64 -out-dir /tmp/x
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Xelckis/floppyURL/linz"
)

func main() {
	f := flag.String("file", "", "arquivo .linz canonico")
	pay := flag.String("payload", "", "arquivo com payload base64url ja comprimido")
	out := flag.String("out-dir", "disks_linz", "saida")
	flag.Parse()
	if *f == "" || *pay == "" {
		log.Fatal("uso: go run ./linz/cmd/linz -file job.linz -payload payload.b64 [-out-dir dir]")
	}
	src, err := os.ReadFile(*f)
	if err != nil {
		log.Fatal(err)
	}
	rawPay, err := os.ReadFile(*pay)
	if err != nil {
		log.Fatal(err)
	}
	payload := strings.TrimSpace(string(rawPay))
	bc, plan, err := linz.Compile(src)
	if err != nil {
		log.Fatalf("compile FAIL-CLOSED: %v", err)
	}
	if _, err := linz.Decode(bc); err != nil {
		log.Fatalf("decode FAIL-CLOSED: %v", err)
	}
	disks, root, err := linz.Assemble(plan, payload)
	if err != nil {
		log.Fatal(err)
	}
	if err := linz.WriteOut(plan, bc, src, disks, root, *out); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("linz OK payload=%s root=%s disks=%d\n", plan.Payload, root, len(disks))
}
