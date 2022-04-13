package dirinfo

import (
	"deepin-upgrade-manager/pkg/module/util"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

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

func GetPartitionFreeSize(dirPath string) uint64 {
	var stat unix.Statfs_t
	unix.Statfs(dirPath, &stat)
	return stat.Bfree * uint64(stat.Bsize)
}

func GetDirPartition(dirPath string) (string, error) {
	out, err := util.ExecCommandWithOut("df", []string{dirPath})
	arrLine := strings.Split(string(out), "\n")
	arrCmd := strings.Split(arrLine[1], " ")
	if err != nil {
		return "", err
	}
	partition := strings.TrimSpace(arrCmd[0])
	return partition, nil
}
