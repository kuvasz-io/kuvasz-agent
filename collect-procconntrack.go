package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

var m Metrics

func sendvar(f string, metricname string) (float32, error) {
	var line string
	var v float32

	//#nosec
	fd, err := os.Open(f)
	if err != nil {
		log.Error(3, "[CONNTRACK] Cannot open %s: %s", f, err)
		return 0, err
	}
	defer fd.Close()
	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanLines)
	scanner.Scan()
	line = scanner.Text()
	log.Debug("[CONNTRACK] %s = %s", f, line)
	fmt.Sscanf(line, "%f", &v)
	m = send_metric(m, metricname, v)
	return v, nil
}

func CollectConntrackStat() {
	var count, max, util float32
	var err error

	ticker := time.NewTicker(time.Duration(DELTA) * time.Second)
	for range ticker.C {
		m = nil
		max, err = sendvar(TEST_PROCROOT+"/proc/sys/net/netfilter/nf_conntrack_max", "sar.net.conntrack.max")
		if err != nil {
			continue
		}

		count, err = sendvar(TEST_PROCROOT+"/proc/sys/net/netfilter/nf_conntrack_count", "sar.net.conntrack.count")
		if err != nil {
			continue
		}

		if max == 0 {
			util = count / max
		} else {
			util = 0
		}
		m = send_metric(m, "sar.net.conntrack.%util", util)
		log.Debug("[CONNTRACK] ----------------")
		metricschannel <- m
		if TEST_ENABLED {
			return
		}
	}
}
