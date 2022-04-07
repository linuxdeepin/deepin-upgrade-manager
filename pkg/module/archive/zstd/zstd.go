package zstd

import (
	"fmt"
	"os/exec"
)

type ZSTD struct{}

func (handler *ZSTD) Compress(files []string, filename string) error {
	var args = []string{"--zstd", "-cf", filename}
	args = append(args, files...)
	out, err := exec.Command("tar", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func (handler *ZSTD) Extract(filename, dstDir string) error {
	out, err := exec.Command("tar", "--zstd", "-xf", filename,
		"-C", dstDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}
