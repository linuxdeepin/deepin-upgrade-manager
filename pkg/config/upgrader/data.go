package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type Target struct {
	Backup_list []string `yaml:"backup_list"`
	Hold_list   []string `yaml:"hold_list"`
	After_run   string   `yaml:"after_run"`
}

type CommitTarget struct {
	Target Target `yaml:"target"`
}

func LoadDataConfig(filename string) (*CommitTarget, error) {
	conf := new(CommitTarget)
	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		return conf, err
	}
	err = yaml.Unmarshal(yamlFile, conf)
	if err != nil {
		return conf, err
	}
	return conf, nil
}
