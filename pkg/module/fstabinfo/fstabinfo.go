package fstabinfo

import (
	"bufio"
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/dirinfo"
	"deepin-upgrade-manager/pkg/module/diskinfo"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FsInfo struct {
	SrcPoint  string
	DestPoint string
	FSType    string
	Options   string
	Bind      bool
}

type FsInfoList []*FsInfo

func (fs FsInfoList) MaxFreePartitionPoint() string {
	var maxFree uint64
	var point string
	for _, v := range fs {
		free, err := dirinfo.GetPartitionFreeSize(v.DestPoint)
		if err != nil {
			logger.Warningf("failed get par:%s size, err: %v", v.DestPoint, err)
			continue
		}
		if maxFree < free {
			maxFree = free
			point = v.DestPoint
		}
	}
	return point
}

func getPartiton(spec string, dsInfos diskinfo.DiskIDList) (string, error) {
	bits := strings.Split(spec, "=")
	var specvalue string
	var dsInfo *diskinfo.DiskID
	if len(bits) == 1 {
		specvalue = spec
	} else {
		specvalue = bits[1]
	}

	switch strings.ToUpper(bits[0]) {
	case "UUID":
		dsInfo = dsInfos.MatchUUID(specvalue)
	case "LABEL":
		dsInfo = dsInfos.MatchLabel(specvalue)
	case "PARTUUID":
		dsInfo = dsInfos.MatchPartUUID(specvalue)
	case "PARTLABEL":
		dsInfo = dsInfos.MatchPartLabel(specvalue)
	default:
		dsInfo = dsInfos.MatchPartition(specvalue)
	}
	if nil == dsInfo {
		return "", fmt.Errorf("cannot match the corresponding partition of the disk,specvalue:%s", specvalue)
	}
	return dsInfo.Partition, nil
}

func parseOptions(optionsString string) (options map[string]string) {
	options = make(map[string]string)
	var key, value string
	for i, option := range strings.Split(optionsString, ",") {
		if i == 0 {
			key = option
		} else {
			value += option
		}
	}
	options[key] = value
	return
}

// /etc/fstab
func Load(filename, rootDir string) (FsInfoList, error) {
	logger.Debugf("load file %s to get mount information", filename)
	fr, err := os.Open(filepath.Clean(filename))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := fr.Close(); err != nil {
			logger.Warningf("error closing file: %v", err)
		}
	}()
	var infos FsInfoList
	dsInfos, err := diskinfo.Load("/dev/disk")
	if err != nil {
		return infos, err
	}
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
			var srcMountPoint string
			var isBind bool
			if items[1] == "none" {
				continue
			}
			optionsMap := parseOptions(items[3])
			if items[3] == "bind" || optionsMap["defaults"] == "bind" {
				srcMountPoint = filepath.Join(rootDir, items[0])
				isBind = true
			} else {
				srcMountPoint, err = getPartiton(items[0], dsInfos)
				if err != nil {
					logger.Warning("failed get mount point, err:", err)
					return infos, err
				}
				isBind = false
			}

			infos = append(infos, &FsInfo{
				SrcPoint:  srcMountPoint,
				DestPoint: items[1],
				FSType:    items[2],
				Options:   items[3],
				Bind:      isBind,
			})
		}
	}
	return infos, nil
}
