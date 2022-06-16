package util

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"deepin-upgrade-manager/pkg/logger"
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	MinRandomLen = 10
)

const (
	_envNamePath = "PATH"
	_envDelim    = ":"
)

func FixEnvPath() error {
	var (
		defaultPath string
		oldPath     string
	)
	var (
		defaultItems = []string{
			"/bin",
			"/usr/bin",
			"/sbin",
			"/usr/sbin",
		}
		oldItems []string
		items    []string
	)

	oldPath = os.Getenv(_envNamePath)
	if len(oldPath) == 0 {
		defaultPath = strings.Join(defaultItems, _envDelim)
		goto fix
	}

	oldItems = strings.Split(oldPath, _envDelim)
	for _, v := range defaultItems {
		if IsItemInList(v, oldItems) {
			continue
		}
		items = append(items, v)
	}

	if len(items) == 0 {
		return nil
	}
	defaultPath = strings.Join(items, _envDelim) + ":" + oldPath

fix:
	return os.Setenv(_envNamePath, defaultPath)
}

func IsExists(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil || os.IsExist(err) {
		return true
	}
	return false
}

func MakeRandomString(num int) string {
	if num < MinRandomLen {
		num = MinRandomLen
	}
	data := make([]byte, num/2)
	_, err := rand.Read(data)
	if err == nil {
		return fmt.Sprintf("%x", data)
	}

	// fallback
	var str = "0123456789qwertyuiopasdfghjklzxcvbnm"
	var length = len(str)
	mrand.Seed(time.Now().Unix())
	for i := 0; i < num; i++ {
		data = append(data, str[mrand.Intn(length)])
	}
	return string(data)
}

func execC(action string, args []string) (io.ReadCloser, error) {
	var cmd *exec.Cmd
	if len(args) != 0 {
		cmd = exec.Command(action, args...)
	} else {
		cmd = exec.Command(action, nil...)
	}
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err != nil {
		return nil, err
	}
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	return stdout, nil
}

func ExecCommandWithOut(action string, args []string) ([]byte, error) {
	var out []byte
	stdout, err := execC(action, args)
	if err != nil {
		return out, err
	}
	for {
		var buffer bytes.Buffer
		buf := make([]byte, 1024)
		_, err = stdout.Read(buf)
		if err != nil {
			break
		}
		buffer.Write(out)
		buffer.Write(buf)
		out = buffer.Bytes()
	}
	return out, nil
}

func ExecCommand(action string, args []string) error {

	stdout, err := execC(action, args)
	if err != nil {
		return err
	}
	for {
		buf := make([]byte, 1024)
		_, err = stdout.Read(buf)
		if err != nil {
			break
		}
	}
	return nil
}

func Chdir(dir string) (string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	err = os.Chdir(dir)
	if err != nil {
		_ = os.Chdir(pwd)
		return "", err
	}
	return pwd, nil
}

func getFilterRootDir(root, filterRoot string) string {
	if root == filterRoot || len(filterRoot) == 1 {
		return ""
	}
	//fmt.Printf("root:%s,filter:%s\n", root, filterRoot)
	dir := filepath.Dir(filterRoot)
	if root == dir {
		return filterRoot
	} else {
		return getFilterRootDir(root, dir)
	}
}

func IsExistsPath(list []string, str string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func IsRootSame(list []string, str string) bool {
	for _, v := range list {
		num := 0
		if num == 0 && str == v {
			num++
			continue
		}
		if strings.HasPrefix(str, v) {
			return true
		}
	}
	return false
}

func GetRealDirList(list []string, rootDir, snapDir string) []string {
	var newList []string
	var rootList []string
	for _, v := range list {
		dir := filepath.Join(rootDir, v)
		rootList = append(rootList, dir)
	}
	for _, v := range list {
		dir := filepath.Join(rootDir, v)
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			real = dir
		}

		if IsRootSame(rootList, real) {
			logger.Infof("dir %s is the same root as dir %s, need ignore dir %s", real, v, dir)
			continue
		}
		// Need to confirm that the snapshot and the local directory exist
		if !IsExists(filepath.Join(rootDir, snapDir, v)) {
			os.RemoveAll(filepath.Join(rootDir, v))
			if err != nil {
				logger.Warning("failed remove dir, err:", err)
			}
			logger.Infof("the %s in the %s does not exist, the local %s needs to be deleted",
				v, snapDir, v)
			continue
		} else {
			if !IsExists(real) {
				Mkdir(filepath.Join(rootDir, snapDir, v), real)
			}
			logger.Infof("the %s does not exist, needs to be created",
				real)
		}
		newList = append(newList, v)
	}
	return newList
}

