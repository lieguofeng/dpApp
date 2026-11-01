package model

// Machine 目标服务器
type Machine struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Mode      string `json:"mode"` // ssh | agent
	SSHUser   string `json:"ssh_user,omitempty"`
	// 凭据加密存储，不直接暴露到 JSON
	Status    string `json:"status"` // on | err
	Apps      int    `json:"apps"`   // 该机器上的实例数（列表时计算）
	CreatedAt string `json:"created_at"`
}

// Application 微服务应用
type Application struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Desc        string   `json:"desc"`
	JarDir      string   `json:"jar_dir"`
	LogDir      string   `json:"log_dir"`
	BinDir      string   `json:"bin_dir"`
	CfgDir      string   `json:"cfg_dir"`
	StartCmd    string   `json:"start_cmd"`
	ConfigFiles []string `json:"config_files"`
	Instances   int      `json:"instances"` // 列表时计算
	Running     int      `json:"running"`   // 列表时计算
	Stopped     int      `json:"stopped"`   // 列表时计算
	CreatedAt   string   `json:"created_at"`
}

// Instance 应用在某台机器上的一个部署
type Instance struct {
	ID        int    `json:"id"`
	AppID     int    `json:"app_id"`
	App       string `json:"app"`
	MachineID int    `json:"machine_id"`
	Machine   string `json:"machine"`
	Jar       string `json:"jar"`
	Version   string `json:"version"`
	Port      int    `json:"port"`
	PID       int    `json:"pid"`
	Status    string `json:"status"` // on | off | err
	Started   string `json:"started"`
	CreatedAt string `json:"created_at"`
}

// Release 发布记录（发布历史 / 滚动发布编排）
type Release struct {
	ID         int    `json:"id"`
	AppID      int    `json:"app_id"`
	App        string `json:"app"`
	Version    string `json:"version"`
	Mode       string `json:"mode"` // single | rolling
	BatchSize  int    `json:"batch_size"`
	Status     string `json:"status"`
	Current    bool   `json:"current"`
	Operator   string `json:"operator"`
	Note       string `json:"note"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// Deployment 单实例部署记录
type Deployment struct {
	ID         int    `json:"id"`
	InstanceID int    `json:"instance_id"`
	ReleaseID  *int   `json:"release_id"`
	App        string `json:"app"`
	Version    string `json:"version"`
	Operator   string `json:"operator"`
	Status     string `json:"status"` // success | failed | rolledback
	Note       string `json:"note"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// DbConfig 数据库主备配置
type DbConfig struct {
	ID      int    `json:"id"`
	AppID   int    `json:"app_id"`
	App     string `json:"app"`
	Primary string `json:"primary"`
	Backup  string `json:"backup"`
	Current string `json:"current"` // primary | backup
}

// Dashboard 总览统计
type Dashboard struct {
	Machines          int          `json:"machines"`
	Apps              int          `json:"apps"`
	Running           int          `json:"running"`
	Stopped           int          `json:"stopped"`
	RecentDeployments []Deployment `json:"recent_deployments"`
}
