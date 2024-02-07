package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

type DiskStat struct {
	major     uint64 // major number
	minor     uint64 // minor mumber
	name      string // device name
	reads     uint64 // reads completed successfully
	mrgreads  uint64 // reads merged
	rsec      uint64 // sectors read
	rtime     uint64 // time spent reading (ms)
	writes    uint64 // writes completed
	mrgwrites uint64 // writes merged
	wsec      uint64 // sectors written
	wtime     uint64 // time spent writing (ms)
	io        uint64 // io currently in progress
	rwtime    uint64 // time spent doing I/Os (ms)
	wghttime  uint64 // weighted time spent doing I/Os (ms)
}

var (
	oldDiskStat, newDiskStat [MAX_DISK]DiskStat
	num_disks                int
)

func ReadProcDiskStat(diskstat *[MAX_DISK]DiskStat) error {
	var line string
	var i int
	var re *regexp.Regexp

	re, err := regexp.Compile(DISKDEV_BLACKLIST)
	if err != nil {
		log.Error(3, "[DISK] Cannot compile blacklist regex %s: %s. Ignoring blacklist.", DISKDEV_BLACKLIST, err)
		re = nil
	}
	f, err := os.Open(TEST_PROCROOT + "/proc/diskstats")
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	for i = 0; i < MAX_DISK; {
		if !scanner.Scan() {
			break
		}
		line = scanner.Text()
		if line == "" {
			break
		}
		log.Debug("[DISK] Line = " + line)
		disk := &diskstat[i]
		_, err = fmt.Sscanf(line, "%d %d %s %d %d %d %d %d %d %d %d %d %d %d", &disk.major, &disk.minor, &disk.name, &disk.reads, &disk.mrgreads, &disk.rsec, &disk.rtime, &disk.writes, &disk.mrgwrites, &disk.wsec, &disk.wtime, &disk.io, &disk.rwtime, &disk.wghttime)
		if re != nil && re.MatchString(disk.name) {
			log.Trace("[DISK] Skipping blacklisted disk: %s", disk.name)
			continue
		}
		log.Debug("[DISK] Read device: %s (%d:%d)", disk.name, disk.major, disk.minor)
		if err != nil {
			return err
		}
		i++
	}
	num_disks = i
	log.Debug("[DISK] num_disks=%d", num_disks)
	return nil
}

func CollectDiskStat() {
	var prefix string
	var x, y, z float32
	var i int
	var m Metrics

	err := ReadProcDiskStat(&oldDiskStat)
	if err != nil {
		log.Error(3, "[DISK] Cannot parse initial /proc/diskstats: %s", err)
		return
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		err := ReadProcDiskStat(&newDiskStat)
		if err != nil {
			log.Error(3, "[DISK] Cannot get new stats: %s", err)
			return
		}

		for i = 0; i < num_disks; i++ {
			prefix = "sar.disk." + newDiskStat[i].name
			x = float32(newDiskStat[i].rwtime-oldDiskStat[i].rwtime) / float32(DELTA) / 10.0
			if x > 100.0 {
				x = 100.0
			}
			m = send_metric(m, prefix+".%util", x)

			x = float32(newDiskStat[i].rtime-oldDiskStat[i].rtime) / float32(DELTA) / 10.0
			if x > 100.0 {
				x = 100.0
			}
			m = send_metric(m, prefix+".%rutil", x)

			x = float32(newDiskStat[i].wtime-oldDiskStat[i].wtime) / float32(DELTA) / 10.0
			if x > 100.0 {
				x = 100.0
			}
			m = send_metric(m, prefix+".%wutil", x)

			x = float32(newDiskStat[i].rsec-oldDiskStat[i].rsec) / float32(DELTA)
			m = send_metric(m, prefix+".rd_sec-s", x)

			x = float32(newDiskStat[i].wsec-oldDiskStat[i].wsec) / float32(DELTA)
			m = send_metric(m, prefix+".wr_sec-s", x)

			y = float32(newDiskStat[i].reads + newDiskStat[i].writes - oldDiskStat[i].reads - oldDiskStat[i].writes)
			x = y / float32(DELTA)
			m = send_metric(m, prefix+".tps", x)

			x = float32(newDiskStat[i].reads-oldDiskStat[i].reads) / float32(DELTA)
			m = send_metric(m, prefix+".rtps", x)

			x = float32(newDiskStat[i].writes-oldDiskStat[i].writes) / float32(DELTA)
			m = send_metric(m, prefix+".wtps", x)

			if y == 0 {
				m = send_metric(m, prefix+".avgrq-sz", 0)
			} else {
				x = float32(newDiskStat[i].rsec+newDiskStat[i].wsec-oldDiskStat[i].rsec-oldDiskStat[i].wsec) / y
				m = send_metric(m, prefix+".avgrq-sz", x)
			}

			x = float32(newDiskStat[i].wghttime-oldDiskStat[i].wghttime) / float32(DELTA) / 1000.0
			m = send_metric(m, prefix+".avgqu-sz", x)

			z = float32(newDiskStat[i].rwtime - oldDiskStat[i].rwtime)
			if z == 0 {
				m = send_metric(m, prefix+".svctm", 0)
			} else {
				x = y / z
				m = send_metric(m, prefix+".svctm", x)
			}

			if y == 0 {
				m = send_metric(m, prefix+".await", 0)
			} else {
				x = float32(newDiskStat[i].rtime+newDiskStat[i].wtime-oldDiskStat[i].rtime-oldDiskStat[i].wtime) / y
				m = send_metric(m, prefix+".await", x)
			}

			y = float32(newDiskStat[i].reads - oldDiskStat[i].reads)
			if y == 0 {
				m = send_metric(m, prefix+".r_await", 0)
			} else {
				x = float32(newDiskStat[i].rtime-oldDiskStat[i].rtime) / y
				m = send_metric(m, prefix+".r_await", x)
			}

			y = float32(newDiskStat[i].writes - oldDiskStat[i].writes)
			if y == 0 {
				m = send_metric(m, prefix+".w_await", 0)
			} else {
				x = float32(newDiskStat[i].wtime-oldDiskStat[i].wtime) / y
				m = send_metric(m, prefix+".w_await", x)
			}
		}
		oldDiskStat = newDiskStat
		log.Debug("[DISK] ----------------")
		metricschannel <- m
		m = nil
		if TEST_ENABLED {
			return
		}
	}

}
