package snapsync

import (
	"os"
	"os/exec"
	"path"
	"peppeosmio/snapsync/structs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func executeSnapshot(config structs.Config, snapshotConfig structs.SnapshotConfig) error {
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
	snapshotPrefix := formattedSnapshotName + "." + formattedSnapshotInterval
	newestSnapshotPath := path.Join(snapshotConfig.SnapshotsDir, snapshotPrefix+".0")
	logrus.Debug(snapshotLogPrefix + "Checking if " + newestSnapshotPath + " exists")
	tmpDir, mkdirErr := os.MkdirTemp(snapshotConfig.SnapshotsDir, "tmp")
	if mkdirErr != nil {
		logrus.Error(snapshotLogPrefix + "Can't crate tmp dir " + tmpDir + ": " + mkdirErr.Error())
		return mkdirErr
	}
	_, err := os.Stat(newestSnapshotPath)
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

	for _, item := range snapshotConfig.Dirs {
		_, err = os.Stat(item.SrcDir)
		if os.IsNotExist(err) {
			logrus.Warn(snapshotLogPrefix + "Source directory " + item.SrcDir + " does not exist.")
		}
		var dirToCopyTo string
		if item.DstDir == "" {
			dirToCopyTo = item.SrcDir
		} else {
			dirToCopyTo = item.DstDir
		}
		dstDirFull := path.Join(tmpDir, dirToCopyTo)
		_, err = os.Stat(dstDirFull)
		if os.IsNotExist(err) {
			err = os.MkdirAll(dstDirFull, 0700)
			if err != nil {
				logrus.Error(snapshotLogPrefix + "Can't create destination dir " + dstDirFull)
				return err
			}
		}
		rsync_command := "rsync -avrhLK --delete"
		for _, exclude := range item.Excludes {
			rsync_command += " --exclude " + exclude
		}
		// rsync is run in verbose mode using the -v flag. It outputs a detailed file list, an empty line and the summary. Now sed is used to take advantage of the fact that the summary is
		// separated by an empty line. Everything up to the first empty line is not printed to stdout. ^$ matches an empty line and d prevents it from being output
		rsync_command += " " + item.SrcDir + "/ " + dstDirFull + " | sed '0,/^$/d'"
		logrus.Debug(snapshotLogPrefix + "Synching dir " + item.SrcDir + "/ to" + dstDirFull)
		_, err := exec.Command(rsync_command).Output()
		if err != nil {
			logrus.Error(snapshotLogPrefix + "Can't sync " + item.SrcDir + "/ to" + dstDirFull + ": " + err.Error())
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
	snapshotPrefixWithNumberRegex, err := regexp.Compile("^" + regexp.QuoteMeta(snapshotPrefix) + `\.([0-9]+)$`)
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
		snapshotOld := path.Join(snapshotConfig.SnapshotsDir, snapshotPrefix+"."+strconv.Itoa(number))
		snapshotRenamed := path.Join(snapshotConfig.SnapshotsDir, snapshotPrefix+"."+strconv.Itoa(number+1))
		_, err := exec.Command("mv " + snapshotOld + " " + snapshotRenamed).Output()
		if err != nil {
			logrus.Error(snapshotLogPrefix + "Can't move " + snapshotOld + "to " + snapshotRenamed + ": " + err.Error())
			return err
		}
	}

	// rename the temporary folder to be the newest snapshot
	_, err = exec.Command("mv " + tmpDir + " " + newestSnapshotPath).Output()
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
				snapshotToRemove := path.Join(snapshotConfig.SnapshotsDir, snapshotPrefix+"."+strconv.Itoa(number))
				logrus.Info(snapshotLogPrefix + "Removing snapshot " + snapshotToRemove)
				err = os.RemoveAll(snapshotToRemove)
				if err != nil {
					logrus.Error("Can't remove snapshot " + snapshotToRemove + ": " + err.Error())
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
