package upgrader

import (
	"deepin-upgrade-manager/pkg/config"
	"deepin-upgrade-manager/pkg/module/mountinfo"
	"deepin-upgrade-manager/pkg/module/mountpoint"
	"deepin-upgrade-manager/pkg/module/repo"
	"deepin-upgrade-manager/pkg/module/util"
	"fmt"
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

func (c *Upgrader) Snapshot(version string, bootEnabled bool) error {
	for _, v := range c.conf.RepoList {
		err := c.repoSnapshot(v, version, bootEnabled)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Upgrader) Rollback(version string) error {
	err := c.Snapshot(version, false)
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
	return nil
}

func (c *Upgrader) repoCommit(repoConf *config.RepoConfig, newVersion, subject string,
	useSysData bool) error {
	handler := c.repoSet[repoConf.Repo]
	dataDir := filepath.Join(c.rootMP, c.conf.CacheDir, c.conf.Distribution)
	if useSysData {
		err := c.copyRepoData(c.rootMP, dataDir, repoConf.SubscribeList)
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
	bootEnabled bool) error {
	handler := c.repoSet[repoConf.Repo]
	dataDir := filepath.Join(c.rootMP, repoConf.SnapshotDir, version)
	_ = os.RemoveAll(dataDir)
	err := handler.Snapshot(version, dataDir)
	if err != nil {
		return err
	}
	if !bootEnabled {
		return nil
	}
	return c.enableSnapshotBoot(dataDir, version)
}

func (c *Upgrader) enableSnapshotBoot(snapDir, version string) error {
	bootDir := filepath.Join(snapDir, "boot")
	fiList, err := ioutil.ReadDir(bootDir)
	if err != nil {
		return err
	}

	dstDir := filepath.Join(c.rootMP, "boot/snapshot", version)
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
			err = util.ExecCommand("cp", []string{
				"-f",
				filepath.Join(bootDir, fi.Name()),
				filepath.Join(dstDir, fi.Name()),
			})
			// err = util.CopyFile(filepath.Join(bootDir, fi.Name()),
			// 	filepath.Join(dstDir, fi.Name()), false)
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

func (c *Upgrader) repoRollback(repoConf *config.RepoConfig, version string) error {
	snapDir := filepath.Join(c.rootMP, repoConf.SnapshotDir, version)
	dstDir := filepath.Join(c.rootMP, repoConf.StageDir, c.conf.Distribution)
	tmpDir := dstDir + "-" + util.MakeRandomString(util.MinRandomLen)
	err := util.CopyDir(snapDir, tmpDir, true)
	if err != nil {
		return err
	}

	_, err = util.Move(dstDir, tmpDir, true)
	if err != nil {
		return err
	}

	for _, dir := range repoConf.SubscribeList {
		err := c.rollbackDir(dir, dstDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Upgrader) rollbackDir(dir, dstDir string) error {
	srcDir := filepath.Join(dstDir, dir)
	if !util.IsExists(srcDir) {
		fmt.Println("[rollbackDir] data dir empty:", srcDir)
		return nil
	}
	dataDir := filepath.Join(c.rootMP, dir)
	tmpDir := dataDir + "-" + util.MakeRandomString(util.MinRandomLen)
	err := c.migrateDirMount(dataDir, tmpDir)
	if err != nil {
		return err
	}
	fmt.Println("[rollbackDir] will copy dir:", filepath.Join(dstDir, dir), tmpDir)
	// TODO(jouyouyun): replace with codes
	err = util.ExecCommand("cp", []string{"-rfp", srcDir, tmpDir})
	// err = util.CopyDir(srcDir, tmpDir, false)
	if err != nil {
		return err
	}

	bakDir, err := util.Move(dataDir, tmpDir, false)
	if err != nil {
		return err
	}

	return c.umountAndRemoveDir(bakDir)
}

func (c *Upgrader) copyRepoData(rootDir, dataDir string,
	subscribeList []string) error {
	for _, dir := range subscribeList {
		srcDir := filepath.Join(rootDir, dir)
		if !util.IsExists(srcDir) {
			fmt.Println("[copyRepoData] src dir empty:", srcDir)
			continue
		}
		err := util.CopyDir(srcDir, filepath.Join(dataDir, dir), true)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Upgrader) migrateDirMount(srcDir, dstDir string) error {
	mountList := c.mountInfos.Query(srcDir)
	if len(mountList) == 0 {
		return nil
	}
	var mpList mountpoint.MountPointList
	srcLen := len(srcDir)
	for _, m := range mountList {
		mpList = append(mpList, &mountpoint.MountPoint{
			Src:     m.Partition,
			Dest:    filepath.Join(dstDir, m.MountPoint[srcLen:]),
			FSType:  m.FSType,
			Options: m.Options,
		})
	}
	mounted, err := mpList.Mount()
	if err != nil {
		err = mounted.Umount()
		if err != nil {
			fmt.Println("[migrateDirMount] umount:", err)
		}
		return err
	}
	return nil
}

func (c *Upgrader) umountAndRemoveDir(dir string) error {
	mountList := c.mountInfos.Query(dir)
	if len(mountList) == 0 {
		return os.RemoveAll(dir)
	}

	var mpList mountpoint.MountPointList
	for _, m := range mountList {
		mpList = append(mpList, &mountpoint.MountPoint{
			Src:     m.Partition,
			Dest:    m.MountPoint,
			FSType:  m.FSType,
			Options: m.Options,
		})
	}
	err := mpList.Umount()
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}