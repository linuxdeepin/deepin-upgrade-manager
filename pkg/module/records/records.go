package records

import (
	"deepin-upgrade-manager/pkg/logger"
	"deepin-upgrade-manager/pkg/module/grub"
	"deepin-upgrade-manager/pkg/module/langselector"
	"deepin-upgrade-manager/pkg/module/login"
	"deepin-upgrade-manager/pkg/module/util"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type RecoredState int

const (
	_UNKNOW_STATE         RecoredState = -1
	_ROLLBACK_READY_START RecoredState = iota
	_ROLLBACK_PREPARE_REPO_FILE
	_ROLLBACK_REPLACE_FILE
	_ROLLBACK_RESTORE

	_ROLLBACK_SUCCESSED RecoredState = 100
	_ROLLBACK_FAILED    RecoredState = 101
)

type RecordsInfo struct {
	CurrentState    RecoredState `json:"CurrentState"`
	RollbackVersion string       `json:"RollbackVersion"`
	RepoMount       string       `json:"Repo_Mount_Point"`
	AferRun         string       `json:"AfterRun"`
	TimeOut         uint32       `json:"GrubTimeout"`

	filename    string
	locker      sync.RWMutex
	grubManager *grub.GrubManager
}

func toRecoredState(state int) RecoredState {
	switch RecoredState(state) {
	case _ROLLBACK_READY_START:
		return _ROLLBACK_READY_START
	case _ROLLBACK_PREPARE_REPO_FILE:
		return _ROLLBACK_PREPARE_REPO_FILE
	case _ROLLBACK_REPLACE_FILE:
		return _ROLLBACK_REPLACE_FILE
	case _ROLLBACK_RESTORE:
		return _ROLLBACK_RESTORE
	case _ROLLBACK_SUCCESSED:
		return _ROLLBACK_SUCCESSED
	case _ROLLBACK_FAILED:
		return _ROLLBACK_FAILED
	default:
		return _UNKNOW_STATE
	}
}

func readFile(recordsfile string, info interface{}) error {
	content, err := ioutil.ReadFile(filepath.Clean(recordsfile))
	if err != nil {
		return err
	}
	return json.Unmarshal(content, info)
}

func LoadRecords(rootfs, recordsfile, repoMount string) *RecordsInfo {
	var info RecordsInfo
	path := filepath.Join(rootfs, recordsfile)
	info.filename = path
	info.CurrentState = _UNKNOW_STATE
	info.TimeOut = 2
	info.RollbackVersion = ""
	info.AferRun = ""
	info.RepoMount = repoMount
	defer info.save()
	if util.IsExists(path) {
		err := readFile(path, &info)
		if err != nil {
			info.CurrentState = _UNKNOW_STATE
		}
	} else {
		dir := filepath.Dir(path)
		_ = os.MkdirAll(dir, 0644)
	}
	info.grubManager = grub.Init()
	return &info
}

func (info *RecordsInfo) save() error {
	info.locker.RLock()
	data, err := json.Marshal(info)
	info.locker.RUnlock()
	if err != nil {
		return err
	}
	tmpFile := info.filename + "-" + util.MakeRandomString(util.MinRandomLen)
	// deepin-upgrade-manager-tool need load file
	f, err := os.OpenFile(filepath.Clean(tmpFile), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600|0064)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}

	if err != nil {
		logger.Warning("failed save the records info, err:", err)
	}
	_, err = util.Move(info.filename, tmpFile, true)
	if err != nil {
		logger.Warning("failed move the records info, err:", err)
	}
	return err
}

func (info *RecordsInfo) SetRecoredState(state int) {
	records := toRecoredState(state)
	info.CurrentState = records
}

func (info *RecordsInfo) IsNeedPrepareRepoFile() bool {
	if int(info.CurrentState) > int(_ROLLBACK_PREPARE_REPO_FILE) {
		return false
	} else {
		info.CurrentState = _ROLLBACK_PREPARE_REPO_FILE
		info.save()
		return true
	}
}

func (info *RecordsInfo) IsNeedReplaceFile() bool {
	if int(info.CurrentState) > int(_ROLLBACK_REPLACE_FILE) {
		return false
	} else {
		info.CurrentState = _ROLLBACK_REPLACE_FILE
		info.save()
		return true
	}
}

func (info *RecordsInfo) SetRollbackInfo(version, rootdir string) {
	info.RollbackVersion = version
	defer info.save()

	if len(rootdir) == 1 {
		out, err := info.grubManager.TimeOut()
		if err != nil && out != 0 {
			logger.Warning("failed get grub out time")
			info.TimeOut = out
		} else {
			info.TimeOut = 2 //default timeout
		}
	}
}

func (info *RecordsInfo) SetReady() {
	info.CurrentState = _ROLLBACK_READY_START
	info.save()
}

func (info *RecordsInfo) SetRestore() {
	info.CurrentState = _ROLLBACK_RESTORE
	info.save()
}

func (info *RecordsInfo) IsRestore() bool {
	return info.CurrentState == _ROLLBACK_RESTORE
}

func (info *RecordsInfo) Version() string {
	return info.RollbackVersion
}

func (info *RecordsInfo) IsFailed() bool {
	return info.CurrentState == _ROLLBACK_FAILED
}

func (info *RecordsInfo) IsSucceeded() bool {
	return info.CurrentState == _ROLLBACK_SUCCESSED
}

func (info *RecordsInfo) IsReady() bool {
	return info.CurrentState == _ROLLBACK_READY_START
}

func (info *RecordsInfo) IsReadyRollback() bool {
	return info.CurrentState >= _ROLLBACK_READY_START
}

func (info *RecordsInfo) SetSuccessfully() {
	info.CurrentState = _ROLLBACK_SUCCESSED
	info.save()
}

func (info *RecordsInfo) SetFailed(version string) {
	info.CurrentState = _ROLLBACK_FAILED
	info.RollbackVersion = version
	info.save()
}

func (info *RecordsInfo) Remove() {
	os.Remove(info.filename)
	logger.Debugf("remove records rollback state file: %s", info.filename)
}

func (info *RecordsInfo) SetAfterRun(cmd string) {
	info.AferRun = cmd
	info.save()
}

func (info *RecordsInfo) ResetState() {
	if len(info.RollbackVersion) != 0 {
		currTimeOut, _ := info.grubManager.TimeOut()

		if info.TimeOut != 0 && currTimeOut != info.TimeOut {
			err := info.grubManager.Reset()
			if err != nil {
				info.grubManager = info.grubManager.ChangeDbusDest()
				err := info.grubManager.Reset()
				if err != nil {
					logger.Warningf("failed set the rollback waiting time, err:%v", err)
				} else {
					time.Sleep(1 * time.Second) // wait for grub set out time
					info.grubManager.Join()
				}
			} else {
				time.Sleep(1 * time.Second) // wait for grub set out time
				info.grubManager.Join()
			}
		}
		// Compatible with many languages
		fd, err := login.Inhibit("shutdown", "org.deepin.AtomicUpgrade1",
			"Updating the grub, please shut down or reboot later.")

		cmd := exec.Command("update-grub")
		cmd.Env = append(cmd.Env, langselector.LocalLangEnv()...)
		_ = cmd.Start()
		cmd.Wait()
		if err != nil {
			logger.Warning(err)
		} else {
			time.Sleep(10 * time.Second)
			login.Close(fd)
		}
	}
}
