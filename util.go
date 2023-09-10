package main

import (
	"go/ast"
	"go/token"
)

// 获取顶级type对象的注释
// pos 对象在文档中的首位置
func getTopTypeDoc(cs []*ast.CommentGroup, pos token.Pos) string {
	for _, c := range cs {
		if c.End().IsValid() {
			// offset=6 len("\n"+"type"+" ")
			if c.End()+6 == pos {
				return c.Text()
			}
		}
	}
	return ""
}

func isPublic(v string) bool {
	return len(v) > 0 && v[0] >= 'A' && v[0] <= 'Z'
}

func stringToUnid(s string) uint {
	n := len(s)
	x := uint(n * 10000)
	for i := 0; i < n; i++ {
		x += uint(s[i])
	}
	return x
}
