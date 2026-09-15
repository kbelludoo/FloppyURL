// CLI linj-P0: .linj -> .linjbc.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Xelckis/floppyURL/linj"
)

func main() {
	f := flag.String("file", "", ".linj canonico")
	out := flag.String("out", "", "saida .linjbc (default: <file>.linjbc)")
	flag.Parse()
	if *f == "" {
		log.Fatal("uso: go run ./linj/cmd/linj -file boot.linj")
	}
	src, err := os.ReadFile(*f)
	if err != nil {
		log.Fatal(err)
	}
	bc, plan, err := linj.Compile(src)
	if err != nil {
		log.Fatalf("compile FAIL-CLOSED: %v", err)
	}
	if _, err := linj.Decode(bc); err != nil {
		log.Fatalf("decode FAIL-CLOSED: %v", err)
	}
	dst := *out
	if dst == "" {
		dst = *f + "bc"
	}
	if err := os.WriteFile(dst, bc, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("linj OK boot=%s algo=%s type=%s bytes=%d\n", plan.Boot, plan.Algo, plan.Dtype, len(bc))
}
