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
	m, err := upgrader.NewUpgraderTool()
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	switch *_action {
	case _ACTION_NOTIFY:
		err = m.SendSystemNotice()
		if err != nil {
			fmt.Printf("%v", err)
		}
	}
}
