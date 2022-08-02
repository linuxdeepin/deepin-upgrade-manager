package upgrader

import (
	"deepin-upgrade-manager/pkg/config"
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/bootkitinfo"
	"deepin-upgrade-manager/pkg/module/dirinfo"
	"deepin-upgrade-manager/pkg/module/fstabinfo"
	"deepin-upgrade-manager/pkg/module/generator"
	"deepin-upgrade-manager/pkg/module/grub"
	"deepin-upgrade-manager/pkg/module/langselector"
	"deepin-upgrade-manager/pkg/module/mountinfo"
	"deepin-upgrade-manager/pkg/module/mountpoint"
	"deepin-upgrade-manager/pkg/module/notify"
	"deepin-upgrade-manager/pkg/module/records"
	"deepin-upgrade-manager/pkg/module/repo"
	"deepin-upgrade-manager/pkg/module/repo/branch"
	"deepin-upgrade-manager/pkg/module/util"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus"
)

var msgSuccessRollBack = util.Tr("Your system is successfully rolled back to %s.")
var msgFailRollBack = util.Tr("Rollback failed. The system is reverted to %s.")

type (
	opType    int32
	stateType int32
)

const (
	SelfMountPath       = "/proc/self/mounts"
	SelfRecordStatePath = "/etc/deepin-upgrade-manager/state.records"
)

const (
	_OP_TY_COMMIT opType = iota + 1
	_OP_TY_ROLLBACK
	_OP_TY_DELETE
)

const (
	_STATE_TY_SUCCESS stateType = -iota
	_STATE_TY_FAILED_NO_REPO
	_STATE_TY_FAILED_NO_SPACE
	_STATE_TY_FAILED_UPDATE_GRUB
	_STATE_TY_FAILED_HANDLING_MOUNTS
	_STATE_TY_FAILED_OSTREE_INIT
	_STATE_TY_FAILED_OSTREE_COMMIT
	_STATE_TY_FAILED_OSTREE_ROLLBACK
	_STATE_TY_FAILED_VERSION_DELETE
)

func (state stateType) String() string {
	switch state {
	case _STATE_TY_SUCCESS:
		return "success"
	case _STATE_TY_FAILED_NO_SPACE:
		return "not enough space"
	case _STATE_TY_FAILED_NO_REPO:
		return "repo does not exist"
	case _STATE_TY_FAILED_HANDLING_MOUNTS:
		return "failed handling mounts"
	case _STATE_TY_FAILED_UPDATE_GRUB:
		return "failed update grub"
	case _STATE_TY_FAILED_OSTREE_COMMIT:
		return "failed ostree commit"
	case _STATE_TY_FAILED_OSTREE_ROLLBACK:
		return "failed ostree rollback"
	case _STATE_TY_FAILED_OSTREE_INIT:
		return "failed ostree init"
	case _STATE_TY_FAILED_VERSION_DELETE:
		return "version not allowed to delete"
	}
	return "unknown"
}

func (op opType) String() string {
	switch op {
	case _OP_TY_ROLLBACK:
		return "rollback"
	case _OP_TY_COMMIT:
		return "commit"
	case _OP_TY_DELETE:
		return "delete"
	}
	return "unknown"
}

type Upgrader struct {
	conf *config.Config

	mountInfos mountinfo.MountInfoList

	recordsInfo *records.RecordsInfo

	repoSet map[string]repo.Repository

	rootMP string
}

func NewUpgraderTool() (*Upgrader, error) {
	recordsInfo, err := records.LoadRecords("/", SelfRecordStatePath)
	if err != nil {
		return nil, err
	}
	info := Upgrader{
		recordsInfo: recordsInfo,
	}
	return &info, nil
}

