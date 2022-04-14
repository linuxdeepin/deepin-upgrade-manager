package upgrader

import (
	"deepin-upgrade-manager/pkg/config"
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/dirinfo"
	"deepin-upgrade-manager/pkg/module/fstabinfo"
	"deepin-upgrade-manager/pkg/module/mountinfo"
	"deepin-upgrade-manager/pkg/module/mountpoint"
	"deepin-upgrade-manager/pkg/module/repo"
	"deepin-upgrade-manager/pkg/module/util"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type Upgrader struct {
	conf *config.Config

	mountInfos mountinfo.MountInfoList

	repoSet map[string]repo.Repository

	rootMP string
}

func NewUpgrader(conf *config.Config,
	rootMP, mountsFile string) (*Upgrader, error) {
	mountInfos, err := mountinfo.Load(mountsFile)
	if err != nil {
		return nil, err
	}
	info := Upgrader{
		conf:       conf,
		mountInfos: mountInfos,
		repoSet:    make(map[string]repo.Repository),
		rootMP:     rootMP,
	}
	for _, v := range conf.RepoList {
		handler, err := repo.NewRepo(repo.REPO_TY_OSTREE, filepath.Join(rootMP, v.Repo))
		if err != nil {
			return nil, err
		}
		info.repoSet[v.Repo] = handler
	}
	return &info, nil
}

func (c *Upgrader) Init() error {
	for _, handler := range c.repoSet {
		err := handler.Init()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Upgrader) Commit(newVersion, subject string, useSysData bool) error {
	for _, v := range c.conf.RepoList {
		err := c.repoCommit(v, newVersion, subject, useSysData)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Upgrader) UpdateGrub() error {
	logger.Info("start update grub")
	err := util.ExecCommand("update-grub", []string{})
	return err
}

func (c *Upgrader) Snapshot(version string, bootEnabled bool) ([]mountpoint.MountPointList, error) {
	var mountedPointRepoList []mountpoint.MountPointList
	for _, v := range c.conf.RepoList {
		mountedPointList, err := c.repoSnapshot(v, version, bootEnabled)
		if err != nil {
			return mountedPointRepoList, err
		}
		mountedPointRepoList = append(mountedPointRepoList, mountedPointList)
	}
	return mountedPointRepoList, nil
}

func (c *Upgrader) Rollback(version string, conf *config.Config) error {
	mountedPointRepoList, err := c.Snapshot(version, false)
	if err != nil {
		return err
	}
	for _, v := range c.conf.RepoList {
		// TODO(jouyouyun): fallback when failure
		err = c.repoRollback(v, version)
		if err != nil {
			return err
		}
	}
	//restore mount points under initramfs and save action version
	if len(c.rootMP) != 1 {
		conf.ActiveVersion = version
		err = conf.Save()
		if err != nil {
			logger.Infof("update version to %q: %v", version, err)
		}
		for _, mountPointList := range mountedPointRepoList {
			for _, v := range mountPointList {
				err = util.ExecCommand("umount", []string{v.Dest})
				logger.Info("restore system mount, will umount:", v.Dest)
				if err != nil {
					logger.Warning("failed umount, err: ", err)
				}
			}
		}
	}
	return nil
}

func (c *Upgrader) repoCommit(repoConf *config.RepoConfig, newVersion, subject string,
	useSysData bool) error {
	handler := c.repoSet[repoConf.Repo]
	dataDir := filepath.Join(c.rootMP, c.conf.CacheDir, c.conf.Distribution)
	if useSysData {
		isEnough, err := c.isDirSpaceEnough(c.rootMP, repoConf.SubscribeList)
		if err != nil {
			return err
		}
		if !isEnough {
			return err
		}
		err = c.copyRepoData(c.rootMP, dataDir, repoConf.SubscribeList)
		if err != nil {
			return err
		}
	}
	err := handler.Commit(newVersion, subject, dataDir)
	if err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(c.rootMP, c.conf.CacheDir))
	return nil
}

func (c *Upgrader) repoSnapshot(repoConf *config.RepoConfig, version string,
	bootEnabled bool) (mountpoint.MountPointList, error) {
	var mountedPointList mountpoint.MountPointList
	handler := c.repoSet[repoConf.Repo]
	dataDir := filepath.Join(c.rootMP, repoConf.SnapshotDir, version)
	_ = os.RemoveAll(dataDir)
	err := handler.Snapshot(version, dataDir)
	if err != nil {
		return mountedPointList, err
	}
	if !bootEnabled {
		mountedPointList, err = c.updataLoaclMount(dataDir)
		if err != nil {
			logger.Warning("the fstab file does not exist in the snapshot, read the local fstabl.")
			mountedPointList, err = c.updataLoaclMount("/")
			if err != nil {
				return mountedPointList, err
			}
		}
	} else {
		err := c.enableSnapshotBoot(dataDir, version)
		if err != nil {
			return mountedPointList, err
		}
	}

	return mountedPointList, nil
}

func (c *Upgrader) enableSnapshotBoot(snapDir, version string) error {
	bootDir := filepath.Join(snapDir, "boot")
	fiList, err := ioutil.ReadDir(bootDir)
	if err != nil {
		return err
	}

	dstDir := filepath.Join(c.rootMP, "boot/snapshot", version)
	localBootDir := filepath.Join(c.rootMP, "/boot")
	err = os.MkdirAll(dstDir, 0700)
	if err != nil {
		return err
	}

	found := false
	for _, fi := range fiList {
		if fi.IsDir() {
			continue
		}
		if strings.HasPrefix(fi.Name(), "vmlinuz-") ||
			strings.HasPrefix(fi.Name(), "kernel-") ||
			strings.HasPrefix(fi.Name(), "vmlinux-") ||
			strings.HasPrefix(fi.Name(), "initrd.img-") {

			snapFile := filepath.Join(bootDir, fi.Name())
			localFile := filepath.Join(localBootDir, fi.Name())
			dstFile := filepath.Join(dstDir, fi.Name())
			isSame, err := util.IsFileSame(localBootDir, snapFile)
			if isSame && err == nil {
				err = util.CopyFile(localFile, dstFile, true)
			} else {
				err = util.CopyFile(snapFile, dstFile, false)
			}

			if err != nil {
				_ = os.Remove(dstDir)
				return err
			}
			found = true
		}
	}
	if !found {
		_ = os.Remove(dstDir)
	}
	return nil
}

// @title    handleRepoRollbak
// @description   handling files on rollback
// @param     realDir         	string         		"original system file path, ex:/etc"
// @param     snapDir         	string         		"snapshot file path, ex:/persitent/osroot"
// @param     version         	string         		"snapshot version, ex:v23.0.0.1"
// @param     rollbackDirList   *[]string      		"rollback produces tmp files, ex:/etc/.old"
// @param     HandlerDir   		function pointer    "file handler function pointer"
func (c *Upgrader) handleRepoRollbak(realDir, snapDir, version string,
	rollbackDirList *[]string, HandlerDir func(src, dst, version, rootDir string, filter []string) (string, error)) error {
	var filterDir []string
	var rollbackDir string
	var err error
	list := c.mountInfos.Query(realDir)
	logger.Debugf("start rolling back, realDir:%s, snapDir:%s, version:%s, list len:%d",
		realDir, snapDir, version, len(list))
	if len(list) > 0 {
		rootPartition, err := dirinfo.GetDirPartition(realDir)
		if err != nil {
			return err
		}
		for _, l := range list {
			if l.MountPoint == realDir {
				continue
			}
			if rootPartition != l.Partition {
				filterDir = append(filterDir, l.MountPoint)
			}
		}
		logger.Debugf("the filter directory path is %s", filterDir)
	}
	rollbackDir, err = HandlerDir(filepath.Join(snapDir+realDir), realDir, version, c.rootMP, filterDir)
	if err != nil {
		logger.Warningf("fail rollback dir:%s,err:%v", realDir, err)
	} else {
		*rollbackDirList = append(*rollbackDirList, rollbackDir)
		logger.Debug("rollbackDir:", rollbackDir)
	}

	for _, l := range filterDir {
		c.handleRepoRollbak(l, snapDir, version, rollbackDirList, HandlerDir)
	}
	return nil
}

func (c *Upgrader) repoRollback(repoConf *config.RepoConfig, version string) error {
	var rollbackDirList []string
	snapDir := filepath.Join(repoConf.SnapshotDir, version)
	realSubscribeList := util.GetRealDirList(repoConf.SubscribeList, c.rootMP)
	for _, dir := range realSubscribeList {
		err := c.handleRepoRollbak(dir, snapDir, version, &rollbackDirList, util.HandlerDirPrepare)
		if err != nil {
			return err
		}
	}
	var bootDir string
	for _, dir := range realSubscribeList {
		logger.Debug("start replacing the dir:", dir)
		if strings.HasPrefix(dir, filepath.Join(c.rootMP, "/boot")) {
			logger.Debugf("the %s needs to be replaced last", dir)
			bootDir = dir
			continue
		}
		err := c.handleRepoRollbak(dir, snapDir, version, &rollbackDirList, util.HandlerDirReplace)
		if err != nil {
			return err
		}
	}
	if len(bootDir) != 0 {
		err := c.handleRepoRollbak(bootDir, snapDir, version, &rollbackDirList, util.HandlerDirReplace)
		if err != nil {
			return err
		}
	}
	for _, v := range rollbackDirList {
		if util.IsExists(v) {
			os.RemoveAll(v)
		}
	}
	return nil
}

func (c *Upgrader) copyRepoData(rootDir, dataDir string,
	subscribeList []string) error {
	for _, dir := range subscribeList {
		srcDir := filepath.Join(rootDir, dir)
		if !util.IsExists(srcDir) {
			logger.Info("[copyRepoData] src dir empty:", srcDir)
			continue
		}
		err := util.CopyDir(srcDir, filepath.Join(dataDir, dir), dataDir, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Upgrader) isDirSpaceEnough(rootDir string, subscribeList []string) (bool, error) {
	var needSize int64
	usrDir := filepath.Join(rootDir, "usr")
	usrPart, err := dirinfo.GetDirPartition(usrDir)
	logger.Debugf("the dir is:%s, the partiton is:%s", usrDir, usrPart)
	if err != nil {
		return false, err
	}
	for _, dir := range subscribeList {
		srcDir := filepath.Join(rootDir, dir)
		part, err := dirinfo.GetDirPartition(srcDir)
		logger.Debugf("the dir is:%s, the partiton is:%s", srcDir, part)
		if err != nil {
			continue
		}
		if !util.IsExists(srcDir) {
			continue
		}
		if part == usrPart {
			continue
		}

		needSize += dirinfo.GetDirSize(srcDir)
	}
	GB := 1024 * 1024 * 1024
	free, _ := dirinfo.GetPartitionFreeSize(usrPart)
	logger.Debugf("the %s partition free size:%.2f GB, the need size is:%.2f GB", usrPart,
		float64(free)/float64(GB), float64(needSize)/float64(GB))
	if uint64(needSize) > free {
		return false, errors.New("the current partition is out of space")
	}
	return true, nil
}

func (c *Upgrader) updataLoaclMount(snapDir string) (mountpoint.MountPointList, error) {
	fstabDir := filepath.Join(snapDir, "etc/fstab")
	_, err := ioutil.ReadFile(fstabDir)
	var mountedPointList mountpoint.MountPointList
	if err != nil {
		return mountedPointList, err
	}
	fsInfo, err := fstabinfo.Load(fstabDir, c.rootMP)
	if err != nil {
		logger.Debugf("the %s file does not exist in the snapshot, read the local fstabl", fstabDir)
		return mountedPointList, err
	}
	rootPartition, err := dirinfo.GetDirPartition(c.rootMP)
	if err != nil {
		return mountedPointList, err
	}
	for _, info := range fsInfo {
		if info.SrcPoint == rootPartition || info.DestPoint == "/" {
			logger.Debugf("ignore mount point %s", info.DestPoint)
			continue
		}
		logger.Debugf("bind:%v,SrcPoint:%v,DestPoint:%v", info.Bind, info.SrcPoint, info.DestPoint)
		m := c.mountInfos.Match(info.DestPoint)
		if m != nil && !info.Bind {
			if m.Partition != info.SrcPoint {
				mp := filepath.Join(c.rootMP, m.MountPoint)
				logger.Infof("the %s is not mounted correctly and needs to be unmouted", mp)
				newInfo := &mountpoint.MountPoint{
					Src:     m.Partition,
					Dest:    mp,
					FSType:  m.FSType,
					Options: m.Options,
				}
				err := newInfo.Umount()
				if err != nil {
					return mountedPointList, err
				}
				err = os.RemoveAll(newInfo.Dest)
				if err != nil {
					return mountedPointList, err
				}
			} else {
				continue
			}
		}
		mp := filepath.Join(c.rootMP, info.DestPoint)
		logger.Infof("the %s is not mounted and needs to be mouted", mp)
		oldInfo := &mountpoint.MountPoint{
			Src:     info.SrcPoint,
			Dest:    mp,
			FSType:  info.FSType,
			Options: info.Options,
			Bind:    info.Bind,
		}
		err := oldInfo.Mount()
		mountedPointList = append(mountedPointList, oldInfo)
		if err != nil {
			logger.Error("failed to mount dir", mp)
			err = oldInfo.Umount()
			if err != nil {
				logger.Error("failed to umount dir:", err)
			}
			return mountedPointList, err
		}
	}
	return mountedPointList, nil
}
