package main

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

const (
	RE_PROCSTAT = "([0-9]+) \\((.*)\\) (.*)"
)

type PidInfo struct {
	selected bool
	user     string
	pid      uint64 // process id
	comm     string // filename of the executable
	state    byte   // R  Running
	// S  Sleeping in an interruptible wait
	// D  Waiting in uninterruptible disk sleep
	// Z  Zombie
	// T  Stopped (on a signal) or (before Linux 2.6.33) trace stopped
	// t  Tracing stop (Linux 2.6.33 onward)
	// W  Paging (only before Linux 2.6.0)
	// X  Dead (from Linux 2.6.0 onward)
	// x  Dead (Linux 2.6.33 to 3.13 only)
	// K  Wakekill (Linux 2.6.33 to 3.13 only)
	// W  Waking (Linux 2.6.33 to 3.13 only)
	// P  Parked (Linux 3.9 to 3.13 only)
	ppid        uint64 // process id of the parent process
	minflt      uint64 // number of minor faults
	majflt      uint64 // number of major faults
	utime       uint64 // user mode jiffies
	stime       uint64 // kernel mode jiffies
	num_threads uint64 // number of threads
	vsize       uint64 // virtual memory size (bytes)
	rss         uint64 // resident set memory size (bytes)
	blkio_ticks uint64 // time spent waiting for block IO
	gtime       uint64 // guest time of the task in jiffies
	// rchar                 uint64 // chars read
	// wchar                 uint64 // chars written
	// syscr                 uint64 // read syscalls
	// syscw                 uint64 // write syscalls
	read_bytes            uint64 // bytes read
	write_bytes           uint64 // bytes written
	cancelled_write_bytes uint64 // bytes to be written but cancelled due to file truncated
}

// Map to access unique pid information
type PidStat map[string]PidInfo

// Array containing cpu load for sorting
type LoadIndex struct {
	key   string
	value float32
}
type LoadSlice []LoadIndex

// Sorting functions
func (slice LoadSlice) Len() int {
	return len(slice)
}

func (slice LoadSlice) Less(i, j int) bool {
	return slice[i].value > slice[j].value
}

func (slice LoadSlice) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

// Tree holding ppid - pid-list
type PidSlice []PidInfo
type PidTree map[uint64]PidSlice

var re_white, re_black *regexp.Regexp
var re_procstat *regexp.Regexp

func re_match(re *regexp.Regexp, s string) bool {
	if re == nil {
		return false
	} else {
		return re.MatchString(s)
	}
}

