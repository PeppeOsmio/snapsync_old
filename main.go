package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"peppeosmio/snapsync/configs"
	"peppeosmio/snapsync/snapshots"
	"time"

	"log/slog"

	"github.com/go-co-op/gocron/v2"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	restoreFlag := flag.String("restore", "", "Restore a snapshot")
	listFlag := flag.String("list", "", "List the snapshot by name")
	expandVarsFlag := flag.Bool("expand-vars", true, "Expand environment variables")

	defaultConfigsPath, err := configs.GetDefaultConfigsDir()
	if err != nil {
		return
	}
	configsDirFlag := flag.String("configs-dir", defaultConfigsPath, "Directory of the config files")

	flag.Parse()

	if len(*listFlag) > 0 {
		snapshotsDir, err := snapshots.GetSnapshotsPathsBySnapshotName(*configsDirFlag, *expandVarsFlag, *listFlag)
		if err != nil {
			slog.Error("Can't get snapshots of snapshot " + *listFlag + ": " + err.Error())
			return
		}
		for _, snapshotDir := range snapshotsDir {
			fileInfo, err := os.Stat(snapshotDir)
			if err != nil {
				slog.Error("Can't stat " + snapshotDir + ": " + err.Error())
				return
			}
			fmt.Println(fileInfo.ModTime().Format(time.RFC3339) + " " + path.Base(snapshotDir))
		}
		return
	}

	if len(*restoreFlag) > 0 {
		return
	}

	config, snapshotsConfigs, err := configs.LoadConfigs(*configsDirFlag, *expandVarsFlag)
	if err != nil {
		slog.Error("Can't get configs: " + err.Error())
		return
	}
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		slog.Error("Can't create scheduler.")
		return
	}

	for _, snapshotConfig := range snapshotsConfigs {
		snapshotTask := func() {
			snapshotErr := snapshots.ExecuteSnapshot(config, snapshotConfig)
			if snapshotErr != nil {
				slog.Error("Can't execute snapshot: " + snapshotErr.Error())
			}
		}
		if len(snapshotConfig.Cron) > 0 {
			_, err := scheduler.NewJob(
				gocron.CronJob(snapshotConfig.Cron, false),
				gocron.NewTask(
					snapshotTask,
				),
			)
			if err != nil {
				slog.Error("Can't add cron job for snapshot " + snapshotConfig.SnapshotName + ". Cron string is " + snapshotConfig.Cron)
				return
			}
		} else {
			snapshotTask()
		}
	}
	if len(scheduler.Jobs()) > 0 {
		scheduler.Start()
		for {
			time.Sleep(1 * time.Second)
		}
	}
}
