package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

type Parse struct {
	fset       *token.FileSet
	structs    map[string]*Struct
	onParseEnd []func()
	defs       []*Schema
	routes     []*RouteGroup
}

func NewParse() *Parse {
	return &Parse{
		fset:    token.NewFileSet(),
		structs: make(map[string]*Struct),
	}
}

func (p *Parse) Parse(dir string) error {
	pkgs, err := parser.ParseDir(p.fset, dir, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	var pkg *ast.Package
	for _, pkg = range pkgs {
		break
	}
	p.parseFilesScopeStruct(pkg.Files)
	// 必须在解析interface路由前生成defs
	// 否则缺少依赖
	for k, v := range p.Structs() {
		if !v.IsDefinition {
			continue
		}
		p.defs = append(p.defs,
			&Schema{
				Name:   k,
				ID:     stringToUnid(k),
				Schema: p.structToJschema(v),
			},
		)
	}
	p.parseFilesScopeInterface(pkg.Files)
	return nil

}

func (p *Parse) parseFilesScopeStruct(files map[string]*ast.File) {
	for _, f := range files {
		for k, v := range f.Scope.Objects {
			t, ok := v.Decl.(*ast.TypeSpec)
			if !ok {
				continue
			}
			tt, ok := t.Type.(*ast.StructType)
			if !ok {
				continue
			}
			obj := p.parseStruct(tt)
			if len(obj.Fields) == 0 {
				continue
			}
			cs := strings.Split(getTopTypeDoc(f.Comments, t.Pos()), "\n")
			for _, v := range cs {
				if strings.HasPrefix(v, "@schema") {
					obj.IsDefinition = true
					break
				}
			}
			p.structs[k] = obj
		}
	}

	for _, callback := range p.onParseEnd {
		callback()
	}
}

func (p *Parse) parseFilesScopeInterface(files map[string]*ast.File) {
	for _, f := range files {
		for k, v := range f.Scope.Objects {
			t, ok := v.Decl.(*ast.TypeSpec)
			if !ok {
				continue
			}
			iface, ok := t.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			if !strings.HasSuffix(k, "Service") {
				continue
			}
			doc := strings.Split(getTopTypeDoc(f.Comments, t.Pos()), "\n")
			obj := RouteGroup{
				Name:   k,
				Routes: make(map[string]*Route),
			}
			cs := []string{}
			for _, v := range doc {
				if strings.HasPrefix(v, "@group") {
					if ps := strings.Fields(v); len(ps) > 1 {
						obj.BasePath = ps[1]
					}
				} else {
					if v != "" {
						cs = append(cs, v)
					}
				}
			}
			obj.Description = strings.Join(cs, "\n")
			if err := p.parseRoutes(iface, &obj); err != nil {
				log.Fatalf("route %s %v", k, err)
			}
			p.routes = append(p.routes, &obj)
		}
	}
}

func (p *Parse) parseRoutes(v *ast.InterfaceType, obj *RouteGroup) error {
	for _, v := range v.Methods.List {
		tt, ok := v.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		if v.Doc == nil {
			continue
		}
		var cs []string
		var r Route
		for _, s := range strings.Split(v.Doc.Text(), "\n") {
			// @route method path)
			if strings.HasPrefix(s, "@route") {
				ps := strings.Fields(s)
				if len(ps) != 3 {
					return fmt.Errorf("路由规则错误 %s", s)
				}
				r.Method = strings.ToUpper(ps[1])
				r.Path = ps[2]
			} else if strings.HasPrefix(s, "@code") {
				ps := strings.Fields(s)
				if code, _ := strconv.ParseInt(ps[1], 10, 32); code > 0 {
					r.StatusCode = int(code)
				}
			} else if s != "" {
				cs = append(cs, s)
			}
		}
		routeName := v.Names[0].String()
		fullname := obj.Name + "." + routeName
		if r.Method == "" {
			return fmt.Errorf("未找到路由规则 %s", fullname)
		}
		r.Description = strings.Join(cs, "\n")
		sigerr := fmt.Errorf("%v %s", routeMethodSigErr, fullname)
		{
			// 处理请求入参
			if tt.Params.NumFields() != 2 {
				return sigerr
			}
			inexp, ok := tt.Params.List[1].Type.(*ast.StarExpr)
			if !ok {
				return sigerr
			}
			paramName := p.quickGetType(inexp.X)
			r.Proto.In = paramName
			if paramName != "ginrpc.Empty" {
				if o := p.lookupStruct(paramName); o != nil {
					r.Paramas = p.parseRequestParam(o)
					if r.Method != http.MethodGet {
						// 非get请求尝试解析请求体body
						// fmt.Println(paramName, o)
						r.Content = p.structToJschema(o, paramName)
					}
				}
			}
		}

		{
			// 处理响应
			if tt.Results.NumFields() != 2 {
				return sigerr
			}
			re, ok := tt.Results.List[0].Type.(*ast.StarExpr)
			if !ok {
				return sigerr
			}

			resultname := p.quickGetType(re.X)
			r.Proto.Out = resultname
			if m := resultname; m != "ginrpc.Empty" {
				if o := p.lookupStruct(m); o != nil {
					r.Response = p.structToJschema(o, m)
				}
			}
			if p.quickGetType(tt.Results.List[1].Type) != "error" {
				return sigerr
			}
		}
		obj.Routes[routeName] = &r
	}

	return nil
}

func (p *Parse) Structs() map[string]*Struct {
	return p.structs
}

func (p *Parse) lookupStruct(n string) *Struct {
	return p.structs[n]
}

func (p *Parse) emitCallback(fn func()) {
	p.onParseEnd = append(p.onParseEnd, fn)
}

func (p *Parse) parseStruct(st *ast.StructType) *Struct {
	info := &Struct{
		Fields: make(map[string]*Field),
	}
	for _, f := range st.Fields.List {
		if len(f.Names) > 0 {
			// 必须有tag tag是定义apicat的有效元素
			if f.Tag == nil {
				continue
			}
			var field Field
			if !p.parseExpr(f.Type, &field) {
				log.Println("warnning")
				continue
			}
			field.Tag = reflect.StructTag(f.Tag.Value[1 : len(f.Tag.Value)-1])
			fdoc := f.Doc.Text()
			// 最后的\n
			if n := len(fdoc); n > 0 {
				fdoc = fdoc[:n-1]
			}
			field.Comment = fdoc
			info.Fields[f.Names[0].String()] = &field
		} else {
			// 嵌入其他类型
			// 需要全部吧类型解析完才能确定这里的内容
			embed := &Field{}
			if !p.parseExpr(f.Type, embed) {
				continue
			}
			// fmt.Printf("%+v\n", embed)
			p.emitCallback(func() {
				// 通过类型名找到结构体
				em, ok := p.structs[embed.Type]
				if ok {
					for k, v := range em.Fields {
						// 根据golang嵌套规则 嵌套的字段优先级低于外层
						if _, exist := info.Fields[k]; !exist {
							newf := *v
							info.Fields[k] = &newf
						}
					}
				}
			})
		}
	}
	return info
}

func (p *Parse) quickGetType(expr ast.Expr) string {
	var f Field
	p.deepParseExpr(expr, &f, true)
	return f.Type
}

func (p *Parse) parseExpr(expr ast.Expr, f *Field) bool {
	return p.deepParseExpr(expr, f, false)
}

func (p *Parse) deepParseExpr(expr ast.Expr, f *Field, onlytype bool) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		f.Type = t.String()
	case *ast.ArrayType:
		f.Type = "array"
		if !onlytype {
			var subf Field
			if p.deepParseExpr(t.Elt, &subf, onlytype) {
				f.sub = &subf
			}
		}
	case *ast.MapType:
		f.Type = "map"
		if !onlytype {
			var subf Field
			if p.deepParseExpr(t.Value, &subf, onlytype) {
				f.sub = &subf
			}
		}
	case *ast.StructType:
		f.Type = "struct"
		if !onlytype {
			sub := p.parseStruct(t)
			f.sub = sub
		}
	case *ast.StarExpr:
		p.deepParseExpr(t.X, f, onlytype)
	case *ast.SelectorExpr:
		f.Type = fmt.Sprintf("%s.%s", t.X, t.Sel.Name)
	default:
		fmt.Printf("unsupport %T\n", t)
		return false
	}
	return true
}

