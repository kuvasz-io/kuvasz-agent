package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

func CollectMemStat() {
	var keyword string
	var value float32
	var line string
	var swaptotal float32
	var kbmemtotal, kbmemfree, kbbuffers, kbmemused float32
	var m Metrics

	for {
		f, err := os.Open(TEST_PROCROOT + "/proc/meminfo")
		if err != nil {
			log.Error(3, "[MEM] Cannot open /proc/meminfo: %s", err)
			return
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			line = scanner.Text()
			log.Trace("[MEM] Line = %s", line)
			fmt.Sscanf(line, "%s %f kB", &keyword, &value)
			switch keyword {
			case "MemTotal:":
				m = send_metric(m, "sar.mem.kbmemtotal", value)
				kbmemtotal = value
			case "MemFree:":
				m = send_metric(m, "sar.mem.kbmemfree", value)
				kbmemfree = value
			case "Buffers:":
				m = send_metric(m, "sar.mem.kbbuffers", value)
				kbbuffers = value
			case "Cached:":
				m = send_metric(m, "sar.mem.kbcached", value)
				kbmemused = kbmemtotal - kbmemfree - kbbuffers - value
				m = send_metric(m, "sar.mem.kbmemused", kbmemused)
				m = send_metric(m, "sar.mem.%used", 100*kbmemused/kbmemtotal)
			case "SwapTotal:":
				m = send_metric(m, "sar.swap.kbswptotal", value)
				swaptotal = value
			case "SwapFree:":
				m = send_metric(m, "sar.swap.kbswpfree", value)
				m = send_metric(m, "sar.swap.kbswpused", swaptotal-value)
				if swaptotal != 0.0 {
					m = send_metric(m, "sar.swap.%used", 100*(swaptotal-value)/swaptotal)
				} else {
					m = send_metric(m, "sar.swap.%used", 0)
				}

			}
			f.Close()
		}
		log.Debug("[MEM] ----------------")
		metricschannel <- m
		m = nil
		if TEST_ENABLED {
			return
		}
		time.Sleep(time.Duration(DELTA) * time.Second)
	}
}
