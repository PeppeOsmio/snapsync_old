package snapshots

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"peppeosmio/snapsync/configs"
	"peppeosmio/snapsync/structs"
	"peppeosmio/snapsync/utils"
	"regexp"
	"slices"
	"strconv"
	"time"
)

func getRsyncDirsCommand(config *structs.Config, srcDir string, dstDir string, excludes []string) string {
	rsyncExecutable := "rsync"
	if len(config.RSyncPath) > 0 {
		rsyncExecutable = config.RSyncPath
	}
	excludesString := ""
	if len(excludes) > 0 {
		for _, exclude := range excludes {
			excludesString += fmt.Sprintf("%s ", exclude)
		}
	}
	return fmt.Sprintf("%s -avrhLK --delete --exclude \"%s\" %s/ %s", rsyncExecutable, excludesString, srcDir, dstDir)
}

func GetSnapshotDirPrefix(snapshotName string, interval string) string {
	return snapshotName + "." + interval + "."
}

func GetSnapshotDirName(snapshotName string, interval string, number int) string {
	return GetSnapshotDirPrefix(snapshotName, interval) + strconv.Itoa(number)
}

func executeOnlySnapshot(config *structs.Config, snapshotConfig *structs.SnapshotConfig) error {
	snapshotLogPrefix := fmt.Sprintf("[%s]", snapshotConfig.SnapshotName)
	before := time.Now().UnixMilli()
	newestSnapshotPath := path.Join(snapshotConfig.SnapshotsDir, GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, 0))
	slog.Debug(snapshotLogPrefix + "Checking if " + newestSnapshotPath + " exists")
	err := os.MkdirAll(snapshotConfig.SnapshotsDir, 0700)
	if err != nil {
		return fmt.Errorf("%s can't create snapshot dir %s: %s", snapshotLogPrefix, snapshotConfig.SnapshotsDir, err.Error())
	}
	tmpDir, mkdirErr := os.MkdirTemp(snapshotConfig.SnapshotsDir, "tmp")
	// in case of errors be sure to remove the tmp directory to avoid creating junk
	defer os.RemoveAll(tmpDir)
	if mkdirErr != nil {
		return fmt.Errorf("%s can't create tmp dir %s: %s", snapshotLogPrefix, tmpDir, mkdirErr.Error())
	}
	_, err = os.Stat(newestSnapshotPath)
	// if the snapshot 0 already exists, copy it with hard links into the tmp dir
	if err == nil {
		slog.Debug(snapshotLogPrefix + "Copying latest snapshot...")
		cpCommand := fmt.Sprintf("%s -lra %s/./ %s", config.CpPath, newestSnapshotPath, tmpDir)
		_, cpErr := exec.Command("sh", "-c", cpCommand).Output()
		if cpErr != nil {
			return fmt.Errorf("%s error copying last snapshot %s to %s: %s", snapshotLogPrefix, newestSnapshotPath, tmpDir, cpErr.Error())
		}
	} else if os.IsNotExist(err) {
		slog.Debug(snapshotLogPrefix + "Creating first snapshot " + newestSnapshotPath)
	} else {
		return fmt.Errorf("%s can't stat %s: %s", newestSnapshotPath, snapshotLogPrefix, err.Error())
	}
	now := time.Now()
	os.Chtimes(tmpDir, now, now)

	for _, dirToSnapshot := range snapshotConfig.Dirs {
		_, err = os.Stat(dirToSnapshot.SrcDirAbspath)
		if os.IsNotExist(err) {
			slog.Warn(snapshotLogPrefix + "Source directory " + dirToSnapshot.SrcDirAbspath + " does not exist.")
			continue
		}
		dstDirFull := path.Join(tmpDir, dirToSnapshot.DstDirInSnapshot)
		_, err = os.Stat(dstDirFull)
		if os.IsNotExist(err) {
			err = os.MkdirAll(dstDirFull, 0700)
			if err != nil {
				return fmt.Errorf("%s can't create destination dir %s", snapshotLogPrefix, dstDirFull)
			}
		}
		rsyncCommand := getRsyncDirsCommand(config, dirToSnapshot.SrcDirAbspath, dstDirFull, dirToSnapshot.Excludes)
		slog.Debug(snapshotLogPrefix + "Synching dir " + dirToSnapshot.SrcDirAbspath + "/ to " + dstDirFull)
		slog.Debug(fmt.Sprintf("Running %s", rsyncCommand))
		_, err := exec.Command("sh", "-c", rsyncCommand).Output()
		if err != nil {
			return fmt.Errorf("%s can't sync %s/ to %s: %s", snapshotLogPrefix, dirToSnapshot.SrcDirAbspath, dstDirFull, err.Error())
		}
	}

	// rename all the snapshots
	snapshotsNumbers := []int{}
	snapshots, err := os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		fmt.Println(snapshotLogPrefix + "Can't read directory " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
	snapshotPrefixWithNumberRegex, err := regexp.Compile("^" + regexp.QuoteMeta(GetSnapshotDirPrefix(snapshotConfig.SnapshotName, snapshotConfig.Interval)) + `([0-9]+)$`)
	if err != nil {
		fmt.Println(snapshotLogPrefix + "Error compiling regex: " + err.Error())
		return err
	}
	for _, snapshot := range snapshots {
		match := snapshotPrefixWithNumberRegex.FindStringSubmatch(snapshot.Name())
		if match != nil {
			number, err := strconv.Atoi(match[1]) // match[1] contains the first capturing group
			if err != nil {
				fmt.Println(snapshotLogPrefix + "Error converting string to int: " + err.Error())
				return err
			}
			snapshotsNumbers = append(snapshotsNumbers, number)
		}
	}
	slices.Sort(snapshotsNumbers)
	slices.Reverse(snapshotsNumbers)
	for _, number := range snapshotsNumbers {
		snapshotOldName := GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, number)
		snapshotOldPath := path.Join(snapshotConfig.SnapshotsDir, snapshotOldName)
		snapshotRenamedName := GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, number+1)
		snapshotRenamedPath := path.Join(snapshotConfig.SnapshotsDir, snapshotRenamedName)
		err = os.Rename(snapshotOldPath, snapshotRenamedPath)
		if err != nil {
			fmt.Println(snapshotLogPrefix + "Can't move " + snapshotOldPath + " to " + snapshotRenamedPath + ": " + err.Error())
			return err
		}
	}

	// rename the temporary folder to be the newest snapshot
	err = os.Rename(tmpDir, newestSnapshotPath)
	if err != nil {
		fmt.Println(snapshotLogPrefix + "Can't rename tmp folder " + tmpDir + " to " + newestSnapshotPath + ": " + err.Error())
	}

	// delete the excess amount of snapshots
	snapshots, err = os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		fmt.Println(snapshotLogPrefix + "Can't read directory " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
	for _, snapshotEntry := range snapshots {
		snapshotInfo, err := utils.GetInfoFromSnapshotPath(snapshotEntry.Name())
		if err != nil {
			return err
		}
		if snapshotInfo.Number >= snapshotConfig.Retention {
			snapshotToRemoveName := GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, snapshotInfo.Number)
			snapshotToRemovePath := path.Join(snapshotConfig.SnapshotsDir, snapshotToRemoveName)
			err = os.RemoveAll(snapshotToRemovePath)
			if err != nil {
				fmt.Println("Can't remove snapshot " + snapshotToRemovePath + ": " + err.Error())
			}
		}
	}

	after := time.Now().UnixMilli()
	seconds := float64(after-before) / 1000
	slog.Info(fmt.Sprintf("%s snapshots done in %.2f s", snapshotLogPrefix, seconds))
	return nil
}

