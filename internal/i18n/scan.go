package i18n

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Site is one place in the source that hands a message to a client.
type Site struct {
	Msg   string
	Where string
}

// ScanSources walks Go sources under root and returns every literal message the
// REST layer can send to a browser.
//
// Written as an AST walk rather than a grep because the catalogue is keyed by
// the Vietnamese sentence itself: the only thing that keeps it honest is a test
// that reads the same literals the handlers pass, and a regex over source text
// would drift the first time somebody wraps a call across two lines.
func ScanSources(root string) ([]Site, error) {
	var out []Site
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		// This package's own catalogue is the answer key, not a call site.
		if strings.Contains(filepath.ToSlash(p), "/i18n/") {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, 0)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", p, perr)
		}
		at := func(n ast.Node) string {
			return fmt.Sprintf("%s:%d", p, fset.Position(n.Pos()).Line)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				name := calleeName(node)
				// abort(c, status, msg), abortCode(c, status, code, msg), and
				// i18n.Msg(c, msg) for the handful of places that build their
				// own response body instead of going through a helper.
				idx := -1
				switch name {
				case "abort":
					idx = 2
				case "abortCode":
					idx = 3
				case "Msg", "Translate":
					idx = 1
				}
				if idx >= 0 && idx < len(node.Args) {
					if s, ok := stringLit(node.Args[idx]); ok {
						out = append(out, Site{s, at(node.Args[idx])})
					}
				}
			case *ast.CompositeLit:
				// gin.H{"error": "..."} and domain.InvalidInput{Message: "..."}.
				for _, e := range node.Elts {
					kv, ok := e.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key := ""
					switch k := kv.Key.(type) {
					case *ast.BasicLit:
						key, _ = strconv.Unquote(k.Value)
					case *ast.Ident:
						key = k.Name
					}
					if key != "error" && key != "Message" {
						continue
					}
					if s, ok := stringLit(kv.Value); ok {
						out = append(out, Site{s, at(kv.Value)})
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Msg < out[j].Msg })
	return out, nil
}

func calleeName(c *ast.CallExpr) string {
	switch fn := c.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil || s == "" {
		return "", false
	}
	return s, true
}
