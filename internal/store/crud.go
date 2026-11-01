package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/lieguofeng/dpApp/internal/model"
)

func nowStr() string { return time.Now().Format("2006-01-02 15:04:05") }

func encodeJSON(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func decodeJSON(s string) []string {
	var v []string
	if s == "" {
		return []string{}
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return []string{}
	}
	return v
}

// ---------- 机器 ----------

func (s *Store) ListMachines() ([]model.Machine, error) {
	rows, err := s.db.Query(`
		SELECT m.id,m.name,m.host,m.port,m.mode,m.ssh_user,m.status,m.created_at,
		       (SELECT COUNT(*) FROM instances i WHERE i.machine_id=m.id) AS apps
		FROM machines m ORDER BY m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Machine
	for rows.Next() {
		var m model.Machine
		if err := rows.Scan(&m.ID, &m.Name, &m.Host, &m.Port, &m.Mode, &m.SSHUser, &m.Status, &m.CreatedAt, &m.Apps); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) CreateMachine(m *model.Machine) error {
	res, err := s.db.Exec(`INSERT INTO machines(name,host,port,mode,ssh_user,status,created_at) VALUES(?,?,?,?,?,?,?)`,
		m.Name, m.Host, m.Port, m.Mode, m.SSHUser, m.Status, nowStr())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	m.ID = int(id)
	return nil
}

func (s *Store) UpdateMachine(m *model.Machine) error {
	_, err := s.db.Exec(`UPDATE machines SET name=?,host=?,port=?,mode=?,ssh_user=?,status=? WHERE id=?`,
		m.Name, m.Host, m.Port, m.Mode, m.SSHUser, m.Status, m.ID)
	return err
}

func (s *Store) DeleteMachine(id int) error {
	_, err := s.db.Exec(`DELETE FROM machines WHERE id=?`, id)
	return err
}

// ---------- 应用 ----------

func (s *Store) ListApplications() ([]model.Application, error) {
	rows, err := s.db.Query(`
		SELECT a.id,a.name,a.description,a.jar_dir,a.log_dir,a.bin_dir,a.cfg_dir,a.start_cmd,a.config_files,a.created_at,
		       (SELECT COUNT(*) FROM instances i WHERE i.application_id=a.id) AS instances,
		       (SELECT COUNT(*) FROM instances i WHERE i.application_id=a.id AND i.status='on') AS running,
		       (SELECT COUNT(*) FROM instances i WHERE i.application_id=a.id AND i.status='off') AS stopped
		FROM applications a ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Application
	for rows.Next() {
		var a model.Application
		var files string
		if err := rows.Scan(&a.ID, &a.Name, &a.Desc, &a.JarDir, &a.LogDir, &a.BinDir, &a.CfgDir, &a.StartCmd, &files, &a.CreatedAt,
			&a.Instances, &a.Running, &a.Stopped); err != nil {
			return nil, err
		}
		a.ConfigFiles = decodeJSON(files)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetApplicationByName(name string) (*model.Application, error) {
	apps, err := s.ListApplications()
	if err != nil {
		return nil, err
	}
	for i := range apps {
		if apps[i].Name == name {
			return &apps[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *Store) CreateApplication(a *model.Application) error {
	res, err := s.db.Exec(`INSERT INTO applications(name,description,jar_dir,log_dir,bin_dir,cfg_dir,start_cmd,config_files,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		a.Name, a.Desc, a.JarDir, a.LogDir, a.BinDir, a.CfgDir, a.StartCmd, encodeJSON(a.ConfigFiles), nowStr())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	a.ID = int(id)
	return nil
}

func (s *Store) UpdateApplication(a *model.Application) error {
	_, err := s.db.Exec(`UPDATE applications SET name=?,description=?,jar_dir=?,log_dir=?,bin_dir=?,cfg_dir=?,start_cmd=?,config_files=? WHERE id=?`,
		a.Name, a.Desc, a.JarDir, a.LogDir, a.BinDir, a.CfgDir, a.StartCmd, encodeJSON(a.ConfigFiles), a.ID)
	return err
}

func (s *Store) DeleteApplication(id int) error {
	// 一并删除其下实例与相关配置
	if _, err := s.db.Exec(`DELETE FROM instances WHERE application_id=?`, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM releases WHERE application_id=?`, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM db_configs WHERE application_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM applications WHERE id=?`, id)
	return err
}

// ---------- 实例 ----------

func (s *Store) ListInstances() ([]model.Instance, error) {
	return s.listInstances("")
}

func (s *Store) ListInstancesByApp(appName string) ([]model.Instance, error) {
	return s.listInstances("WHERE a.name = '" + strings.ReplaceAll(appName, "'", "''") + "'")
}

func (s *Store) listInstances(where string) ([]model.Instance, error) {
	query := `
		SELECT i.id,i.application_id,a.name,i.machine_id,m.name,i.jar,i.version,i.port,i.pid,i.status,i.started,i.created_at
		FROM instances i
		JOIN applications a ON a.id=i.application_id
		JOIN machines m ON m.id=i.machine_id ` + where + `
		ORDER BY i.id`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Instance
	for rows.Next() {
		var it model.Instance
		if err := rows.Scan(&it.ID, &it.AppID, &it.App, &it.MachineID, &it.Machine, &it.Jar, &it.Version, &it.Port, &it.PID, &it.Status, &it.Started, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) CreateInstance(it *model.Instance) error {
	res, err := s.db.Exec(`INSERT INTO instances(application_id,machine_id,jar,version,port,pid,status,started,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		it.AppID, it.MachineID, it.Jar, it.Version, it.Port, it.PID, it.Status, it.Started, nowStr())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	it.ID = int(id)
	return nil
}

func (s *Store) UpdateInstance(it *model.Instance) error {
	_, err := s.db.Exec(`UPDATE instances SET application_id=?,machine_id=?,jar=?,version=?,port=?,pid=?,status=?,started=? WHERE id=?`,
		it.AppID, it.MachineID, it.Jar, it.Version, it.Port, it.PID, it.Status, it.Started, it.ID)
	return err
}

func (s *Store) DeleteInstance(id int) error {
	_, err := s.db.Exec(`DELETE FROM instances WHERE id=?`, id)
	return err
}

// ---------- 发布历史 ----------

func (s *Store) ListReleasesByApp(appName string) ([]model.Release, error) {
	rows, err := s.db.Query(`
		SELECT r.id,r.application_id,a.name,r.version,r.mode,r.batch_size,r.status,r.is_current,r.operator,r.note,r.started_at,r.finished_at
		FROM releases r
		JOIN applications a ON a.id=r.application_id
		WHERE a.name = ?
		ORDER BY r.id DESC`, appName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Release
	for rows.Next() {
		var r model.Release
		var cur int
		if err := rows.Scan(&r.ID, &r.AppID, &r.App, &r.Version, &r.Mode, &r.BatchSize, &r.Status, &cur, &r.Operator, &r.Note, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		r.Current = cur == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------- 部署记录 ----------

func (s *Store) ListRecentDeployments(limit int) ([]model.Deployment, error) {
	rows, err := s.db.Query(`
		SELECT d.id,d.instance_id,d.release_id,d.app,d.version,d.operator,d.status,d.note,d.started_at,d.finished_at
		FROM deployments d ORDER BY d.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Deployment
	for rows.Next() {
		var d model.Deployment
		var relID sql.NullInt64
		if err := rows.Scan(&d.ID, &d.InstanceID, &relID, &d.App, &d.Version, &d.Operator, &d.Status, &d.Note, &d.StartedAt, &d.FinishedAt); err != nil {
			return nil, err
		}
		if relID.Valid {
			v := int(relID.Int64)
			d.ReleaseID = &v
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---------- 数据库主备 ----------

func (s *Store) ListDbConfigs() ([]model.DbConfig, error) {
	rows, err := s.db.Query(`
		SELECT d.id,d.application_id,a.name,d.primary_url,d.backup_url,d.current
		FROM db_configs d
		JOIN applications a ON a.id=d.application_id
		ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.DbConfig
	for rows.Next() {
		var d model.DbConfig
		if err := rows.Scan(&d.ID, &d.AppID, &d.App, &d.Primary, &d.Backup, &d.Current); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---------- 总览统计 ----------

func (s *Store) Dashboard() (*model.Dashboard, error) {
	d := &model.Dashboard{}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM machines`).Scan(&d.Machines); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&d.Apps); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM instances WHERE status='on'`).Scan(&d.Running); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM instances WHERE status='off'`).Scan(&d.Stopped); err != nil {
		return nil, err
	}
	recent, err := s.ListRecentDeployments(10)
	if err != nil {
		return nil, err
	}
	d.RecentDeployments = recent
	return d, nil
}
