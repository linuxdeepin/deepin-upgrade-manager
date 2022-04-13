package diskinfo

import (
	"deepin-upgrade-manager/pkg/logger"
	"io/ioutil"
	"path/filepath"
)

type DiskID struct {
	Partition string
	Label     string
	Partlabel string
	UUID      string
	PartUUID  string
}

type DiskIDList []*DiskID

const (
	_DISK_UUID      = "by-uuid"
	_DISK_PARTLABEL = "by-partlabel"
	_DISK_LABEL     = "by-label"
	_DISK_PARTUUID  = "by-partuuid"
)

func (infos DiskIDList) MatchPartition(partition string) *DiskID {
	for _, info := range infos {
		if info.Partition == partition {
			return info
		}
	}
	return nil
}

func (infos DiskIDList) MatchUUID(uuid string) *DiskID {
	for _, info := range infos {
		if info.UUID == uuid {
			return info
		}
	}
	return nil
}

func (infos DiskIDList) MatchPartUUID(partuuid string) *DiskID {
	for _, info := range infos {
		if info.PartUUID == partuuid {
			return info
		}
	}
	return nil
}

func (infos DiskIDList) MatchLabel(uuid string) *DiskID {
	for _, info := range infos {
		if info.UUID == uuid {
			return info
		}
	}
	return nil
}

func (infos DiskIDList) MatchPartLabel(partlable string) *DiskID {
	for _, info := range infos {
		if info.Partlabel == partlable {
			return info
		}
	}
	return nil
}

func (infos DiskIDList) handleLoad(dir, filename string) error {
	dirpath := filepath.Join(dir, filename)
	fiList, err := ioutil.ReadDir(dirpath)
	if err != nil {
		return err
	}
	for _, fi := range fiList {
		srcSub := filepath.Join(dirpath, fi.Name())
		path, err := filepath.EvalSymlinks(srcSub)
		if err != nil {
			continue
		}
		diskInfo := infos.MatchPartition(path)
		if diskInfo == nil {
			continue
		}
		switch filename {
		case _DISK_PARTLABEL:
			diskInfo.Partlabel = fi.Name()
		case _DISK_LABEL:
			diskInfo.Label = fi.Name()
		case _DISK_PARTUUID:
			diskInfo.PartUUID = fi.Name()
		}
	}
	return nil
}

// /dev/disk/
func Load(filename string) (DiskIDList, error) {
	var infos DiskIDList

	uuidDir := filepath.Join(filename, _DISK_UUID)
	uuidList, err := ioutil.ReadDir(uuidDir)
	if err != nil {
		return infos, err
	}
	for _, fi := range uuidList {
		srcSub := filepath.Join(uuidDir, fi.Name())
		path, err := filepath.EvalSymlinks(srcSub)
		if err != nil {
			continue
		}
		infos = append(infos, &DiskID{
			Partition: path,
			UUID:      fi.Name(),
		})
	}
	err = infos.handleLoad(filename, _DISK_PARTLABEL)
	if err != nil {
		logger.Warningf("failed to read %s, err: %v", _DISK_PARTLABEL, err)
	}
	err = infos.handleLoad(filename, _DISK_LABEL)
	if err != nil {
		logger.Warningf("failed to read %s, err: %v", _DISK_LABEL, err)
	}
	err = infos.handleLoad(filename, _DISK_PARTUUID)
	if err != nil {
		logger.Warningf("failed to read %s, err: %v", _DISK_PARTUUID, err)
	}
	return infos, nil
}
