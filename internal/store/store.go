package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 数据访问。
type Store struct {
	db *sql.DB
}

// Open 打开（不存在则创建）SQLite 数据库并执行建表迁移。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite 单写连接，避免并发写锁。
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层连接（供扩展使用）。
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate() error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS machines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 22,
			mode TEXT NOT NULL DEFAULT 'ssh',
			ssh_user TEXT DEFAULT '',
			ssh_secret_enc TEXT DEFAULT '',
			ssh_key_enc TEXT DEFAULT '',
			agent_url TEXT DEFAULT '',
			agent_token_enc TEXT DEFAULT '',
			status TEXT DEFAULT 'on',
			created_at TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS applications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			description TEXT DEFAULT '',
			jar_dir TEXT DEFAULT '',
			log_dir TEXT DEFAULT '',
			bin_dir TEXT DEFAULT '',
			cfg_dir TEXT DEFAULT '',
			start_cmd TEXT DEFAULT '',
			config_files TEXT DEFAULT '[]',
			created_at TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS instances (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			application_id INTEGER NOT NULL,
			machine_id INTEGER NOT NULL,
			jar TEXT DEFAULT '',
			version TEXT DEFAULT '',
			port INTEGER DEFAULT 0,
			pid INTEGER DEFAULT 0,
			status TEXT DEFAULT 'off',
			started TEXT DEFAULT '',
			created_at TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS releases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			application_id INTEGER NOT NULL,
			version TEXT DEFAULT '',
			jar_sha256 TEXT DEFAULT '',
			operator TEXT DEFAULT '',
			mode TEXT DEFAULT 'rolling',
			batch_size INTEGER DEFAULT 1,
			status TEXT DEFAULT 'success',
			is_current INTEGER DEFAULT 0,
			current_batch INTEGER DEFAULT 0,
			note TEXT DEFAULT '',
			started_at TEXT DEFAULT '',
			finished_at TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS deployments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			instance_id INTEGER NOT NULL,
			release_id INTEGER,
			app TEXT DEFAULT '',
			version TEXT DEFAULT '',
			jar_sha256 TEXT DEFAULT '',
			operator TEXT DEFAULT '',
			status TEXT DEFAULT 'success',
			note TEXT DEFAULT '',
			started_at TEXT DEFAULT '',
			finished_at TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS release_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			release_id INTEGER NOT NULL,
			instance_id INTEGER NOT NULL,
			batch_no INTEGER DEFAULT 0,
			status TEXT DEFAULT 'pending',
			old_jar TEXT DEFAULT '',
			error TEXT DEFAULT '',
			finished_at TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS config_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			instance_id INTEGER,
			source TEXT DEFAULT 'local',
			file_path TEXT DEFAULT '',
			old_content TEXT DEFAULT '',
			new_content TEXT DEFAULT '',
			operator TEXT DEFAULT '',
			created_at TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS db_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			application_id INTEGER NOT NULL,
			primary_url TEXT DEFAULT '',
			backup_url TEXT DEFAULT '',
			current TEXT DEFAULT 'primary'
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT DEFAULT ''
		)`,
	}
	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// Seed 首次启动时（表为空）写入演示数据，便于与原型对照联调。
func (s *Store) Seed() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM machines`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04")

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 机器
	machines := []struct {
		name, host string
		port       int
		mode       string
		status     string
	}{
		{"prod-web-01", "192.168.1.11", 22, "ssh", "on"},
		{"prod-web-02", "192.168.1.12", 22, "ssh", "on"},
		{"prod-app-01", "192.168.2.21", 8848, "agent", "on"},
		{"test-01", "192.168.3.31", 8848, "agent", "err"},
	}
	machineIDs := make([]int, len(machines))
	for i, m := range machines {
		res, err := tx.Exec(`INSERT INTO machines(name,host,port,mode,status,created_at) VALUES(?,?,?,?,?,?)`,
			m.name, m.host, m.port, m.mode, m.status, now)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		machineIDs[i] = int(id)
	}

	// 应用
	apps := []struct {
		name, desc, jarDir, logDir, binDir, cfgDir, startCmd string
		files                                               string
	}{
		{"nsys", "订单核心服务", "/opt/nsys", "/opt/nsys/logs", "/opt/nsys/bin", "/opt/nsys/config",
			"java -Xms1024m -Xmx2048m -XX:+UseG1GC -jar /opt/nsys/nsys.jar --spring.profiles.active=prod",
			`["application.yml","application-dev.yml","log4j2.xml"]`},
		{"gateway", "API 网关", "/opt/gateway", "/opt/gateway/logs", "/opt/gateway/bin", "/opt/gateway/config",
			"java -Xms512m -Xmx1024m -jar /opt/gateway/gateway.jar",
			`["application.yml","log4j2.xml"]`},
		{"user-service", "用户服务", "/opt/user-service", "/opt/user-service/logs", "/opt/user-service/bin", "/opt/user-service/config",
			"java -Xms512m -Xmx1024m -jar /opt/user-service/user.jar",
			`["application.properties","log4j2.xml"]`},
		{"pay-service", "支付服务", "/opt/pay-service", "/opt/pay-service/logs", "/opt/pay-service/bin", "/opt/pay-service/config",
			"java -Xms1024m -Xmx2048m -jar /opt/pay-service/pay.jar",
			`["application.yml","log4j2.xml"]`},
	}
	appIDs := make([]int, len(apps))
	for i, a := range apps {
		res, err := tx.Exec(`INSERT INTO applications(name,description,jar_dir,log_dir,bin_dir,cfg_dir,start_cmd,config_files,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			a.name, a.desc, a.jarDir, a.logDir, a.binDir, a.cfgDir, a.startCmd, a.files, now)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		appIDs[i] = int(id)
	}

	// 实例（app 索引, 机器 索引, jar, ver, port, pid, status）
	insts := []struct {
		app, m  int
		jar, ver string
		port, pid int
		status, started string
	}{
		{0, 0, "nsys-1.4.2.jar", "1.4.2", 8081, 12001, "on", "2026-10-31 16:42"},
		{0, 1, "nsys-1.4.2.jar", "1.4.2", 8081, 12002, "on", "2026-10-31 16:43"},
		{0, 2, "nsys-1.4.2.jar", "1.4.2", 8081, 12003, "on", "2026-10-31 16:45"},
		{1, 0, "gateway-2.1.0.jar", "2.1.0", 8080, 11001, "on", "2026-10-30 09:12"},
		{1, 1, "gateway-2.1.0.jar", "2.1.0", 8080, 11002, "on", "2026-10-30 09:13"},
		{2, 2, "user-3.0.5.jar", "3.0.5", 8090, 13001, "on", "2026-10-31 10:00"},
		{2, 3, "user-3.0.5.jar", "3.0.5", 8090, 0, "off", ""},
		{3, 0, "pay-1.8.0.jar", "1.8.0", 8082, 14001, "on", "2026-10-29 18:30"},
		{3, 1, "pay-1.8.0.jar", "1.8.0", 8082, 14002, "on", "2026-10-29 18:31"},
	}
	for _, it := range insts {
		if _, err := tx.Exec(`INSERT INTO instances(application_id,machine_id,jar,version,port,pid,status,started,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			appIDs[it.app], machineIDs[it.m], it.jar, it.ver, it.port, it.pid, it.status, it.started, now); err != nil {
			return err
		}
	}

	// 发布历史
	rels := []struct {
		app  int
		ver  string
		cur  bool
		note string
		t    string
	}{
		{0, "1.4.2", true, "滚动发布", "2026-10-31 16:42"},
		{0, "1.4.1", false, "滚动发布", "2026-10-28 14:20"},
		{0, "1.4.0", false, "初始发布", "2026-10-20 09:00"},
		{1, "2.1.0", true, "滚动发布", "2026-10-30 09:12"},
		{1, "2.0.9", false, "滚动发布", "2026-10-21 11:30"},
		{2, "3.0.5", true, "滚动发布", "2026-10-30 08:47"},
		{2, "3.0.4", false, "滚动发布", "2026-10-25 15:10"},
		{3, "1.8.0", true, "滚动发布", "2026-10-29 18:30"},
		{3, "1.7.9", false, "滚动发布", "2026-10-22 10:00"},
	}
	for _, r := range rels {
		cur := 0
		if r.cur {
			cur = 1
		}
		if _, err := tx.Exec(`INSERT INTO releases(application_id,version,operator,mode,batch_size,status,is_current,note,started_at,finished_at) VALUES(?,?,'admin','rolling',2,'success',?,?,?,?)`,
			appIDs[r.app], r.ver, cur, r.note, r.t, r.t); err != nil {
			return err
		}
	}

	// 最近部署记录（供总览展示）
	deps := []struct {
		app, ver, res string
		t             string
	}{
		{"nsys", "1.4.2", "success", "2026-10-31 16:42"},
		{"gateway", "2.1.0", "success", "2026-10-30 09:12"},
		{"user-service", "3.0.5", "success", "2026-10-30 08:47"},
		{"nsys", "1.4.1", "failed", "2026-10-28 14:20"},
	}
	for _, d := range deps {
		if _, err := tx.Exec(`INSERT INTO deployments(instance_id,app,version,operator,status,note,started_at,finished_at) VALUES(0,?,?,'admin',?,'',?,?)`,
			d.app, d.ver, d.res, d.t, d.t); err != nil {
			return err
		}
	}

	// 数据库主备配置
	dbs := []struct {
		app  int
		prim, bak, cur string
	}{
		{0, "10.0.0.8:3306/nsys", "10.0.0.9:3306/nsys", "primary"},
		{1, "10.0.0.8:3306/gateway", "10.0.0.9:3306/gateway", "primary"},
		{2, "10.0.0.8:3306/user", "10.0.0.9:3306/user", "backup"},
		{3, "10.0.0.8:3306/pay", "10.0.0.9:3306/pay", "primary"},
	}
	for _, d := range dbs {
		if _, err := tx.Exec(`INSERT INTO db_configs(application_id,primary_url,backup_url,current) VALUES(?,?,?,?)`,
			appIDs[d.app], d.prim, d.bak, d.cur); err != nil {
			return err
		}
	}

	return tx.Commit()
}
