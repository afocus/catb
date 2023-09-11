package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"text/template"
)

func (p *Parse) BuildDoc() []byte {
	collects := make([]any, 0)
	for _, v := range p.routes {
		c := v.toCollectMap()
		collects = append(collects, c)
	}
	doc := map[string]any{
		"apicat":      "2.0",
		"info":        map[string]any{},
		"servers":     []any{},
		"definitions": map[string]any{"schemas": p.defs},
		"collections": collects,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
	return buf.Bytes()
}

var tplmapfun = map[string]any{
	"replacePath": func(s string) string {
		ps := strings.Split(s, "/")
		for i, v := range ps {
			if len(v) > 0 {
				if v[0] == '{' && v[1] == '}' {
					ps[i] = ":" + v[1:len(v)-1]
				}
			}
		}
		return strings.Join(ps, "/")
	},
}

func (p *Parse) BuildRoute(protoDir, outDir string) ([]byte, error) {
	t := template.Must(template.New("").Funcs(tplmapfun).Parse(tpl))
	protopkg := filepath.Base(protoDir)
	outpkg := filepath.Base(outDir)
	x := struct {
		Package          string
		ProtoPackage     string
		ProtoPackagePath string
		Collects         []*RouteGroup
	}{
		outpkg,
		protopkg,
		protoDir,
		p.routes,
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, x); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const tpl = `// DOT EDIT IT
// 自动生成
package {{.Package}}

import (
    "{{.ProtoPackagePath}}"

    "github.com/gin-gonic/gin"
    "github.com/apicat/ginrpc"
)

{{range .Collects}}
func AddRoute{{.Name}}(r *gin.Engine, srv {{$.ProtoPackage}}.{{.Name}}, middel ...gin.HandlerFunc) {
    g := r.Group("{{replacePath .BasePath}}", middel...)
    {{- range $method,$item := .Routes}}
    g.Handle("{{$item.Method}}", "{{replacePath $item.Path}}", ginrpc.Handle(srv.{{$method}}))
    {{- end}}
}
{{end}}
`
