package upgrader

import (
	config "deepin-upgrade-manager/pkg/config/upgrader"
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/bootkitinfo"
	"deepin-upgrade-manager/pkg/module/dirinfo"
	"deepin-upgrade-manager/pkg/module/fstabinfo"
	"deepin-upgrade-manager/pkg/module/generator"
	"deepin-upgrade-manager/pkg/module/grub"
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
	"os/exec"
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
	SelfMountPath          = "/proc/self/mounts"
	SelfRecordStatePath    = "/etc/deepin-upgrade-manager/state.records"
	LocalNotifyDesktopPath = "/usr/share/deepin-upgrade-manager/deepin-upgrade-manager-tool.desktop"
	AutoStartDesktopPath   = "/etc/xdg/autostart/deepin-upgrade-manager-tool.desktop"

	LessKeepSize = 5 * 1024 * 1024 * 1024
)

const (
	_OP_TY_COMMIT_START opType = iota*10 + 100
	_OP_TY_COMMIT_PREPARE_DATA
	_OP_TY_COMMIT_REPO_SUBMIT
	_OP_TY_COMMIT_REPO_CLEAN
	_OP_TY_COMMIT_GRUB_UPDATE
	_OP_TY_COMMIT_END opType = 199
)

const (
	_OP_TY_ROLLBACK_PREPARING_START opType = iota*10 + 200
	_OP_TY_ROLLBACK_PREPARING_SET_CONFIG
	_OP_TY_ROLLBACK_PREPARING_SET_WAITTIME
	_OP_TY_ROLLBACK_PREPARING_END opType = 299
)

const (
	_OP_TY_DELETE_START opType = iota*10 + 300
	_OP_TY_DELETE_GRUB_UPDATE
	_OP_TY_DELETE_END opType = 399
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
	_STATE_TY_FAILED_NO_VERSION
	_STATE_TY_RUNING stateType = 1
)

func (state stateType) String() string {
	switch state {
	case _STATE_TY_RUNING:
		return "running"
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
	case _STATE_TY_FAILED_NO_VERSION:
		return "version does not exist"
	}
	return "unknown"
}

func (op opType) String() string {
	switch op {
	case _OP_TY_COMMIT_START:
		return "start version submited"
	case _OP_TY_COMMIT_PREPARE_DATA:
		return "start to submit data preparation"
	case _OP_TY_COMMIT_REPO_SUBMIT:
		return "start to submit data"
	case _OP_TY_COMMIT_REPO_CLEAN:
		return "start to repo cleaning"
	case _OP_TY_COMMIT_GRUB_UPDATE:
		return "start to grub updating"
	case _OP_TY_COMMIT_END:
		return "end version submited"

	case _OP_TY_ROLLBACK_PREPARING_START:
		return "start preparing rollback"
	case _OP_TY_ROLLBACK_PREPARING_SET_CONFIG:
		return "start set preparing rollback configuration file"
	case _OP_TY_ROLLBACK_PREPARING_SET_WAITTIME:
		return "start set the grub waiting time "
	case _OP_TY_ROLLBACK_PREPARING_END:
		return "end preparing rollback"

	case _OP_TY_DELETE_START:
		return "start remove the repo version"
	case _OP_TY_DELETE_GRUB_UPDATE:
		return "start to grub updating"
	case _OP_TY_DELETE_END:
		return "end remove the repo version"
	}
	return "unknown"
}

type Upgrader struct {
	conf *config.Config

	mountInfos mountinfo.MountInfoList

	recordsInfo *records.RecordsInfo

	fsInfo fstabinfo.FsInfoList

	repoSet map[string]repo.Repository

	rootMP string
}

func NewUpgraderTool() *Upgrader {
	info := Upgrader{}
	return &info
}

