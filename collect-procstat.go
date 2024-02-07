package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

type CpuUsage struct {
	user      uint64 // normal processes executing in user mode
	nice      uint64 // niced processes executing in user mode
	system    uint64 // processes executing in kernel mode
	idle      uint64 // twiddling thumbs
	iowait    uint64 // waiting for I/O to complete
	irq       uint64 // servicing interrupts
	softirq   uint64 // servicing softirqs
	steal     uint64 // involuntary wait
	guest     uint64 // running a normal guest
	niceguest uint64 // running a niced guest
}

type ProcStat struct {
	num_cpu     int
	num_metrics int
	cpu         [MAX_CPU]CpuUsage
	cswch       uint64 // context switches
	proc        uint64 // processes created
	running     uint64 // running processes
	blocked     uint64 // blocked processes
}

var (
	oldStat, newStat ProcStat
)

func ReadProcStat(procstat *ProcStat) error {
	var keyword string
	var line string
	var i, n int

	f, err := os.Open(TEST_PROCROOT + "/proc/stat")
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	scanner.Scan()
	line = scanner.Text()
	for i = 0; line[0] == 'c'; i++ {
		log.Debug("[CPU] Line = " + line)
		cpu := &procstat.cpu[i]
		n, err = fmt.Sscanf(line, "%s %d %d %d %d %d %d %d %d %d %d", &keyword, &cpu.user, &cpu.nice, &cpu.system, &cpu.idle, &cpu.iowait, &cpu.irq, &cpu.softirq, &cpu.steal, &cpu.guest, &cpu.niceguest)
		if (err != nil) && (n < 7) {
			return err
		}
		log.Debug("[CPU] Metrics = %d", n-1)
		scanner.Scan()
		line = scanner.Text()
	}
	procstat.num_cpu = i - 1
	procstat.num_metrics = n - 1

	// Skip lines then read context switches
	for !strings.HasPrefix(line, "ctxt") {
		log.Debug("[CPU] Skipped line: %s", line)
		scanner.Scan()
		line = scanner.Text()
	}
	log.Debug("[CPU] Reading context switches line: %s", line)
	_, err = fmt.Sscanf(line, "%s %d", &keyword, &procstat.cswch)
	if err != nil {
		return err
	}

	// Skip lines then read process creations
	scanner.Scan()
	line = scanner.Text()
	for !strings.HasPrefix(line, "processes") {
		log.Debug("[CPU] Skipped line: %s", line)
		scanner.Scan()
		line = scanner.Text()
	}
	log.Debug("[CPU] Reading process creation line: %s", line)
	_, err = fmt.Sscanf(line, "%s %d", &keyword, &procstat.proc)
	if err != nil {
		return err
	}

	// Skip lines then read running processes (gauge)
	scanner.Scan()
	line = scanner.Text()
	for !strings.HasPrefix(line, "procs_running") {
		log.Debug("[CPU] Skipped line: %s", line)
		scanner.Scan()
		line = scanner.Text()
	}
	log.Debug("[CPU] Reading running processes line: %s", line)
	_, err = fmt.Sscanf(line, "%s %d", &keyword, &procstat.running)
	if err != nil {
		return err
	}

	// Skip lines then read running processes (gauge)
	scanner.Scan()
	line = scanner.Text()
	for !strings.HasPrefix(line, "procs_blocked") {
		log.Debug("[CPU] Skipped line: %s", line)
		scanner.Scan()
		line = scanner.Text()
	}
	log.Debug("[CPU] Reading blocked processes line: %s", line)
	_, err = fmt.Sscanf(line, "%s %d", &keyword, &procstat.blocked)
	return err
}

func CollectProcStat() {
	var prefix string
	var m Metrics

	err := ReadProcStat(&oldStat)
	if err != nil {
		log.Error(3, "[CPU] Cannot get initial stats: %s", err)
		return
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		err := ReadProcStat(&newStat)
		if err != nil {
			log.Error(3, "[CPU] Cannot get stats: %s", err)
			return
		}

		for i := 0; i <= oldStat.num_cpu; i++ {
			user := newStat.cpu[i].user - oldStat.cpu[i].user
			nice := newStat.cpu[i].nice - oldStat.cpu[i].nice
			system := newStat.cpu[i].system - oldStat.cpu[i].system
			idle := newStat.cpu[i].idle - oldStat.cpu[i].idle
			iowait := newStat.cpu[i].iowait - oldStat.cpu[i].iowait
			irq := newStat.cpu[i].irq - oldStat.cpu[i].irq
			softirq := newStat.cpu[i].softirq - oldStat.cpu[i].softirq
			steal := newStat.cpu[i].steal - oldStat.cpu[i].steal
			guest := newStat.cpu[i].guest - oldStat.cpu[i].guest
			niceguest := newStat.cpu[i].niceguest - oldStat.cpu[i].niceguest
			scale := 100.0 / float32(user+nice+system+idle+iowait+irq+softirq+steal+guest+niceguest)
			if i == 0 {
				prefix = "sar.cpu.all."
			} else {
				prefix = fmt.Sprintf("sar.cpu.cpu%d.", i-1)
			}
			m = send_metric(m, prefix+"%user", float32(user)*scale)
			m = send_metric(m, prefix+"%nice", float32(nice)*scale)
			m = send_metric(m, prefix+"%system", float32(system)*scale)
			m = send_metric(m, prefix+"%idle", float32(idle)*scale)
			m = send_metric(m, prefix+"%iowait", float32(iowait)*scale)
			m = send_metric(m, prefix+"%irq", float32(irq)*scale)
			m = send_metric(m, prefix+"%softirq", float32(softirq)*scale)
			if oldStat.num_metrics > 7 {
				m = send_metric(m, prefix+"%steal", float32(steal)*scale)
			}
			if oldStat.num_metrics > 8 {
				m = send_metric(m, prefix+"%guest", float32(guest)*scale)
			}
			if oldStat.num_metrics > 9 {
				m = send_metric(m, prefix+"%niceguest", float32(niceguest)*scale)
			}
			m = send_metric(m, "sar.task.cswch-s", float32(newStat.cswch-oldStat.cswch)/float32(DELTA))
			m = send_metric(m, "sar.task.proc-s", float32(newStat.proc-oldStat.proc)/float32(DELTA))
			m = send_metric(m, "sar.task.running", float32(newStat.running))
			m = send_metric(m, "sar.task.blocked", float32(newStat.blocked))
		}
		oldStat = newStat
		log.Debug("[CPU] ----------------")
		metricschannel <- m
		m = nil
		if TEST_ENABLED {
			return
		}
	}
}
