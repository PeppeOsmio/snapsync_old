package snapshots

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"peppeosmio/snapsync/configs"
	"peppeosmio/snapsync/structs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func getRsyncCommand(config *structs.Config) string {
	rsyncExecutable := "rsync"
	if len(config.RSyncPath) > 0 {
		rsyncExecutable = config.RSyncPath
	}
	return rsyncExecutable + " -avrhLK --delete"
}

func getRsyncDirsCommand(config *structs.Config, srcDir string, dstDir string, excludes []string) string {
	rsyncCommand := getRsyncCommand(config)
	if len(excludes) > 0 {
		rsyncCommand += " --exclude"
		for _, exclude := range excludes {
			rsyncCommand += exclude
		}
	}
	// rsync is run in verbose mode using the -v flag. It outputs a detailed file list, an empty line and the summary. Now sed is used to take advantage of the fact that the summary is
	// separated by an empty line. Everything up to the first empty line is not printed to stdout. ^$ matches an empty line and d prevents it from being output
	rsyncCommand += " " + srcDir + "/ " + dstDir + " | sed '0,/^$/d'"
	return rsyncCommand
}

func GetSnapshotDirPrefix(snapshotName string, interval string) string {
	return snapshotName + "." + interval + "."
}

func GetSnapshotDirName(snapshotName string, interval string, number int) string {
	return GetSnapshotDirPrefix(snapshotName, interval) + strconv.Itoa(number)
}

