package structs

type Config struct {
	LogLevel  string
	CpPath    string
	RSyncPath string
}

type SnapshotConfig struct {
	SnapshotName         string
	Dirs                 []SnapshotDir
	SnapshotsDir         string
	Interval             string
	Retention            int
	Cron                 string
	PreSnapshotCommands  []string
	PostSnapshotCommands []string
}

type SnapshotDir struct {
	SrcDir   string
	Excludes []string
	DstDir   string
}