func NewUpgrader(conf *config.Config,
	rootMP string) (*Upgrader, error) {
	mountInfos, err := mountinfo.Load(SelfMountPath)
	if err != nil {
		return nil, err
	}
	recordsInfo, err := records.LoadRecords(rootMP, SelfRecordStatePath)
	if err != nil {
		return nil, err
	}
	info := Upgrader{
		conf:        conf,
		mountInfos:  mountInfos,
		repoSet:     make(map[string]repo.Repository),
		rootMP:      rootMP,
		recordsInfo: recordsInfo,
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

func (c *Upgrader) Init() (int, error) {
	exitCode := _STATE_TY_SUCCESS
	if c.IsExistRepo() {
		exitCode = _STATE_TY_FAILED_OSTREE_INIT
		return int(exitCode), errors.New("failed to initialize because repository exists")
	}
	for _, handler := range c.repoSet {
		err := handler.Init()
		if err != nil {
			exitCode = _STATE_TY_FAILED_OSTREE_INIT
			return int(exitCode), err
		}
	}
	return int(exitCode), nil
}

func (c *Upgrader) SaveActiveVersion(version string) {
	c.conf.ActiveVersion = version
	err := c.conf.Save()
	if err != nil {
		logger.Infof("failed update version to %q: %v", version, err)
	}
}

func (c *Upgrader) Commit(newVersion, subject string, useSysData bool,
	evHandler func(op, state int32, target, desc string)) (excode int, err error) {
	exitCode := _STATE_TY_SUCCESS
	if len(newVersion) == 0 {
		newVersion, err = bootkitinfo.NewVersion()
		if err != nil {
			logger.Warning("failed add version, from deepin boot kit, err:", err)
			newVersion, err = c.GenerateBranchName()
			if err != nil {
				exitCode = _STATE_TY_FAILED_NO_REPO
				goto failure
			}
		}
	}
	if len(subject) == 0 {
		subject = fmt.Sprintf("Release %s", newVersion)
	}
	logger.Info("the version number of this submission is:", newVersion)
	for _, v := range c.conf.RepoList {
		err = c.repoCommit(v, newVersion, subject, useSysData)
		if err != nil {
			exitCode = _STATE_TY_FAILED_OSTREE_COMMIT
			goto failure
		}
	}
	c.SaveActiveVersion(newVersion)

	// automatically clear redundant versions
	if c.IsAutoClean() {
		err = c.RepoAutoCleanup()
		if err != nil {
			logger.Error("failed auto cleanup repo, err:", err)
		}
	} else {
		exitCode, err = c.UpdateGrub()
		if err != nil {
			exitCode = _STATE_TY_FAILED_UPDATE_GRUB
			goto failure
		}
	}

	if evHandler != nil {
		evHandler(int32(_OP_TY_COMMIT), int32(_STATE_TY_SUCCESS), newVersion,
			fmt.Sprintf("%s: %s", _OP_TY_COMMIT.String(), _STATE_TY_SUCCESS.String()))
	}
	return int(exitCode), nil
failure:
	if evHandler != nil {
		evHandler(int32(_OP_TY_COMMIT), int32(exitCode), newVersion,
			fmt.Sprintf("%s: %s: %s", _OP_TY_COMMIT.String(), exitCode.String(), err))
	}
	return int(exitCode), err
}

func (c *Upgrader) IsExistRepo() bool {
	for _, v := range c.conf.RepoList {
		if !util.IsExists(v.Repo) {
			logger.Debugf("%s does not exist", v.Repo)
			return false
		}
		handler := c.repoSet[v.Repo]
		list, err := handler.List()
		if err != nil {
			logger.Debugf("%s does not exist", v.Repo)
			return false
		}
		if len(list) == 0 {
			logger.Debugf("%s does not exist", v.Repo)
			return false
		}
	}
	return true
}

func (c *Upgrader) UpdateGrub() (stateType, error) {
	exitCode := _STATE_TY_SUCCESS
	logger.Info("start update grub")
	err := util.ExecCommand("update-grub", []string{})
	if err != nil {
		exitCode = _STATE_TY_FAILED_UPDATE_GRUB
	}
	return exitCode, err
}

func (c *Upgrader) EnableBoot(version string) (stateType, error) {
	exitCode := _STATE_TY_SUCCESS
	err := c.Snapshot(version)
	if err != nil {
		exitCode = _STATE_TY_FAILED_NO_REPO
		return exitCode, err
	}
	for _, v := range c.conf.RepoList {
		dataDir := filepath.Join(c.rootMP, v.SnapshotDir, version)
		err := c.enableSnapshotBoot(dataDir, version)
		if err != nil {
			exitCode = _STATE_TY_FAILED_NO_REPO
			return exitCode, err
		}
	}
	return exitCode, nil
}

func (c *Upgrader) EnableBootList() (string, int, error) {
	var exitCode int
	list, exitCode, err := c.ListVersion()
	if err != nil {
		return "", exitCode, err
	}
	bootSnapDir := filepath.Join(c.rootMP, "/boot/snapshot")
	if util.IsExists(bootSnapDir) {
		os.RemoveAll(bootSnapDir)
	}
	var showList []string
	for _, v := range list {
		if generator.Less(v, c.conf.ActiveVersion) {
			continue
		}
		c.EnableBoot(v)
		showList = append(showList, v)
	}
	handler, _ := repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(c.rootMP, c.conf.RepoList[0].Repo))
	systemName, err := util.GetOSInfo("SystemName")
	if err != nil {
		logger.Warning("failed get system name, err:", err)
	}
	MinorVersion, err := util.GetOSInfo("MinorVersion")
	if err != nil {
		logger.Warning("failed get minor version, err:", err)
	}
	listInfo := bootkitinfo.Load(showList)
	for _, v := range showList {
		time, err := handler.CommitTime(v)
		commitName := systemName + " " + MinorVersion + " " + "(" + strings.ReplaceAll(time, "-", "/") + ")"
		if err != nil {
			logger.Warning("failed get commit time, err:", err)
		}
		listInfo.SetVersionName(v, commitName)
	}

	return listInfo.ToJson(), exitCode, nil
}

func (c *Upgrader) Snapshot(version string) error {
	for _, v := range c.conf.RepoList {
		err := c.repoSnapShot(v, version)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Upgrader) UpdataMount(repoConf *config.RepoConfig, version string) (mountpoint.MountPointList, error) {
	dataDir := filepath.Join(c.rootMP, repoConf.SnapshotDir, version)
	mountedPointList, err := c.updataLoaclMount(dataDir)
	if err != nil {
		logger.Warning("the fstab file does not exist in the snapshot, read the local fstabl.")
		mountedPointList, err = c.updataLoaclMount("/")
		if err != nil {
			return mountedPointList, err
		}
	}
	// need to get mount information again
	mountinfos, err := mountinfo.Load(SelfMountPath)
	logger.Infof("to update the local mount, you need to reload the mount information")
	if err != nil {
		return nil, err
	}
	c.mountInfos = mountinfos
	return mountedPointList, nil
}

func (c *Upgrader) IsExistVersion(version string) bool {
	list, _, err := c.ListVersion()
	if err != nil {
		return false
	}
	for _, v := range list {
		if v == version {
			return true
		}
	}
	return false
}

func (c *Upgrader) getRollbackInfo(version, rootdir string) (string, bool, error) {
	if len(version) != 0 {
		if !c.IsExistVersion(version) {
			return "", false, errors.New("version does not exist")
		}
		var isCanRollback bool
		if len(c.rootMP) != 1 {
			logger.Info("start rollback a old version in initramfs")
			isCanRollback = true
		} else {
			logger.Info("start ready a old version to rollback")
			isCanRollback = false
		}
		c.recordsInfo.SetReady()
		c.recordsInfo.SetRollbackInfo(version, rootdir)
		return version, isCanRollback, nil
	}
	if len(version) == 0 && len(c.rootMP) != 1 && c.recordsInfo.IsReadyRollback() &&
		!c.recordsInfo.IsFailed() && !c.recordsInfo.IsSucceeded() {
		logger.Info("begin to rollback version has been set in the initramfs")
		backVersion := c.recordsInfo.Version()
		return backVersion, true, nil
	}
	return "", false, nil
}

func (c *Upgrader) Rollback(version string,
	evHandler func(op, state int32, target, desc string)) (excode int, err error) {
	exitCode := _STATE_TY_SUCCESS
	backVersion, isCanRollback, err := c.getRollbackInfo(version, c.rootMP)
	if err != nil {
		exitCode = _STATE_TY_FAILED_NO_REPO
		goto failure
	}
	if isCanRollback && len(backVersion) != 0 {
		logger.Info("start rollback a old version:", backVersion)
		var mountedPointList mountpoint.MountPointList

		// checkout specified version file
		err = c.Snapshot(backVersion)
		if err != nil {
			exitCode = _STATE_TY_FAILED_NO_REPO
			goto failure
		}

		// update the mount of the first repo
		mountedPointList, err = c.UpdataMount(c.conf.RepoList[0], backVersion)
		if err != nil {
			exitCode = _STATE_TY_FAILED_HANDLING_MOUNTS
			goto failure
		}

		// rollback system files
		for _, v := range c.conf.RepoList {
			err = c.repoRollback(v, backVersion)
			if err != nil {
				exitCode = _STATE_TY_FAILED_OSTREE_ROLLBACK
				goto failure
			}
		}
		c.SaveActiveVersion(backVersion)
		// restore mount points under initramfs and save action version
		if len(c.rootMP) != 1 {
			for _, v := range mountedPointList {
				err = util.ExecCommand("umount", []string{v.Dest})
				logger.Info("restore system mount, will umount:", v.Dest)
				if err != nil {
					logger.Warning("failed umount, err:", err)
				}
			}
		}
	} else {
		err := grub.SetTimeout(0)
		if err != nil {
			logger.Warning("failed set the rollback waiting time")
		} else {
			time.Sleep(1 * time.Second) // wait for grub set out time
			grub.Join()
		}

		logger.Info("start set rollback a old version:", backVersion)
	}
	if evHandler != nil {
		evHandler(int32(_OP_TY_ROLLBACK), int32(_STATE_TY_SUCCESS), version,
			fmt.Sprintf("%s: %s", _OP_TY_ROLLBACK.String(), _STATE_TY_SUCCESS.String()))
	}
	logger.Info("successed run rollback action")
	return int(exitCode), nil
failure:
	if evHandler != nil {
		evHandler(int32(_OP_TY_ROLLBACK), int32(exitCode), version,
			fmt.Sprintf("%s: %s: %s", _OP_TY_ROLLBACK.String(), exitCode.String(), err))
	}
	return int(exitCode), err
}

func (c *Upgrader) repoCommit(repoConf *config.RepoConfig, newVersion, subject string,
	useSysData bool) error {
	handler := c.repoSet[repoConf.Repo]
	dataDir := filepath.Join(c.rootMP, c.conf.CacheDir, c.conf.Distribution)
	defer func() {
		// remove tmp dir
		_ = os.RemoveAll(filepath.Join(c.rootMP, c.conf.CacheDir))
	}()
	if useSysData {
		// judging that the space for creating temporary files is sufficient
		isEnough, err := c.isDirSpaceEnough(c.rootMP, repoConf.SubscribeList)
		if err != nil || !isEnough {
			return err
		}
		err = c.copyRepoData(c.rootMP, dataDir, repoConf.SubscribeList, repoConf.FilterList)
		if err != nil {
			return err
		}
	}
	logger.Debugf("will submitted version to the repo, version:%s, sub:%s, dataDir:%s", newVersion, subject, dataDir)
	err := handler.Commit(newVersion, subject, dataDir)
	if err != nil {
		return err
	}
	return nil
}

func (c *Upgrader) repoSnapShot(repoConf *config.RepoConfig, version string) error {
	handler := c.repoSet[repoConf.Repo]
	dataDir := filepath.Join(c.rootMP, repoConf.SnapshotDir, version)
	_ = os.RemoveAll(dataDir)
	return handler.Snapshot(version, dataDir)
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
			isSame, err := util.IsFileSame(localFile, snapFile)
			if isSame && err == nil {
				// create file hard link
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
	var filterDirs []string
	var rollbackDir string
	var err error
	// need trim root dir
	realDir = util.TrimRootdir(c.rootMP, realDir)
	list := c.mountInfos.Query(filepath.Join(c.rootMP, realDir))
	logger.Debugf("start rolling back, realDir:%s, snapDir:%s, version:%s, list len:%d",
		realDir, snapDir, version, len(list))
	if len(list) > 0 {
		rootPartition, err := dirinfo.GetDirPartition(filepath.Join(c.rootMP, realDir))
		if err != nil {
			return err
		}
		for _, l := range list {
			if l.MountPoint == filepath.Join(c.rootMP, realDir) {
				continue
			}
			if rootPartition != l.Partition {
				filterDirs = append(filterDirs, l.MountPoint)
			}
		}
		logger.Debugf("the filter directory path is %s", filterDirs)
	}

	rollbackDir, err = HandlerDir(filepath.Join(snapDir+realDir), realDir, version, c.rootMP, filterDirs)
	if err != nil {
		logger.Warningf("fail rollback dir:%s,err:%v", realDir, err)
		return err
	} else {
		*rollbackDirList = append(*rollbackDirList, rollbackDir)
		logger.Debug("rollbackDir:", rollbackDir)
	}

	for _, l := range filterDirs {
		err = c.handleRepoRollbak(l, snapDir, version, rollbackDirList, HandlerDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Upgrader) repoRollback(repoConf *config.RepoConfig, version string) error {
	var rollbackDirList []string
	snapDir := filepath.Join(repoConf.SnapshotDir, version)
	realSubscribeList := util.GetRealDirList(repoConf.SubscribeList, c.rootMP, snapDir)
	var err error

	defer func() {
		// if failed update, restoring the system
		if err != nil || c.recordsInfo.IsRestore() {
			c.recordsInfo.SetRestore()
			logger.Warning("failed rollback, recover rollback action")
			for _, dir := range realSubscribeList {
				err := c.handleRepoRollbak(dir, snapDir, version, &rollbackDirList, util.HandlerDirRecover)
				if err != nil {
					logger.Error("failed recover rollback, err:", err)
				}
			}
			c.recordsInfo.SetFailed(c.conf.ActiveVersion)
		} else {
			c.recordsInfo.SetSuccessfully()
		}
		logger.Debug("need to be deleted tmp dirs:", rollbackDirList)
		// remove all tmp dir and compatible rollback
		for _, v := range rollbackDirList {
			oldDir := filepath.Join(path.Dir(v), string("/.old")+version)
			newDir := filepath.Join(path.Dir(v), string("/.")+version)
			if util.IsExists(oldDir) {
				err = os.RemoveAll(oldDir)
				if err != nil {
					logger.Warning("failed remove dir, err:", err)
				}
			}
			if util.IsExists(newDir) {
				err = os.RemoveAll(newDir)
				if err != nil {
					logger.Warning("failed remove dir, err:", err)
				}
			}
		}
	}()
	// prepare the repo file under the system path
	if c.recordsInfo.IsNeedPrepareRepoFile() {
		for _, dir := range realSubscribeList {
			err = c.handleRepoRollbak(dir, snapDir, version, &rollbackDirList, util.HandlerDirPrepare)
			if err != nil {
				return err
			}
		}
	}

	// hardlink need to filter file or dir to prepare dir
	for _, dir := range rollbackDirList {
		dirRoot := filepath.Dir(dir)
		filterDirs, filterFiles := util.HandlerFilterList(c.rootMP, dirRoot, repoConf.FilterList)
		rootPartition, err := dirinfo.GetDirPartition(dirRoot)
		if err != nil {
			logger.Warningf("failed get %s partition", dirRoot)
			continue
		}
		for _, v := range filterDirs {
			dirPartition, err := dirinfo.GetDirPartition(v)
			if err != nil {
				logger.Warningf("failed get %s partition", v)
				continue
			}
			if dirPartition != rootPartition {
				continue
			}
			dest := filepath.Join(dir, strings.TrimPrefix(v, dirRoot))
			util.CopyDir(v, dest, nil, nil, false)
			logger.Debugf("ignore dir path:%s", dest)
		}
		for _, v := range filterFiles {
			filePartition, err := dirinfo.GetDirPartition(v)
			if err != nil {
				logger.Warningf("failed get %s partition", v)
				continue
			}
			if filePartition != rootPartition {
				continue
			}
			dest := filepath.Join(dir, strings.TrimPrefix(v, dirRoot))
			util.CopyFile(v, dest, false)
			logger.Debugf("ignore file path:%s", dest)
		}
	}
	var bootDir string
	// repo files replace system files

	for _, dir := range realSubscribeList {
		logger.Debug("start replacing the dir:", dir)
		if strings.HasSuffix(filepath.Join(c.rootMP, "/boot"), dir) {
			logger.Debugf("the %s needs to be replaced last", dir)
			bootDir = dir
			continue
		}
		err = c.handleRepoRollbak(dir, snapDir, version, &rollbackDirList, util.HandlerDirRollback)
		if err != nil {
			return err
		}
	}
	// last replace /boot, protect system boot
	if len(bootDir) != 0 {
		err = c.handleRepoRollbak(bootDir, snapDir, version, &rollbackDirList, util.HandlerDirRollback)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Upgrader) copyRepoData(rootDir, dataDir string,
	subscribeList []string, filterList []string) error {
	//need filter '/usr/.v23'
	repoCacheDir := filepath.Join(c.rootMP, c.conf.CacheDir)
	filterList = append(filterList, repoCacheDir)

	for _, dir := range subscribeList {
		srcDir := filepath.Join(rootDir, dir)
		filterDirs, filterFiles := util.HandlerFilterList(rootDir, srcDir, filterList)

		if !util.IsExists(srcDir) {
			logger.Info("[copyRepoData] src dir empty:", srcDir)
			continue
		}
		err := util.CopyDir(srcDir, filepath.Join(dataDir, dir), filterDirs, filterFiles, true)
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

func (c *Upgrader) RepoAutoCleanup() error {
	handler, err := repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(c.rootMP, c.conf.RepoList[0].Repo))
	if err != nil {
		return err
	}
	maxVersion := int(c.conf.MaxVersionRetention)
	list, err := handler.List()
	if err != nil {
		return err
	}
	if len(list) <= maxVersion {
		logger.Infof("current version is less than %d, no need for auto cleanup", maxVersion)
		return nil
	}
	logger.Infof("current version is more than %d, need for cleanup repo", maxVersion)

	for i, v := range list {
		if i == len(list)-1 {
			continue
		}
		if i < maxVersion-1 {
			continue
		}
		_, err = c.Delete(v, nil)
		if err != nil {
			logger.Warning(err)
			break
		}
	}
	return nil
}

func (c *Upgrader) Delete(version string,
	evHandler func(op, state int32, target, desc string)) (excode int, err error) {
	exitCode := _STATE_TY_SUCCESS
	var bootDir, snapshotDir string
	var handler repo.Repository

	if len(c.conf.RepoList) == 0 || len(version) == 0 {
		err = errors.New("wrong version number")
		exitCode = _STATE_TY_FAILED_NO_REPO
		goto failure
	}
	if version == c.conf.ActiveVersion {
		err = errors.New("the current activated version does not allow deletion")
		exitCode = _STATE_TY_FAILED_VERSION_DELETE
		goto failure
	}
	handler, err = repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(c.rootMP, c.conf.RepoList[0].Repo))
	if err != nil {
		exitCode = _STATE_TY_FAILED_NO_REPO
		goto failure
	}
	err = handler.Delete(version)
	if err != nil {
		exitCode = _STATE_TY_FAILED_VERSION_DELETE
		goto failure
	}
	snapshotDir = filepath.Join(c.rootMP, c.conf.RepoList[0].SnapshotDir, version)
	logger.Debug("delete tmp snapshot directory:", snapshotDir)
	_ = os.RemoveAll(snapshotDir)
	bootDir = filepath.Join(c.rootMP, "boot/snapshot", version)
	logger.Debug("delete kernel snapshot directory:", bootDir)
	_ = os.RemoveAll(bootDir)

	exitCode, err = c.UpdateGrub()
	if err != nil {
		exitCode = _STATE_TY_FAILED_UPDATE_GRUB
		goto failure
	}
	if evHandler != nil {
		evHandler(int32(_OP_TY_DELETE), int32(exitCode), version,
			fmt.Sprintf("%s: %s", _OP_TY_DELETE.String(), _STATE_TY_SUCCESS.String()))
	}
	return int(exitCode), nil
failure:
	if evHandler != nil {
		evHandler(int32(_OP_TY_DELETE), int32(exitCode), version,
			fmt.Sprintf("%s: %s: %s", _OP_TY_DELETE.String(), exitCode.String(), err))
	}
	return int(exitCode), err
}

func (c *Upgrader) IsAutoClean() bool {
	if len(c.conf.RepoList) == 0 {
		return true
	}
	return c.conf.AutoCleanup
}

func (c *Upgrader) GenerateBranchName() (string, error) {
	if len(c.conf.RepoList) != 0 {
		handler, err := repo.NewRepo(repo.REPO_TY_OSTREE,
			c.conf.RepoList[0].Repo)
		if err != nil {
			return "", err
		}
		name, err := handler.Last()
		if err != nil {
			return "", err
		}
		return branch.Increment(name)
	}
	return branch.GenInitName(c.conf.Distribution), nil
}

func (c *Upgrader) ListVersion() ([]string, int, error) {
	exitCode := _STATE_TY_SUCCESS
	if len(c.conf.RepoList) == 0 {
		exitCode = _STATE_TY_FAILED_NO_REPO
		return nil, int(exitCode), nil
	}

	handler, err := repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(c.rootMP, c.conf.RepoList[0].Repo))
	if err != nil {
		exitCode = _STATE_TY_FAILED_NO_REPO
		return nil, int(exitCode), err
	}
	list, err := handler.List()
	if err != nil {
		exitCode = _STATE_TY_FAILED_NO_REPO
		return nil, int(exitCode), err
	}
	return list, int(exitCode), err
}

func (c *Upgrader) DistributionName() string {
	return c.conf.Distribution
}

func (c *Upgrader) Subject(version string) (string, error) {
	var sub string
	handler, err := repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(c.rootMP, c.conf.RepoList[0].Repo))
	if err != nil {
		return sub, err
	}
	if !handler.Exist(version) {
		return sub, errors.New("failed get subject, the current version does not exist version")
	}
	return handler.Subject(version)
}

func (c *Upgrader) ResetGrub(locale string) {
	c.recordsInfo.ResetState(locale)
}

func (c *Upgrader) SendSystemNotice() error {
	var backMsg string

	if len(c.recordsInfo.RollbackVersion) == 0 {
		return errors.New("the rollback version is empty")
	}

	if c.recordsInfo.IsSucceeded() {
		text, err := util.GetUpgradeText(msgSuccessRollBack)
		if err != nil {
			logger.Warningf("run gettext error: %v", err)
		}
		backMsg = fmt.Sprintf(text, c.recordsInfo.RollbackVersion)
	}

	if c.recordsInfo.IsFailed() {
		text, err := util.GetUpgradeText(msgFailRollBack)
		if err != nil {
			logger.Warningf("run gettext error: %v", err)
		}
		backMsg = fmt.Sprintf(text, c.recordsInfo.RollbackVersion)
	}
	if len(backMsg) != 0 {
		time.Sleep(5 * time.Second) // wait for osd dbus
		err := notify.SetNotifyText(backMsg)
		if err != nil {
			logger.Warning("failed send system notice, err:", err)
		}
		sysBus, err := dbus.SystemBus()
		if err != nil {
			return err
		}
		grubServiceObj := sysBus.Object("org.deepin.AtomicUpgrade1",
			"/org/deepin/AtomicUpgrade1")
		metho := "org.deepin.AtomicUpgrade1.Reset"
		lang, _ := langselector.GetCurrentLocale()
		return grubServiceObj.Call(metho, 0, lang).Store()
	}
	return nil
}
