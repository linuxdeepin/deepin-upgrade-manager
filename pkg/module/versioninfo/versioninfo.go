package versioninfo

import (
	"deepin-upgrade-manager/pkg/module/dirinfo"
	"deepin-upgrade-manager/pkg/module/generator"
	"deepin-upgrade-manager/pkg/module/util"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
)

type VersionInfo struct {
	Version     string `json:"version"`
	Kernel      string `json:"kernel"`
	Initrd      string `json:"initrd"`
	Scheme      string `json:"scheme"`
	DisplayInfo string `json:"display"`
}

type VersionInfos []*VersionInfo

type VersionInfoList struct {
	VersionList VersionInfos `json:"version_list"`
}

const (
	BOOT_SNAPSHOT_DIR = "/boot/snapshot"
	SCHEME            = "atomic"
	DEEPIN_BOOT_KIT   = "/usr/sbin/deepin-boot-kit"
)

func (infolist VersionInfoList) ToJson() string {
	b, _ := json.Marshal(&infolist)
	return string(b)
}

func (list VersionInfos) Less(i, j int) bool {
	return generator.Less(list[i].Version, list[j].Version)

}

func (infolist VersionInfoList) Sort() VersionInfoList {
	list := infolist
	sort.SliceStable(list.VersionList, func(i, j int) bool {
		return list.VersionList.Less(i, j)
	})
	return list
}

func NewVersion() (string, error) {
	action := string("--action=") + "version"
	out, err := util.ExecCommandWithOut(DEEPIN_BOOT_KIT, []string{action})
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(out))
	return version, nil
}

func (infolist *VersionInfoList) SetVersionName(version, display string) {
	for _, v := range infolist.VersionList {
		if v.Version == version {
			v.DisplayInfo = display
		}
	}
}

func Load(versionList []string) VersionInfoList {
	var infolist VersionInfoList
	var isAcrossPart bool

	rootPartition, _ := dirinfo.GetDirPartition("/")
	bootPartition, _ := dirinfo.GetDirPartition("/boot")
	if bootPartition == rootPartition {
		isAcrossPart = false
	} else {
		isAcrossPart = true
	}
	for _, v := range versionList {
		var info VersionInfo
		bootDir := filepath.Join(BOOT_SNAPSHOT_DIR, v)
		fiList, err := ioutil.ReadDir(bootDir)
		if err != nil {
			continue
		}
		var vmlinux, initrd string
		for _, fi := range fiList {
			if fi.IsDir() {
				continue
			}
			if strings.HasPrefix(fi.Name(), "vmlinuz-") ||
				strings.HasPrefix(fi.Name(), "kernel-") ||
				strings.HasPrefix(fi.Name(), "vmlinux-") {
				vmlinux = filepath.Join(bootDir, fi.Name())
			}
			if strings.HasPrefix(fi.Name(), "initrd.img-") {
				initrd = filepath.Join(bootDir, fi.Name())
			}
		}
		if len(vmlinux) != 0 && len(initrd) != 0 {
			if isAcrossPart {
				info.Initrd = strings.TrimPrefix(initrd, "/boot")
				info.Kernel = strings.TrimPrefix(vmlinux, "/boot")
			} else {
				info.Initrd = initrd
				info.Kernel = vmlinux
			}
			info.Version = v
			info.Scheme = SCHEME
		} else {
			continue
		}
		infolist.VersionList = append(infolist.VersionList, &info)
	}
	return infolist
}