// @title    MoveDirSubFile
// @description   move system files to sub dir
// @param     orig         		string         		"files that need to be moved, ex:/etc"
// @param     dst         		string         		"tmp dir, ex:/etc/tmp"
// @param     newDir         	string         		"snapshot file path, ex:/etc/.v23.0.0.1"
// @param     filter		    *[]string      		"list of files to filter, ex:/etc/a/b/"
func MoveDirSubFile(orig, dst, newDir string, filter []string) error {
	var filterRoots []string
	for _, v := range filter {
		dir := getFilterRootDir(orig, v)
		if dir == "" {
			continue
		}
		filterRoots = append(filterRoots, dir)
	}
	if len(filter) != 0 && len(filterRoots) == 0 {
		return nil
	}
	if !IsExistsPath(filterRoots, orig) {
		err := Mkdir(orig, dst)
		if err != nil {
			return err
		}
	}

	fiList, err := ioutil.ReadDir(orig)
	if err != nil {
		return err
	}
	for _, fi := range fiList {
		origSub := filepath.Join(orig, fi.Name())
		dstSub := filepath.Join(dst, fi.Name())

		if len(filterRoots) > 0 && IsExistsPath(filterRoots, origSub) {
			continue
		}
		if origSub == newDir {
			continue
		}
		os.Rename(origSub, dstSub)
	}

	for _, v := range filterRoots {
		dstdir := filepath.Join(dst, filepath.Base(v))
		MoveDirSubFile(v, dstdir, newDir, filter)
	}
	return nil
}

// @title    SubMoveOut
// @description    sub files moved out
// @param     orig         		string         		"sub dir, ex:/etc/2020"
// @param     dst         		string         		"out dir, ex:/etc"
func SubMoveOut(orig, dst string) error {
	err := Mkdir(orig, dst)
	if err != nil {
		return err
	}

	fiList, err := ioutil.ReadDir(orig)
	if err != nil {
		return err
	}

	for _, fi := range fiList {
		srcSub := filepath.Join(orig, fi.Name())
		dstSub := filepath.Join(dst, fi.Name())

		fiStat, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to get raw stat for: %s", srcSub)
		}

		switch {
		case fiStat.Mode&syscall.S_IFDIR == syscall.S_IFDIR:
			err = SubMoveOut(srcSub, dstSub)
		case fiStat.Mode&syscall.S_IFREG == syscall.S_IFREG:
			err = os.Rename(srcSub, dstSub)
		default:
			logger.Debug("unknown file type:", srcSub)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func Move(orig, dst string, deleted bool) (string, error) {
	if !IsExists(orig) {
		return "", os.Rename(dst, orig)
	}

	bakDir := orig + "-bak-" + MakeRandomString(MinRandomLen)
	err := os.Rename(orig, bakDir)
	if err != nil {
		return "", err
	}

	err = os.Rename(dst, orig)
	if err != nil {
		_ = os.Rename(bakDir, orig)
		return "", err
	}
	if deleted {
		_ = os.RemoveAll(bakDir)
	}
	return bakDir, nil
}

func Chown(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return err
	}

	ssi, ok := si.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to get raw stat for: %s", src)
	}
	return os.Lchown(dst, int(ssi.Uid), int(ssi.Gid))
}

