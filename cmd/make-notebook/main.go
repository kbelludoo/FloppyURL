package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	pyCode, err := os.ReadFile("kaggle/build_bot_brain.py")
	if err != nil {
		fmt.Printf("Error reading py file: %v\n", err)
		return
	}

	lines := strings.Split(string(pyCode), "\n")
	formattedLines := make([]string, len(lines))
	for i, l := range lines {
		formattedLines[i] = l + "\n"
	}

	cells := []map[string]interface{}{
		{
			"cell_type": "markdown",
			"metadata":  map[string]interface{}{},
			"source": []string{
				"# 🧠 Go Bot Knowledge Brain Generator (~100 MB Dense Vector Index)\n",
				"Gera o binário `knowledge_brain.bin` (GOBRAIN1) com embeddings neurais MiniLM (384-dim) para o **Agent Go** na VPS.\n",
			},
		},
		{
			"cell_type":       "code",
			"metadata":        map[string]interface{}{},
			"execution_count": nil,
			"outputs":         []interface{}{},
			"source":          formattedLines,
		},
	}

	nb := map[string]interface{}{
		"nbformat":       4,
		"nbformat_minor": 2,
		"metadata": map[string]interface{}{
			"kernelspec": map[string]interface{}{
				"display_name": "Python 3",
				"language":     "python",
				"name":         "python3",
			},
			"language_info": map[string]interface{}{
				"name": "python",
			},
		},
		"cells": cells,
	}

	out, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		fmt.Printf("Error marshaling notebook: %v\n", err)
		return
	}

	if err := os.WriteFile("kaggle/build_bot_brain.ipynb", out, 0644); err != nil {
		fmt.Printf("Error writing notebook: %v\n", err)
		return
	}

	fmt.Printf("✅ Notebook kaggle/build_bot_brain.ipynb gerado com sucesso (%d bytes)!\n", len(out))
}
