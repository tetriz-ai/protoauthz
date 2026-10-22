// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

// Package protoauthz_test keeps the README's quick start, its example program
// and the pkg.go.dev Example the same code.
package protoauthz_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// quickStartBlock returns the first go block after the Quick start heading.
func quickStartBlock(t *testing.T) string {
	t.Helper()
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(readme), "\n## Quick start\n")
	if !ok {
		t.Fatal("README has no Quick start section")
	}
	for _, b := range fencedBlocks(t, rest) {
		if b.lang == "go" {
			return b.body
		}
	}
	t.Fatal("Quick start has no go block")
	return ""
}

// funcBody prints the body of the named function in file, without comments
// or blank lines (an Example's "// Output:" comment leaves one behind).
func funcBody(t *testing.T, file, name string) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
			var b bytes.Buffer
			if err := printer.Fprint(&b, fset, fn.Body); err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, l := range strings.Split(b.String(), "\n") {
				if strings.TrimSpace(l) != "" {
					lines = append(lines, l)
				}
			}
			return strings.Join(lines, "\n")
		}
	}
	t.Fatalf("%s has no func %s", file, name)
	return ""
}

func TestQuickStartMatchesExample(t *testing.T) {
	const mainFile = "examples/deny-by-role/main.go"
	want, err := os.ReadFile(mainFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := quickStartBlock(t); got != string(want) {
		t.Fatalf("README quick start differs from %s:\n%s", mainFile, got)
	}
	if a, b := funcBody(t, mainFile, "main"), funcBody(t, "example_test.go", "ExampleNewAuthLevelInterceptor"); a != b {
		t.Fatalf("ExampleNewAuthLevelInterceptor body differs from main:\n%s\n---\n%s", b, a)
	}
}

// fence matches one fenced block of the README, at any indent: the opening
// indent, the language, the body and the closing indent.
var fence = regexp.MustCompile("(?ms)^( *)```(\\w*)\n(.*?)^( *)```$")

// block is one fenced block with its indent removed.
type block struct{ lang, body string }

// fencedBlocks returns every fenced block in text. A block indented under a
// list item has that indent removed from each line. A block whose closing
// fence or a non-blank line lacks the opening indent fails the test, so the
// dedented body is exactly what a reader copies.
func fencedBlocks(t *testing.T, text string) []block {
	t.Helper()
	var blocks []block
	for _, m := range fence.FindAllStringSubmatch(text, -1) {
		indent, lang, body := m[1], m[2], m[3]
		if m[4] != indent {
			t.Fatalf("%s block: closing fence indent %q differs from opening %q", lang, m[4], indent)
		}
		lines := strings.SplitAfter(body, "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			trimmed, ok := strings.CutPrefix(line, indent)
			if !ok {
				t.Fatalf("%s block: line %q lacks the fence indent %q", lang, line, indent)
			}
			lines[i] = trimmed
		}
		blocks = append(blocks, block{lang, strings.Join(lines, "")})
	}
	return blocks
}

// TestReadmeGoBlocks compiles and runs every Go block in the README and in
// docs/. A program's standard output must equal the next text block, when one
// comes before the next Go block. A test file must pass. Any other Go block
// fails: every block a reader copies is complete.
func TestReadmeGoBlocks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs each README program")
	}
	docs, err := filepath.Glob("docs/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for i, file := range append([]string{"README.md"}, docs...) {
		t.Run(file, func(t *testing.T) {
			n := runGoBlocks(t, file, fmt.Sprintf("_readmeblock%d_", i))
			if file == "README.md" && n == 0 {
				t.Fatal("README has no Go blocks")
			}
		})
	}
}

