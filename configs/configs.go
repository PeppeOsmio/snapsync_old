package configs

import (
	"os"
	"peppeosmio/snapsync/structs"

	"github.com/sirupsen/logrus"
)

func LoadConfigs() (config *structs.Config, snapshotConfigs []structs.SnapshotConfig, err error) {
	configsDir, err := GetConfigsDir()
	if err != nil {
		logrus.Error("Can't get configs dir: " + err.Error())
		return nil, []structs.SnapshotConfig{}, err
	}
}

func GetConfigsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return homeDir, nil
}
