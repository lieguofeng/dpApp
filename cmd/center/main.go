package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/lieguofeng/dpApp/internal/api"
	"github.com/lieguofeng/dpApp/internal/store"
	"github.com/lieguofeng/dpApp/web"
)

func main() {
	var (
		addr      = flag.String("addr", envOr("DPAPP_ADDR", ":8080"), "监听地址")
		dbPath    = flag.String("db", envOr("DPAPP_DB", "dpapp.db"), "SQLite 文件路径")
		secret    = flag.String("secret", envOr("DPAPP_SECRET", ""), "JWT 密钥")
		adminUser = flag.String("admin-user", envOr("DPAPP_ADMIN_USER", "admin"), "管理员用户名")
		adminPass = flag.String("admin-pass", envOr("DPAPP_ADMIN_PASS", "admin"), "管理员密码")
	)
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.Seed(); err != nil {
		log.Fatalf("seed: %v", err)
	}

	srv, err := api.New(st, api.Config{
		Secret:    *secret,
		AdminUser: *adminUser,
		AdminPass: *adminPass,
	})
	if err != nil {
		log.Fatalf("init api: %v", err)
	}

	r := srv.Router()
	if dist, err := web.Dist(); err == nil {
		serveStatic(r, dist)
	} else {
		log.Printf("frontend not embedded: %v", err)
	}

	log.Printf("微服务部署服务 center 监听 %s (db=%s)", *addr, *dbPath)
	if err := r.Run(*addr); err != nil {
		log.Fatal(err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// serveStatic 挂载内嵌前端，SPA 回退到 index.html。
func serveStatic(r *gin.Engine, dist fs.FS) {
	fileServer := http.FileServer(http.FS(dist))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
