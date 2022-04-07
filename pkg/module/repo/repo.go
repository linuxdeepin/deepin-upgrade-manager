package repo

import (
	"deepin-upgrade-manager/pkg/module/repo/branch"
	"deepin-upgrade-manager/pkg/module/repo/ostree"
	"fmt"
)

type Repository interface {
	Init() error
	Exist(branchName string) bool
	Last() (string, error)
	List() (branch.BranchList, error)
	ListByName(branchName string, offset, limit int) (branch.BranchList, int, error)
	Snapshot(branchName, dstDir string) error
	Commit(branchName, subject, dataDir string) error
	Diff(baseBranch, targetBranch, dstFile string) error
	Cat(branchName, filepath, dstFile string) error
	Previous(targetName string) (string, error)
}

const (
	REPO_TY_OSTREE = iota + 1
)

func NewRepo(ty int, dir string) (Repository, error) {
	var _repo Repository
	switch ty {
	case REPO_TY_OSTREE:
		_repo, _ = ostree.NewRepo(dir)
	default:
		return nil, fmt.Errorf("unknown repo type: %d", ty)
	}

	return _repo, nil
}