func executeOnlySnapshot(config *structs.Config, snapshotConfig *structs.SnapshotConfig) error {
	snapshotLogPrefix := "[" + snapshotConfig.SnapshotName + "] "
	before := time.Now().UnixMilli()
	newestSnapshotPath := path.Join(snapshotConfig.SnapshotsDir, GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, 0))
	slog.Debug(snapshotLogPrefix + "Checking if " + newestSnapshotPath + " exists")
	err := os.MkdirAll(snapshotConfig.SnapshotsDir, 0700)
	if err != nil {
		slog.Error(snapshotLogPrefix + "Can't create snapshot dir " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
	tmpDir, mkdirErr := os.MkdirTemp(snapshotConfig.SnapshotsDir, "tmp")
	//defer os.RemoveAll(tmpDir)
	if mkdirErr != nil {
		slog.Error(snapshotLogPrefix + "Can't crate tmp dir " + tmpDir + ": " + mkdirErr.Error())
		return mkdirErr
	}
	_, err = os.Stat(newestSnapshotPath)
	if err == nil {
		slog.Debug(snapshotLogPrefix + "Copying latest snapshot...")
		_, cpErr := exec.Command("sh", "-c", config.CpPath, "-lra", newestSnapshotPath+"/./", tmpDir).Output()
		if cpErr != nil {
			slog.Error(snapshotLogPrefix + "Error copying last snapshot " + newestSnapshotPath + " to " + tmpDir + ": " + cpErr.Error())
			return cpErr
		}
	} else if os.IsNotExist(err) {
		slog.Debug(snapshotLogPrefix + "Creating first snapshot " + newestSnapshotPath)
	} else {
		slog.Error(snapshotLogPrefix + err.Error())
		return err
	}
	now := time.Now()
	os.Chtimes(tmpDir, now, now)

	for _, dirToSnapshot := range snapshotConfig.Dirs {
		if !path.IsAbs(dirToSnapshot.SrcDir) {
			tmp, _ := filepath.Abs(dirToSnapshot.SrcDir)
			dirToSnapshot.SrcDir = tmp
		}
		_, err = os.Stat(dirToSnapshot.SrcDir)
		if os.IsNotExist(err) {
			slog.Warn(snapshotLogPrefix + "Source directory " + dirToSnapshot.SrcDir + " does not exist.")
			continue
		}
		dstDirFull := path.Join(tmpDir, dirToSnapshot.SrcDir)
		_, err = os.Stat(dstDirFull)
		if os.IsNotExist(err) {
			err = os.MkdirAll(dstDirFull, 0700)
			if err != nil {
				slog.Error(snapshotLogPrefix + "Can't create destination dir " + dstDirFull)
				return err
			}
		}
		rsyncCommand := getRsyncDirsCommand(config, dirToSnapshot.SrcDir, dstDirFull, dirToSnapshot.Excludes)
		// rsync is run in verbose mode using the -v flag. It outputs a detailed file list, an empty line and the summary. Now sed is used to take advantage of the fact that the summary is
		// separated by an empty line. Everything up to the first empty line is not printed to stdout. ^$ matches an empty line and d prevents it from being output
		slog.Debug(snapshotLogPrefix + "Synching dir " + dirToSnapshot.SrcDir + "/ to " + dstDirFull)

		_, err = os.Stat(dstDirFull)
		if !os.IsNotExist(err) {
			fmt.Println(dstDirFull + " exists! Running " + rsyncCommand)
		}
		_, err := exec.Command("sh", "-c", rsyncCommand).Output()
		if err != nil {
			slog.Error(snapshotLogPrefix + "Can't sync " + dirToSnapshot.SrcDir + "/ to " + dstDirFull + ": " + err.Error())
			return err
		}
	}

	// rename all the snapshots
	snapshotsNumbers := []int{}
	snapshots, err := os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		slog.Error(snapshotLogPrefix + "Can't read directory " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
	snapshotPrefixWithNumberRegex, err := regexp.Compile("^" + regexp.QuoteMeta(GetSnapshotDirPrefix(snapshotConfig.SnapshotName, snapshotConfig.Interval)) + `\.([0-9]+)$`)
	if err != nil {
		slog.Error(snapshotLogPrefix + "Error compiling regex: " + err.Error())
		return err
	}
	for _, snapshot := range snapshots {
		match := snapshotPrefixWithNumberRegex.FindStringSubmatch(snapshot.Name())
		if match != nil {
			number, err := strconv.Atoi(match[1]) // match[1] contains the first capturing group
			if err != nil {
				slog.Error(snapshotLogPrefix + "Error converting string to int: " + err.Error())
				return err
			}
			snapshotsNumbers = append(snapshotsNumbers, number)
		}
	}
	sort.Ints(snapshotsNumbers)
	for _, number := range snapshotsNumbers {
		snapshotOldName := GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, number)
		snapshotOldPath := path.Join(snapshotConfig.SnapshotsDir, snapshotOldName)
		snapshotRenamedName := GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, number+1)
		snapshotRenamedPath := path.Join(snapshotConfig.SnapshotsDir, snapshotRenamedName)
		err = os.Rename(snapshotOldPath, snapshotRenamedPath)
		if err != nil {
			slog.Error(snapshotLogPrefix + "Can't move " + snapshotOldPath + "to " + snapshotRenamedPath + ": " + err.Error())
			return err
		}
	}

	// rename the temporary folder to be the newest snapshot
	err = os.Rename(tmpDir, newestSnapshotPath)
	if err != nil {
		slog.Error(snapshotLogPrefix + "Can't rename tmp folder " + tmpDir + " to " + newestSnapshotPath + ": " + err.Error())
	}

	// delete the excess amount of snapshots
	snapshots, err = os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		slog.Error(snapshotLogPrefix + "Can't read directory " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
	for _, snapshot := range snapshots {
		match := snapshotPrefixWithNumberRegex.FindStringSubmatch(snapshot.Name())
		if match != nil {
			number, err := strconv.Atoi(match[1]) // match[1] contains the first capturing group
			if err != nil {
				slog.Error(snapshotLogPrefix + "Error converting string to int: " + err.Error())
				return err
			}
			if number >= snapshotConfig.Retention {
				snapshotToRemoveName := GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, number)
				snapshotToRemovePath := path.Join(snapshotConfig.SnapshotsDir, snapshotToRemoveName)
				slog.Info(snapshotLogPrefix + "Removing snapshot " + snapshotToRemovePath)
				err = os.RemoveAll(snapshotToRemovePath)
				if err != nil {
					slog.Error("Can't remove snapshot " + snapshotToRemovePath + ": " + err.Error())
				}
			}
		}
	}

	after := time.Now().UnixMilli()
	seconds := float64(after-before) / 1000
	slog.Info(snapshotLogPrefix + "Snapshots done in " + strconv.FormatFloat(seconds, 'f', -1, 64))
	return nil
}

