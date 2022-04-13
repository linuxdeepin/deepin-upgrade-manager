package mountinfo

import (
	"bufio"
	"deepin-upgrade-manager/pkg/logger"
	"os"
	"strings"
)

const (
	_MOUNTS_DELIM = " "

	_MOUNTS_ITEM_NUM = 6
)

type MountInfo struct {
	Partition  string
	MountPoint string
	FSType     string
	Options    string
}
type MountInfoList []*MountInfo

func (infos MountInfoList) Query(dir string) MountInfoList {
	var list MountInfoList
	for _, info := range infos {
		if !strings.HasPrefix(info.MountPoint, dir) {
			continue
		}
		list = append(list, info)
	}
	return list
}

func Load(filename string) (MountInfoList, error) {
	fr, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fr.Close()

	var infos MountInfoList
	scanner := bufio.NewScanner(fr)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		items := strings.Split(line, _MOUNTS_DELIM)
		if len(items) != _MOUNTS_ITEM_NUM {
			logger.Debug("invalid mounts line:", line)
			continue
		}
		if isFilterFSType(items[2]) {
			continue
		}
		infos = append(infos, &MountInfo{
			Partition:  items[0],
			MountPoint: items[1],
			FSType:     items[2],
			Options:    items[3],
		})
	}
	return infos, nil
}

func isFilterFSType(ty string) bool {
	list := []string{
		"proc",
		"sysfs",
		"devpts",
		"devtmpfs",
		"tmpfs",
		"efivarfs",
		"securityfs",
		"cgroup",
		"cgroup2",
		"pstore",
		"bpf",
		"autofs",
		"hugetlbfs",
		"mqueue",
		"debugfs",
		"tracefs",
		"configfs",
		"ramfs",
		"fusectl",
		"fuse.gvfsd-fuse",
		"fuse.portal",
		"binfmt_misc",
	}
	for _, v := range list {
		if v == ty {
			return true
		}
	}
	return false
}
