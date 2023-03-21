// SPDX-FileCopyrightText: 2018 - 2023 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package records

import (
	"deepin-upgrade-manager/pkg/module/util"
	"errors"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

const (
	SelfRecordResultPath = "/etc/deepin-upgrade-manager/result.records"
)

func ReadResult() (int, string, error) {
	res := -1
	var cmd string
	if !util.IsExists(SelfRecordResultPath) {
		return res, cmd, errors.New("file isn't exist")
	}

	file, err := os.Open(SelfRecordResultPath)
	if err != nil {
		return res, cmd, err
	}
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	if err != nil {
		return res, cmd, err
	}
	line := strings.Split(string(content), ",")
	if len(line) == 0 {
		return res, cmd, errors.New("content isn't exist")
	}
	result, err := strconv.Atoi(line[0])
	if err != nil {
		return res, cmd, err
	}
	if len(line) == 2 {
		cmd = line[1]
	}
	if RecoredState(result) == _ROLLBACK_SUCCESSED {
		res = 1
	}
	if RecoredState(result) == _ROLLBACK_FAILED {
		res = 0
	}
	return res, cmd, nil
}

func RemoveResult() bool {
	if util.IsExists(SelfRecordResultPath) {
		os.RemoveAll(SelfRecordResultPath)
		return true
	} else {
		return false
	}
}