func Mkdir(srcDir, dstDir string) error {
	if IsExists(dstDir) {
		return nil
	}

	fi, err := os.Stat(srcDir)
	if err != nil {
		return err
	}
	err = os.MkdirAll(dstDir, fi.Mode())
	if err != nil {
		return err
	}

	// set uid, gid, suid, sgid and sbit
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		fileMode := fi.Mode().Perm()
		var isChanged bool

		if stat.Mode&syscall.S_ISUID == syscall.S_ISUID {
			fileMode |= os.ModeSetuid
			isChanged = true
		}
		if stat.Mode&syscall.S_ISGID == syscall.S_ISGID {
			fileMode |= os.ModeSetgid
			isChanged = true
		}
		if stat.Mode&syscall.S_ISVTX == syscall.S_ISVTX {
			fileMode |= os.ModeSticky
			isChanged = true
		}
		if isChanged {
			err = os.Chmod(dstDir, fileMode)
			if err != nil {
				return err
			}
		}
		err = os.Lchown(dstDir, int(stat.Uid), int(stat.Gid))
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to get raw stat for: %s", srcDir)
	}

	return nil
}

func Mkchr(filename string) error {
	var (
		major             = 1
		minor             = 0
		fmode os.FileMode = 0600
	)

	fmode |= syscall.S_IFCHR
	dev := int((major << 8) | (minor & 0xff) | ((minor & 0xfff00) << 12))

	_ = os.MkdirAll(filepath.Dir(filename), 0755)
	return syscall.Mknod(filename, uint32(fmode), dev)
}

func Symlink(src, dst string) error {
	origin, err := os.Readlink(src)
	if err != nil {
		return err
	}
	dstOrigin, _ := os.Readlink(dst)
	if origin == dstOrigin {
		return nil
	}

	_ = os.RemoveAll(dst)
	_ = Mkdir(filepath.Dir(src), filepath.Dir(dst))
	return os.Symlink(origin, dst)
}