func ReadPidStat() PidStat {
	var p PidInfo
	var pidstat PidStat
	var keyword string
	var value uint64

	f, err := os.Open(TEST_PROCROOT + "/proc")
	if err != nil {
		log.Error(3, "[PS] Can't open /proc directory: %s", err)
		return nil
	}
	list, err := f.Readdir(-1)
	if err != nil {
		log.Error(3, "[PS] Can't read /proc directory: %s", err)
		f.Close()
		return nil
	}
	f.Close()
	pidstat = make(PidStat)
	for i := range list {
		if !list[i].IsDir() {
			continue
		}
		pid, err := strconv.Atoi(list[i].Name())
		if err != nil {
			log.Debug("[PS] Can't get pid file name, skipping: %s", err)
			continue
		}
		uid := list[i].Sys().(*syscall.Stat_t).Uid
		u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
		if err != nil {
			log.Error(3, "[PS] Can't get owner name for %d, skipping: ", pid, err)
			p.user = "root"
		} else {
			log.Trace("[PS] Found owner %s for directory %s", u.Username, list[i].Name())
			p.user = u.Username
		}
		f, err = os.Open(TEST_PROCROOT + "/proc/" + list[i].Name() + "/stat")
		if err != nil {
			log.Error(3, "[PS] Can't open /proc/%s/stat: %s", list[i].Name(), err)
			continue
		}
		scanner := bufio.NewScanner(f)
		scanner.Split(bufio.ScanLines)
		scanner.Scan()
		line := scanner.Text()
		f.Close()
		log.Debug("[PS] /proc/%d/stat:%s", pid, line)
		fields_command := re_procstat.FindStringSubmatch(line)
		log.Trace("[PS] fields=%q", fields_command)
		p.pid, _ = strconv.ParseUint(fields_command[1], 0, 64)
		name := strings.ReplaceAll(fields_command[2], "/", "-")
		name = strings.ReplaceAll(name, " ", "-")
		name = strings.ReplaceAll(name, ".", "-")
		p.comm = name

		fields := strings.Split(fields_command[3], " ")
		if len(fields) < 40 {
			log.Error(3, "[PS] /proc/%d/stat format error, expecting at least 40 values, found %d", pid, len(fields))
			continue
		}
		p.state = fields[0][0]
		p.ppid, _ = strconv.ParseUint(fields[1], 0, 64)
		if (PS_CONS_KERNEL == 1) && ((p.ppid == 2) || p.pid == 2) {
			p.comm = "kernel"
		}
		p.minflt, _ = strconv.ParseUint(fields[7], 0, 64)
		p.majflt, _ = strconv.ParseUint(fields[9], 0, 64)
		p.utime, _ = strconv.ParseUint(fields[11], 0, 64)
		p.stime, _ = strconv.ParseUint(fields[12], 0, 64)
		p.num_threads, _ = strconv.ParseUint(fields[17], 0, 64)
		p.vsize, _ = strconv.ParseUint(fields[20], 0, 64)
		p.rss, _ = strconv.ParseUint(fields[21], 0, 64)
		if len(fields) >= 42 {
			p.blkio_ticks, _ = strconv.ParseUint(fields[39], 0, 64)
		}
		if len(fields) >= 43 {
			p.gtime, _ = strconv.ParseUint(fields[40], 0, 64)
		}

		f, err = os.Open(TEST_PROCROOT + "/proc/" + list[i].Name() + "/io")
		if err != nil {
			log.Error(3, "[PS] Can't open /proc/%s/io: %s", list[i].Name(), err)
			continue
		}
		scanner = nil
		scanner = bufio.NewScanner(f)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			line = scanner.Text()
			log.Trace("[PS] IO Line = %s", line)
			fmt.Sscanf(line, "%s %d", &keyword, &value)
			switch keyword {
			case "read_bytes:":
				p.read_bytes = value
			case "write_bytes:":
				p.write_bytes = value
			case "cancelled_write_bytes:":
				p.cancelled_write_bytes = value
			}
		}
		f.Close()
		pidstat[fields_command[1]+p.comm] = p
		log.Trace("[PS] pid = %d, comm=%s, state=%c, ppid=%d, minflt=%d, majflt=%d, utime=%d, stime=%d, num_threads=%d, vsize=%d, rss=%d, blkio_ticks=%d, gtime=%d, read_bytes=%d, write_bytes=%d, cancelled_write_bytes=%d", p.pid, p.comm, p.state, p.ppid, p.minflt, p.majflt, p.utime, p.stime, p.num_threads, p.vsize, p.rss, p.blkio_ticks, p.gtime, p.read_bytes, p.write_bytes, p.cancelled_write_bytes)
	}
	return pidstat
}

func SendPIDMetrics(m Metrics, p *PidInfo) Metrics {
	prefix := "ps." + p.comm + "."
	if PS_CONS_USER == 1 {
		prefix = prefix + p.user + "."
	} else {
		prefix = prefix + strconv.FormatUint(p.pid, 10) + "."
	}
	m = send_metric_uint64_delta(m, prefix+"user", p.utime)
	m = send_metric_uint64_delta(m, prefix+"system", p.stime)
	m = send_metric_uint64_delta(m, prefix+"iowait", p.blkio_ticks)
	m = send_metric_uint64_delta(m, prefix+"guest", p.gtime)
	m = send_metric_uint64_delta(m, prefix+"minflt", p.minflt)
	m = send_metric_uint64_delta(m, prefix+"majflt", p.minflt)
	m = send_metric_uint64_gauge(m, prefix+"vsize", p.vsize)
	m = send_metric_uint64_gauge(m, prefix+"rss", p.rss*PAGE_SIZE)
	m = send_metric_uint64_gauge(m, prefix+"num_threads", p.num_threads)
	m = send_metric_uint64_delta(m, prefix+"read", p.read_bytes)
	m = send_metric_uint64_delta(m, prefix+"write", p.write_bytes)
	m = send_metric_uint64_delta(m, prefix+"cancelled_write", p.cancelled_write_bytes)
	return m
}