func ExecuteSnapshot(config *structs.Config, snapshotConfig *structs.SnapshotConfig) error {
	snapshotLogPrefix := fmt.Sprintf("[%s]", snapshotConfig.SnapshotName)
	before := time.Now().UnixMilli()
	if len(snapshotConfig.PreSnapshotCommands) > 0 {
		slog.Info(fmt.Sprintf("%s executing pre snapshot commands", snapshotLogPrefix))
		for _, command := range snapshotConfig.PreSnapshotCommands {
			slog.Info(snapshotLogPrefix + " " + command)
			result, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				fmt.Println(snapshotLogPrefix + command + ": " + err.Error())
				return err
			}
			if len(result) > 0 {
				slog.Info(snapshotLogPrefix + command + ": " + string(result))
			}
		}
		after := time.Now().UnixMilli()
		seconds := float64(after-before) / 1000
		slog.Info(fmt.Sprintf("%s pre snapshot commands done in %.2f s", snapshotLogPrefix, seconds))
	} else {
		slog.Info(fmt.Sprintf("%s no pre snapshot commands to run", snapshotLogPrefix))
	}

	err := executeOnlySnapshot(config, snapshotConfig)
	if err != nil && !snapshotConfig.AlwaysRunPostSnapshotCommands {
		return err
	}

	if len(snapshotConfig.PostSnapshotCommands) > 0 {
		slog.Info(fmt.Sprintf("%s executing post snapshot commands", snapshotLogPrefix))
		for _, command := range snapshotConfig.PostSnapshotCommands {
			slog.Info(fmt.Sprintf("%s %s", snapshotLogPrefix, command))
			result, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				return fmt.Errorf("%s %s: %s", snapshotLogPrefix, command, err.Error())
			}
			if len(result) > 0 {
				slog.Info(snapshotLogPrefix + command + ": " + string(result))
			}
		}
		after := time.Now().UnixMilli()
		seconds := float64(after-before) / 1000
		slog.Info(fmt.Sprintf("%s post snapshot commands done in %.2f s", snapshotLogPrefix, seconds))
	} else {
		slog.Info(fmt.Sprintf("%s no pre snapshot commands to run", snapshotLogPrefix))
	}

	now := time.Now()
	os.Chtimes(GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, 0), now, now)
	return nil
}

