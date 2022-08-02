package main

import (
	"deepin-upgrade-manager/pkg/upgrader"
	"flag"
	"fmt"
	"os"
)

const (
	_ACTION_NOTIFY = "notify"
)

var (
	_action = flag.String("action", "", "the available actions: notify")
)

func main() {
	flag.Parse()
	m := upgrader.NewUpgraderTool()
	switch *_action {
	case _ACTION_NOTIFY:
		err := m.LoadRollbackRecords(false)
		if err != nil {
			fmt.Printf("%v", err)
			os.Exit(-1)
		}
		err = m.SendSystemNotice()
		if err != nil {
			fmt.Printf("%v", err)
		}
	}
}