func add_pid(pidslice PidSlice, p, oldp *PidInfo) PidSlice {
	var pidinfo PidInfo

	pidinfo.pid = p.pid
	pidinfo.user = p.user
	pidinfo.comm = p.comm
	pidinfo.state = p.state
	pidinfo.ppid = p.ppid
	pidinfo.utime = p.utime - oldp.utime
	pidinfo.stime = p.stime - oldp.stime
	pidinfo.blkio_ticks = p.blkio_ticks - oldp.blkio_ticks
	pidinfo.gtime = p.gtime - oldp.gtime
	pidinfo.minflt = p.minflt - oldp.minflt
	pidinfo.majflt = p.majflt - oldp.majflt
	pidinfo.vsize = p.vsize
	pidinfo.rss = p.rss
	pidinfo.num_threads = p.num_threads
	pidinfo.read_bytes = p.read_bytes - oldp.read_bytes
	pidinfo.write_bytes = p.write_bytes - oldp.write_bytes
	pidinfo.cancelled_write_bytes = p.cancelled_write_bytes - oldp.cancelled_write_bytes
	pidslice = append(pidslice, pidinfo)
	return pidslice
}

func consolidate(pidtree PidTree, consPidStat PidStat, pidslice PidSlice) {
	var pidinfo, conspidinfo PidInfo
	var ok bool

	log.Trace("[PS] Consolidating pidslice %v", pidslice)
	log.Trace("[PS] Consolidated data %v", consPidStat)
	for _, pidinfo = range pidslice {
		k := pidinfo.comm + "-" + strconv.FormatUint(pidinfo.ppid, 10)
		conspidinfo, ok = consPidStat[k]
		if ok {
			log.Trace("[PS] Consolidating ppid=%d, comm=%s with pid=%d", conspidinfo.pid, pidinfo.comm, pidinfo.pid)
			conspidinfo.utime += pidinfo.utime
			conspidinfo.stime += pidinfo.stime
			conspidinfo.blkio_ticks += pidinfo.blkio_ticks
			conspidinfo.gtime += pidinfo.gtime
			conspidinfo.minflt += pidinfo.minflt
			conspidinfo.majflt += pidinfo.majflt
			conspidinfo.vsize += pidinfo.vsize
			conspidinfo.rss += pidinfo.rss
			conspidinfo.num_threads += pidinfo.num_threads
			conspidinfo.read_bytes += pidinfo.read_bytes
			conspidinfo.write_bytes += pidinfo.write_bytes
			conspidinfo.cancelled_write_bytes += pidinfo.cancelled_write_bytes
			consPidStat[k] = conspidinfo
		} else {
			log.Trace("[PS] New pid = %d", pidinfo.pid)
			k = pidinfo.comm + "-" + strconv.FormatUint(pidinfo.pid, 10)
			if re_match(re_white, pidinfo.comm) {
				log.Debug("[PS] Selecting whitelisted process %s", pidinfo.comm)
				pidinfo.selected = true
			}
			consPidStat[k] = pidinfo
		}
		childslice, ok := pidtree[pidinfo.pid]
		if ok {
			consolidate(pidtree, consPidStat, childslice)
		} else {
			log.Trace("[PS] Pid %d has no children", pidinfo.pid)
		}
	}
}

