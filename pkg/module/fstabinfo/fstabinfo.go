package fstabinfo

import (
	"bufio"
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/diskinfo"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MountInfo struct {
	Partition  string
	MountPoint string
	FSType     string
	Options    string
}
type MountInfoList []*MountInfo

func getPartition(spec, rootDir string) (string, error) {
	bits := strings.Split(spec, "=")
	var specValue string
	var dsInfo *diskinfo.DiskID
	if len(bits) == 1 {
		specValue = spec
	} else {
		specValue = bits[1]
	}
	diskDir := filepath.Join(rootDir, "dev/disk")
	dsInfos, err := diskinfo.Load(diskDir)
	if err != nil {
		return "", err
	}
	switch strings.ToUpper(bits[0]) {
	case "UUID":
		dsInfo = dsInfos.MatchUUID(specValue)
	case "LABEL":
		dsInfo = dsInfos.MatchLabel(specValue)
	case "PARTUUID":
		dsInfo = dsInfos.MatchPartUUID(specValue)
	case "PARTLABEL":
		dsInfo = dsInfos.MatchPartLabel(specValue)
	default:
		dsInfo = dsInfos.MatchPartition(specValue)
	}
	if nil == dsInfo {
		return "", fmt.Errorf("cannot match the corresponding partition of the disk,specvalue:%s", specValue)
	}
	return dsInfo.Partition, err
}

// /etc/fstab
func Load(filename, rootDir string) (MountInfoList, error) {
	logger.Debugf("load file %s to get mount information", filename)
	fr, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fr.Close()
	var infos MountInfoList
	scanner := bufio.NewScanner(fr)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if (line == "") || (line[0] == '#') {
			continue
		}
		items := strings.Fields(line)
		if len(items) < 4 {
			logger.Warningf("too few fields (%d), at least 4 are expected", len(items))
			continue
		} else {
			if items[1] == "none" {
				continue
			}
			partition, err := getPartition(items[0], rootDir)
			if err != nil {
				logger.Warning("failed get partiton, err:", err)
				continue
			}
			infos = append(infos, &MountInfo{
				Partition:  partition,
				MountPoint: items[1],
				FSType:     items[2],
				Options:    items[3],
			})
		}
	}
	return infos, nil
}
