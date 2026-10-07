package transpile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reJob  = regexp.MustCompile(`^job\(\s*"([^"]+)"\s*\)$`)
	rePack = regexp.MustCompile(`^pack\(\s*"([^"]+)"\s*(?:,\s*algo\s*=\s*"([^"]+)"\s*)?(?:,\s*chunk\s*=\s*(\d+)\s*)?\)$`)
)

var validAlgos = map[string]bool{
	"deflate": true,
	"gzip":    true,
	"brotli":  true,
}

type packEntry struct {
	file  string
	algo  string
	chunk int
}

// PyToLinp transpile Python-subset to canonical .linp (pure Go, fail-closed).
func PyToLinp(src string) (string, error) {
	var jobs []string
	var packs []packEntry

	lines := strings.Split(src, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if m := reJob.FindStringSubmatch(line); m != nil {
			if len(jobs) > 0 {
				return "", fmt.Errorf("linha %d: multiplos jobs fora do P0", i+1)
			}
			jobs = append(jobs, m[1])
			continue
		}

		if m := rePack.FindStringSubmatch(line); m != nil {
			file := m[1]
			algo := m[2]
			if algo == "" {
				algo = "deflate"
			}
			chunk := 1800000
			if m[3] != "" {
				c, err := strconv.Atoi(m[3])
				if err != nil {
					return "", fmt.Errorf("linha %d: chunk invalido", i+1)
				}
				chunk = c
			}

			if !validAlgos[algo] {
				return "", fmt.Errorf("linha %d: algo %q desconhecido", i+1, algo)
			}
			if chunk < 64 || chunk > 1800000 {
				return "", fmt.Errorf("linha %d: chunk fora de [64,1800000]", i+1)
			}
			if strings.Contains(file, "..") || strings.HasPrefix(file, "/") {
				return "", fmt.Errorf("linha %d: path fora do repo", i+1)
			}

			packs = append(packs, packEntry{file: file, algo: algo, chunk: chunk})
			continue
		}

		return "", fmt.Errorf("linha %d: fora do subset linp-P0: %q", i+1, line)
	}

	if len(jobs) == 0 {
		return "", fmt.Errorf("job() ausente")
	}
	if len(packs) == 0 {
		return "", fmt.Errorf("pack() ausente")
	}
	if len(packs) > 1 {
		return "", fmt.Errorf("P0: 1 PACK por job (multi-pack no roadmap)")
	}

	var out strings.Builder
	out.WriteString("@LINP:1.0\n")
	out.WriteString(fmt.Sprintf("JOB %s\n", jobs[0]))
	for _, p := range packs {
		out.WriteString(fmt.Sprintf("FILE %s\n", p.file))
		out.WriteString(fmt.Sprintf("ALGO %s\n", p.algo))
		out.WriteString(fmt.Sprintf("CHUNK %d\n", p.chunk))
		out.WriteString("PACK\n")
	}
	out.WriteString("END\n")

	return out.String(), nil
}
