// Package rararc reads RAR archives by running the bundled 7-Zip as a
// separate program (LGPL; see the third-party notices). Go has no maintained
// RAR reader that handles RAR5, solid archives and multi-part sets, and the
// RAR format may only be decompressed with unRAR-licensed code, so 7-Zip's
// reader is used as it is, through its command line.
package rararc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"romcure/internal/proc"
)

// Tool is a 7-Zip executable that can read RAR (7z.exe with 7z.dll, or 7zz).
type Tool struct{ Path string }

// Member is one file inside an archive, as its headers describe it.
type Member struct {
	Name  string // slash-separated
	Size  int64
	CRC32 string // lowercase hex, 8 digits; "" when the archive has none
}

// ErrNoTool is returned when RAR support was asked for but 7-Zip was not found.
var ErrNoTool = errors.New("RAR files need 7-Zip, which was not found next to the app")

var partRE = regexp.MustCompile(`(?i)\.part0*(\d+)\.rar$`)

// IsContinuation reports whether path is a later volume of a multi-part set
// ("Game.part2.rar"): 7-Zip finds those from the first volume, so they are
// not archives of their own.
func IsContinuation(path string) bool {
	m := partRE.FindStringSubmatch(path)
	if m == nil {
		return false
	}
	n, _ := strconv.Atoi(m[1])
	return n > 1
}

func (t Tool) command(ctx context.Context, args ...string) *exec.Cmd {
	return proc.Quiet(exec.CommandContext(ctx, t.Path, args...))
}

// List returns the files of archive from its headers; nothing is
// decompressed. Folders are left out.
func (t Tool) List(ctx context.Context, archive string) ([]Member, error) {
	if t.Path == "" {
		return nil, ErrNoTool
	}
	var out, errb bytes.Buffer
	cmd := t.command(ctx, "l", "-slt", "-sccUTF-8", "-y", "-bd", "-spd", archive)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("7-Zip could not read it: %s", firstProblem(out.String(), errb.String(), err))
	}
	return parseList(out.String()), nil
}

// parseList reads the output of "7z l -slt": a header block, a line of dashes,
// then one block of "key = value" lines per entry.
func parseList(text string) []Member {
	var members []Member
	var cur map[string]string
	flush := func() {
		if cur == nil {
			return
		}
		name := cur["Path"]
		if runtime.GOOS == "windows" {
			name = strings.ReplaceAll(name, `\`, "/") // 7-Zip prints the OS separator there
		}
		if name != "" && cur["Folder"] != "+" {
			size, _ := strconv.ParseInt(cur["Size"], 10, 64)
			crc := strings.ToLower(cur["CRC"])
			if crc != "" && len(crc) < 8 {
				crc = strings.Repeat("0", 8-len(crc)) + crc
			}
			members = append(members, Member{Name: name, Size: size, CRC32: crc})
		}
		cur = nil
	}
	set := func(k, v string) {
		if cur == nil {
			cur = map[string]string{}
		}
		cur[k] = v
	}
	inEntries := false
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case line == "----------":
			inEntries = true
		case !inEntries:
		case line == "":
			flush()
		default:
			if k, v, ok := strings.Cut(line, " = "); ok {
				set(k, v)
			} else if k, ok := strings.CutSuffix(line, " ="); ok {
				set(k, "")
			}
		}
	}
	flush()
	return members
}

// Extract writes member of archive to dest. It goes through a temporary file
// and a rename, so a failed or cancelled extraction leaves nothing at dest.
func (t Tool) Extract(ctx context.Context, archive, member, dest string) error {
	if t.Path == "" {
		return ErrNoTool
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".romcure-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	var errb bytes.Buffer
	// -so: file data to stdout. -spd: the name is literal, not a wildcard.
	cmd := t.command(ctx, "e", "-so", "-y", "-bd", "-spd", "-sccUTF-8", archive, member)
	cmd.Stdout, cmd.Stderr = out, &errb
	runErr := cmd.Run()
	if cerr := out.Close(); runErr == nil {
		runErr = cerr
	}
	if runErr != nil {
		os.Remove(tmp)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("extract %s from %s: %s", member, archive, firstProblem("", errb.String(), runErr))
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, dest, err)
	}
	return nil
}

// Open streams member of archive. Read to the end, Close reports a failed
// extraction (damaged data, CRC error); closed early (only the start was
// needed), it stops 7-Zip at once and reports nothing.
func (t Tool) Open(ctx context.Context, archive, member string) (io.ReadCloser, error) {
	if t.Path == "" {
		return nil, ErrNoTool
	}
	cmd := t.command(ctx, "e", "-so", "-y", "-bd", "-spd", "-sccUTF-8", archive, member)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &stream{ReadCloser: out, cmd: cmd, errb: &errb, what: member}, nil
}

type stream struct {
	io.ReadCloser
	cmd  *exec.Cmd
	errb *bytes.Buffer
	what string
	eof  bool
}

func (s *stream) Read(p []byte) (int, error) {
	n, err := s.ReadCloser.Read(p)
	if err == io.EOF {
		s.eof = true
	}
	return n, err
}

func (s *stream) Close() error {
	if !s.eof {
		s.cmd.Process.Kill()
		s.ReadCloser.Close()
		s.cmd.Wait()
		return nil
	}
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("extract %s: %s", s.what, firstProblem("", s.errb.String(), err))
	}
	return nil
}

// firstProblem picks the line of 7-Zip's output that says what went wrong.
func firstProblem(stdout, stderr string, err error) string {
	for _, text := range []string{stderr, stdout} {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ERROR:") || strings.Contains(line, "Wrong password") ||
				strings.Contains(line, "Cannot open") || strings.Contains(line, "Data Error") || strings.Contains(line, "CRC Failed") {
				return strings.TrimSpace(strings.TrimPrefix(line, "ERROR:"))
			}
		}
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}
