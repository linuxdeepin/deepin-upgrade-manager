package squashfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Mkfs(dataDir, filename string) error {
	out, err := exec.Command("mksquashfs", dataDir, filename,
		"-comp", "zstd").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func Mount(filename, dstDir string) error {
	_ = os.MkdirAll(filepath.Dir(dstDir), 0755)
	out, err := exec.Command("mount", "-t", "squashfs", "-o", "loop",
		filename, dstDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func Umount(dir string) error {
	out, err := exec.Command("umount", dir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}
