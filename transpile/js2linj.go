package transpile

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reBoot     = regexp.MustCompile(`^boot\(\s*"([^"]+)"\s*\)\s*;?$`)
	reVerify   = regexp.MustCompile(`^verify\(\s*"sha256"\s*\)\s*;?$`)
	reInflate  = regexp.MustCompile(`^inflate\(\s*"(deflate|gzip)"\s*\)\s*;?$`)
	reDispatch = regexp.MustCompile(`^dispatch\(\s*"(html|lin|lay)"\s*\)\s*;?$`)
)

// JSToLinj transpile JS-subset to canonical .linj (pure Go, fail-closed).
func JSToLinj(src string) (string, error) {
	var boot, inf, disp string

	lines := strings.Split(src, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		if m := reBoot.FindStringSubmatch(line); m != nil {
			if boot != "" {
				return "", fmt.Errorf("linha %d: boot duplicado", i+1)
			}
			boot = m[1]
			continue
		}

		if reVerify.MatchString(line) {
			continue // implicito no STEP VERIFY
		}

		if m := reInflate.FindStringSubmatch(line); m != nil {
			if inf != "" {
				return "", fmt.Errorf("linha %d: inflate duplicado", i+1)
			}
			inf = m[1]
			continue
		}

		if m := reDispatch.FindStringSubmatch(line); m != nil {
			if disp != "" {
				return "", fmt.Errorf("linha %d: dispatch duplicado", i+1)
			}
			disp = m[1]
			continue
		}

		return "", fmt.Errorf("linha %d: fora do subset linj-P0: %q", i+1, line)
	}

	if boot == "" {
		return "", fmt.Errorf("boot() ausente")
	}
	if inf == "" {
		return "", fmt.Errorf("inflate() ausente")
	}
	if disp == "" {
		return "", fmt.Errorf("dispatch() ausente")
	}

	return fmt.Sprintf("@LINJ:1.0\nBOOT %s\nEXPECT %s\nEXPECT %s\nSTEP VERIFY\nSTEP INFLATE\nSTEP DISPATCH\nEND\n",
		boot, inf, disp), nil
}
