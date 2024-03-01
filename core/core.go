package core

import (
	"os"
	"os/exec"
	"path"
	"peppeosmio/snapsync/configs"
	"peppeosmio/snapsync/structs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
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

func ExecuteSnapshot(config structs.Config, snapshotConfig structs.SnapshotConfig) error {
	snapshotLogPrefix := "[" + snapshotConfig.SnapshotName + "] "
	before := time.Now().UnixMilli()
	if len(snapshotConfig.PreSnapshotCommands) > 0 {
		logrus.Info(snapshotLogPrefix + "Executing pre snapshot commands")
		for _, command := range snapshotConfig.PreSnapshotCommands {
			logrus.Info(snapshotLogPrefix + " " + command)
			result, err := exec.Command(command).Output()
			if err != nil {
				logrus.Error(snapshotLogPrefix + command + ": " + err.Error())
				return err
			}
			if len(result) > 0 {
				logrus.Info(snapshotLogPrefix + command + ": " + string(result))
			}
		}
		after := time.Now().UnixMilli()
		seconds := float64(after-before) / 1000
		logrus.Info(snapshotLogPrefix + "Pre snapshots commands done in " + strconv.FormatFloat(seconds, 'f', -1, 64))
	} else {
		logrus.Info(snapshotLogPrefix + "No pre snapshot commands to run")
	}
	before = time.Now().UnixMilli()
	formattedSnapshotName := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(snapshotConfig.SnapshotName)), " ", "_")
	formattedSnapshotInterval := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(snapshotConfig.Interval)), " ", "_")
	newestSnapshotPath := path.Join(snapshotConfig.SnapshotsDir, GetSnapshotDirName(formattedSnapshotName, formattedSnapshotInterval, 0))
	logrus.Debug(snapshotLogPrefix + "Checking if " + newestSnapshotPath + " exists")
	err := os.MkdirAll(snapshotConfig.SnapshotsDir, 0700)
	if err != nil {
		logrus.Error(snapshotLogPrefix + "Can't create snapshot dir " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
	tmpDir, mkdirErr := os.MkdirTemp(snapshotConfig.SnapshotsDir, "tmp")
	if mkdirErr != nil {
		logrus.Error(snapshotLogPrefix + "Can't crate tmp dir " + tmpDir + ": " + mkdirErr.Error())
		return mkdirErr
	}
	_, err = os.Stat(newestSnapshotPath)
	if err == nil {
		logrus.Debug(snapshotLogPrefix + "Copying latest snapshot...")
		cpCommand := config.CpPath + " -lra  " + newestSnapshotPath + " " + tmpDir
		_, cpErr := exec.Command(cpCommand).Output()
		if cpErr != nil {
			logrus.Error(snapshotLogPrefix + ": " + cpErr.Error())
			return cpErr
		}
	} else if os.IsNotExist(err) {
		logrus.Debug(snapshotLogPrefix + "Creating first snapshot " + newestSnapshotPath)
	} else {
		logrus.Error(snapshotLogPrefix + err.Error())
		return err
	}
	now := time.Now()
	os.Chtimes(tmpDir, now, now)

	for _, dirToSnapshot := range snapshotConfig.Dirs {
		_, err = os.Stat(dirToSnapshot.SrcDir)
		if os.IsNotExist(err) {
			logrus.Warn(snapshotLogPrefix + "Source directory " + dirToSnapshot.SrcDir + " does not exist.")
		}
		dirToCopyTo := path.Join(snapshotConfig.SnapshotsDir, dirToSnapshot.SrcDir)
		dstDirFull := path.Join(tmpDir, dirToCopyTo)
		_, err = os.Stat(dstDirFull)
		if os.IsNotExist(err) {
			err = os.MkdirAll(dstDirFull, 0700)
			if err != nil {
				logrus.Error(snapshotLogPrefix + "Can't create destination dir " + dstDirFull)
				return err
			}
		}
		rsyncCommand := getRsyncDirsCommand(&config, dirToSnapshot.SrcDir, dstDirFull, dirToSnapshot.Excludes)
		// rsync is run in verbose mode using the -v flag. It outputs a detailed file list, an empty line and the summary. Now sed is used to take advantage of the fact that the summary is
		// separated by an empty line. Everything up to the first empty line is not printed to stdout. ^$ matches an empty line and d prevents it from being output
		logrus.Debug(snapshotLogPrefix + "Synching dir " + dirToSnapshot.SrcDir + "/ to" + dstDirFull)
		_, err := exec.Command(rsyncCommand).Output()
		if err != nil {
			logrus.Error(snapshotLogPrefix + "Can't sync " + dirToSnapshot.SrcDir + "/ to" + dstDirFull + ": " + err.Error())
			return err
		}
	}

	// rename all the snapshots
	snapshotsNumbers := []int{}
	snapshots, err := os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		logrus.Error(snapshotLogPrefix + "Can't read directory " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
	snapshotPrefixWithNumberRegex, err := regexp.Compile("^" + regexp.QuoteMeta(GetSnapshotDirPrefix(snapshotConfig.SnapshotName, snapshotConfig.Interval)) + `\.([0-9]+)$`)
	if err != nil {
		logrus.Error(snapshotLogPrefix + "Error compiling regex: " + err.Error())
		return err
	}
	for _, snapshot := range snapshots {
		match := snapshotPrefixWithNumberRegex.FindStringSubmatch(snapshot.Name())
		if match != nil {
			number, err := strconv.Atoi(match[1]) // match[1] contains the first capturing group
			if err != nil {
				logrus.Error(snapshotLogPrefix + "Error converting string to int: " + err.Error())
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
			logrus.Error(snapshotLogPrefix + "Can't move " + snapshotOldPath + "to " + snapshotRenamedPath + ": " + err.Error())
			return err
		}
	}

	// rename the temporary folder to be the newest snapshot
	err = os.Rename(tmpDir, newestSnapshotPath)
	if err != nil {
		logrus.Error(snapshotLogPrefix + "Can't rename tmp folder " + tmpDir + " to " + newestSnapshotPath + ": " + err.Error())
	}

	// delete the excess amount of snapshots
	snapshots, err = os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		logrus.Error(snapshotLogPrefix + "Can't read directory " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
	for _, snapshot := range snapshots {
		match := snapshotPrefixWithNumberRegex.FindStringSubmatch(snapshot.Name())
		if match != nil {
			number, err := strconv.Atoi(match[1]) // match[1] contains the first capturing group
			if err != nil {
				logrus.Error(snapshotLogPrefix + "Error converting string to int: " + err.Error())
				return err
			}
			if number >= snapshotConfig.Retention {
				snapshotToRemoveName := GetSnapshotDirName(snapshotConfig.SnapshotName, snapshotConfig.Interval, number)
				snapshotToRemovePath := path.Join(snapshotConfig.SnapshotsDir, snapshotToRemoveName)
				logrus.Info(snapshotLogPrefix + "Removing snapshot " + snapshotToRemovePath)
				err = os.RemoveAll(snapshotToRemovePath)
				if err != nil {
					logrus.Error("Can't remove snapshot " + snapshotToRemovePath + ": " + err.Error())
				}
			}
		}
	}

	after := time.Now().UnixMilli()
	seconds := float64(after-before) / 1000
	logrus.Info(snapshotLogPrefix + "Snapshots done in " + strconv.FormatFloat(seconds, 'f', -1, 64))

	if len(snapshotConfig.PostSnapshotCommands) > 0 {
		logrus.Info(snapshotLogPrefix + "Executing post snapshot commands")
		for _, command := range snapshotConfig.PostSnapshotCommands {
			logrus.Info(snapshotLogPrefix + " " + command)
			result, err := exec.Command(command).Output()
			if err != nil {
				logrus.Error(snapshotLogPrefix + command + ": " + err.Error())
				return err
			}
			if len(result) > 0 {
				logrus.Info(snapshotLogPrefix + command + ": " + string(result))
			}
		}
		after := time.Now().UnixMilli()
		seconds := float64(after-before) / 1000
		logrus.Info(snapshotLogPrefix + "Post snapshots commands done in " + strconv.FormatFloat(seconds, 'f', -1, 64))
	} else {
		logrus.Info(snapshotLogPrefix + "No post snapshot commands to run")
	}

	now = time.Now()
	os.Chtimes(newestSnapshotPath, now, now)
	return nil
}

func GetSnapshotsPathsBySnapshotName(configsDir string, expandVars bool, snapshotName string) (snapshotsDirs []string, err error) {
	snapshotConfig, err := configs.GetSnapshotConfigByName(configsDir, expandVars, snapshotName)
	if err != nil {
		logrus.Error("Can't list snapshot of " + snapshotName + ": " + err.Error())
		return snapshotsDirs, err
	}
	if snapshotConfig == nil {
		logrus.Warn("Snapshot " + snapshotName + " does not exist.")
		return snapshotsDirs, nil
	}
	snapshotsDirsEntries, err := os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		logrus.Error("Can't list snapshot of " + snapshotName + ": " + err.Error())
		return snapshotsDirs, err
	}
	for _, entry := range snapshotsDirsEntries {
		snapshotsDirs = append(snapshotsDirs, path.Join(snapshotConfig.SnapshotsDir, entry.Name()))
	}
	return snapshotsDirs, nil
}

func RestoreSnapshot(config structs.Config, snapshotConfig structs.SnapshotConfig, interval string, number int) error {
	snapshotPath := path.Join(snapshotConfig.SnapshotsDir, GetSnapshotDirName(snapshotConfig.SnapshotName, interval, number))
	snapshotContents, err := os.ReadDir(snapshotPath)
	snapshotLogPrefix := "[" + snapshotConfig.SnapshotName + "] "
	if err != nil {
		logrus.Error("Can't read snapshot " + snapshotPath + ": " + err.Error())
		return err
	}
	for _, snapshotDirEntry := range snapshotContents {
		dirToRestoreTo := strings.TrimPrefix(snapshotDirEntry.Name(), snapshotConfig.SnapshotsDir)
		err = os.MkdirAll(dirToRestoreTo, 0700)
		if err != nil {
			logrus.Error("Can't create directory " + dirToRestoreTo + ": " + err.Error())
			return err
		}
		rsyncCommand := getRsyncDirsCommand(&config, snapshotDirEntry.Name(), dirToRestoreTo, []string{})
		// rsync is run in verbose mode using the -v flag. It outputs a detailed file list, an empty line and the summary. Now sed is used to take advantage of the fact that the summary is
		// separated by an empty line. Everything up to the first empty line is not printed to stdout. ^$ matches an empty line and d prevents it from being output
		logrus.Debug(snapshotLogPrefix + "Synching dir " + snapshotDirEntry.Name() + "/ to" + dirToRestoreTo)
		_, err := exec.Command(rsyncCommand).Output()
		if err != nil {
			logrus.Error(snapshotLogPrefix + "Can't sync " + snapshotDirEntry.Name() + "/ to" + dirToRestoreTo + ": " + err.Error())
			return err
		}
	}
	return nil
}