// @title    CompareDirAndCopy
// @description   compare files, hardlink if they are the same, copy if they are different
// @param     src         		string         		"snapshot dir, ex:/persitent/osroot/v23/2020/etc"
// @param     dst         		string         		"snapshot storage path, ex:/etc/.2020/"
// @param     compare         	string         		"need compare dir, ex:/etc"
// @param     filter		    *[]string      		"list of files to filter"
func CompareDirAndCopy(src, dst, cmp string, filter []string) error {
	sfi, err := os.Stat(src)
	if err != nil {
		return err
	}
	for _, v := range filter {
		if cmp == v {
			return nil
		}
	}

	err = Mkdir(src, dst)
	if err != nil {
		return err
	}
	fiList, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}
	for _, fi := range fiList {
		srcSub := filepath.Join(src, fi.Name())
		dstSub := filepath.Join(dst, fi.Name())
		cmpSub := filepath.Join(cmp, fi.Name())
		fiStat, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to get raw stat for: %s", srcSub)
		}

		switch {
		case fiStat.Mode&syscall.S_IFSOCK == syscall.S_IFSOCK:
			logger.Debug("sock files need to be filtered:", srcSub)
		case fiStat.Mode&syscall.S_IFLNK == syscall.S_IFLNK:
			err = Symlink(srcSub, dstSub)
		case fiStat.Mode&syscall.S_IFCHR == syscall.S_IFCHR:
			logger.Debug("[CopyDir] will remove(char):", dstSub)
			err = os.RemoveAll(dstSub)
		case fiStat.Mode&syscall.S_IFDIR == syscall.S_IFDIR:
			err = CompareDirAndCopy(srcSub, dstSub, cmpSub, filter)
		case fiStat.Mode&syscall.S_IFREG == syscall.S_IFREG:
			equal, errC := IsFileSame(srcSub, cmpSub)
			if errC == nil {
				if equal {
					err = CopyFile2(cmpSub, dstSub, sfi, true)
				} else {
					err = CopyFile2(srcSub, dstSub, sfi, false)
				}
			} else {
				err = CopyFile2(srcSub, dstSub, sfi, false)
			}
		default:
			logger.Debug("unknown file type:", srcSub)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func CopyDir(src, dst string, filterDirs, filterFiles []string, enableHardlink bool) error {

	if len(filterDirs) != 0 && IsExistsPath(filterDirs, src) {
		logger.Debugf("ignore dir path:%s", src)
		return nil
	}

	sfi, err := os.Stat(src)
	if err != nil {
		return err
	}

	err = Mkdir(src, dst)
	if err != nil {
		return err
	}

	fiList, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	for _, fi := range fiList {
		srcSub := filepath.Join(src, fi.Name())
		dstSub := filepath.Join(dst, fi.Name())

		fiStat, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to get raw stat for: %s", srcSub)
		}

		switch {
		case fiStat.Mode&syscall.S_IFLNK == syscall.S_IFLNK:
			err = Symlink(srcSub, dstSub)
		case fiStat.Mode&syscall.S_IFCHR == syscall.S_IFCHR:
			logger.Debug("[CopyDir] will remove(char):", dstSub)
			err = os.RemoveAll(dstSub)
		case fiStat.Mode&syscall.S_IFDIR == syscall.S_IFDIR:
			err = CopyDir(srcSub, dstSub, filterDirs, filterFiles, enableHardlink)
		case fiStat.Mode&syscall.S_IFREG == syscall.S_IFREG:
			if len(filterFiles) != 0 && IsExistsPath(filterFiles, srcSub) {
				logger.Debugf("ignore file path:%s", srcSub)
			} else {
				err = CopyFile2(srcSub, dstSub, sfi, enableHardlink)
			}
		default:
			logger.Debug("unknown file type:", srcSub)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func CopyFile(src, dst string, enableHardlink bool) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if !fi.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", src)
	}
	return CopyFile2(src, dst, fi, enableHardlink)
}

func CopyFile2(src, dst string, sfi os.FileInfo, enableHardlink bool) error {
	equal, err := IsFileSame(src, dst)
	if equal {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	err = Mkdir(filepath.Dir(src), filepath.Dir(dst))
	if err != nil {
		return err
	}

	_ = os.Remove(dst)
	if enableHardlink {
		err = os.Link(src, dst)
	}
	if err != nil || !enableHardlink {
		err = doCopy(src, dst, sfi)
	}
	return err
}

func IsFileSame(file1, file2 string) (bool, error) {
	equal, err := IsFileSameByInode(file1, file2)
	if err != nil {
		return false, err
	}
	if equal {
		return equal, nil
	}
	return IsFileSameByMD5(file1, file2)
}

func IsFileSameByInode(file1, file2 string) (bool, error) {
	fi1, err := os.Lstat(file1)
	if err != nil {
		return false, err
	}
	fi2, err := os.Lstat(file2)
	if err != nil {
		return false, err
	}
	if !fi1.Mode().IsRegular() {
		return false, fmt.Errorf("%s must be regular file", file1)
	}
	if !fi2.Mode().IsRegular() {
		return false, fmt.Errorf("%s must be regular file", file2)
	}

	stat1, ok := fi1.Sys().(*syscall.Stat_t)
	if !ok {
		return false, nil
	}
	stat2, ok := fi2.Sys().(*syscall.Stat_t)
	if !ok {
		return false, nil
	}
	return stat1.Ino == stat2.Ino, nil
}

func IsFileSameByMD5(file1, file2 string) (bool, error) {
	hash1, err := SumFileMD5(file1)
	if err != nil {
		return false, err
	}
	hash2, err := SumFileMD5(file2)
	if err != nil {
		return false, err
	}

	return hash1 == hash2, nil
}

func SumFileMD5(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func IsItemInList(item string, list []string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func doCopy(src, dst string, sfi os.FileInfo) error {
	err := ExecCommand("cp", []string{"-r", "-P", "--preserve=all", src, dst})
	return err
}

// func doCopy(src, dst string, sfi os.FileInfo) error {
// 	fs, err := os.Open(src)
// 	if err != nil {
// 		return err
// 	}
// 	defer fs.Close()
// 	fd, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, sfi.Mode())
// 	if err != nil {
// 		return err
// 	}
// 	defer fd.Close()

// 	_, err = io.Copy(fd, fs)
// 	_ = fd.Sync()
// 	return err
// }

// @title    HandlerDirPrepare
// @description   file preparation on rollback
// @param     src         		string         		"snapshot dir, ex:/persitent/osroot/v23/2020/etc"
// @param     dst         		string         		"dir to be rolled back, ex:/etc"
// @param     version         	string         		"snapshot version, ex:v23.0.0.1"
// @param     filter		    *[]string      		"list of files to filter"
// @return    dir				string   			"generated temporary directory"
func HandlerDirPrepare(src, dst, version, rootdir string, filter []string) (string, error) {
	dst = filepath.Join(rootdir, dst)
	src = filepath.Join(rootdir, src)
	dir := filepath.Join(dst, string("/.")+version)
	if IsExists(dir) {
		os.RemoveAll(dir)
	}
	err := Mkdir(dst, dir)
	logger.Debugf("start preparing the dir, src:%s, dir:%s, dst:%s, version:%s", src, dir, dst, version)
	if err != nil {
		return "", err
	}
	return dir, CompareDirAndCopy(src, dir, dst, filter)
}

func handlerDirReplace(dst, newDir, dir string, filter []string) (string, error) {
	// move files out of subfolders
	err := MoveDirSubFile(dst, dir, newDir, filter)
	if err != nil {
		return "", err
	}

	if IsExists(newDir) {
		logger.Debugf("start file replacement, dst:%s, newDir:%s", dst, newDir)
		// Move files out of repo
		err := SubMoveOut(newDir, dst)
		if err != nil {
			logger.Warningf("failed move sub dir, orig:%s, newDir:%s", dst, newDir)
			return dir, err
		}
	}
	return dir, nil
}

// @title    HandlerDirRollback
// @description   file replace on rollback
// @param     src         		string         		"snapshot dir, ex:/persitent/osroot/v23/2020/etc"
// @param     dst         		string         		"dir to be rolled back, ex:/etc"
// @param     version         	string         		"snapshot version, ex:v23.0.0.1"
// @param     filter		    *[]string      		"list of files to filter"
// @return    dir				string   			"generated temporary directory"
func HandlerDirRollback(src, dst, version, rootdir string, filter []string) (string, error) {
	dst = filepath.Join(rootdir, dst)
	src = filepath.Join(rootdir, src)
	newDir := filepath.Join(dst, string(".")+version)
	dir := filepath.Join(dst, string("/.old")+version)

	if IsExists(dir) {
		os.RemoveAll(dir)
	}
	err := Mkdir(dst, dir)
	logger.Debugf("start replace the dir, src:%s, dir:%s, dst:%s, version:%s", src, dir, dst, version)
	if err != nil {
		return "", err
	}
	return handlerDirReplace(dst, newDir, dir, filter)
}

// @title    HandlerDirRecover
// @description   file replace on rollback
// @param     src         		string         		"snapshot dir, ex:/persitent/osroot/v23/2020/etc"
// @param     dst         		string         		"dir to be rolled back, ex:/etc"
// @param     version         	string         		"snapshot version, ex:v23.0.0.1"
// @param     filter		    *[]string      		"list of files to filter"
// @return    dir				string   			"generated temporary directory"
func HandlerDirRecover(src, dst, version, rootdir string, filter []string) (string, error) {
	dst = filepath.Join(rootdir, dst)
	src = filepath.Join(rootdir, src)
	newDir := filepath.Join(dst, string(".")+version)
	dir := filepath.Join(dst, string("/.old")+version)

	if !IsExists(dir) {
		logger.Infof("%s are not rolled back and do not need to be restored", dir)
		return "", nil
	}
	logger.Debugf("start replace the dir, src:%s, dir:%s, dst:%s, version:%s", src, dir, dst, version)
	return handlerDirReplace(dst, dir, newDir, filter)
}

func TrimRootdir(rootDir, src string) string {
	if len(rootDir) != 1 && strings.HasPrefix(src, rootDir) {
		if strings.HasSuffix(rootDir, "/") {
			rootDir = strings.TrimRight(rootDir, "/")
		}
		return strings.TrimPrefix(src, rootDir)
	} else {
		return src
	}
}

func HandlerFilterList(rootDir, realDir string, filterList []string) (filterDirs, filterFiles []string) {
	for _, v := range filterList {
		realFilter := filepath.Join(rootDir, v)
		fi, e := os.Stat(realFilter)
		if e != nil {
			continue
		}
		real, err := filepath.EvalSymlinks(realFilter)
		if err != nil {
			real = realFilter
		}
		if strings.HasPrefix(real, realDir) {
			if fi.IsDir() {
				filterDirs = append(filterDirs, real)
			} else {
				filterFiles = append(filterFiles, real)
			}
		}
	}
	return filterDirs, filterFiles
}