func ExecuteSnapshot(config *structs.Config, snapshotConfig *structs.SnapshotConfig) error {
	snapshotLogPrefix := "[" + snapshotConfig.SnapshotName + "] "
	before := time.Now().UnixMilli()
	if len(snapshotConfig.PreSnapshotCommands) > 0 {
		slog.Info(snapshotLogPrefix + "Executing pre snapshot commands")
		for _, command := range snapshotConfig.PreSnapshotCommands {
			slog.Info(snapshotLogPrefix + " " + command)
			result, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				slog.Error(snapshotLogPrefix + command + ": " + err.Error())
				return err
			}
			if len(result) > 0 {
				slog.Info(snapshotLogPrefix + command + ": " + string(result))
			}
		}
		after := time.Now().UnixMilli()
		seconds := float64(after-before) / 1000
		slog.Info(snapshotLogPrefix + "Pre snapshots commands done in " + strconv.FormatFloat(seconds, 'f', -1, 64))
	} else {
		slog.Info(snapshotLogPrefix + "No pre snapshot commands to run")
	}

	err := executeOnlySnapshot(config, snapshotConfig)
	if err != nil && !snapshotConfig.AlwaysRunPostSnapshotCommands {
		return err
	}

	if len(snapshotConfig.PostSnapshotCommands) > 0 {
		slog.Info(snapshotLogPrefix + "Executing post snapshot commands")
		for _, command := range snapshotConfig.PostSnapshotCommands {
			slog.Info(snapshotLogPrefix + " " + command)
			result, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				slog.Error(snapshotLogPrefix + command + ": " + err.Error())
				return err
			}
			if len(result) > 0 {
				slog.Info(snapshotLogPrefix + command + ": " + string(result))
			}
		}
		after := time.Now().UnixMilli()
		seconds := float64(after-before) / 1000
		slog.Info(snapshotLogPrefix + "Post snapshots commands done in " + strconv.FormatFloat(seconds, 'f', -1, 64))
	} else {
		slog.Info(snapshotLogPrefix + "No post snapshot commands to run")
	}

	now := time.Now()
	os.Chtimes(GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, 0), now, now)
	return nil
}

func GetSnapshotsPathsBySnapshotName(configsDir string, expandVars bool, snapshotName string) (snapshotsDirs []string, err error) {
	snapshotConfig, err := configs.GetSnapshotConfigByName(configsDir, expandVars, snapshotName)
	if err != nil {
		slog.Error("Can't list snapshots of " + snapshotName + ": " + err.Error())
		return snapshotsDirs, err
	}
	if snapshotConfig == nil {
		slog.Warn("Snapshot template " + snapshotName + " does not exist.")
		return snapshotsDirs, nil
	}
	_, err = os.Stat(snapshotConfig.SnapshotsDir)
	if os.IsNotExist(err) {
		slog.Info("No snapshots found for " + snapshotName)
		return snapshotsDirs, nil
	}
	snapshotsDirsEntries, err := os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		slog.Error("Can't list snapshot of " + snapshotName + ": " + err.Error())
		return snapshotsDirs, err
	}
	for _, entry := range snapshotsDirsEntries {
		snapshotsDirs = append(snapshotsDirs, path.Join(snapshotConfig.SnapshotsDir, entry.Name()))
	}
	return snapshotsDirs, nil
}

func RestoreSnapshot(config structs.Config, snapshotConfig structs.SnapshotConfig, interval string, number int) error {
	snapshotPath := path.Join(snapshotConfig.SnapshotsDir, GetSnapshotDirName(snapshotConfig.SnapshotName, interval, number))
	snapshotLogPrefix := "[" + snapshotConfig.SnapshotName + "] "
	snapshotContents, err := os.ReadDir(snapshotPath)
	if err != nil {
		slog.Error("Can't read snapshot " + snapshotPath + ": " + err.Error())
		return err
	}
	for _, snapshotDirEntry := range snapshotContents {
		dirToRestoreTo := strings.TrimPrefix(snapshotDirEntry.Name(), snapshotConfig.SnapshotsDir)
		err = os.MkdirAll(dirToRestoreTo, 0700)
		if err != nil {
			slog.Error("Can't create directory " + dirToRestoreTo + ": " + err.Error())
			return err
		}
		rsyncCommand := getRsyncDirsCommand(&config, snapshotDirEntry.Name(), dirToRestoreTo, []string{})
		// rsync is run in verbose mode using the -v flag. It outputs a detailed file list, an empty line and the summary. Now sed is used to take advantage of the fact that the summary is
		// separated by an empty line. Everything up to the first empty line is not printed to stdout. ^$ matches an empty line and d prevents it from being output
		slog.Debug(snapshotLogPrefix + "Synching dir " + snapshotDirEntry.Name() + "/ to" + dirToRestoreTo)
		_, err := exec.Command("sh", "-c", rsyncCommand).Output()
		if err != nil {
			slog.Error(snapshotLogPrefix + "Can't sync " + snapshotDirEntry.Name() + "/ to" + dirToRestoreTo + ": " + err.Error())
			return err
		}
	}
	return nil
}
