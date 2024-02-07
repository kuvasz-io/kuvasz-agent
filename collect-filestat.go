package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

func stof(s string) float32 {
	x, _ := strconv.ParseUint(s, 10, 64)
	return float32(x)
}

func CollectFileStat() {
	var line string
	var m Metrics

	for {
		f, err := os.Open(TEST_PROCROOT + "/proc/sys/fs/file-nr")
		if err != nil {
			log.Error(3, "[FILE] Cannot open /proc/sys/fs/file-nr: %s", err)
			return
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Split(bufio.ScanLines)
		scanner.Scan()
		line = scanner.Text()
		log.Trace("[FILE] Line = %s", line)
		fields := strings.Fields(line)
		m = send_metric(m, "sar.file.file-nr", stof(fields[0]))
		m = send_metric(m, "sar.file.file-max", stof(fields[2]))
		f.Close()

		f, err = os.Open(TEST_PROCROOT + "/proc/sys/fs/inode-nr")
		if err != nil {
			log.Error(3, "[FILE] Cannot open /proc/sys/fs/inode-nr: %s", err)
			return
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
		scanner.Split(bufio.ScanLines)
		scanner.Scan()
		line = scanner.Text()
		log.Trace("[FILE] Line = %s", line)
		fields = strings.Fields(line)
		m = send_metric(m, "sar.file.inode-nr", stof(fields[1]))
		m = send_metric(m, "sar.file.inode-max", stof(fields[0]))
		f.Close()
		log.Debug("[FILE] ----------------")
		metricschannel <- m
		m = nil
		if TEST_ENABLED {
			return
		} else {
			time.Sleep(time.Duration(DELTA) * time.Second)
		}
	}
}
