package main

import (
	config "deepin-upgrade-manager/pkg/config/upgrader"
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/bootkitinfo"
	"deepin-upgrade-manager/pkg/module/repo/branch"
	"deepin-upgrade-manager/pkg/module/single"
	"deepin-upgrade-manager/pkg/module/util"
	"deepin-upgrade-manager/pkg/upgrader"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus"
)

type Manager struct {
	conn    *dbus.Conn
	upgrade *upgrader.Upgrader

	mu                sync.RWMutex
	quit              chan struct{}
	quitCheckInterval time.Duration

	running       bool
	hasCall       bool
	ActiveVersion string
	RepoUUID      string
	DefaultConfig string
}

func NewManager(config *config.Config, daemon bool) (*Manager, error) {
	upgrade, err := upgrader.NewUpgrader(config,
		*_rootDir)
	if err != nil {
		logger.Fatal("Failed to new upgrade:", err)
		return nil, err
	}

	_, uuid, err := upgrade.RepoMountpointAndUUID()
	if err != nil {
		uuid = ""
	}
	var m = &Manager{
		upgrade:       upgrade,
		ActiveVersion: config.ActiveVersion,
		running:       false,
		RepoUUID:      uuid,
		DefaultConfig: upgrade.ReadyDataPath(),
	}

	if daemon {
		conn, err := dbus.SystemBus()
		if err != nil {
			logger.Fatal("Failed to connect dbus:", err)
			return nil, err
		}
		m.conn = conn
	}

	return m, nil
}

func (m *Manager) emitStateChanged(op, state int32, target, desc string) {
	err := m.conn.Emit(dbusPath, dbusIFC+"."+dbusSigStateChanged,
		op, state, target, desc)
	if err != nil {
		logger.Warning("Failed to emit 'StateChanged':", err, op, state, desc)
	}
}

func (m *Manager) ListVersion() ([]string, *dbus.Error) {
	vers, _, err := m.upgrade.ListVersion()
	if err != nil {
		logger.Error("Failed to list version:", err)
		return nil, dbus.MakeFailedError(err)
	}
	return vers, nil
}

func (m *Manager) SetRepoMount(repomount string) *dbus.Error {
	config, err := m.upgrade.SetRepoMount(repomount)
	if err != nil {
		logger.Error("Failed to list version:", err)
		return dbus.MakeFailedError(err)
	}
	logger.Debugf("restart to obtain version %s", config.ActiveVersion)
	m.ActiveVersion = config.ActiveVersion
	upgrade, err := upgrader.NewUpgrader(config,
		*_rootDir)
	if err != nil {
		logger.Debugf("%v", err)
	}
	m.upgrade = upgrade
	return nil
}

func (m *Manager) CancelRollback(sender dbus.Sender) *dbus.Error {
	if !single.SetSingleInstance() {
		return dbus.MakeFailedError(errors.New("process already exists"))
	}

	go func() {
		m.DelayAutoQuit()
		m.mu.Lock()
		m.running = true
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			single.Remove()
		}()
		envVars, _ := getLocaleEnvVarsWithSender(m.conn, sender)
		m.upgrade.ResetGrub(envVars)
	}()
	return nil
}

func (m *Manager) Rollback(version string) *dbus.Error {
	if !single.SetSingleInstance() {
		return dbus.MakeFailedError(errors.New("process already exists"))
	}
	go func() {
		m.DelayAutoQuit()
		m.mu.Lock()
		m.running = true
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			single.Remove()
		}()
		exitCode, err := m.upgrade.Rollback(version, m.emitStateChanged)
		if err != nil {
			logger.Errorf("failed to rollback upgrade, err: %v, exit code: %d", err, exitCode)
			return
		}
	}()
	return nil
}

func (m *Manager) Commit(subject string, sender dbus.Sender) *dbus.Error {
	if !single.SetSingleInstance() {
		return dbus.MakeFailedError(errors.New("process already exists"))
	}
	go func() {
		m.DelayAutoQuit()
		m.mu.Lock()
		m.running = true
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			single.Remove()
		}()
		var version string
		var err error
		if !m.upgrade.IsExistRepo() {
			m.upgrade.Init()
			version, err = bootkitinfo.NewVersion()
			if err != nil {
				version = branch.GenInitName(m.upgrade.DistributionName())
			}
		}
		envVars, _ := getLocaleEnvVarsWithSender(m.conn, sender)
		exitCode, err := m.upgrade.Commit(version, subject, true, envVars, m.emitStateChanged)
		if err != nil {
			logger.Errorf("failed to commit version, err: %v, exit code: %d:", err, exitCode)
			return
		}
		logger.Info("ending commit a new version")
	}()
	return nil
}

func (m *Manager) Delete(version string) *dbus.Error {
	if !single.SetSingleInstance() {
		return dbus.MakeFailedError(errors.New("process already exists"))
	}
	go func() {
		m.DelayAutoQuit()
		m.mu.Lock()
		m.running = true
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			single.Remove()
		}()
		exitCode, err := m.upgrade.Delete(version, m.emitStateChanged)
		if err != nil {
			logger.Errorf("failed to delete version, err: %v, exit code: %d:", err, exitCode)
			return
		}
		logger.Info("ending delete a new version")
	}()
	return nil
}

func (m *Manager) QuerySubject(versions []string) ([]string, *dbus.Error) {
	var subjects []string

	if len(versions) == 0 {
		logger.Error("must special version")
		return nil, dbus.MakeFailedError(errors.New("must special version"))
	}
	for _, v := range versions {
		sub, err := m.upgrade.Subject(v)
		if err != nil {
			logger.Warningf("Failed to get %s subject, err:%v", v, err)
		}
		subjects = append(subjects, sub)
	}
	return subjects, nil
}

func (m *Manager) GetGrubTitle(versions string) (string, *dbus.Error) {
	if len(versions) == 0 {
		logger.Error("must special version")
		return "", dbus.MakeFailedError(errors.New("must special version"))
	}
	return m.upgrade.GrubTitle(versions), nil
}

func (m *Manager) SetDefaultConfig(path string) *dbus.Error {
	if !util.IsExists(path) {
		logger.Errorf("%s does not exist.", path)
		return dbus.MakeFailedError(fmt.Errorf("%s does not exist", path))
	}
	err := m.upgrade.SetReadyData(path)
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}