func GetSnapshotsInfo(configsDir string, expandVars bool, snapshotName string) (snapshotsInfo []*structs.SnapshotInfo, err error) {
	snapshotConfig, err := configs.GetSnapshotConfigByName(configsDir, expandVars, snapshotName)
	if err != nil {
		fmt.Println("Can't list snapshots of " + snapshotName + ": " + err.Error())
		return snapshotsInfo, err
	}
	if snapshotConfig == nil {
		slog.Warn("Snapshot template " + snapshotName + " does not exist.")
		return snapshotsInfo, nil
	}
	_, err = os.Stat(snapshotConfig.SnapshotsDir)
	if os.IsNotExist(err) {
		slog.Info("No snapshots found for " + snapshotName)
		return snapshotsInfo, nil
	}
	snapshotsDirsEntries, err := os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		fmt.Println("Can't list snapshot of " + snapshotName + ": " + err.Error())
		return snapshotsInfo, err
	}
	for _, entry := range snapshotsDirsEntries {
		snapshotFullPath := path.Join(snapshotConfig.SnapshotsDir, entry.Name())
		_, err := os.Stat(snapshotFullPath)
		if err != nil {
			fmt.Println("Can't stat " + snapshotFullPath + ": " + err.Error())
			return snapshotsInfo, err
		}
		snapshotInfo, err := utils.GetInfoFromSnapshotPath(path.Join(snapshotConfig.SnapshotsDir, entry.Name()))
		if err != nil {
			return snapshotsInfo, fmt.Errorf("can't parse snapshot name: %s", err.Error())
		}
		snapshotsInfo = append(snapshotsInfo, snapshotInfo)
	}
	return snapshotsInfo, nil
}

func RestoreSnapshot(config *structs.Config, snapshotInfo *structs.SnapshotInfo, snapshotConfig *structs.SnapshotConfig) (err error) {
	snapshotLogPrefix := fmt.Sprintf("[%s]", snapshotConfig.SnapshotName)
	for _, dir := range snapshotConfig.Dirs {
		err = os.MkdirAll(dir.SrcDirAbspath, 0700)
		if err != nil {
			fmt.Println("Can't create directory " + dir.SrcDirAbspath + ": " + err.Error())
			return err
		}
		snapshottedDirPath := path.Join(snapshotInfo.Abspath, dir.DstDirInSnapshot)
		rsyncCommand := getRsyncDirsCommand(config, snapshottedDirPath, dir.SrcDirAbspath, nil)
		slog.Debug(rsyncCommand)
		_, err = exec.Command("sh", "-c", rsyncCommand).Output()
		if err != nil {
			fmt.Println(snapshotLogPrefix + "Can't sync " + snapshottedDirPath + "/ to " + dir.SrcDirAbspath + ": " + err.Error())
		}
	}
	return err
}