func NewUpgrader(conf *config.Config,
	rootMP string) (*Upgrader, error) {
	mountInfos, err := mountinfo.Load(SelfMountPath)
	if err != nil {
		return nil, err
	}
	fstabDir := filepath.Clean(filepath.Join(rootMP, "/etc/fstab"))
	fsInfo, err := fstabinfo.Load(fstabDir, rootMP)
	if err != nil {
		return nil, err
	}
	info := Upgrader{
		conf:       conf,
		mountInfos: mountInfos,
		repoSet:    make(map[string]repo.Repository),
		rootMP:     rootMP,
		fsInfo:     fsInfo,
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

func (c *Upgrader) ResetRepo() {
	for key := range c.repoSet {
		delete(c.repoSet, key)
	}
	for _, v := range c.conf.RepoList {
		handler, err := repo.NewRepo(repo.REPO_TY_OSTREE, filepath.Join(c.rootMP, v.Repo))
		if err != nil {
			logger.Warning("failed reset repo, err:", err)
		}
		c.repoSet[v.Repo] = handler
	}
}

func (c *Upgrader) Init() (int, error) {
	exitCode := _STATE_TY_SUCCESS
	var repoMountPoint string

	repoMountPoint, _, err := c.RepoMountpointAndUUID()
	if err != nil {
		exitCode = _STATE_TY_FAILED_OSTREE_INIT
		return int(exitCode), err
	}
	if len(repoMountPoint) != 0 {
		point := strings.TrimSpace(repoMountPoint)
		c.conf.ChangeRepoMountPoint(point)
		c.ResetRepo()
		logger.Debugf("find present system max partition is %s ,changed the repo mount point", point)
	}

	osVersion, err := util.GetOSInfo("MajorVersion")
	if nil != err {
		logger.Error("failed get new version, err:", err)
	} else {
		c.conf.SetDistribution(osVersion)
	}

	err = c.conf.Prepare()
	if err != nil {
		exitCode = _STATE_TY_FAILED_OSTREE_INIT
		return int(exitCode), errors.New("failed to initialize config")
	}
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

func (c *Upgrader) Commit(newVersion, subject string, useSysData bool, envVars []string,
	evHandler func(op, state int32, target, desc string)) (excode int, err error) {
	exitCode := _STATE_TY_SUCCESS
	var isClean bool
	c.SendingSignal(evHandler, _OP_TY_COMMIT_START, _STATE_TY_RUNING, newVersion, "")

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
	c.conf.SetVersionConfig(newVersion)
	c.conf.LoadReadyData()
	if len(subject) == 0 {
		subject = fmt.Sprintf("Release %s", newVersion)
	}
	logger.Info("the version number of this submission is:", newVersion)
	for _, v := range c.conf.RepoList {
		err = c.repoCommit(v, newVersion, subject, useSysData, evHandler)
		if err != nil {
			exitCode = _STATE_TY_FAILED_OSTREE_COMMIT
			goto failure
		}
	}
	c.SaveActiveVersion(newVersion)

	// automatically clear redundant versions
	if c.IsAutoClean() {
		c.SendingSignal(evHandler, _OP_TY_COMMIT_REPO_CLEAN, _STATE_TY_RUNING, newVersion, "")
		isClean, err = c.RepoAutoCleanup()
		if err != nil {
			logger.Error("failed auto cleanup repo, err:", err)
		}

	}
	// prevent another update grub
	if !isClean {
		c.SendingSignal(evHandler, _OP_TY_COMMIT_GRUB_UPDATE, _STATE_TY_RUNING, newVersion, "")
		exitCode, err = c.UpdateGrub(envVars)
		if err != nil {
			exitCode = _STATE_TY_FAILED_UPDATE_GRUB
			goto failure
		}
	}

	c.SendingSignal(evHandler, _OP_TY_COMMIT_END, _STATE_TY_SUCCESS, newVersion, "")
	return int(exitCode), nil
failure:
	c.SendingSignal(evHandler, _OP_TY_COMMIT_END, exitCode, newVersion, err.Error())
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

func (c *Upgrader) UpdateGrub(envVars []string) (stateType, error) {
	exitCode := _STATE_TY_SUCCESS
	logger.Info("start update grub")
	cmd := exec.Command("update-grub")
	cmd.Env = append(cmd.Env, envVars...)
	_, err := cmd.Output()
	if err != nil {
		exitCode = _STATE_TY_FAILED_UPDATE_GRUB
		logger.Warning(err)
	}
	cmd.Wait()
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

func (c Upgrader) GrubTitle(version string) string {
	var commitName string

	systemName, err := util.GetOSInfo("SystemName")
	if err != nil {
		logger.Warning("failed get system name, err:", err)
	}
	MinorVersion, err := util.GetOSInfo("MinorVersion")
	if err != nil {
		logger.Warning("failed get minor version, err:", err)
	}
	handler, _ := repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(c.rootMP, c.conf.RepoList[0].Repo))
	time, err := handler.CommitTime(version)
	if err != nil {
		commitName = fmt.Sprintf("Rollback to %s", version)
		logger.Warning("failed get commit time, err:", err)
	} else {
		commitName = systemName + " " + MinorVersion + " " + "(" + strings.ReplaceAll(time, "-", "/") + ")"
	}
	return commitName
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
	diskInfo := c.fsInfo.MatchDestPoint(c.conf.RepoList[0].RepoMountPoint)
	listInfo := bootkitinfo.Load(showList, diskInfo.DiskUUID)
	for _, v := range showList {
		commitName := c.GrubTitle(v)
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
		return mountedPointList, err
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
		logger.Debugf("set rollback info, version:%s, repo mount point:%s", version, c.conf.RepoList[0].RepoMountPoint)
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
	c.SendingSignal(evHandler, _OP_TY_ROLLBACK_PREPARING_START, _STATE_TY_RUNING, version, "")
	c.LoadRollbackRecords(true)
	c.SendingSignal(evHandler, _OP_TY_ROLLBACK_PREPARING_SET_CONFIG, _STATE_TY_RUNING, version, "")
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
			exitCode = _STATE_TY_FAILED_OSTREE_ROLLBACK
			goto failure
		}
		// need load rollback version config
		err := c.conf.LoadVersionData(backVersion, c.rootMP)
		if err != nil {
			exitCode = _STATE_TY_FAILED_OSTREE_ROLLBACK
			goto failure
		}
		// update the mount of the first repo
		mountedPointList, err = c.UpdataMount(c.conf.RepoList[0], backVersion)
		if err != nil {
			exitCode = _STATE_TY_FAILED_HANDLING_MOUNTS
			if err != nil {
				logger.Warning(err)
			}
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
		// rollback ending and need notify
		err = util.CopyFile(filepath.Join(c.rootMP, LocalNotifyDesktopPath), filepath.Join(c.rootMP, AutoStartDesktopPath), false)
		if err != nil {
			logger.Warning(err)
		}
		// restore mount points under initramfs and save action version
		c.SaveActiveVersion(backVersion)

		if len(c.rootMP) != 1 {
			var needUmountList []string
			for _, v := range mountedPointList {
				needUmountList = append(needUmountList, v.Dest)
			}
			needUmountList = util.SortSubDir(needUmountList)
			for _, v := range needUmountList {
				err = util.ExecCommand("umount", []string{v})
				logger.Info("restore system mount, will umount:", v)
				if err != nil {
					logger.Warning("failed umount, err:", err)
				}
			}
		}
	} else {
		c.SendingSignal(evHandler, _OP_TY_ROLLBACK_PREPARING_SET_WAITTIME, _STATE_TY_RUNING, version, "")
		if len(c.rootMP) == 1 {
			grubManager := grub.Init()
			err := grubManager.SetTimeout(0)
			if err != nil {
				logger.Warningf("failed set the rollback waiting time, err:%v", err)
			} else {
				time.Sleep(1 * time.Second) // wait for grub set out time
				grubManager.Join()
			}
		}
		logger.Info("start set rollback a old version:", backVersion)
	}
	c.SendingSignal(evHandler, _OP_TY_ROLLBACK_PREPARING_END, _STATE_TY_SUCCESS, version, "")

	logger.Info("successed run rollback action")
	return int(exitCode), nil
failure:
	if int(exitCode) < int(_STATE_TY_FAILED_NO_REPO) {
		c.recordsInfo.SetFailed(c.conf.ActiveVersion)
		util.CopyFile(filepath.Join(c.rootMP, LocalNotifyDesktopPath), filepath.Join(c.rootMP, AutoStartDesktopPath), false)
	}
	c.SendingSignal(evHandler, _OP_TY_ROLLBACK_PREPARING_END, exitCode, version, err.Error())
	return int(exitCode), err
}

func (c *Upgrader) repoCommit(repoConf *config.RepoConfig, newVersion, subject string,
	useSysData bool, evHandler func(op, state int32, target, desc string)) error {
	handler := c.repoSet[repoConf.Repo]
	dataDir := filepath.Join(c.rootMP, c.conf.CacheDir, c.conf.Distribution)
	defer func() {
		// remove tmp dir
		_ = os.RemoveAll(filepath.Join(c.rootMP, c.conf.CacheDir))
	}()
	if useSysData {
		c.SendingSignal(evHandler, _OP_TY_COMMIT_PREPARE_DATA, _STATE_TY_RUNING, newVersion, "")
		// judging that the space for creating temporary files is sufficient
		usrDir := filepath.Join(c.rootMP, "/usr")

		// need to delete the repo to take up space, if a repo in the subscribeList
		var extraSize int64
		for _, v := range repoConf.SubscribeList {
			if repoConf.RepoMountPoint == v {
				extraSize += dirinfo.GetDirSize(repoConf.Repo)
				extraSize += dirinfo.GetDirSize(repoConf.SnapshotDir)
				extraSize += dirinfo.GetDirSize(repoConf.StageDir)
				break
			}
		}
		isEnough, err := c.isDirSpaceEnough(usrDir, c.rootMP, repoConf.SubscribeList, 0-extraSize, true)
		if err != nil || !isEnough {
			return err
		}
		// if rollback "/", need filter '"/media", "/proc", "/dev", "/sys", "/tmp", "/run"'
		if util.IsItemInList("/", repoConf.SubscribeList) {
			repoConf.FilterList = append(repoConf.FilterList, util.IsDiffInList(repoConf.FilterList, util.FullNeedFilters())...)
		}
		err = c.copyRepoData(c.rootMP, dataDir, repoConf.SubscribeList, repoConf.FilterList)
		if err != nil {
			return err
		}
	}
	c.SendingSignal(evHandler, _OP_TY_COMMIT_REPO_SUBMIT, _STATE_TY_RUNING, newVersion, "")
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

	// if rollback "/", need filter '"/media", "/proc", "/dev", "/sys", "/tmp", "/run"'
	if util.IsItemInList("/", repoConf.SubscribeList) {
		repoConf.FilterList = append(repoConf.FilterList, util.IsDiffInList(repoConf.FilterList, util.FullNeedFilters())...)
	}

	snapDir := filepath.Join(repoConf.SnapshotDir, version)
	realDirSubscribeList, realFileSubcribeList := util.GetRealDirList(repoConf.SubscribeList, c.rootMP, snapDir)
	logger.Debugf("will recovery dirs %v, files %v", realDirSubscribeList, realFileSubcribeList)
	var err error
	defer func() {
		// if failed update, restoring the system
		if err != nil || c.recordsInfo.IsRestore() {
			c.recordsInfo.SetRestore()
			logger.Warning("failed rollback, recover rollback action")
			for _, dir := range realDirSubscribeList {
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
					// When failure to delete extended attributes 'i' delete again
					util.RemoveDirAttr(oldDir)
					err = os.RemoveAll(oldDir)
					if err != nil {
						logger.Warning("failed remove dir, err:", err)
					}
				}
			}
			if util.IsExists(newDir) {
				err = os.RemoveAll(newDir)
				if err != nil {
					// When failure to delete extended attributes 'i' delete again
					util.RemoveDirAttr(newDir)
					err = os.RemoveAll(newDir)
					if err != nil {
						logger.Warning("failed remove dir, err:", err)
					}
				}
			}
		}
	}()
	// prepare the repo file under the system path
	if c.recordsInfo.IsNeedPrepareRepoFile() {
		for _, dir := range realDirSubscribeList {
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
			util.CopyDir(v, dest, nil, nil, true)
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
			util.CopyFile(v, dest, true)
			logger.Debugf("ignore file path:%s", dest)
		}
	}
	var bootDir string
	// repo files replace system files

	for _, dir := range realDirSubscribeList {
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
	// last replace /boot dir, protect system boot
	if len(bootDir) != 0 {
		err = c.handleRepoRollbak(bootDir, snapDir, version, &rollbackDirList, util.HandlerDirRollback)
		if err != nil {
			return err
		}
	}

	// replace file is fast
	if len(realFileSubcribeList) != 0 {
		for _, v := range realFileSubcribeList {
			realFile := util.TrimRootdir(c.rootMP, v)
			snapFile := filepath.Join(snapDir, realFile)
			logger.Debugf("start rolling back file, realfile:%s, snapFile:%s",
				realFile, snapFile)
			err := util.CopyFile(filepath.Join(c.rootMP, snapFile), filepath.Join(c.rootMP, realFile), false)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Upgrader) copyRepoData(rootDir, dataDir string,
	subscribeList []string, filterList []string) error {
	//need filter '/usr/.v23'
	repoCacheDir := filepath.Join(c.rootMP, c.conf.CacheDir)
	os.Mkdir(repoCacheDir, 0755)
	filterList = append(filterList, repoCacheDir)

	for _, dir := range subscribeList {
		srcDir := filepath.Join(rootDir, dir)
		filterDirs, filterFiles := util.HandlerFilterList(rootDir, srcDir, filterList)
		logger.Debugf("the filter directory path is %s, the filter file path is %s", filterDirs, filterFiles)
		if !util.IsExists(srcDir) {
			logger.Info("[copyRepoData] src dir empty:", srcDir)
			continue
		}
		fi, err := os.Stat(srcDir)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			logger.Info("[copyRepoData] src dir:", srcDir)
			err := util.CopyDir(srcDir, filepath.Join(dataDir, dir), filterDirs, filterFiles, true)
			if err != nil {
				return err
			}
		} else {
			logger.Info("[copyRepoData] src file:", srcDir)
			real, err := filepath.EvalSymlinks(srcDir)
			if err != nil {
				continue
			}
			if !util.IsRootSame(subscribeList, real) || real == srcDir {
				util.CopyFile2(real, filepath.Join(dataDir, dir), fi, true)
			}
		}

	}
	return nil
}

func (c *Upgrader) isDirSpaceEnough(mountpoint, rootDir string, subscribeList []string,
	extraSize int64, isFilterPartiton bool) (bool, error) {
	var needSize int64

	mountPart, err := dirinfo.GetDirPartition(mountpoint)
	logger.Debugf("the dir is:%s, the partiton is:%s", mountpoint, mountPart)
	if err != nil {
		return false, err
	}
	for _, dir := range subscribeList {
		srcDir := filepath.Join(rootDir, dir)
		if !util.IsExists(srcDir) {
			continue
		}

		part, err := dirinfo.GetDirPartition(srcDir)
		logger.Debugf("the dir is:%s, the partiton is:%s", srcDir, part)
		if err != nil {
			continue
		}

		if isFilterPartiton && part == mountPart {
			continue
		}
		//the repo is full submission, so need add hard link size
		needSize += dirinfo.GetDirSize(srcDir)
	}
	GB := 1024 * 1024 * 1024
	free, _ := dirinfo.GetPartitionFreeSize(mountPart)

	logger.Debugf("the %s partition free size:%.2f GB, extra size:%.2f GB, the need size is:%.2f GB", mountPart,
		float64(free)/float64(GB), float64(extraSize)/float64(GB), float64(needSize)/float64(GB)+float64(extraSize)/float64(GB))
	if uint64(needSize+extraSize) > free {
		return false, errors.New("the current partition is out of space")
	}
	return true, nil
}

func (c *Upgrader) updataLoaclMount(snapDir string) (mountpoint.MountPointList, error) {
	fstabDir := filepath.Clean(filepath.Join(snapDir, "/etc/fstab"))
	if !util.IsExists(fstabDir) || util.IsEmptyFile(fstabDir) {
		fstabDir = filepath.Clean(filepath.Join(c.rootMP, "/etc/fstab"))
	}
	_, err := ioutil.ReadFile(fstabDir)
	var mountedPointList mountpoint.MountPointList
	if err != nil {
		return mountedPointList, err
	}
	c.fsInfo, err = fstabinfo.Load(fstabDir, c.rootMP)
	if err != nil {
		logger.Debugf("the %s file does not exist in the snapshot, read the local fstabl", fstabDir)
		return mountedPointList, err
	}
	rootPartition, err := dirinfo.GetDirPartition(c.rootMP)
	if err != nil {
		return mountedPointList, err
	}
	for _, info := range c.fsInfo {
		if info.SrcPoint == rootPartition || info.DestPoint == "/" {
			logger.Debugf("ignore mount point %s", info.DestPoint)
			continue
		}
		logger.Debugf("bind:%v,SrcPoint:%v,DestPoint:%v", info.Bind,
			info.SrcPoint, filepath.Clean(filepath.Join(c.rootMP, info.DestPoint)))
		m := c.mountInfos.Match(filepath.Clean(filepath.Join(c.rootMP, info.DestPoint)))
		if m != nil && !info.Bind {
			if m.Partition != info.SrcPoint || strings.Contains(m.Options, "ro") {
				logger.Infof("the %s is mounted %s, not mounted correctly and needs to be unmouted",
					m.Partition, m.MountPoint)
				newInfo := &mountpoint.MountPoint{
					Src:     m.Partition,
					Dest:    m.MountPoint,
					FSType:  m.FSType,
					Options: m.Options,
				}
				err := newInfo.Umount()
				if err != nil {
					continue
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

func (c *Upgrader) RepoAutoCleanup() (bool, error) {
	handler, err := repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(c.rootMP, c.conf.RepoList[0].Repo))
	if err != nil {
		return false, err
	}
	maxVersion := int(c.conf.MaxVersionRetention)
	list, err := handler.List()
	if err != nil {
		return false, err
	}
	if len(list) <= maxVersion {
		logger.Infof("current version is less than %d, no need for auto cleanup", maxVersion)
		return false, nil
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
	return true, nil
}

func (c *Upgrader) Delete(version string,
	evHandler func(op, state int32, target, desc string)) (excode int, err error) {
	exitCode := _STATE_TY_SUCCESS
	var bootDir, snapshotDir, fisrt string
	var handler repo.Repository
	c.SendingSignal(evHandler, _OP_TY_DELETE_START, _STATE_TY_RUNING, version, "")
	if len(c.conf.RepoList) == 0 || len(version) == 0 {
		err = errors.New("wrong version number")
		exitCode = _STATE_TY_FAILED_NO_REPO
		goto failure
	}
	handler, err = repo.NewRepo(repo.REPO_TY_OSTREE,
		filepath.Join(c.rootMP, c.conf.RepoList[0].Repo))
	if err != nil {
		exitCode = _STATE_TY_FAILED_NO_REPO
		goto failure
	}
	fisrt, err = handler.First()
	if err != nil {
		exitCode = _STATE_TY_FAILED_NO_REPO
		goto failure
	}
	if fisrt == version || c.conf.ActiveVersion == version {
		err = errors.New("the current activated version does not allow deletion")
		exitCode = _STATE_TY_FAILED_VERSION_DELETE
		goto failure
	}
	err = handler.Delete(version)
	if err != nil {
		exitCode = _STATE_TY_FAILED_NO_VERSION
		goto failure
	}
	snapshotDir = filepath.Join(c.rootMP, c.conf.RepoList[0].SnapshotDir, version)
	logger.Debug("delete tmp snapshot directory:", snapshotDir)
	_ = os.RemoveAll(snapshotDir)
	bootDir = filepath.Join(c.rootMP, "boot/snapshot", version)
	logger.Debug("delete kernel snapshot directory:", bootDir)
	_ = os.RemoveAll(bootDir)

	c.SendingSignal(evHandler, _OP_TY_DELETE_GRUB_UPDATE, _STATE_TY_RUNING, version, "")
	exitCode, err = c.UpdateGrub(util.LocalLangEnv())
	if err != nil {
		exitCode = _STATE_TY_FAILED_UPDATE_GRUB
		goto failure
	}
	c.SendingSignal(evHandler, _OP_TY_DELETE_END, exitCode, version, "")
	return int(exitCode), nil
failure:
	c.SendingSignal(evHandler, _OP_TY_DELETE_END, exitCode, version, err.Error())
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

func (c *Upgrader) RepoMountpointAndUUID() (string, string, error) {
	list, _, _ := c.ListVersion()
	if len(list) != 0 {
		diskInfo := c.fsInfo.MatchDestPoint(c.conf.RepoList[0].RepoMountPoint)
		return diskInfo.SrcPoint, diskInfo.DiskUUID, nil
	}
	repoMountPoint, uuid := c.fsInfo.MaxFreePartitionPoint()
	for _, v := range c.conf.RepoList {
		// need keep 5GB free space
		isEnough, err := c.isDirSpaceEnough(repoMountPoint, c.rootMP, v.SubscribeList, LessKeepSize, false)
		if err != nil {
			logger.Warning(err)
			continue
		}
		if !isEnough {
			return "", "", err
		}
	}
	return repoMountPoint, uuid, nil
}

func (c *Upgrader) SetReadyData(path string) error {
	return c.conf.SetReadyData(path)
}

func (c *Upgrader) ReadyDataPath() string {
	return c.conf.ReadyDataPath()
}

func (c *Upgrader) ResetGrub(envVars []string) {
	err := c.LoadRollbackRecords(false)
	if err != nil {
		logger.Warning(err)
		return
	}
	c.recordsInfo.ResetState(envVars)
	c.recordsInfo.Remove()

	// need remove auto start desktop
	if util.IsExists(AutoStartDesktopPath) {
		os.RemoveAll(AutoStartDesktopPath)
	}
}

func (c *Upgrader) SendingSignal(evHandler func(op, state int32, target, desc string),
	op opType, exitCode stateType, version, err string) {
	if evHandler == nil {
		return
	}
	if len(err) != 0 {
		evHandler(int32(op), int32(exitCode), (version),
			fmt.Sprintf("%s: %s: %s", op.String(), exitCode.String(), err))
	} else {
		evHandler(int32(op), int32(exitCode), (version),
			fmt.Sprintf("%s: %s", op.String(), exitCode.String()))
	}
}

func (c *Upgrader) SendSystemNotice() error {
	var backMsg, grubTitle string
	const atomicUpgradeDest = "org.deepin.AtomicUpgrade1"
	const atomicUpgradePath = "/org/deepin/AtomicUpgrade1"

	sysBus, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	grubServiceObj := sysBus.Object(atomicUpgradeDest,
		atomicUpgradePath)

	if len(c.recordsInfo.RollbackVersion) == 0 {
		return errors.New("the rollback version is empty")
	} else {
		metho := atomicUpgradeDest + ".GetGrubTitle"
		var ret dbus.Variant
		grubServiceObj.Call(metho, 0, c.recordsInfo.RollbackVersion).Store(&ret)
		grubTitle = ret.Value().(string)
	}
	if c.recordsInfo.IsSucceeded() {
		text, err := util.GetUpgradeText(msgSuccessRollBack)
		if err != nil {
			logger.Warningf("run gettext error: %v", err)
		}
		msg := fmt.Sprintf(" %s", grubTitle)
		backMsg = fmt.Sprintf(text, msg)
	}

	if c.recordsInfo.IsFailed() {
		text, err := util.GetUpgradeText(msgFailRollBack)
		if err != nil {
			logger.Warningf("run gettext error: %v", err)
		}
		msg := fmt.Sprintf(" %s", grubTitle)
		backMsg = fmt.Sprintf(text, msg)
	}
	if len(backMsg) != 0 {
		time.Sleep(5 * time.Second) // wait for osd dbus
		err := notify.SetNotifyText(backMsg)
		if err != nil {
			logger.Warning("failed send system notice, err:", err)
		}
		metho := atomicUpgradeDest + ".CancelRollback"
		return grubServiceObj.Call(metho, 0).Store()
	}
	return nil
}

func (c *Upgrader) LoadRollbackRecords(needcreated bool) error {
	var recordsInfo *records.RecordsInfo

	// save rollback records state
	if needcreated || util.IsExists(filepath.Join(c.rootMP, SelfRecordStatePath)) {
		var repoPart string
		//prevent user power can't read config
		if c.conf != nil {
			repoPart = c.conf.RepoList[0].RepoMountPoint
		}
		recordsInfo = records.LoadRecords(c.rootMP, SelfRecordStatePath, repoPart)
	} else {
		return errors.New("failed load rollback records, the file does not exist")
	}
	c.recordsInfo = recordsInfo
	return nil
}
