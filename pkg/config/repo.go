package config

import (
	"deepin-upgrade-manager/pkg/module/util"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
)

type RepoConfig struct {
	RepoMountPoint string `json:"repo_mount_point"`
	Repo           string `json:"repo"`
	SnapshotDir    string `json:"snapshot_dir"`
	StageDir       string `json:"stage_dir"`

	SubscribeList []string `json:"subscribe_list"`
	FilterList    []string `json:"filter_list"`
}
type RepoListConfig []*RepoConfig

type Config struct {
	filename string

	Version       string `json:"config_version"`
	Distribution  string `json:"distribution"`
	ActiveVersion string `json:"active_version"`
	CacheDir      string `json:"cache_dir"`

	AutoCleanup bool           `json:"auto_cleanup"`
	RepoList    RepoListConfig `json:"repo_list"`

	MaxVersionRetention int32 `json:"max_version_retention"`
	MaxRepoRetention    int32 `json:"max_repo_retention"`
}

func (c *Config) Prepare() error {
	for _, repo := range c.RepoList {
		err := os.MkdirAll(repo.StageDir, 0755)
		if err != nil {
			return err
		}
		err = os.MkdirAll(repo.SnapshotDir, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) GetRepoConfig(repoDir string) *RepoConfig {
	for _, v := range c.RepoList {
		if v.Repo == repoDir {
			return v
		}
	}
	return nil
}

func (c *Config) Save() error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}

	tmpFile := c.filename + "-" + util.MakeRandomString(util.MinRandomLen)
	err = ioutil.WriteFile(tmpFile, data, 0640)
	if err != nil {
		return err
	}
	_, err = util.Move(c.filename, tmpFile, true)
	return err
}

func (c *Config) ChangeRepoMountPoint(mountpoint string) {
	for _, v := range c.RepoList {
		if v.RepoMountPoint == mountpoint {
			continue
		}
		var isExist bool
		for _, v := range v.SubscribeList {
			if strings.HasPrefix(mountpoint, v) {
				isExist = true
			}
		}

		if mountpoint == "/" {
			v.Repo = strings.Replace(v.Repo, v.RepoMountPoint, "", 1)
			v.SnapshotDir = strings.Replace(v.SnapshotDir, v.RepoMountPoint, "", 1)
			v.StageDir = strings.Replace(v.StageDir, v.RepoMountPoint, "", 1)
		} else {
			v.Repo = strings.Replace(v.Repo, v.RepoMountPoint, mountpoint, 1)
			v.SnapshotDir = strings.Replace(v.SnapshotDir, v.RepoMountPoint, mountpoint, 1)
			v.StageDir = strings.Replace(v.StageDir, v.RepoMountPoint, mountpoint, 1)
		}
		if isExist {
			v.FilterList = append(v.FilterList, v.Repo, v.SnapshotDir, v.StageDir)
		}
		v.RepoMountPoint = mountpoint
	}
}

func (c *Config) SetDistribution(version string) {
	if c.Distribution != version {
		c.Distribution = version
	}
}

func LoadConfig(filename string) (*Config, error) {
	var info Config
	err := loadFile(&info, filename)
	if err != nil {
		return nil, err
	}
	info.filename = filename
	return &info, nil
}
