package dirinfo

import (
	"deepin-upgrade-manager/pkg/module/util"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Node struct {
	Path     string
	Children []*Node
}

var DirTree []Node

func GetSubDirList(list []string, rootdir string) ([]string, error) {
	if len(list) == 1 {
		return list, nil
	}
	rootPartition, err := GetDirPartition(rootdir)
	if err != nil {
		return list, err
	}
	var subList []string
	for _, l := range list {
		partition, err := GetDirPartition(l)
		if err != nil {
			continue
		}
		if l == rootdir {
			continue
		}
		if rootPartition != partition {
			subList = append(subList, l)
		}
	}
	return subList, nil
}

func GetDirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size
}

func GetPartitionFreeSize(dirPath string) (uint64, error) {
	out, err := util.ExecCommandWithOut("df", []string{dirPath})
	arrLine := strings.Split(string(out), "\n")
	if len(arrLine) < 2 {
		return 0, errors.New("failed get dir parttiton")
	}
	arrCmd := strings.Fields(arrLine[1])
	if err != nil {
		return 0, err
	}
	partition := strings.TrimSpace(arrCmd[3])
	size, err := strconv.Atoi(partition)
	if err != nil {
		return 0, err
	}
	return uint64(size * 1024), nil
}

func GetDirPartition(dirPath string) (string, error) {
	out, err := util.ExecCommandWithOut("df", []string{dirPath})
	arrLine := strings.Split(string(out), "\n")
	if len(arrLine) < 2 {
		return "", errors.New("failed get dir parttiton")
	}

	arrCmd := strings.Split(arrLine[1], " ")
	if err != nil {
		return "", err
	}
	partition := strings.TrimSpace(arrCmd[0])
	return partition, nil
}
