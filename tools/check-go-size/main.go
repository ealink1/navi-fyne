// Command check-go-size checks authored Go file and function size limits.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	failed := false
	for _, root := range []string{"internal", "cmd", "tools"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && (entry.Name() == "upstream" || entry.Name() == ".cache") {
				return filepath.SkipDir
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			limit := 800
			test := strings.HasSuffix(path, "_test.go")
			if test {
				limit = 1500
			}
			lines := strings.Count(strings.TrimSuffix(string(raw), "\n"), "\n") + 1
			if lines > limit {
				fmt.Printf("%s: %d lines (max %d)\n", path, lines, limit)
				failed = true
			}
			if test {
				return nil
			}
			positions := token.NewFileSet()
			file, err := parser.ParseFile(positions, path, raw, 0)
			if err != nil {
				return err
			}
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				start, end := positions.Position(function.Body.Pos()), positions.Position(function.Body.End())
				if end.Line-start.Line+1 > 120 {
					fmt.Printf("%s:%d %s: %d lines (max 120)\n", path, start.Line, function.Name.Name, end.Line-start.Line+1)
					failed = true
				}
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("Authored Go source size checks passed.")
}
