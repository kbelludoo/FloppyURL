// Go / examples/opencode-acp/main.go / Go 1.26+ / OpenCode CLI >= 1.14.20.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/kbelludoo/FloppyURL/pkg/opencode"
)

func main() {
	model := flag.String("model", "", "OpenCode provider/model (defaults to OPENCODE_MODEL)")
	flag.Parse()
	prompt := strings.Join(flag.Args(), " ")
	if strings.TrimSpace(prompt) == "" {
		log.Fatal("usage: go run ./examples/opencode-acp [-model provider/model] prompt")
	}
	if err := os.Setenv("OPENCODE_TRANSPORT", "acp"); err != nil {
		log.Fatal(err)
	}
	client, err := opencode.NewACPFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	result, err := client.Query(context.Background(), *model, prompt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.AssistantText)
}
