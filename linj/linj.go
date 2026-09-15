// Package linj — compilador linj-P0: .linj -> .linjbc (magic "LNJ1").
// Source canonico: @LINJ:1.0 / BOOT <n> / EXPECT <algo> / EXPECT <type> / STEP VERIFY/INFLATE/DISPATCH / END.
// Fail-closed: ordem fixa, 1 BOOT, algos deflate|gzip, tipos html|lin|lay.
package linj

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

const Magic = "LNJ1"
const Version = 0x01

type Plan struct {
	Boot  string
	Algo  string
	Dtype string
}

func Compile(src []byte) ([]byte, *Plan, error) {
	p := &Plan{}
	var expects []string
	var steps []string
	seenBoot := false
	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if i == 0 && line == "@LINJ:1.0" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "BOOT "):
			if seenBoot {
				return nil, nil, fmt.Errorf("linha %d: BOOT duplicado", i+1)
			}
			seenBoot = true
			p.Boot = strings.TrimSpace(line[5:])
			if p.Boot == "" {
				return nil, nil, fmt.Errorf("linha %d: BOOT vazio", i+1)
			}
		case strings.HasPrefix(line, "EXPECT "):
			expects = append(expects, strings.TrimSpace(line[7:]))
		case strings.HasPrefix(line, "STEP "):
			steps = append(steps, strings.TrimSpace(line[5:]))
		case line == "END":
		default:
			return nil, nil, fmt.Errorf("linha %d: %q fora do subset linj-P0", i+1, line)
		}
	}
	if len(expects) != 2 {
		return nil, nil, fmt.Errorf("P0 exige 2 EXPECT (algo+tipo), veio %d", len(expects))
	}
	p.Algo, p.Dtype = expects[0], expects[1]
	if p.Algo != "deflate" && p.Algo != "gzip" {
		return nil, nil, fmt.Errorf("algo %q fora do P0", p.Algo)
	}
	if p.Dtype != "html" && p.Dtype != "lin" && p.Dtype != "lay" {
		return nil, nil, fmt.Errorf("tipo %q fora do P0", p.Dtype)
	}
	want := []string{"VERIFY", "INFLATE", "DISPATCH"}
	if len(steps) != 3 {
		return nil, nil, fmt.Errorf("P0 exige 3 STEPs, veio %d", len(steps))
	}
	for i := range want {
		if steps[i] != want[i] {
			return nil, nil, fmt.Errorf("STEP %d deve ser %s, veio %s", i+1, want[i], steps[i])
		}
	}
	if !seenBoot {
		return nil, nil, fmt.Errorf("BOOT ausente")
	}
	aid := uint8(0)
	if p.Algo == "gzip" {
		aid = 1
	}
	tid := uint8(0)
	switch p.Dtype {
	case "lin":
		tid = 1
	case "lay":
		tid = 2
	}
	strs := []string{p.Boot}
	var buf bytes.Buffer
	buf.WriteString(Magic)
	buf.WriteByte(Version)
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	buf.WriteByte(aid)
	buf.WriteByte(tid)
	binary.Write(&buf, binary.LittleEndian, uint16(len(strs)))
	for _, s := range strs {
		binary.Write(&buf, binary.LittleEndian, uint16(len(s)))
		buf.WriteString(s)
	}
	return buf.Bytes(), p, nil
}

// Decode re-le o bytecode (fail-closed).
func Decode(bc []byte) (*Plan, error) {
	if len(bc) < 4+1+2+1+1+2 {
		return nil, fmt.Errorf("bytecode truncado")
	}
	if string(bc[:4]) != Magic {
		return nil, fmt.Errorf("magic invalido")
	}
	if bc[4] != Version {
		return nil, fmt.Errorf("versao desconhecida")
	}
	off := 5
	si := int(binary.LittleEndian.Uint16(bc[off:]))
	off += 2
	aid, tid := bc[off], bc[off+1]
	off += 2
	nstr := int(binary.LittleEndian.Uint16(bc[off:]))
	off += 2
	strs := []string{}
	for i := 0; i < nstr; i++ {
		if off+2 > len(bc) {
			return nil, fmt.Errorf("string table truncada")
		}
		ln := int(binary.LittleEndian.Uint16(bc[off:]))
		off += 2
		if off+ln > len(bc) {
			return nil, fmt.Errorf("string truncada")
		}
		strs = append(strs, string(bc[off:off+ln]))
		off += ln
	}
	if off != len(bc) || si >= len(strs) {
		return nil, fmt.Errorf("bytecode malformado")
	}
	p := &Plan{Boot: strs[si]}
	switch aid {
	case 0:
		p.Algo = "deflate"
	case 1:
		p.Algo = "gzip"
	default:
		return nil, fmt.Errorf("algo id %d", aid)
	}
	switch tid {
	case 0:
		p.Dtype = "html"
	case 1:
		p.Dtype = "lin"
	case 2:
		p.Dtype = "lay"
	default:
		return nil, fmt.Errorf("tipo id %d", tid)
	}
	return p, nil
}
