package main

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

func getfstypes() map[string]bool {
	var fstypes map[string]bool
	var line string

	fstypes = make(map[string]bool)
	f, err := os.Open(TEST_PROCROOT + "/proc/filesystems")
	if err != nil {
		log.Error(3, "[FS] Cannot open /proc/filesystems: %s", err)
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line = scanner.Text()
		log.Trace("[FS] Line = %s", line)
		if strings.HasPrefix(line, "nodev") {
			continue
		}
		fstypes[strings.Trim(line, " \t")] = true
	}
	return fstypes
}

func CollectFSStat() {
	var line string
	var fstypes map[string]bool
	var name string
	var stat syscall.Statfs_t
	var m Metrics

	re, err := regexp.Compile(FS_BLACKLIST)
	if err != nil {
		log.Error(3, "[FS] Cannot compile blacklist regex %s: %s. Ignoring blacklist.", FS_BLACKLIST, err)
		re = nil
	}

	for {
		fstypes = getfstypes()
		f, err := os.Open(TEST_PROCROOT + "/proc/mounts")
		if err != nil {
			log.Error(3, "[FS] Cannot open /proc/mounts: %s", err)
			return
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			line = scanner.Text()
			log.Trace("[FS] Line = %s", line)
			fields := strings.Fields(line)
			if fstypes[fields[2]] {
				components := strings.Split(fields[1], "/")
				name = components[len(components)-1]
				if re != nil && re.MatchString(name) {
					continue
				}
				if name == "" {
					name = "root"
				}
				err = syscall.Statfs(fields[1], &stat)
				if err != nil {
					log.Error(3, "Can't stat %s: %s", fields[1], err)
					continue
				}
				m = send_metric(m, "sar.fs."+name+".size", float32(stat.Bsize)*float32(stat.Blocks))
				m = send_metric(m, "sar.fs."+name+".used", float32(stat.Bsize)*float32(stat.Blocks-stat.Bfree))
				m = send_metric(m, "sar.fs."+name+".free", float32(stat.Bsize)*float32(stat.Bfree))
				m = send_metric(m, "sar.fs."+name+".%util", 100*(1-float32(stat.Bfree)/float32(stat.Blocks)))
				m = send_metric(m, "sar.fs."+name+".files", float32(stat.Files))
			}
		}
		f.Close()
		log.Debug("[FS] ----------------")
		metricschannel <- m
		m = nil
		if TEST_ENABLED {
			return
		}
		time.Sleep(time.Duration(DELTA) * time.Second)
	}
}
