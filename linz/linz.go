// Package linz — compilador + assembler linz-P0.
//
// Source (.linz, canonico, gerado por transpile_spec_to_linz.py):
// @LINZ:1.0 / ENVELOPE v2 / PAYLOAD <nome> / ALGO deflate|gzip|brotli / CHUNK <n> / ASSEMBLE / END
// P0: envelope v2 apenas. linz nao comprime (excelencia = envelope+prova): recebe o payload
// base64url ja comprimido (ex: saida do linp) e monta os discos v2 + manifesto RuleL.
// Bytecode (.linzbc): "LNZ1"+ver(1)+algo u8+pad+chunk u32+ nstr u16 + stab[payload].
// Seguranca reusada do LIN (sem tocar no LIN): SHA-256 por chunk e raiz (FIPS 180-4 via
// crypto/sha256), formato v2 identico, manifesto RuleL identico, receipt (source,bytecode,root).
package linz

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const Magic = "LNZ1"
const Version = 0x01

type Plan struct {
	Payload string
	Algo    string
	Chunk   int
}

func algoID(a string) (uint8, bool) {
	switch a {
	case "deflate":
		return 0, true
	case "gzip":
		return 1, true
	case "brotli":
		return 2, true
	}
	return 0, false
}

// Compile parses canonical .linz and emits deterministic .linzbc.
func Compile(src []byte) ([]byte, *Plan, error) {
	p := &Plan{}
	seen := map[string]bool{}
	assembles := 0
	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if i == 0 && line == "@LINZ:1.0" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "ENVELOPE "):
			if seen["ENVELOPE"] {
				return nil, nil, fmt.Errorf("linha %d: ENVELOPE duplicado", i+1)
			}
			seen["ENVELOPE"] = true
			if strings.TrimSpace(line[9:]) != "v2" {
				return nil, nil, fmt.Errorf("linha %d: P0 aceita so envelope v2", i+1)
			}
		case strings.HasPrefix(line, "PAYLOAD "):
			if seen["PAYLOAD"] {
				return nil, nil, fmt.Errorf("linha %d: PAYLOAD duplicado", i+1)
			}
			seen["PAYLOAD"] = true
			p.Payload = strings.TrimSpace(line[8:])
			if strings.Contains(p.Payload, "..") || strings.HasPrefix(p.Payload, "/") || p.Payload == "" {
				return nil, nil, fmt.Errorf("linha %d: payload invalido", i+1)
			}
		case strings.HasPrefix(line, "ALGO "):
			if seen["ALGO"] {
				return nil, nil, fmt.Errorf("linha %d: ALGO duplicado", i+1)
			}
			seen["ALGO"] = true
			p.Algo = strings.TrimSpace(line[5:])
			if _, ok := algoID(p.Algo); !ok {
				return nil, nil, fmt.Errorf("linha %d: algo %q desconhecido", i+1, p.Algo)
			}
		case strings.HasPrefix(line, "CHUNK "):
			if seen["CHUNK"] {
				return nil, nil, fmt.Errorf("linha %d: CHUNK duplicado", i+1)
			}
			seen["CHUNK"] = true
			n, err := strconv.Atoi(strings.TrimSpace(line[6:]))
			if err != nil || n < 64 || n > 1800000 {
				return nil, nil, fmt.Errorf("linha %d: CHUNK fora de [64,1800000]", i+1)
			}
			p.Chunk = n
		case line == "ASSEMBLE":
			assembles++
			if assembles > 1 {
				return nil, nil, fmt.Errorf("linha %d: P0 aceita 1 ASSEMBLE", i+1)
			}
		case line == "END":
		default:
			return nil, nil, fmt.Errorf("linha %d: %q fora do subset linz-P0", i+1, line)
		}
	}
	if !seen["ENVELOPE"] || p.Payload == "" || p.Algo == "" || p.Chunk == 0 || assembles != 1 {
		return nil, nil, fmt.Errorf("plano incompleto (ENVELOPE/PAYLOAD/ALGO/CHUNK/ASSEMBLE/END)")
	}
	aid, _ := algoID(p.Algo)
	var buf bytes.Buffer
	buf.WriteString(Magic)
	buf.WriteByte(Version)
	buf.WriteByte(aid)
	buf.WriteByte(0x00)
	binary.Write(&buf, binary.LittleEndian, uint32(p.Chunk))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(len(p.Payload)))
	buf.WriteString(p.Payload)
	return buf.Bytes(), p, nil
}

