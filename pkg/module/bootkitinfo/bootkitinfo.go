package bootkitinfo

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

type BootInfo struct {
	Version     string `json:"version"`
	Kernel      string `json:"kernel"`
	Initrd      string `json:"initrd"`
	Scheme      string `json:"scheme"`
	DisplayInfo string `json:"display"`
}

type BootInfos []*BootInfo

type BootInfoList struct {
	VersionList BootInfos `json:"version_list"`
}

const (
	BOOT_SNAPSHOT_DIR = "/boot/snapshot"
	SCHEME            = "atomic"
	DEEPIN_BOOT_KIT   = "/usr/sbin/deepin-boot-kit"
)

func (infolist BootInfoList) ToJson() string {
	b, _ := json.Marshal(&infolist)
	return string(b)
}

func (list BootInfos) Less(i, j int) bool {
	return generator.Less(list[i].Version, list[j].Version)
}

func (infolist BootInfoList) Sort() BootInfoList {
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

func Update() error {
	action := string("--action=") + "update"
	err := util.ExecCommand(DEEPIN_BOOT_KIT, []string{action})
	if err != nil {
		return err
	}
	return nil
}

func (infolist *BootInfoList) SetVersionName(version, display string) {
	for _, v := range infolist.VersionList {
		if v.Version == version {
			v.DisplayInfo = display
		}
	}
}

func Load(versionList []string) BootInfoList {
	var infolist BootInfoList
	var isAcrossPart bool

	rootPartition, _ := dirinfo.GetDirPartition("/")
	bootPartition, _ := dirinfo.GetDirPartition("/boot")
	if bootPartition == rootPartition {
		isAcrossPart = false
	} else {
		isAcrossPart = true
	}
	for _, v := range versionList {
		var info BootInfo
		bootDir := filepath.Join(BOOT_SNAPSHOT_DIR, v)
		fiList, err := ioutil.ReadDir(bootDir)
		if err != nil {
			continue
		}
		var vmlinuxPath, vmlinux string
		var initrdPaths []string

		for _, fi := range fiList {
			if fi.IsDir() {
				continue
			}
			if strings.HasPrefix(fi.Name(), "vmlinuz-") ||
				strings.HasPrefix(fi.Name(), "kernel-") ||
				strings.HasPrefix(fi.Name(), "vmlinux-") {
				vmlinuxPath = filepath.Join(bootDir, fi.Name())
				vmlinux = fi.Name()
			}
			if strings.HasPrefix(fi.Name(), "initrd.img-") {
				initrdPaths = append(initrdPaths, filepath.Join(bootDir, fi.Name()))
			}
		}
		if len(vmlinuxPath) != 0 && len(initrdPaths) != 0 {
			index := strings.IndexRune(vmlinux, '-')
			var initrdPath string
			for _, v := range initrdPaths {
				if strings.HasSuffix(v, vmlinux[index:]) {
					initrdPath = v
					break
				}
			}
			if isAcrossPart {
				info.Initrd = strings.TrimPrefix(initrdPath, "/boot")
				info.Kernel = strings.TrimPrefix(vmlinuxPath, "/boot")
			} else {
				info.Initrd = initrdPath
				info.Kernel = vmlinuxPath
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
