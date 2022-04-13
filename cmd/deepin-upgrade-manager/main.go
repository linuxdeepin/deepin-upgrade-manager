package main

import (
	"deepin-upgrade-manager/pkg/config"
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/repo"
	"deepin-upgrade-manager/pkg/module/repo/branch"
	"deepin-upgrade-manager/pkg/module/single"
	"deepin-upgrade-manager/pkg/upgrader"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	_ACTION_INIT     = "init"
	_ACTION_COMMIT   = "commit"
	_ACTION_ROLLBACK = "rollback"
	_ACTION_SNAPSHOT = "snapshot"
	_ACTION_LIST     = "list"
)

var (
	_config  = flag.String("config", "/persistent/osroot/config.json", "the configuration file path")
	_action  = flag.String("action", "commit", "the available actions: init, commit, rollback, list")
	_version = flag.String("version", "", "the version which rollback")
	_rootDir = flag.String("root", "/", "the rootfs mount point")
)

func main() {
	flag.Parse()

	conf, err := config.LoadConfig(*_config)
	if err != nil {
		fmt.Println("load config wrong:", err)
		os.Exit(-1)
	}
	err = conf.Prepare()
	if err != nil {
		fmt.Println("config prepare wrong:", err)
		os.Exit(-1)
	}
	if len(*_rootDir) == 1 {
		logger.NewLogger("deepin-upgrade-manager", false)
	} else {
		logger.NewLogger("deepin-upgrade-manager", true)
	}
	operator, err := upgrader.NewUpgrader(conf,
		*_rootDir, "/proc/self/mounts")
	if err != nil {
		fmt.Println("new repo operator:", err)
		os.Exit(-1)
	}
	switch *_action {
	case _ACTION_INIT:
		logger.Info("start initialize a new empty repo")
		err = operator.Init()
		if err != nil {
			logger.Error("init repo failed:", err)
			os.Exit(-1)
		}
		*_version = branch.GenInitName(conf.Distribution)
		fallthrough
	case _ACTION_COMMIT:
		if !single.SetSingleInstance() {
			fmt.Println("process already exists")
			os.Exit(-1)
		}
		if len(*_version) == 0 {
			*_version, err = generateBranchName(conf)
			if err != nil {
				fmt.Println("generate version failed:", err)
				os.Exit(-1)
			}
		}
		logger.Info("the version number of this submission is:", *_version)
		err = operator.Commit(*_version, fmt.Sprintf("Release %s", *_version), true)
		if err != nil {
			fmt.Println("commit failed:", err)
			os.Exit(-1)
		}
		logger.Info("ending commit a new version")
	case _ACTION_ROLLBACK:
		if !single.SetSingleInstance() {
			fmt.Println("process already exists")
			os.Exit(-1)
		}
		logger.Info("start rollback a old version:", *_version)
		if len(*_version) == 0 {
			logger.Error("Must special version")
			os.Exit(-1)
		}
		// NOTICE(jouyouyun): must ensure the partition which in fstab had mounted.
		err = operator.Rollback(*_version)
		if err != nil {
			logger.Errorf("rollback %q: %v", *_version, err)
			os.Exit(-1)
		}
		logger.Info("end rollback a old version:", *_version)
	case _ACTION_SNAPSHOT:
		if len(*_version) == 0 {
			logger.Error("Must special version")
			os.Exit(-1)
		}
		err = operator.Snapshot(*_version, true)
		if err != nil {
			logger.Errorf("snapshot %q: %v", *_version, err)
			os.Exit(-1)
		}
		return
	case _ACTION_LIST:
		verList, err := listVersion(conf)
		if err != nil {
			logger.Error("list version:", err)
			os.Exit(-1)
		}
		fmt.Printf("ActiveVersion:%s\n", conf.ActiveVersion)
		fmt.Printf("AvailVersionList:%s\n", strings.Join(verList, " "))
		return
	}

	conf.ActiveVersion = *_version
	err = conf.Save()
	if err != nil {
		logger.Infof("update version to %q: %v", *_version, err)
	}
}

func generateBranchName(conf *config.Config) (string, error) {
	name := conf.ActiveVersion
	if len(name) != 0 {
		newName, err := branch.Increment(name)
		if err == nil {
			return newName, nil
		}
	}

	if len(conf.RepoList) != 1 {
		handler, err := repo.NewRepo(repo.REPO_TY_OSTREE,
			conf.RepoList[0].Repo)
		if err != nil {
			return "", err
		}
		name, err = handler.Last()
		if err != nil {
			return "", err
		}
		return branch.Increment(name)
	}

	return branch.GenInitName(conf.Distribution), nil
}

func listVersion(conf *config.Config) ([]string, error) {
	if len(conf.RepoList) == 0 {
		return nil, nil
	}

	handler, err := repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(*_rootDir, conf.RepoList[0].Repo))
	if err != nil {
		return nil, err
	}
	return handler.List()
}
