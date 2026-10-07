package transpile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reEnvelope = regexp.MustCompile(`^envelope\(\s*"([^"]+)"\s*\)$`)
	rePayload  = regexp.MustCompile(`^payload\(\s*"([^"]+)"\s*\)$`)
	reAlgo     = regexp.MustCompile(`^algo\(\s*"(deflate|gzip|brotli)"\s*\)$`)
	reChunk    = regexp.MustCompile(`^chunk\(\s*(\d+)\s*\)$`)
)

// SpecToLinz transpile spec textual to canonical .linz (pure Go, fail-closed).
func SpecToLinz(src string) (string, error) {
	var env, pay, algo string
	var chunk int

	lines := strings.Split(src, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if m := reEnvelope.FindStringSubmatch(line); m != nil {
			if m[1] != "v2" {
				return "", fmt.Errorf("linha %d: envelope so v2 no P0", i+1)
			}
			env = "v2"
			continue
		}

		if m := rePayload.FindStringSubmatch(line); m != nil {
			if strings.Contains(m[1], "..") || strings.HasPrefix(m[1], "/") {
				return "", fmt.Errorf("linha %d: path fora do repo", i+1)
			}
			pay = m[1]
			continue
		}

		if m := reAlgo.FindStringSubmatch(line); m != nil {
			algo = m[1]
			continue
		}

		if m := reChunk.FindStringSubmatch(line); m != nil {
			c, err := strconv.Atoi(m[1])
			if err != nil || c < 64 || c > 1800000 {
				return "", fmt.Errorf("linha %d: chunk fora de [64,1800000]", i+1)
			}
			chunk = c
			continue
		}

		return "", fmt.Errorf("linha %d: fora do subset linz-P0: %q", i+1, line)
	}

	if env == "" || pay == "" || algo == "" || chunk == 0 {
		return "", fmt.Errorf("spec incompleta (envelope/payload/algo/chunk)")
	}

	return fmt.Sprintf("@LINZ:1.0\nENVELOPE %s\nPAYLOAD %s\nALGO %s\nCHUNK %d\nASSEMBLE\nEND\n",
		env, pay, algo, chunk), nil
}
