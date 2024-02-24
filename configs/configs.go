package configs

import (
	"os"
	"path"
	"peppeosmio/snapsync/structs"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func LoadConfigs(configsDir string, expandVars bool) (config *structs.Config, snapshotsConfigs []structs.SnapshotConfig, err error) {
	err = os.MkdirAll(configsDir, 0700)
	if err != nil {
		logrus.Error("Can't create configs dir " + configsDir + ": " + err.Error())
		return nil, snapshotsConfigs, err
	}
	if err != nil {
		logrus.Error("Can't get configs dir: " + err.Error())
		return nil, snapshotsConfigs, err
	}
	configPath := path.Join(configsDir, "config.yml")
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		logrus.Error("Can't read " + configPath + ": " + err.Error())
		return nil, snapshotsConfigs, err
	}
	configFileContent := string(configFile)
	if expandVars {
		configFileContent = os.ExpandEnv(configFileContent)
	}
	err = yaml.Unmarshal([]byte(configFileContent), config)
	if err != nil {
		logrus.Error("Can't parse " + configPath + ": " + err.Error())
		return nil, snapshotsConfigs, err
	}
	snapshots, err := os.ReadDir(snapshotConfig.SnapshotsDir)
	if err != nil {
		logrus.Error(snapshotLogPrefix + "Can't read directory " + snapshotConfig.SnapshotsDir + ": " + err.Error())
		return err
	}
}

func GetDefaultConfigsDir() (configsDir string, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logrus.Error("Can't get user home dir: " + err.Error())
		return "", err
	}
	configsDir = path.Join(homeDir, ".config/snapsync")
	return configsDir, nil
}
