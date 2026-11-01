package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lieguofeng/dpApp/internal/auth"
	"github.com/lieguofeng/dpApp/internal/store"
)

// Config API 运行配置。
type Config struct {
	Secret    string        // JWT 签名密钥
	AdminUser string        // 默认管理员用户名
	AdminPass string        // 默认管理员密码（首次启动写入哈希）
	TokenTTL  time.Duration // 登录 token 有效期
}

// Server 持有依赖并实现全部 HTTP handler。
type Server struct {
	store     *store.Store
	cfg       Config
	adminHash string
}

// New 初始化 Server，确保管理员密码哈希已落库。
func New(st *store.Store, cfg Config) (*Server, error) {
	s := &Server{store: st, cfg: cfg}
	if cfg.Secret == "" {
		cfg.Secret = "dpapp-dev-secret-change-me"
	}
	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	if cfg.AdminPass == "" {
		cfg.AdminPass = "admin"
	}
	if cfg.TokenTTL == 0 {
		cfg.TokenTTL = 24 * time.Hour
	}
	s.cfg = cfg

	hash, err := st.GetSetting("admin_password_hash")
	if err != nil || hash == "" {
		h, err := auth.HashPassword(cfg.AdminPass)
		if err != nil {
			return nil, err
		}
		if err := st.SetSetting("admin_password_hash", h); err != nil {
			return nil, err
		}
		s.adminHash = h
	} else {
		s.adminHash = hash
	}
	return s, nil
}

// Router 构建并返回 Gin 路由。
func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), cors())

	r.GET("/api/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	api := r.Group("/api")
	api.POST("/login", s.login)

	authd := api.Group("")
	authd.Use(auth.Middleware(s.cfg.Secret))
	{
		authd.POST("/logout", s.logout)
		authd.GET("/me", s.me)

		authd.GET("/dashboard", s.dashboard)

		authd.GET("/machines", s.listMachines)
		authd.POST("/machines", s.createMachine)
		authd.PUT("/machines/:id", s.updateMachine)
		authd.DELETE("/machines/:id", s.deleteMachine)
		authd.POST("/machines/:id/test", s.testMachine)

		authd.GET("/applications", s.listApplications)
		authd.POST("/applications", s.createApplication)
		authd.PUT("/applications/:id", s.updateApplication)
		authd.DELETE("/applications/:id", s.deleteApplication)

		authd.GET("/releases", s.listReleases)

		authd.GET("/instances", s.listInstances)
		authd.POST("/instances", s.createInstance)
		authd.PUT("/instances/:id", s.updateInstance)
		authd.DELETE("/instances/:id", s.deleteInstance)

		authd.GET("/db-configs", s.listDbConfigs)
	}
	return r
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
