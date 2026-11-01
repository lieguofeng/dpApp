package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/lieguofeng/dpApp/internal/auth"
	"github.com/lieguofeng/dpApp/internal/model"
)

func ok(c *gin.Context, v interface{}) { c.JSON(http.StatusOK, v) }

func fail(c *gin.Context, code int, msg string) { c.JSON(code, gin.H{"error": msg}) }

func idParam(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的 ID")
		return 0, false
	}
	return id, true
}

// ---------- 鉴权 ----------

func (s *Server) login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.Username != s.cfg.AdminUser || !auth.CheckPassword(s.adminHash, req.Password) {
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	token, err := auth.GenerateToken(s.cfg.Secret, req.Username, s.cfg.TokenTTL)
	if err != nil {
		fail(c, http.StatusInternalServerError, "签发失败")
		return
	}
	ok(c, gin.H{"token": token, "user": req.Username})
}

func (s *Server) logout(c *gin.Context) { ok(c, gin.H{"ok": true}) }

func (s *Server) me(c *gin.Context) {
	user, _ := c.Get("user")
	ok(c, gin.H{"user": user})
}

// ---------- 总览 ----------

func (s *Server) dashboard(c *gin.Context) {
	d, err := s.store.Dashboard()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, d)
}

// ---------- 机器 ----------

func (s *Server) listMachines(c *gin.Context) {
	items, err := s.store.ListMachines()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, items)
}

func (s *Server) createMachine(c *gin.Context) {
	var m model.Machine
	if err := c.ShouldBindJSON(&m); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if m.Name == "" || m.Host == "" {
		fail(c, http.StatusBadRequest, "名称与主机必填")
		return
	}
	if m.Port == 0 {
		m.Port = 22
	}
	if m.Mode == "" {
		m.Mode = "ssh"
	}
	if m.Status == "" {
		m.Status = "on"
	}
	if err := s.store.CreateMachine(&m); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, m)
}

func (s *Server) updateMachine(c *gin.Context) {
	id, okID := idParam(c)
	if !okID {
		return
	}
	var m model.Machine
	if err := c.ShouldBindJSON(&m); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	m.ID = id
	if err := s.store.UpdateMachine(&m); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, m)
}

func (s *Server) deleteMachine(c *gin.Context) {
	id, okID := idParam(c)
	if !okID {
		return
	}
	if err := s.store.DeleteMachine(id); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

func (s *Server) testMachine(c *gin.Context) {
	id, okID := idParam(c)
	if !okID {
		return
	}
	// M1 占位：真实连接测试在 M2 经 Connector 实现
	ok(c, gin.H{"ok": true, "message": "连接正常（占位）", "id": id})
}

// ---------- 应用 ----------

func (s *Server) listApplications(c *gin.Context) {
	items, err := s.store.ListApplications()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, items)
}

func (s *Server) createApplication(c *gin.Context) {
	var a model.Application
	if err := c.ShouldBindJSON(&a); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if a.Name == "" {
		fail(c, http.StatusBadRequest, "应用名必填")
		return
	}
	applyAppDefaults(&a)
	if err := s.store.CreateApplication(&a); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			fail(c, http.StatusConflict, "应用名已存在")
			return
		}
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, a)
}

func (s *Server) updateApplication(c *gin.Context) {
	id, okID := idParam(c)
	if !okID {
		return
	}
	var a model.Application
	if err := c.ShouldBindJSON(&a); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	a.ID = id
	applyAppDefaults(&a)
	if err := s.store.UpdateApplication(&a); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, a)
}

func (s *Server) deleteApplication(c *gin.Context) {
	id, okID := idParam(c)
	if !okID {
		return
	}
	if err := s.store.DeleteApplication(id); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

// applyAppDefaults 目录/命令留空时按 /opt/{应用名} 自动推导。
func applyAppDefaults(a *model.Application) {
	if a.JarDir == "" {
		a.JarDir = "/opt/" + a.Name
	}
	if a.LogDir == "" {
		a.LogDir = a.JarDir + "/logs"
	}
	if a.BinDir == "" {
		a.BinDir = a.JarDir + "/bin"
	}
	if a.CfgDir == "" {
		a.CfgDir = a.JarDir + "/config"
	}
	if a.StartCmd == "" {
		a.StartCmd = "java -jar " + a.JarDir + "/" + a.Name + ".jar"
	}
	if a.ConfigFiles == nil {
		a.ConfigFiles = []string{"application.yml", "log4j2.xml"}
	}
}

// ---------- 发布历史 ----------

func (s *Server) listReleases(c *gin.Context) {
	app := c.Query("app")
	if app == "" {
		ok(c, []model.Release{})
		return
	}
	items, err := s.store.ListReleasesByApp(app)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, items)
}

// ---------- 实例 ----------

func (s *Server) listInstances(c *gin.Context) {
	app := c.Query("app")
	var (
		items []model.Instance
		err   error
	)
	if app != "" {
		items, err = s.store.ListInstancesByApp(app)
	} else {
		items, err = s.store.ListInstances()
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, items)
}

func (s *Server) createInstance(c *gin.Context) {
	var it model.Instance
	if err := c.ShouldBindJSON(&it); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if it.Status == "" {
		it.Status = "off"
	}
	if err := s.store.CreateInstance(&it); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, it)
}

func (s *Server) updateInstance(c *gin.Context) {
	id, okID := idParam(c)
	if !okID {
		return
	}
	var it model.Instance
	if err := c.ShouldBindJSON(&it); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	it.ID = id
	if err := s.store.UpdateInstance(&it); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, it)
}

func (s *Server) deleteInstance(c *gin.Context) {
	id, okID := idParam(c)
	if !okID {
		return
	}
	if err := s.store.DeleteInstance(id); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

// ---------- 数据库主备 ----------

func (s *Server) listDbConfigs(c *gin.Context) {
	items, err := s.store.ListDbConfigs()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, items)
}
