package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
)

type Schema struct {
	ID     uint   `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Schema *JsonSchema
}

type JsonSchema struct {
	Type                 string                 `json:"type"`
	Format               string                 `json:"format,omitempty"`
	XOrder               []string               `json:"x-apicat-order,omitempty"`
	Items                *JsonSchema            `json:"items,omitempty"`
	Properties           map[string]*JsonSchema `json:"properties,omitempty"`
	AdditionalProperties *JsonSchema            `json:"additionalProperties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Reference            string                 `json:"$ref,omitempty"`
}

type Field struct {
	Type    string
	Comment string
	Tag     reflect.StructTag

	// struct,map,array ele
	sub any
}

type Struct struct {
	Name         string
	Fields       map[string]*Field
	IsDefinition bool
}

var paramterTypes = []string{
	"header",
	"query",
	"cookie",
	"uri",
}

type Paramter struct {
	Schema
	Required bool `json:"required,omitempty"`
}

type RouteGroup struct {
	Name        string
	Description string
	// 根路由
	BasePath string
	// 路由方法
	// key 原始interface内部方法名称
	Routes map[string]*Route
}

func (rg *RouteGroup) toCollectMap() any {
	parentID := stringToUnid(rg.Name)
	m := map[string]any{
		"title": rg.Description,
		"id":    parentID,
		"type":  "category",
	}
	items := make([]map[string]any, 0)
	for k, v := range rg.Routes {
		item := map[string]any{
			"title":    v.Description,
			"id":       stringToUnid(rg.Name + k),
			"parentid": parentID,
			"type":     "http",
		}
		item["content"] = []any{
			v.getUrlNode(rg.BasePath),
			v.getRequestNode(),
			v.getResponseNode(),
		}
		items = append(items, item)
	}
	m["items"] = items
	return m
}

type Route struct {
	Description string
	Path        string
	Method      string
	Paramas     map[string][]*Paramter
	Content     *JsonSchema
	StatusCode  int
	Response    *JsonSchema
	Proto       struct {
		In  string
		Out string
	}
}

func (r *Route) getUrlNode(base string) map[string]any {
	return map[string]any{
		"type": "apicat-http-url",
		"attrs": map[string]any{
			"path":   base + r.Path,
			"method": r.Method,
		},
	}
}

func (r *Route) getRequestNode() map[string]any {
	requestAttrs := map[string]any{
		"parameters": r.Paramas,
	}
	if r.Content != nil {
		requestAttrs["content"] = map[string]any{
			contentType: map[string]any{
				"schema": r.Content,
			},
		}
	}
	return map[string]any{
		"type":  "apicat-http-request",
		"attrs": requestAttrs,
	}
}

const contentType = "application/json"

func (r *Route) getResponseNode() map[string]any {
	statusCode := 200
	if r.StatusCode != 0 {
		statusCode = r.StatusCode
	}
	content := make(map[string]any)
	if r.Response != nil {
		content[contentType] = map[string]any{
			"schema": r.Response,
		}
	}
	ret := map[string]any{
		"code":        statusCode,
		"description": "success",
		"content":     content,
	}
	return map[string]any{
		"type": "apicat-http-response",
		"attrs": map[string]any{
			"list": []any{ret},
		},
	}
}

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

var routeMethodSigErr = errors.New("路由方法签名错误 必须是func(*gin.Context,*struct)(*struct,error)")
