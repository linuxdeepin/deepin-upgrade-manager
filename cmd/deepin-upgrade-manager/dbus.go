// SPDX-FileCopyrightText: 2018 - 2023 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"deepin-upgrade-manager/pkg/logger"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/godbus/dbus"
	"github.com/godbus/dbus/introspect"
	"github.com/godbus/dbus/prop"
)

const (
	dbusDest            = "org.deepin.AtomicUpgrade1"
	dbusPath            = "/org/deepin/AtomicUpgrade1"
	dbusIFC             = dbusDest
	dbusSigStateChanged = "StateChanged"
)

func (m *Manager) setupDBus() error {
	err := m.conn.Export(m, dbusPath, dbusIFC)
	if err != nil {
		return err
	}
	props := prop.New(m.conn, dbusPath, m.makeProps())
	node := &introspect.Node{
		Name: dbusDest,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name:       dbusIFC,
				Methods:    introspect.Methods(m),
				Properties: props.Introspection(dbusIFC),
				Signals: []introspect.Signal{
					{
						Name: dbusSigStateChanged,
						Args: []introspect.Arg{
							{Name: "op", Type: "i"},
							{Name: "state", Type: "i"},
							{Name: "target", Type: "s"},
							{Name: "desc", Type: "s"},
						},
					},
				},
			},
		},
	}
	err = m.conn.Export(introspect.NewIntrospectable(node), dbusPath,
		"org.freedesktop.DBus.Introspectable")
	if err != nil {
		return err
	}

	reply, err := m.conn.RequestName(dbusDest, dbus.NameFlagDoNotQueue)
	if err != nil {
		return err
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("service %q has owned", dbusDest)
	}
	return nil
}

func (m *Manager) makeProps() map[string]map[string]*prop.Prop {
	return map[string]map[string]*prop.Prop{
		dbusIFC: {
			"ActiveVersion": &prop.Prop{
				Value:    &m.ActiveVersion,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					logger.Debugf("ActiveVersion changed: %s -> %s", c.Name, c.Value)
					return nil
				},
			},
			"RepoUUID": &prop.Prop{
				Value:    &m.RepoUUID,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					logger.Debugf("RepoUUID changed: %s -> %s", c.Name, c.Value)
					return nil
				},
			},
			"DefaultConfig": &prop.Prop{
				Value:    &m.DefaultConfig,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					logger.Debugf("DefaultConfig changed: %s -> %s", c.Name, c.Value)
					return nil
				},
			},
			"Running": &prop.Prop{
				Value:    &m.running,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					logger.Debugf("Running changed: %s -> %s", c.Name, c.Value)
					return nil
				},
			},
		},
	}
}

func (m *Manager) Quit() {
	if err := m.conn.Close(); err != nil {
		logger.Warningf("error closing file: %v", err)
	}
	close(m.quit)
}

func (m *Manager) Wait() {
	m.quit = make(chan struct{})
	if m.quitCheckInterval > 0 {
		go func() {
			ticker := time.NewTicker(m.quitCheckInterval)
			for {
				select {
				case <-m.quit:
					return
				case <-ticker.C:
					m.mu.RLock()
					hasCall := m.hasCall
					m.mu.RUnlock()
					if !hasCall {
						if m.canQuit() {
							m.Quit()
							return
						}
					} else {
						m.mu.Lock()
						m.hasCall = false
						m.mu.Unlock()
					}
				}
			}
		}()
	}
	<-m.quit
}

func (m *Manager) SetAutoQuitHandler(interval time.Duration) {
	m.quitCheckInterval = interval
}

func (m *Manager) canQuit() bool {
	m.mu.Lock()
	running := m.running
	m.mu.Unlock()
	return !running
}

func (m *Manager) DelayAutoQuit() {
	m.mu.Lock()
	m.hasCall = true
	m.mu.Unlock()
}

func (m *Manager) listenQuit() {
	c := make(chan os.Signal)
	//监听指定信号 ctrl+c kill
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		for s := range c {
			logger.Debugf("signal receiving system: %v", s)
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				m.upgrade.SendingExitSignal(m.emitStateChanged)
				time.Sleep(1 * time.Second)
				os.Exit(0)
			default:

			}
		}
	}()
}
