// CLI linp-P0: .linp -> .linpbc -> discos v2. Uso:
// go run ./linp/cmd/linp -file job.linp -base . -out-dir /tmp/x
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Xelckis/floppyURL/linp"
)

func main() {
	f := flag.String("file", "", "arquivo .linp canonico")
	base := flag.String("base", ".", "diretorio base dos FILEs")
	out := flag.String("out-dir", "disks_linp", "saida")
	flag.Parse()
	if *f == "" {
		log.Fatal("uso: go run ./linp/cmd/linp -file job.linp [-base .] [-out-dir dir]")
	}
	src, err := os.ReadFile(*f)
	if err != nil {
		log.Fatal(err)
	}
	bc, plan, err := linp.Compile(src)
	if err != nil {
		log.Fatalf("compile FAIL-CLOSED: %v", err)
	}
	if _, err := linp.Decode(bc); err != nil {
		log.Fatalf("decode FAIL-CLOSED: %v", err)
	}
	root, err := linp.Run(plan, bc, src, *base, *out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("linp OK job=%s root=%s\n", plan.Job, root)
}
