package config

import (
	"encoding/json"
)

type Info struct {
	SubmissionTime string `json:"SubmissionTime"`
	SystemVersion  string `json:"SystemVersion"`
	SubmissionType int    `json:"SubmissionType"`
	UUID           string `json:"UUID"`
	Note           string `json:"Note"`
}

func (info Info) Time() string {
	return info.SubmissionTime
}

func LoadSubject(subject string) (Info, error) {
	var info Info

	err := json.Unmarshal([]byte(subject), &info)
	if err != nil {
		return info, nil
	}
	return info, nil
}
