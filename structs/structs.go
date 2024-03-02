package structs

type Config struct {
	LogLevel  string `yaml:"log_level"`
	CpPath    string `yaml:"cp_path"`
	RSyncPath string `yaml:"rsync_path"`
}

type SnapshotConfig struct {
	SnapshotName                  string        `yaml:"snapshot_name"`
	Dirs                          []SnapshotDir `yaml:"dirs"`
	SnapshotsDir                  string        `yaml:"snapshots_dir"`
	Interval                      string        `yaml:"interval"`
	Retention                     int           `yaml:"retention"`
	Cron                          string        `yaml:"cron"`
	AlwaysRunPostSnapshotCommands bool          `yaml:"always_run_post_snapshot_commands"`
	PreSnapshotCommands           []string      `yaml:"pre_snapshot_commands"`
	PostSnapshotCommands          []string      `yaml:"post_snapshot_commands"`
}

type SnapshotDir struct {
	SrcDir   string   `yaml:"src_dir"`
	Excludes []string `yaml:"excludes"`
}
