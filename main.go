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
	config, snapshotsConfigs, err := configs.LoadConfigs(configsDir)
	// create a scheduler
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		// handle error
	}

	for _, snapshotConfig := range snapshotsConfigs {
		if len(snapshotConfig.Cron) > 0 {
			// add a job to the scheduler
			_, err := scheduler.NewJob(
				gocron.CronJob(snapshotConfig.Cron, false),
				gocron.NewTask(
					snapsync.ExecuteSnapshot,
					config,
					snapshotsConfigs,
				),
			)
			if err != nil {
				logrus.Error("Can't add cron job for snapshot " + snapshotConfig.SnapshotName + ". Cron string is " + snapshotConfig.Cron)
				return
			}
		}
	}

	// start the scheduler
	scheduler.Start()

	for true {
		time.Sleep(1 * time.Second)
	}
}
