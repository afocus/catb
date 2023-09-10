package main

import (
	"bytes"
	"path/filepath"
	"text/template"
)

func (p *Parse) BuildRoute(protoDir, outDir string) ([]byte, error) {
	t := template.Must(template.New("").Parse(tpl))
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
    g := r.Group("{{.BasePath}}", middel...)
    {{- range $method,$item := .Routes}}
    g.Handle("{{$item.Method}}", "{{$item.Path}}", ginrpc.Handle(srv.{{$method}}))
    {{- end}}
}
{{end}}
`
