// Package web 内嵌前端构建产物（Vite 构建输出到 web/dist）。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist 返回以 dist 为根的前端文件系统。
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
