package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
)

func main() {
	var (
		protoDir  string
		outputDir string
	)
	flag.StringVar(&protoDir, "in_dir", "", "协议所在的文件夹")
	flag.StringVar(&outputDir, "out_dir", "", "生成的路由的文档文件夹")
	flag.Parse()
	if protoDir == "" || outputDir == "" {
		log.Fatalln("in_dir,in_dir error")
	}

	p := NewParse()
	if err := p.Parse(protoDir); err != nil {
		log.Fatalln("解析协议失败", err)
	}

	if err := os.MkdirAll(outputDir, 0777); err != nil {
		log.Fatalln("创建输出文件夹失败", outputDir, err)
	}

	rs, err := p.BuildRoute(protoDir, outputDir)
	if err != nil {
		log.Fatalln("生成路由文件失败", err)
	}

	if err := os.WriteFile(
		filepath.Join(outputDir, "route.go"),
		rs,
		0666); err != nil {
		log.Fatalln("写入文件失败", err)
	}
	if err := os.WriteFile(
		filepath.Join(outputDir, "apicat.json"),
		p.BuildDoc(),
		0666); err != nil {
		log.Fatalln("写入文件失败", err)
	}
}
