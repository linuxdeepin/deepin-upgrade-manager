module github.com/linuxdeepin/deepin-upgrade-manager

go 1.15

replace github.com/linuxdeepin/go-lib => github.com/Decodetalkers/go-lib v0.0.0-20230207102150-285b65f72371

replace github.com/linuxdeepin/go-dbus-factory => github.com/Decodetalkers/go-dbus-factory v0.0.0-20230214081229-2794c96a723b

require (
	github.com/godbus/dbus/v5 v5.1.0
	gopkg.in/yaml.v2 v2.4.0
)
