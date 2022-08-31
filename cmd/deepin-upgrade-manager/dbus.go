package main

import (
	"deepin-upgrade-manager/pkg/logger"
	"fmt"
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
			"Runing": &prop.Prop{
				Value:    &m.running,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					logger.Debugf("Runing changed: %s -> %s", c.Name, c.Value)
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
