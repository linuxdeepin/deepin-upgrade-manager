// SPDX-FileCopyrightText: 2018 - 2023 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	config "deepin-upgrade-manager/pkg/config/upgrader"
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/bootkitinfo"
	"deepin-upgrade-manager/pkg/module/process"
	"deepin-upgrade-manager/pkg/module/repo/branch"
	"deepin-upgrade-manager/pkg/module/single"
	"deepin-upgrade-manager/pkg/module/util"
	"deepin-upgrade-manager/pkg/upgrader"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	_ACTION_INIT     = "init"
	_ACTION_COMMIT   = "commit"
	_ACTION_ROLLBACK = "rollback"
	_ACTION_SNAPSHOT = "snapshot"
	_ACTION_BOOTLIST = "bootlist"
	_ACTION_LIST     = "list"
	_ACTION_DELETE   = "delete"
	_ACTION_SUBJECT  = "subject"
	_ACTION_CANCEL   = "cancel"
	_ACTION_SET      = "setdefaultconfig"
)

const (
	FAILED_PROCESS_EXISTS = -255
	FAILED_VERSION_EXISTS = -256
)

var (
	_config  = flag.String("config", "/etc/deepin-upgrade-manager/config.json", "the repo config file path")
	_data    = flag.String("data", "/etc/deepin-upgrade-manager/ready/data.yaml", "the deepin v23 commit data config file path")
	_action  = flag.String("action", "list", "the available actions: init, commit, rollback, list, cancel, setdefaultconfig")
	_version = flag.String("version", "", "the version which rollback")
	_rootDir = flag.String("root", "/", "the rootfs mount point")
	_daemon  = flag.Bool("daemon", false, "start dbus service")
	_subject = flag.String("subject", "", "the commit subject")
)

func main() {
	flag.Parse()

	conf, err := config.LoadConfig(*_config, *_rootDir)
	if err != nil {
		fmt.Println("load config wrong:", err)
		os.Exit(-1)
	}
	if len(*_rootDir) == 1 {
		logger.NewLogger("deepin-upgrade-manager", false)
	} else {
		logger.NewLogger("deepin-upgrade-manager", true)
	}

	if os.Geteuid() != 0 {
		logger.Info("Must run with privileged user")
		os.Exit(-1)
	}
	err = util.FixEnvPath()
	if err != nil {
		logger.Warning("Failed to setenv:", err)
	}
	m, err := NewManager(conf, *_daemon)
	if err != nil {
		logger.Fatal("Failed to setup dbus:", err)
		os.Exit(-1)
	}
	if *_daemon {
		logger.Info("start running dbus service")
		err = m.setupDBus()
		if err != nil {
			logger.Fatal("Failed to setup dbus:", err)
			os.Exit(-1)
		}
		m.SetAutoQuitHandler(30 * time.Second)
		m.Wait()
		return
	}
	handleAction(m.upgrade, conf)
}

func handleAction(m *upgrader.Upgrader, c *config.Config) {
	var err error
	var exitCode int
	switch *_action {
	case _ACTION_INIT:
		logger.Info("start initialize a new empty repo")
		exitCode, err = m.Init()
		if err != nil {
			logger.Error("init repo failed:", err)
			os.Exit(exitCode)
		}
		*_version, err = bootkitinfo.NewVersion()
		if err != nil {
			*_version = branch.GenInitName(c.Distribution)
		}
		fallthrough
	case _ACTION_COMMIT:
		if !single.SetSingleInstance() {
			logger.Error("process already exists")
			os.Exit(FAILED_PROCESS_EXISTS)
		}
		exitCode, err = m.Commit("", *_version, *_subject, true, nil)
		if err != nil {
			logger.Error("commit failed:", err)
			os.Exit(exitCode)
		}
		single.Remove()
		logger.Info("ending commit a new version")
	case _ACTION_ROLLBACK:
		if !single.SetSingleInstance() {
			logger.Error("process already exists")
			os.Exit(FAILED_PROCESS_EXISTS)
		}
		exitCode, err = m.Rollback(*_version, nil)
		if err != nil {
			logger.Errorf("rollback %q: %v", *_version, err)
			os.Exit(exitCode)
		}
		single.Remove()
	case _ACTION_SNAPSHOT:
		if len(*_version) == 0 {
			logger.Error("must special version")
			os.Exit(FAILED_VERSION_EXISTS)
		}
		exCode, err := m.EnableBoot(*_version)
		if err != nil {
			logger.Errorf("snapshot %q: %v", *_version, err)
			os.Exit(int(exCode))
		}
	case _ACTION_BOOTLIST:
		// close log
		// logger.Disable()
		versionInfo, exCode, err := m.EnableBootList()
		if err != nil {
			logger.Error("failed enable boot list, err:", err)
			os.Exit(int(exCode))
		}
		fmt.Println(versionInfo)
	case _ACTION_LIST:
		verList, exitCode, err := m.ListVersion()
		if err != nil {
			logger.Error("list version:", err)
			os.Exit(exitCode)
		}
		fmt.Printf("ActiveVersion:%s\n", c.ActiveVersion)
		fmt.Printf("AvailVersionList:%s\n", strings.Join(verList, " "))
	case _ACTION_DELETE:
		if !single.SetSingleInstance() {
			logger.Error("process already exists")
			os.Exit(FAILED_PROCESS_EXISTS)
		}
		exitCode, err := m.Delete(*_version, nil)
		if err != nil {
			logger.Error("failed delete version:", err)
			os.Exit(exitCode)
		}
	case _ACTION_SUBJECT:
		if len(*_version) == 0 {
			logger.Error("must special version")
			os.Exit(FAILED_VERSION_EXISTS)
		}
		sub, err := m.Subject(*_version)
		if err != nil {
			logger.Error(err)
			os.Exit(FAILED_VERSION_EXISTS)
		}
		fmt.Println(sub)
	case _ACTION_CANCEL:
		if !single.SetSingleInstance() {
			logger.Error("process already exists")
			os.Exit(FAILED_PROCESS_EXISTS)
		}
		m.ResetGrub()
	case _ACTION_SET:
		if !util.IsExists(*_data) {
			logger.Error("data isn't exist")
			os.Exit(FAILED_PROCESS_EXISTS)
		}
		err := m.SetReadyData(*_data)
		if err != nil {
			logger.Error(err)
		}
	}
}

func getLocaleEnvVarsWithSender(conn *dbus.Conn, sender dbus.Sender) ([]string, error) {
	var result []string
	var pid uint32
	err := conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixProcessID",
		0, string(sender)).Store(&pid)
	if err != nil {
		return result, err
	}

	if err != nil {
		return nil, err
	}

	p := process.Process(pid)
	environ, err := p.Environ()
	if err != nil {
		return nil, err
	} else {
		v, ok := environ.Lookup("LANG")
		if ok {
			result = append(result, "LANG="+v)
		}
		v, ok = environ.Lookup("LANGUAGE")
		if ok {
			result = append(result, "LANGUAGE="+v)
		}
	}
	return result, nil
}
