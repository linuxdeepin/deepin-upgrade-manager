package notify

import (
	"github.com/godbus/dbus"
)

const (
	NodifydbusDest      = "com.deepin.dde.Notification"
	NodifydbusPath      = "/com/deepin/dde/Notification"
	NodifydbusInterface = NodifydbusDest
)

func SetNotifyText(text string) error {
	sysBus, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	grubServiceObj := sysBus.Object(NodifydbusDest,
		NodifydbusPath)
	metho := NodifydbusInterface + ".Notify"
	var arg0 string
	var arg1 uint32
	var arg2 string
	var arg3 string
	var arg4 string
	var arg5 []string
	var map_variable map[string]dbus.Variant
	var arg7 int32
	arg0 = "deepin-upgrade-manager"
	arg1 = 101
	arg2 = "preferences-system"
	arg3 = text
	arg7 = 10000
	return grubServiceObj.Call(metho, 0, arg0, arg1, arg2, arg3, arg4, arg5, map_variable, arg7).Store()
}
