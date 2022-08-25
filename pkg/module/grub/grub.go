package grub

import (
	"time"

	"github.com/godbus/dbus"
)

const (
	dbusDest      = "com.deepin.daemon.Grub2"
	dbusPath      = "/com/deepin/daemon/Grub2"
	dbusInterface = dbusDest
)

const (
	editAuthDBusPath      = dbusPath + "/EditAuthentication"
	editAuthDBusInterface = dbusInterface + ".EditAuthentication"
)

func Join() error {
	ch := make(chan bool)
	go func(ch chan bool) {
		for {
			time.Sleep(100 * time.Millisecond)
			canExit, err := IsUpdating()
			if !canExit || err != nil {
				ch <- true
			}
		}
	}(ch)
	canExit, err := IsUpdating()
	if canExit && nil == err {
		ticker := time.NewTicker(3 * time.Minute)
		for {
			select {
			case <-ticker.C:
				return nil
			case <-ch:
				return nil
			}
		}
	}
	return nil
}

func SetTimeout(timeout uint32) error {
	sysBus, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	grubServiceObj := sysBus.Object(dbusDest,
		dbusPath)
	metho := dbusDest + ".SetTimeout"

	err = grubServiceObj.Call(metho, 0, timeout).Store()
	if err != nil {
		return err
	}
	return nil
}

func CancelRollback() error {
	sysBus, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	grubServiceObj := sysBus.Object(dbusDest,
		dbusPath)
	metho := dbusDest + ".CancelRollback"

	return grubServiceObj.Call(metho, 0).Store()
}

func TimeOut() (uint32, error) {
	sysBus, err := dbus.SystemBus()
	if err != nil {
		return 0, err
	}
	grubServiceObj := sysBus.Object(dbusDest,
		dbusPath)

	var ret dbus.Variant
	err = grubServiceObj.Call("org.freedesktop.DBus.Properties.Get", 0, dbusInterface, "Timeout").Store(&ret)
	if err != nil {
		return 0, err
	}
	v := ret.Value().(uint32)

	return v, nil
}

func IsUpdating() (bool, error) {
	sysBus, err := dbus.SystemBus()
	if err != nil {
		return false, err
	}
	grubServiceObj := sysBus.Object(dbusDest,
		dbusPath)

	var ret dbus.Variant
	err = grubServiceObj.Call("org.freedesktop.DBus.Properties.Get", 0, dbusInterface, "Updating").Store(&ret)
	if err != nil {
		return false, err
	}
	v := ret.Value().(bool)
	return v, nil
}

func GetEnabledUsers() ([]string, error) {
	var userList []string
	sysBus, err := dbus.SystemBus()
	if err != nil {
		return userList, err
	}

	grubServiceObj := sysBus.Object(dbusDest,
		editAuthDBusPath)

	var ret dbus.Variant
	err = grubServiceObj.Call("org.freedesktop.DBus.Properties.Get", 0, editAuthDBusInterface, "EnabledUsers").Store(&ret)
	if err != nil {
		return userList, err
	}
	v := ret.Value().([]string)
	return v, nil
}
