// Package linp — compilador + executor linp-P0.
//
// Source (.linp, canonico, gerado por transpile_py_to_linp.py):
// @LINP:1.0 / JOB <nome> / FILE <path> / ALGO deflate|gzip|brotli / CHUNK <n> / PACK / END
// P0: exatamente 1 PACK por job. Bytes crus (sem minify HTML) — excelencia = empacotar bytes.
// Bytecode (.linpbc): "LNP1"+ver(1)+job u16+file u16+algo u8+pad+chunk u32+ nstr u16 + stab.
// Seguranca reusada do LIN (sem tocar no LIN): SHA-256 por disco e raiz (FIPS 180-4 via
// crypto/sha256), formato v2 identico ao main.go, manifesto RuleL identico, receipt que amarra
// (source_sha, bytecode_sha, root_sha). Tudo verificado por oracle Python independente.
package linp

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
)

const Magic = "LNP1"
const Version = 0x01

type Plan struct {
	Job   string
	File  string
	Algo  string
	Chunk int
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

// Compile parses canonical .linp and emits deterministic .linpbc.
func Compile(src []byte) ([]byte, *Plan, error) {
	p := &Plan{}
	seen := map[string]bool{}
	packs := 0
	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if i == 0 && line == "@LINP:1.0" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "JOB "):
			if seen["JOB"] {
				return nil, nil, fmt.Errorf("linha %d: JOB duplicado (P0: 1 por job)", i+1)
			}
			seen["JOB"] = true
			p.Job = strings.TrimSpace(line[4:])
			if p.Job == "" {
				return nil, nil, fmt.Errorf("linha %d: JOB vazio", i+1)
			}
		case strings.HasPrefix(line, "FILE "):
			if seen["FILE"] {
				return nil, nil, fmt.Errorf("linha %d: FILE duplicado (P0: 1 PACK)", i+1)
			}
			seen["FILE"] = true
			p.File = strings.TrimSpace(line[5:])
			if strings.Contains(p.File, "..") || strings.HasPrefix(p.File, "/") {
				return nil, nil, fmt.Errorf("linha %d: path fora do repo", i+1)
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
		case line == "PACK":
			packs++
			if packs > 1 {
				return nil, nil, fmt.Errorf("linha %d: P0 aceita 1 PACK", i+1)
			}
		case line == "END":
		default:
			return nil, nil, fmt.Errorf("linha %d: diretiva %q fora do subset linp-P0", i+1, line)
		}
	}
	if p.Job == "" || p.File == "" || p.Algo == "" || p.Chunk == 0 || packs != 1 {
		return nil, nil, fmt.Errorf("plano incompleto (JOB/FILE/ALGO/CHUNK/PACK/END)")
	}
	strs := []string{p.Job, p.File}
	var buf bytes.Buffer
	buf.WriteString(Magic)
	buf.WriteByte(Version)
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // job idx
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // file idx
	aid, _ := algoID(p.Algo)
	buf.WriteByte(aid)
	buf.WriteByte(0x00)
	binary.Write(&buf, binary.LittleEndian, uint32(p.Chunk))
	binary.Write(&buf, binary.LittleEndian, uint16(len(strs)))
	for _, s := range strs {
		binary.Write(&buf, binary.LittleEndian, uint16(len(s)))
		buf.WriteString(s)
	}
	return buf.Bytes(), p, nil
}

