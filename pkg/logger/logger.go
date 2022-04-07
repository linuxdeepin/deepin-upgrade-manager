package logger

import (
	"fmt"
	"log"
	"log/syslog"
)

var _logger *syslog.Writer

func Open(debug bool, tag string) error {
	if _logger != nil {
		return nil
	}

	var err error

	priority := syslog.LOG_INFO
	if debug {
		priority = syslog.LOG_DEBUG
	}

	_logger, err = syslog.New(priority, tag)
	if err != nil {
		return err
	}
	return nil
}

func Debug(a ...interface{}) {
	log.Println(a...)
	_logger.Debug(fmt.Sprintln(a...))
}

func Info(a ...interface{}) {
	log.Println(a...)
	_logger.Info(fmt.Sprintln(a...))
}

func Warning(a ...interface{}) {
	log.Println(a...)
	_logger.Warning(fmt.Sprintln(a...))
}

func Error(a ...interface{}) {
	log.Println(a...)
	_logger.Err(fmt.Sprintln(a...))
}

func Fatal(a ...interface{}) {
	log.Println(a...)
	_logger.Crit(fmt.Sprintln(a...))
}

func Debugf(format string, a ...interface{}) {
	log.Printf(format+"\n", a...)
	_logger.Debug(fmt.Sprintf(format+"\n", a...))
}

func Infof(format string, a ...interface{}) {
	log.Printf(format+"\n", a...)
	_logger.Info(fmt.Sprintf(format+"\n", a...))
}

func Warningf(format string, a ...interface{}) {
	log.Printf(format+"\n", a...)
	_logger.Warning(fmt.Sprintf(format+"\n", a...))
}

func Errorf(format string, a ...interface{}) {
	log.Printf(format+"\n", a...)
	_logger.Err(fmt.Sprintf(format+"\n", a...))
}

func Fatalf(format string, a ...interface{}) {
	log.Printf(format+"\n", a...)
	_logger.Crit(fmt.Sprintf(format+"\n", a...))
}

func Close() error {
	return _logger.Close()
}