func (p *Parse) fieldToJschema(f *Field) (*JsonSchema, bool) {
	var s JsonSchema
	switch f.Type {
	case "int", "int64", "int32", "uint", "uint32", "uint64":
		s.Type = "integer"
	case "float32", "float64":
		s.Type = "number"
	case "string":
		s.Type = f.Type
	case "bool":
		s.Type = "boolean"
	case "time.Time":
		s.Type = "string"
		s.Format = "date-time"
	case "struct":
		ns := p.structToJschema(f.sub.(*Struct), f.Type)
		s = *ns
	case "map":
		s.Type = "object"
		s.AdditionalProperties = &JsonSchema{
			Type: "any",
		}
	case "array":
		if f.sub != nil {
			// []byte,[]uint8
			// 会被json自动base64化
			af := f.sub.(*Field)
			if af.Type == "byte" || af.Type == "uint8" {
				s.Type = "string"
			} else {
				s.Items = &JsonSchema{}
			}
		} else {
			s.Items = &JsonSchema{Type: "any"}
		}
	default:
		o := p.lookupStruct(f.Type)
		if o != nil {
			s.Type = "object"
			jso := p.structToJschema(o, f.Type)
			s = *jso
		}
	}
	s.Description = f.Comment
	var required bool
	if bind, ok := f.Tag.Lookup("binding"); ok {
		if strings.Contains(bind, "required") {
			required = true
		}
	}
	return &s, required
}

func (p *Parse) structToJschema(v *Struct, n ...string) *JsonSchema {
	js := &JsonSchema{
		Type: "object",
	}
	if len(n) > 0 {
		if v.IsDefinition {
			js.Reference = fmt.Sprintf("#/definitions/schemas/%d", stringToUnid(n[0]))
			return js
		}
	}
	js.Properties = make(map[string]*JsonSchema)
	for k, f := range v.Fields {
		// 只处理导出的字段
		if !isPublic(k) {
			continue
		}
		ns := strings.Split(f.Tag.Get("json"), ",")
		name := ns[0]
		if name == "" || name == "-" {
			continue
		}
		s, required := p.fieldToJschema(f)
		if required {
			s.Required = append(s.Required, name)
		}
		js.XOrder = append(js.XOrder, name)
		js.Properties[name] = s
	}
	return js
}

func (p *Parse) parseRequestParam(o *Struct) map[string][]*Paramter {
	pts := make(map[string][]*Paramter)
	for k, v := range o.Fields {
		if !isPublic(k) {
			continue
		}
		// find paramter
		for _, tag := range paramterTypes {
			name, ok := v.Tag.Lookup(tag)
			if !ok {
				continue
			}
			js, required := p.fieldToJschema(v)
			pam := &Paramter{
				Required: required,
				Schema: Schema{
					Name:   name,
					Schema: js,
				},
			}
			if _, ok := pts[tag]; !ok {
				pts[tag] = []*Paramter{}
			}
			pts[tag] = append(pts[name], pam)
		}
	}
	return pts
}