// Decode re-reads .linzbc (fail-closed).
func Decode(bc []byte) (*Plan, error) {
	if len(bc) < 4+1+1+1+4+2 {
		return nil, fmt.Errorf("bytecode truncado")
	}
	if string(bc[:4]) != Magic {
		return nil, fmt.Errorf("magic invalido")
	}
	if bc[4] != Version {
		return nil, fmt.Errorf("versao desconhecida")
	}
	aid := bc[5]
	chunk := int(binary.LittleEndian.Uint32(bc[7:]))
	nstr := int(binary.LittleEndian.Uint16(bc[11:]))
	off := 13
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
	if off != len(bc) || len(strs) != 1 {
		return nil, fmt.Errorf("bytecode malformado")
	}
	var algo string
	switch aid {
	case 0:
		algo = "deflate"
	case 1:
		algo = "gzip"
	case 2:
		algo = "brotli"
	default:
		return nil, fmt.Errorf("algo id %d", aid)
	}
	return &Plan{Payload: strs[0], Algo: algo, Chunk: chunk}, nil
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Assemble monta o envelope v2 a partir do payload base64url ja comprimido.
// Funcao pura sobre strings: mesmo payload -> mesmos discos, sempre.
func Assemble(plan *Plan, payloadB64 string) (disks []string, root string, err error) {
	if payloadB64 == "" {
		return nil, "", fmt.Errorf("payload vazio")
	}
	root = sha256Hex([]byte(payloadB64))
	total := (len(payloadB64) + plan.Chunk - 1) / plan.Chunk
	for i := 0; i < total; i++ {
		end := (i + 1) * plan.Chunk
		if end > len(payloadB64) {
			end = len(payloadB64)
		}
		chunk := payloadB64[i*plan.Chunk : end]
		ch := sha256Hex([]byte(chunk))
		disks = append(disks, fmt.Sprintf("v2;%s;[%d/%d];%s;%s;%s",
			plan.Algo, i+1, total, ch, root, chunk))
	}
	return disks, root, nil
}

// Manifest is the in-memory manifest pair.
type Manifest struct {
	JSON  []byte
	RuleL []byte
}

// BuildManifest renders manifest.json + manifest.rulel purely in memory.
func BuildManifest(plan *Plan, disks []string, root string) *Manifest {
	hashes := make([]string, 0, len(disks))
	encChars := 0
	for _, d := range disks {
		parts := strings.SplitN(d, ";", 6)
		if len(parts) != 6 {
			continue
		}
		hashes = append(hashes, parts[3])
		encChars += len(parts[5])
	}
	man := map[string]interface{}{
		"payload": plan.Payload, "algorithm": plan.Algo, "encoded_chars": encChars,
		"total_disks": len(disks), "root_sha256": root, "disk_hashes": hashes,
		"format_version": "v2", "assembler": "linz-P0",
	}
	mj, _ := json.MarshalIndent(man, "", "  ")
	var rb bytes.Buffer
	rb.WriteString("@RULEL:FLOPPY_MANIFEST:2.0.0\n")
	rb.WriteString("~R{.s=payload .a=algo .e=enc_chars .t=total_disks .r=root_sha .f=format}\n")
	fmt.Fprintf(&rb, ".payload=%q\n.algo=%q\n.format=%q\n.enc_chars=%d\n.total_disks=%d\n.root_sha256=%q\n.disk_hashes=[\n",
		plan.Payload, plan.Algo, "v2", encChars, len(disks), root)
	for _, h := range hashes {
		fmt.Fprintf(&rb, "  %q,\n", h)
	}
	rb.WriteString("]\n")
	return &Manifest{JSON: mj, RuleL: rb.Bytes()}
}

// WriteOut persiste discos + manifest.json + manifest.rulel + .linzbc + receipt.
func WriteOut(plan *Plan, bytecode, source []byte, disks []string, root, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	hashes := []string{}
	for i, d := range disks {
		parts := strings.SplitN(d, ";", 6)
		if len(parts) != 6 {
			return fmt.Errorf("disco %d malformado", i+1)
		}
		hashes = append(hashes, parts[3])
		if err := os.WriteFile(filepath.Join(outDir, fmt.Sprintf("disk_%02d.txt", i+1)), []byte(d), 0644); err != nil {
			return err
		}
	}
	m := BuildManifest(plan, disks, root)
	_ = os.WriteFile(filepath.Join(outDir, "manifest.json"), m.JSON, 0644)
	_ = os.WriteFile(filepath.Join(outDir, "manifest.rulel"), m.RuleL, 0644)
	_ = os.WriteFile(filepath.Join(outDir, "job.linzbc"), bytecode, 0644)
	receipt := map[string]string{
		"assembler": "linz-P0", "source_sha256": sha256Hex(source),
		"bytecode_sha256": sha256Hex(bytecode), "root_sha256": root, "algo": plan.Algo,
	}
	rj, _ := json.MarshalIndent(receipt, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "receipt.json"), rj, 0644)
	return nil
}
