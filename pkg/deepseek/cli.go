package deepseek

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// RunTerminalChat starts an interactive terminal chat backed by the free
// DeepSeek Web session (no API key). If there is no cached session it tries to
// capture one from the CDP-connected Chrome.
func RunTerminalChat(ctx context.Context) error {
	client, err := NewClient(ctx, "", true)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	if sess := client.Session(); sess != nil {
		token := sess.Token
		if len(token) > 10 {
			token = token[:10]
		}
		fmt.Printf("DeepSeek Web (grátis, sem API key) — sessão: %s... (%d cookies)\n", token, len(sess.Cookies))
	}
	fmt.Println("Comandos: /new  /think  /expert  /exit")

	reader := bufio.NewReader(os.Stdin)
	var convID string
	thinking := false
	expert := false

	for {
		fmt.Print("\n> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return nil
		}
		line = strings.TrimSpace(line)

		switch line {
		case "":
			continue
		case "/exit", "/quit":
			return nil
		case "/new":
			convID = ""
			fmt.Println("[nova conversa]")
			continue
		case "/think":
			thinking = !thinking
			fmt.Printf("[thinking=%v]\n", thinking)
			continue
		case "/expert":
			expert = !expert
			fmt.Printf("[expert=%v]\n", expert)
			continue
		}

		opts := StreamOptions{ConversationID: convID, Thinking: thinking}
		if convID == "" {
			if expert {
				opts.Model = ModelExpert
			} else {
				opts.Model = ModelDefault
			}
		}

		fmt.Println()
		reply, err := client.Stream(ctx, line, opts, func(delta string) error {
			fmt.Print(delta)
			return nil
		})
		fmt.Println()
		if err != nil {
			fmt.Printf("[erro] %v\n", err)
			if IsLoginRequired(err) {
				return err
			}
			continue
		}
		convID = reply.ConversationID
	}
}
