package utils

import (
	"fmt"
	"path"
	"peppeosmio/snapsync/structs"
	"strconv"
	"strings"
)

func HumanReadableSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func GetInfoFromSnapshotPath(snapshotPath string) (snapshotInfo *structs.SnapshotInfo, err error) {
	snapshotDirName := strings.TrimSuffix(path.Base(snapshotPath), "/")
	items := strings.Split(snapshotDirName, ".")
	if len(items) != 3 {
		return nil, fmt.Errorf("snapshot name must be in format <name>.<interval>.<number>")
	}
	name := items[0]
	interval := items[1]
	number, err := strconv.Atoi(items[2])
	if err != nil {
		return nil, fmt.Errorf("can't parse snapshot number: %s", snapshotPath)
	}
	return &structs.SnapshotInfo{
		Abspath:      snapshotPath,
		SnapshotName: name,
		Interval:     interval,
		Number:       number,
	}, nil
}
