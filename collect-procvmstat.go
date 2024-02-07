package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

type VMStat struct {
	pgpgin, pgpgout, pswpin, pswpout, pgfault, pgmajfault uint64
}

func ReadVMStat(vmstat *VMStat) error {
	var line string
	var keyword string
	var value uint64

	f, err := os.Open(TEST_PROCROOT + "/proc/vmstat")
	if err != nil {
		log.Error(3, "[VM] Cannot open /proc/vmstat: %s", err)
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line = scanner.Text()
		log.Trace("[VM] Line = %s", line)
		fmt.Sscanf(line, "%s %d", &keyword, &value)
		switch keyword {
		case "pgpgin":
			vmstat.pgpgin = value
		case "pgpgout":
			vmstat.pgpgout = value
		case "pswpin":
			vmstat.pswpin = value
		case "pswpout":
			vmstat.pswpout = value
		case "pgfault":
			vmstat.pgfault = value
		case "pgmajfault":
			vmstat.pgmajfault = value
		}
	}
	return nil
}

func CollectVMStat() {
	var OldVMStat, NewVMStat VMStat
	var m Metrics

	err := ReadVMStat(&OldVMStat)
	if err != nil {
		return
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		err := ReadVMStat(&NewVMStat)
		if err != nil {
			return
		}
		m = send_metric(m, "sar.paging.pgpgin-s", float32(NewVMStat.pgpgin-OldVMStat.pgpgin)/float32(DELTA))
		m = send_metric(m, "sar.paging.pgpgout-s", float32(NewVMStat.pgpgout-OldVMStat.pgpgout)/float32(DELTA))
		m = send_metric(m, "sar.swap.pswpin-s", float32(NewVMStat.pswpin-OldVMStat.pswpin)/float32(DELTA))
		m = send_metric(m, "sar.swap.pswpout-s", float32(NewVMStat.pswpout-OldVMStat.pswpout)/float32(DELTA))
		m = send_metric(m, "sar.paging.pgflt-s", float32(NewVMStat.pgfault-OldVMStat.pgfault)/float32(DELTA))
		m = send_metric(m, "sar.paging.pgmajflt-s", float32(NewVMStat.pgmajfault-OldVMStat.pgmajfault)/float32(DELTA))
		log.Debug("[VM] ----------------")
		OldVMStat = NewVMStat
		metricschannel <- m
		m = nil
		if TEST_ENABLED {
			return
		}
	}
}
