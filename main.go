package main

import (
	"flag"
	"fmt"
	"os"
	"peppeosmio/snapsync/configs"
	"peppeosmio/snapsync/snapshots"
	"peppeosmio/snapsync/structs"
	"peppeosmio/snapsync/utils"
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
		slog.Error("Can't get default config path: " + err.Error())
		return
	}
	configsDirFlag := flag.String("configs-dir", defaultConfigsPath, "Directory of the config files")
	err = os.MkdirAll(*configsDirFlag, 0700)
	if err != nil {
		slog.Error("Can't create configs dir " + *configsDirFlag + ": " + err.Error())
		return
	}

	flag.Parse()

	if len(*listFlag) > 0 {
		snapshotsInfo, err := snapshots.GetSnapshotsInfo(*configsDirFlag, *expandVarsFlag, *listFlag)
		if err != nil {
			slog.Error("Can't get snapshots of snapshot " + *listFlag + ": " + err.Error())
			return
		}
		for _, snapshotInfo := range snapshotsInfo {
			fmt.Println("")
			fmt.Printf("Name: %s\n", snapshotInfo.SnapshotName)
			fmt.Printf("Interval: %s\n", snapshotInfo.Interval)
			fmt.Printf("Number: %d\n", snapshotInfo.Number)
			size, err := snapshotInfo.Size()
			if err != nil {
				fmt.Printf("Size: (error: %s)\n", err.Error())
			} else {
				fmt.Printf("Size: %s\n", utils.HumanReadableSize(size))
			}
		}
		return
	}

	if len(*restoreFlag) > 0 {
		return
	}

	config, err := configs.LoadConfig(*configsDirFlag, *expandVarsFlag)
	if err != nil {
		slog.Error("Can't get " + *configsDirFlag + ": " + err.Error())
		return
	}

	snapshotsConfigs, err := configs.LoadSnapshotsConfigs(*configsDirFlag, *expandVarsFlag)
	if err != nil {
		slog.Error("Can't get snapshots configs in " + *configsDirFlag + ": " + err.Error())
	}
	snapshotTask := func(snapshotConfig *structs.SnapshotConfig) {
		snapshotErr := snapshots.ExecuteSnapshot(config, snapshotConfig)
		if snapshotErr != nil {
			slog.Error("Can't execute snapshot: " + snapshotErr.Error())
		}
	}
	snapshotsConfigsToSchedule := []*structs.SnapshotConfig{}
	for _, snapshotConfig := range snapshotsConfigs {

		if len(snapshotConfig.Cron) > 0 {
			snapshotsConfigsToSchedule = append(snapshotsConfigsToSchedule, snapshotConfig)

		} else {
			snapshotTask(snapshotConfig)
		}
	}
	if len(snapshotsConfigsToSchedule) > 0 {
		scheduler, err := gocron.NewScheduler()
		for _, snapshotConfig := range snapshotsConfigsToSchedule {
			_, err := scheduler.NewJob(
				gocron.CronJob(snapshotConfig.Cron, false),
				gocron.NewTask(
					snapshotTask,
					snapshotConfig,
				),
			)
			if err != nil {
				slog.Error("Can't add cron job for snapshot " + snapshotConfig.SnapshotName + ". Cron string is " + snapshotConfig.Cron)
				return
			}
		}
		if err != nil {
			slog.Error("Can't create scheduler.")
			return
		}
		scheduler.Start()
		for {
			time.Sleep(1 * time.Second)
		}
	}
}
