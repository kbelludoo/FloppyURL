// CLI mempipe-P0: pipeline em memoria, saida JSON no stdout, ZERO escritas.
// Propositalmente sem flag -out-dir: e impossivel pedir escrita em disco.
// Uso: go run ./mempipe/cmd/mempipe -file examples/demo.html [-algo deflate] [-chunk-size N]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Xelckis/floppyURL/mempipe"
)

func main() {
	f := flag.String("file", "", "entrada (.html|.lin|.lay|raw)")
	algo := flag.String("algo", "deflate", "deflate|gzip|brotli")
	chunk := flag.Int("chunk-size", 1800000, "[64,1800000]")
	flag.Parse()
	if *f == "" {
		log.Fatal("uso: go run ./mempipe/cmd/mempipe -file <arq> [-algo deflate] [-chunk-size N]")
	}
	raw, err := os.ReadFile(*f)
	if err != nil {
		log.Fatal(err)
	}
	var b *mempipe.Bundle
	switch filepath.Ext(*f) {
	case ".lay":
		b, err = mempipe.PipeLAY(filepath.Base(*f), raw, *algo, *chunk)
	case ".lin":
		b, err = mempipe.PipeLIN(filepath.Base(*f), raw, *algo, *chunk)
	case ".html":
		b, err = mempipe.PipeHTML(filepath.Base(*f), raw, *algo, *chunk)
	default:
		b, err = mempipe.PipeRAW(filepath.Base(*f), raw, *algo, *chunk)
	}
	if err != nil {
		log.Fatalf("mempipe FAIL-CLOSED: %v", err)
	}
	out, _ := json.Marshal(b)
	fmt.Println(string(out))
}
