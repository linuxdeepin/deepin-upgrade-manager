package grub

import (
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

func SetTimeout(timeout uint32) error {
	sysBus, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	grubServiceObj := sysBus.Object(dbusDest,
		dbusPath)
	metho := dbusDest + ".SetTimeout"

	return grubServiceObj.Call(metho, 0, timeout).Store()
}

func Reset() error {
	sysBus, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	grubServiceObj := sysBus.Object(dbusDest,
		dbusPath)
	metho := dbusDest + ".Reset"

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
