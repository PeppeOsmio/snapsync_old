package main

import (
	"flag"
	"peppeosmio/snapsync/configs"
	"peppeosmio/snapsync/logging"
	"peppeosmio/snapsync/snapsync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/sirupsen/logrus"
)

func main() {
	logging.SetupLogging()
	expandVarsFlag := flag.Bool("expand-vars", true, "Expand environment variables")
	defaultConfigsPath, err := configs.GetDefaultConfigsDir()
	configsDirFlag := flag.String("configs-dir", defaultConfigsPath, "Directory of the config files")
	if err != nil {
		return
	}
	var configsDir string
	if configsDirFlag != nil {
		configsDir = *configsDirFlag
	} else {
		configsDir = defaultConfigsPath
	}
	config, snapshotsConfigs, err := configs.LoadConfigs(configsDir, *expandVarsFlag)
	if err != nil {
		logrus.Error("Can't get configs: " + err.Error())
		return
	}
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		logrus.Error("Can't create scheduler.")
		return
	}

	for _, snapshotConfig := range snapshotsConfigs {
		snapshotTask := func() {
			snapshotErr := snapsync.ExecuteSnapshot(*config, *snapshotConfig)
			if snapshotErr != nil {
				logrus.Error("Can't execute snapshot: " + snapshotErr.Error())
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
				logrus.Error("Can't add cron job for snapshot " + snapshotConfig.SnapshotName + ". Cron string is " + snapshotConfig.Cron)
				return
			}
		} else {
			snapshotTask()
		}
	}
	scheduler.Start()
	for {
		time.Sleep(1 * time.Second)
	}
}
