package main

import (
	"bufio"
	"fmt"
	"kuvasz/log"
	"os"
	"time"
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

func CollectConntrackStat() error {
	var count, max, util float32
	var err error

	defer wg.Done()
	for {
		m = nil
		max, err = sendvar(TEST_PROCROOT+"/proc/sys/net/netfilter/nf_conntrack_max", "sar.net.conntrack.max")
		if err != nil {
			return err
		}

		count, err = sendvar(TEST_PROCROOT+"/proc/sys/net/netfilter/nf_conntrack_count", "sar.net.conntrack.count")
		if err != nil {
			return err
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
			return nil
		}
		time.Sleep(time.Duration(DELTA) * time.Second)
	}
}
