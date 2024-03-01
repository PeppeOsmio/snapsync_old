package configs

import (
	"errors"
	"fmt"
	"os"
	"path"
	"peppeosmio/snapsync/structs"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func LoadConfigs(configsDir string, expandVars bool) (config *structs.Config, snapshotsConfigs []*structs.SnapshotConfig, err error) {
	err = os.MkdirAll(configsDir, 0700)
	if err != nil {
		logrus.Error("Can't create configs dir " + configsDir + ": " + err.Error())
		return nil, snapshotsConfigs, err
	}
	configPath := path.Join(configsDir, "config.yml")
	fmt.Println(configPath)
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		logrus.Error("Can't read " + configPath + ": " + err.Error())
		return nil, snapshotsConfigs, err
	}
	configFileContent := string(configFile)
	if expandVars {
		configFileContent = os.ExpandEnv(configFileContent)
	}
	config = &structs.Config{}
	err = yaml.Unmarshal([]byte(configFileContent), config)
	if err != nil {
		logrus.Error("Can't parse " + configPath + ": " + err.Error())
		return nil, snapshotsConfigs, err
	}
	snapshotConfigsEntries, err := os.ReadDir(configsDir)
	if err != nil {
		logrus.Error("Can't read directory " + configsDir + ": " + err.Error())
		return nil, snapshotsConfigs, err
	}
	for _, snapshotConfigEntry := range snapshotConfigsEntries {
		if strings.HasPrefix(snapshotConfigEntry.Name(), "config.yml") {
			continue
		}
		if !strings.HasSuffix(snapshotConfigEntry.Name(), ".yml") {
			continue
		}
		absPath := path.Join(configsDir, snapshotConfigEntry.Name())
		snapshotConfigFile, err := os.ReadFile(absPath)
		if err != nil {
			logrus.Error("Can't read snapshot config file " + absPath + ": " + err.Error())
			return nil, snapshotsConfigs, err
		}
		configFileContent := string(snapshotConfigFile)
		if expandVars {
			configFileContent = os.ExpandEnv(configFileContent)
		}
		snapshotConfig := structs.SnapshotConfig{}
		err = yaml.Unmarshal([]byte(configFileContent), &snapshotConfig)
		if err != nil {
			logrus.Error("Can't parse snapshot config file " + absPath + ": " + err.Error())
			return nil, snapshotsConfigs, err
		}
		snapshotsConfigs = append(snapshotsConfigs, &snapshotConfig)
	}
	return config, snapshotsConfigs, nil
}

func ValidateSnapshotConfig(snapshotConfig *structs.SnapshotConfig) error {
	if strings.Contains(snapshotConfig.SnapshotName, ".") {
		return errors.New("Snapshot " + snapshotConfig.SnapshotName + " contains a \".\"")
	}
	return nil
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

func GetSnapshotConfigByName(configsDir string, expandVars bool, snapshotName string) (*structs.SnapshotConfig, error) {
	_, snapshotConfigs, err := LoadConfigs(configsDir, expandVars)
	if err != nil {
		return nil, err
	}
	for _, snapshotConfig := range snapshotConfigs {
		if snapshotConfig.SnapshotName == snapshotName {
			return snapshotConfig, nil
		}
	}
	return nil, nil
}