func CollectProcPidStat() {
	var OldProcPidStat, NewProcPidStat, ConsPidStat PidStat
	var pidtree PidTree
	var top_n_cpu LoadSlice
	var top_n_disk LoadSlice
	var loadindex LoadIndex
	var err error
	var usage float32
	var m Metrics

	re_black, err = regexp.Compile(PS_BLACKLIST)
	if err != nil {
		log.Error(3, "[PS] Cannot compile blacklist regex %s: %s. Ignoring blacklist.", PS_BLACKLIST, err)
		re_black = nil
	}
	re_white, err = regexp.Compile(PS_WHITELIST)
	if err != nil {
		log.Error(3, "[PS] Cannot compile whitelist regex %s: %s. Ignoring whitelist.", PS_WHITELIST, err)
		re_white = nil
	}
	re_procstat = regexp.MustCompile(RE_PROCSTAT)
	log.Debug("[PS] Reading initial process tree")
	OldProcPidStat = ReadPidStat()
	if OldProcPidStat == nil {
		return
	}

	for {
		time.Sleep(time.Duration(DELTA) * time.Second)

		// Collect new round of metrics
		log.Debug("[PS] Reading process tree")
		NewProcPidStat = ReadPidStat()
		if NewProcPidStat == nil {
			return
		}

		// Put metrics in tree by parent pid
		log.Trace("[PS] Create first tree level by consolidating by ppid")
		pidtree = make(PidTree)
		for k, p := range NewProcPidStat {
			oldp, ok := OldProcPidStat[k]
			if !ok {
				log.Trace("[PS] Process %s is new", k)
				continue
			}
			log.Trace("[PS] Adding pid %d to ppid %d", p.pid, p.ppid)
			pidtree[p.ppid] = add_pid(pidtree[p.ppid], &p, &oldp)
			// log.Trace("pidtree = %v", pidtree)
		}

		// Consolidate child processes with same name under their parent
		ConsPidStat = make(PidStat)
		log.Trace("[PS] Pidtree = %v", pidtree)
		consolidate(pidtree, ConsPidStat, pidtree[0])
		log.Trace("[PS] Consolidated pidtree = %v", ConsPidStat)

		// Sort by total CPU usage
		for k, p := range ConsPidStat {
			usage = float32(p.utime+p.stime+p.blkio_ticks) / float32(DELTA)
			if usage < PS_THRESHOLD_CPU {
				log.Trace("[PS] Skipping %s: cpu usage %f < %f", p.comm, usage, PS_THRESHOLD_CPU)
				continue
			}
			loadindex.key = k
			loadindex.value = usage
			top_n_cpu = append(top_n_cpu, loadindex)
		}
		sort.Sort(top_n_cpu)

		// Now sort by total disk read+write+cancelledwrite usage
		for k, p := range ConsPidStat {
			usage = float32(p.read_bytes+p.write_bytes+p.cancelled_write_bytes) / float32(DELTA)
			log.Trace("[PS] %s: read=%d, write=%d, cancelled_write=%d, usage=%f", p.comm, p.read_bytes, p.write_bytes, p.cancelled_write_bytes, usage)
			if usage < PS_THRESHOLD_DISK {
				log.Trace("[PS] Skipping %s: disk usage %f < %f", p.comm, usage, PS_THRESHOLD_DISK)
				continue
			}
			loadindex.key = k
			loadindex.value = usage
			top_n_disk = append(top_n_disk, loadindex)
		}
		sort.Sort(top_n_disk)

		// Select top n cpu, exclude blacklisted processes
		log.Trace("[PS] Top n cpu user processes, including whitelist, excluding blacklist")
		for i, loadindex := range top_n_cpu {
			p := ConsPidStat[loadindex.key]
			if !re_match(re_black, p.comm) { // select non-blacklisted processes only
				p.selected = true
				ConsPidStat[loadindex.key] = p
				log.Debug("[PS] Selecting: %s", p.comm)
			}
			if i > PS_TOP_N_CPU {
				break
			}
		}
		log.Debug("[PS] ----------------")

		// Select top n disk, exclude blacklisted processes
		log.Trace("[PS] Top n disk using processes, excluding blacklist")
		for i, loadindex := range top_n_disk {
			p := ConsPidStat[loadindex.key]
			if !re_match(re_black, p.comm) { // select non-blacklisted processes only
				p.selected = true
				ConsPidStat[loadindex.key] = p
				log.Debug("[PS] Selecting: %s", p.comm)
			}
			if i > PS_TOP_N_DISK {
				break
			}
		}
		log.Debug("[PS] ----------------")

		// Now send all selected processes
		for _, p := range ConsPidStat {
			if p.selected {
				m = SendPIDMetrics(m, &p)
			}
		}
		top_n_cpu = nil
		top_n_disk = nil
		log.Debug("[PS] ----------------")
		OldProcPidStat = NewProcPidStat
		metricschannel <- m
		m = nil
		if TEST_ENABLED {
			return
		}
	}
}