// runGoBlocks runs every Go block in file, each in its own directory named
// prefix plus the block's number, and returns how many it found.
func runGoBlocks(t *testing.T, file, prefix string) int {
	t.Helper()
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	blocks := fencedBlocks(t, string(text))
	n := 0
	for i, b := range blocks {
		if b.lang != "go" {
			continue
		}
		n++
		body := b.body
		t.Run(fmt.Sprintf("block %d", n), func(t *testing.T) {
			// A directory starting with "_" is inside the module, so the block
			// can import it, and outside every ./... pattern.
			dir := fmt.Sprintf("%s%d", prefix, n)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			switch {
			case strings.Contains(body, "package main\n") && strings.Contains(body, "func main()"):
				writeFile(t, filepath.Join(dir, "main.go"), body)
				out, err := exec.Command("go", "run", "./"+dir).Output()
				if err != nil {
					t.Fatalf("go run: %v\n%s", err, stderr(err))
				}
				if want, ok := nextText(blocks[i+1:]); ok && string(out) != want {
					t.Fatalf("output:\n%s\nwant:\n%s", out, want)
				}
			case strings.Contains(body, "func Test"):
				writeFile(t, filepath.Join(dir, "readme_block_test.go"), body)
				if out, err := exec.Command("go", "test", "-count=1", "./"+dir).CombinedOutput(); err != nil {
					t.Fatalf("go test: %v\n%s", err, out)
				}
			default:
				t.Fatalf("not a complete program or test file:\n%s", body)
			}
		})
	}
	return n
}

// nextText returns the first text block before the next Go block.
func nextText(rest []block) (string, bool) {
	for _, b := range rest {
		switch b.lang {
		case "text":
			return b.body, true
		case "go":
			return "", false
		}
	}
	return "", false
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stderr(err error) string {
	if ee, ok := err.(*exec.ExitError); ok {
		return string(ee.Stderr)
	}
	return ""
}

// Exercise the exact server wiring readers copy, including stream login.
func TestReadmeServerAuthorization(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	var program string
	for _, b := range fencedBlocks(t, string(readme)) {
		if b.lang == "go" && strings.Contains(b.body, "server := grpc.NewServer") {
			program = b.body
			break
		}
	}
	if program == "" {
		t.Fatal("server example missing")
	}
	program = strings.Replace(program, `"fmt"`, "", 1)
	program = strings.Replace(program, "func main() {", "func readmeServer() *grpc.Server {", 1)
	program = strings.Replace(program, `fmt.Println("public:", protoauthz.UnauthenticatedMethods(levels))`, "return server", 1)
	dir, err := os.MkdirTemp(".", "_readmeauth_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	writeFile(t, filepath.Join(dir, "main.go"), program)
	writeFile(t, filepath.Join(dir, "server_test.go"), readmeServerTest)
	if out, err := exec.Command("go", "test", "-count=1", "./"+dir).CombinedOutput(); err != nil {
		t.Fatalf("README authorization: %v\n%s", err, out)
	}
}

const readmeServerTest = `package main
import (
 "context"
 "net"
 "testing"
 "google.golang.org/grpc"
 "google.golang.org/grpc/codes"
 "google.golang.org/grpc/credentials/insecure"
 "google.golang.org/grpc/metadata"
 "google.golang.org/grpc/status"
 "google.golang.org/grpc/test/bufconn"
 examplev1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1"
)
func TestAuthorization(t *testing.T) {
 server := readmeServer()
 listener := bufconn.Listen(1<<20)
 go func() { _ = server.Serve(listener) }()
 defer server.Stop()
 conn, err := grpc.NewClient("passthrough:///notes", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context,string)(net.Conn,error){ return listener.Dial() }))
 if err != nil { t.Fatal(err) }
 defer conn.Close()
 client := examplev1.NewNoteServiceClient(conn)
 for _, tt := range []struct{ name, token string; want codes.Code }{
  {"anonymous", "", codes.Unauthenticated},
  {"forged", "forged", codes.Unauthenticated},
  {"authenticated", "token-jane", codes.Unimplemented},
 } {
  t.Run(tt.name, func(t *testing.T) {
   ctx := context.Background()
   if tt.token != "" { ctx = metadata.AppendToOutgoingContext(ctx,"authorization","Bearer "+tt.token) }
   _, err := client.ListNotes(ctx, &examplev1.ListNotesRequest{})
   if status.Code(err) != tt.want { t.Errorf("unary: %v, want %v",err,tt.want) }
   stream, err := client.WatchNotes(ctx, &examplev1.WatchNotesRequest{})
   if err == nil { _, err = stream.Recv() }
   if status.Code(err) != tt.want { t.Errorf("stream: %v, want %v",err,tt.want) }
  })
 }
}
`
