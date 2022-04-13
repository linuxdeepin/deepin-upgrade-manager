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

	// set uid and gid
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
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

func CopyDir(src, dst, dataDir string, enableHardlink bool) error {
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
	if strings.HasPrefix(src, dataDir) {
		logger.Debugf("ignore data dir:%s", src)
		return nil
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
			err = CopyDir(srcSub, dstSub, dataDir, enableHardlink)
		case fiStat.Mode&syscall.S_IFREG == syscall.S_IFREG:
			err = CopyFile2(srcSub, dstSub, sfi, enableHardlink)
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
	if err != nil {
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
	fs, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fs.Close()
	fd, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, sfi.Mode())
	if err != nil {
		return err
	}
	defer fd.Close()

	_, err = io.Copy(fd, fs)
	_ = fd.Sync()
	return err
}
