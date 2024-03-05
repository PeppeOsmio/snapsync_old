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

	"github.com/go-co-op/gocron/v2"
	"golang.org/x/exp/slog"
)

func main() {
	lvl := new(slog.LevelVar)
	lvl.Set(slog.LevelDebug)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: lvl,
	})))
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

	config, err := configs.LoadConfig(*configsDirFlag, *expandVarsFlag)
	if err != nil {
		slog.Error("Can't get " + *configsDirFlag + ": " + err.Error())
		return
	}

	snapshotsConfigs, err := configs.LoadSnapshotsConfigs(*configsDirFlag, *expandVarsFlag)
	if err != nil {
		slog.Error("Can't get snapshots configs in " + *configsDirFlag + ": " + err.Error())
	}

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
		snapshotsInfo, err := snapshots.GetSnapshotsInfo(*configsDirFlag, *expandVarsFlag, *restoreFlag)
		if err != nil {
			slog.Error("Can't get snapshots of snapshot " + *listFlag + ": " + err.Error())
			return
		}
		for len(snapshotsInfo) == 0 {
			slog.Info("There are no snapshots to restore for " + *restoreFlag)
			return
		}
		printSnapshots := func() {
			for i, snapshotInfo := range snapshotsInfo {
				fmt.Println()
				fmt.Printf("[%d]\tName: %s\n", i, snapshotInfo.SnapshotName)
				fmt.Printf("\tInterval: %s\n", snapshotInfo.Interval)
				fmt.Printf("\tNumber: %d\n", snapshotInfo.Number)
				size, err := snapshotInfo.Size()
				if err != nil {
					fmt.Printf("\tSize: (error: %s)\n", err.Error())
				} else {
					fmt.Printf("\tSize: %s\n", utils.HumanReadableSize(size))
				}
			}
		}
		printSnapshots()
		fmt.Println()
		fmt.Print("Choose which snapshot to restore: ")
		input := 0
		fmt.Scan(&input)
		for input < 0 || input > len(snapshotsInfo)-1 {
			fmt.Println("Invalid number.")
			printSnapshots()
			fmt.Print("Choose which snapshot to restore: ")
			fmt.Scan(&input)
		}
		snapshotConfig, err := configs.GetSnapshotConfigByName(*configsDirFlag, *expandVarsFlag, snapshotsInfo[input].SnapshotName)
		if err != nil {
			slog.Error("An error occurred: " + err.Error())
			return
		}
		err = snapshots.RestoreSnapshot(config, snapshotsInfo[input], snapshotConfig)
		if err != nil {
			slog.Error("An error occurred while restoring the snapshot: " + err.Error())
			return
		}
		return
	}

	snapshotTask := func(snapshotConfig *structs.SnapshotConfig) {
		snapshotErr := snapshots.ExecuteSnapshot(config, snapshotConfig)
		if snapshotErr != nil {
			slog.Error(fmt.Sprintf("[%s] can't execute snapshot: %s", snapshotConfig.SnapshotName, err.Error()))
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
			slog.Info(fmt.Sprintf("[%s] scheduled with cron %s", snapshotConfig.SnapshotName, snapshotConfig.Cron))
		}
		if err != nil {
			slog.Error("can't create scheduler.")
			return
		}
		scheduler.Start()
		for {
			time.Sleep(1 * time.Second)
		}
	}
}
