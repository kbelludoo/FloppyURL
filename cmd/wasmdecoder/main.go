//go:build js && wasm

// FloppyURL WASM kernel: exposes window.decoder(base64urlBrotli) to the
// bootloader as a fallback Brotli decompressor. Compiled with:
//
//	GOOS=js GOARCH=wasm go build -o Website/wasm.wasm ./cmd/wasmdecoder
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"syscall/js"

	"github.com/andybalholm/brotli"
)

func main() {
	decoderFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return "Error: No text found"
		}
		name := args[0].String()

		decoded, err := base64.RawURLEncoding.DecodeString(name)
		if err != nil {
			return fmt.Sprintf("Error decoding base64: %v", err)
		}

		brotliReader := brotli.NewReader(bytes.NewReader(decoded))

		uncompressedData, err := io.ReadAll(brotliReader)
		if err != nil {
			return fmt.Sprintf("Error decompressing brotli: %v", err)
		}

		return string(uncompressedData)
	})

	js.Global().Set("decoder", decoderFunc)

	select {}
}