// Decode re-reads .linpbc (fail-closed).
func Decode(bc []byte) (*Plan, error) {
	if len(bc) < 4+1+2+2+1+1+4+2 {
		return nil, fmt.Errorf("bytecode truncado")
	}
	if string(bc[:4]) != Magic {
		return nil, fmt.Errorf("magic %q invalido", bc[:4])
	}
	if bc[4] != Version {
		return nil, fmt.Errorf("versao %d desconhecida", bc[4])
	}
	off := 5
	job := int(binary.LittleEndian.Uint16(bc[off:]))
	off += 2
	file := int(binary.LittleEndian.Uint16(bc[off:]))
	off += 2
	aid := bc[off]
	off += 2 // algo + pad
	chunk := int(binary.LittleEndian.Uint32(bc[off:]))
	off += 4
	nstr := int(binary.LittleEndian.Uint16(bc[off:]))
	off += 2
	strs := make([]string, 0, nstr)
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
	if off != len(bc) {
		return nil, fmt.Errorf("trailing bytes: %d", len(bc)-off)
	}
	if job >= len(strs) || file >= len(strs) {
		return nil, fmt.Errorf("indice de string fora do range")
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
		return nil, fmt.Errorf("algo id %d desconhecido", aid)
	}
	return &Plan{Job: strs[job], File: strs[file], Algo: algo, Chunk: chunk}, nil
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Compress is the exported, pure in-memory compressor (deflate|gzip|brotli,
// BestCompression). Same bytes as the disk pipeline — shared core, not a copy.
func Compress(data []byte, algo string) ([]byte, error) {
	return compress(data, algo)
}

// Artifacts is the full in-memory result of a linp job: zero disk writes.
type Artifacts struct {
	Disks        []string
	Root         string
	ManifestJSON []byte
	RuleL        []byte
	Receipt      []byte
	Compressed   []byte
	Encoded      string
	RawLen       int
}

// RunInMemory executes a plan over raw input bytes, 100% in memory.
// raw = file content the plan points at (caller loads it however it wants).
func RunInMemory(plan *Plan, bytecode, source, raw []byte) (*Artifacts, error) {
	comp, err := compress(raw, plan.Algo)
	if err != nil {
		return nil, err
	}
	enc := base64.RawURLEncoding.EncodeToString(comp)
	root := sha256Hex([]byte(enc))
	total := (len(enc) + plan.Chunk - 1) / plan.Chunk
	hashes := make([]string, 0, total)
	disks := make([]string, 0, total)
	for i := 0; i < total; i++ {
		end := (i + 1) * plan.Chunk
		if end > len(enc) {
			end = len(enc)
		}
		chunk := enc[i*plan.Chunk : end]
		ch := sha256Hex([]byte(chunk))
		hashes = append(hashes, ch)
		disks = append(disks, fmt.Sprintf("v2;%s;[%d/%d];%s;%s;%s",
			plan.Algo, i+1, total, ch, root, chunk))
	}
	man := map[string]interface{}{
		"source_file": plan.File, "algorithm": plan.Algo, "original_bytes": len(raw),
		"compressed_bytes": len(comp), "encoded_chars": len(enc),
		"total_disks": total, "root_sha256": root, "disk_hashes": hashes, "format_version": "v2",
		"job": plan.Job,
	}
	mj, _ := json.MarshalIndent(man, "", "  ")
	var rb bytes.Buffer
	rb.WriteString("@RULEL:FLOPPY_MANIFEST:2.0.0\n")
	rb.WriteString("~R{.s=source .a=algo .o=orig_bytes .c=comp_bytes .e=enc_chars .t=total_disks .r=root_sha .f=format}\n")
	fmt.Fprintf(&rb, ".source=%q\n.algo=%q\n.format=%q\n.orig_bytes=%d\n.comp_bytes=%d\n.enc_chars=%d\n.total_disks=%d\n.root_sha256=%q\n.disk_hashes=[\n",
		plan.File, plan.Algo, "v2", len(raw), len(comp), len(enc), total, root)
	for _, h := range hashes {
		fmt.Fprintf(&rb, "  %q,\n", h)
	}
	rb.WriteString("]\n")
	receipt := map[string]string{
		"job": plan.Job, "source_sha256": sha256Hex(source),
		"bytecode_sha256": sha256Hex(bytecode), "root_sha256": root, "algo": plan.Algo,
	}
	rj, _ := json.MarshalIndent(receipt, "", "  ")
	return &Artifacts{
		Disks: disks, Root: root, ManifestJSON: mj, RuleL: rb.Bytes(),
		Receipt: rj, Compressed: comp, Encoded: enc, RawLen: len(raw),
	}, nil
}

func compress(data []byte, algo string) ([]byte, error) {
	var buf bytes.Buffer
	switch algo {
	case "brotli":
		w := brotli.NewWriterLevel(&buf, brotli.BestCompression)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	case "deflate":
		w, err := flate.NewWriter(&buf, flate.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	case "gzip":
		w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("algo %q desconhecido", algo)
	}
	return buf.Bytes(), nil
}

// Run executes a decoded plan from disk files (thin wrapper over RunInMemory).
// Emite tambem o .linpbc e o receipt que amarra source/bytecode/root (seguranca estilo LIN).
func Run(plan *Plan, bytecode, source []byte, baseDir, outDir string) (root string, err error) {
	raw, err := os.ReadFile(filepath.Join(baseDir, plan.File))
	if err != nil {
		return "", err
	}
	art, err := RunInMemory(plan, bytecode, source, raw)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}
	for i, disk := range art.Disks {
		if err := os.WriteFile(filepath.Join(outDir, fmt.Sprintf("disk_%02d.txt", i+1)), []byte(disk), 0644); err != nil {
			return "", err
		}
	}
	_ = os.WriteFile(filepath.Join(outDir, "manifest.json"), art.ManifestJSON, 0644)
	_ = os.WriteFile(filepath.Join(outDir, "manifest.rulel"), art.RuleL, 0644)
	_ = os.WriteFile(filepath.Join(outDir, "job.linpbc"), bytecode, 0644)
	_ = os.WriteFile(filepath.Join(outDir, "receipt.json"), art.Receipt, 0644)
	return art.Root, nil
}
