// Make directory hardlink
package hardlink

import (
	"deepin-upgrade-manager/pkg/module/util"
)

func HardlinkDir(srcDir, dstDir string) error {
	return util.CopyDir(srcDir, dstDir, true)
}
