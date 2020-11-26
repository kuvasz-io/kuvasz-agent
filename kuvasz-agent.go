package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/ini.v1"
	"kuvasz.io/kuvasz-agent/log"
	"kuvasz.io/kuvasz-agent/util"
)

const MAX_NETDEV = 256
const MAX_CPU = 256
const MAX_DISK = 256
const PAGE_SIZE = 4096

type WebLog struct {
	service       string
	logfile       string
	format        string
	url_format    string
	status_url    string
	status_format string
	urlre         *regexp.Regexp
	upstream_time int
}

var (
	wg sync.WaitGroup

	// Configuration file
	Cfg *ini.File

	// Log settings.
	LogModes   []string
	LogConfigs []util.DynMap

	// Sampling
	SAMPLING_ENABLED      int
	DELTA                 float32
	DISKDEV_BLACKLIST     string
	NETDEV_BLACKLIST      string
	FS_BLACKLIST          string
	ORGANIZATION          string
	SITE                  string
	HOSTNAME              string
	APIKEY                string
	APIURL                string
	CARBONURL             string
	PREFIX                string
	PS_CONS_KERNEL        int
	PS_CONS_USER          int
	PS_THRESHOLD_CPU      float32
	PS_THRESHOLD_DISK     float32
	PS_TOP_N_CPU          int
	PS_TOP_N_DISK         int
	PS_BLACKLIST          string
	PS_WHITELIST          string
	MYSQL_DSN             string
	POSTGRES_DSN          string
	POSTGRES_DB_DSN       string
	POSTGRES_DB_BLACKLIST string
	WEBLOGS               []WebLog
	SNMP_JSON_DIR         string
	TEST_ENABLED          bool
	TEST_PROCROOT         string

	metricschannel chan Metrics
)

var logLevels = map[string]int{
	"Trace":    0,
	"Debug":    1,
	"Info":     2,
	"Warn":     3,
	"Error":    4,
	"Critical": 5,
}

func initLogging() {
	// Get and check log mode.
	LogModes = strings.Split(Cfg.Section("log").Key("mode").MustString("console"), ",")

	LogConfigs = make([]util.DynMap, len(LogModes))
	for i, mode := range LogModes {
		mode = strings.TrimSpace(mode)
		sec, err := Cfg.GetSection("log." + mode)
		if err != nil {
			log.Fatal(4, "Unknown log mode: %s", mode)
		}

		// Log level.
		levelName := Cfg.Section("log."+mode).Key("level").In("Trace",
			[]string{"Trace", "Debug", "Info", "Warn", "Error", "Critical"})
		level, ok := logLevels[levelName]
		if !ok {
			log.Fatal(4, "Unknown log level: %s", levelName)
		}

		// Generate log configuration.
		switch mode {
		case "console":
			LogConfigs[i] = util.DynMap{"level": level}
		case "file":
			logPath := sec.Key("file_name").MustString("kuvasz-agent.log")
			os.MkdirAll(filepath.Dir(logPath), os.ModePerm)
			LogConfigs[i] = util.DynMap{
				"level":    level,
				"filename": logPath,
				"rotate":   sec.Key("log_rotate").MustBool(true),
				"maxlines": sec.Key("max_lines").MustInt(100000000),
				"maxsize":  1 << uint(sec.Key("max_size_shift").MustInt(31)),
				"daily":    sec.Key("daily_rotate").MustBool(true),
				"maxdays":  sec.Key("max_days").MustInt(7),
			}
		}

		cfgJsonBytes, _ := json.Marshal(LogConfigs[i])
		log.NewLogger(Cfg.Section("log").Key("buffer_len").MustInt64(10000), mode, string(cfgJsonBytes))
	}
}

func ProfilingServer() {
	defer wg.Done()
	_ = http.ListenAndServe("localhost:6060", nil)
}

func main() {
	var err error
	var configfile string

	// TEST_ENABLED = true
	// TEST_PROCROOT = "/Users/gyazbek/go/src/kuvasz/kuvasz-agent/test/rhel6.7"
	metricschannel = make(chan Metrics, 1000)
	if len(os.Args) == 2 {
		configfile = os.Args[1]
	} else {
		configfile = "/etc/kuvasz/kuvasz-agent.ini"
	}

	Cfg, err = ini.Load(configfile)
	if (Cfg == nil) || (err != nil) {
		fmt.Printf("Cannot read configuration file: %s", err)
		return
	}
	Cfg.BlockMode = false
	initLogging()
	SAMPLING_ENABLED = Cfg.Section("sampling").Key("enabled").MustInt(1)
	DELTA = float32(Cfg.Section("sampling").Key("interval").MustFloat64(60.0))
	DISKDEV_BLACKLIST = Cfg.Section("sampling").Key("diskdev_blacklist").MustString("^$")
	NETDEV_BLACKLIST = Cfg.Section("sampling").Key("netdev_blacklist").MustString("^$")
	FS_BLACKLIST = Cfg.Section("sampling").Key("fs_blacklist").MustString("^$")
	ORGANIZATION = Cfg.Section("customer").Key("organization").MustString("")
	SITE = Cfg.Section("customer").Key("site").MustString("main")
	HOSTNAME = Cfg.Section("customer").Key("hostname").MustString("")
	APIKEY = Cfg.Section("customer").Key("apikey").MustString("")
	APIURL = Cfg.Section("customer").Key("apiurl").MustString("https://api.kuvasz.io/api/v1/set/")
	CARBONURL = Cfg.Section("customer").Key("carbonurl").MustString("")
	PS_CONS_KERNEL = Cfg.Section("sampling").Key("ps_cons_kernel").MustInt(1)
	PS_CONS_USER = Cfg.Section("sampling").Key("ps_cons_user").MustInt(1)
	PS_THRESHOLD_CPU = float32(Cfg.Section("sampling").Key("ps_threshold_cpu").MustFloat64(3.0))
	PS_THRESHOLD_DISK = float32(Cfg.Section("sampling").Key("ps_threshold_disk").MustFloat64(1000000.0))
	PS_TOP_N_CPU = Cfg.Section("sampling").Key("ps_top_n_cpu").MustInt(5)
	PS_TOP_N_DISK = Cfg.Section("sampling").Key("ps_top_n_disk").MustInt(5)
	PS_BLACKLIST = Cfg.Section("sampling").Key("ps_blacklist").MustString("^$")
	PS_WHITELIST = Cfg.Section("sampling").Key("ps_whitelist").MustString("^$")
	MYSQL_DSN = Cfg.Section("sampling").Key("mysql_dsn").MustString("")
	POSTGRES_DSN = Cfg.Section("sampling").Key("postgres_dsn").MustString("")
	POSTGRES_DB_DSN = Cfg.Section("sampling").Key("postgres_db_dsn").MustString("")
	POSTGRES_DB_BLACKLIST = Cfg.Section("sampling").Key("postgres_db_blacklist").MustString("^template0$|^template1$|^postgres$")
	if HOSTNAME == "" {
		h, _ := os.Hostname()
		HOSTNAME = strings.Split(h, ".")[0]
	}
	services := Cfg.Section("web").Key("services").MustString("")
	w := strings.FieldsFunc(services, func(r rune) bool {
		switch r {
		case ',', ' ':
			return true
		}
		return false
	})
	for i := range w {
		WEBLOGS = append(WEBLOGS, WebLog{
			service:       w[i],
			logfile:       Cfg.Section("web." + w[i]).Key("logfile").MustString(""),
			format:        Cfg.Section("web." + w[i]).Key("format").MustString(""),
			url_format:    Cfg.Section("web." + w[i]).Key("url_format").MustString("/([^? /]*)"),
			status_url:    Cfg.Section("web." + w[i]).Key("status_url").MustString(""),
			status_format: strings.Replace(Cfg.Section("web."+w[i]).Key("status_format").MustString(""), "\\n", "\n", -1),
			upstream_time: Cfg.Section("web." + w[i]).Key("upstream_time").MustInt(1),
		})
	}
	log.Debug("Web logs: %v", WEBLOGS)
	SNMP_JSON_DIR = Cfg.Section("SNMP").Key("directory").MustString("/etc/kuvasz/snmp")
	log.Debug("Organization=%s, Site=%s, Hostname=%s", ORGANIZATION, SITE, HOSTNAME)
	PREFIX = ORGANIZATION + "." + SITE + "." + HOSTNAME + "."
	log.Debug("API Key=%s", APIKEY)
	log.Debug("Sampling interval=%d", DELTA)
	log.Debug("Disk device blacklist=%s", DISKDEV_BLACKLIST)
	log.Debug("Network device blacklist=%s", NETDEV_BLACKLIST)
	log.Debug("Filesystem blacklist=%s", FS_BLACKLIST)
	log.Debug("Process CPU Utilization threshold=%f", PS_THRESHOLD_CPU)
	log.Debug("Process CPU Utilization top n=%d", PS_TOP_N_CPU)
	log.Debug("Process blacklist = %s", PS_BLACKLIST)
	log.Debug("Process whitelist = %s", PS_WHITELIST)
	log.Debug("Database blacklist = %s", POSTGRES_DB_BLACKLIST)

	if SAMPLING_ENABLED == 1 {
		// Start /proc/stat CPU and Tasks collector
		wg.Add(1)
		go CollectProcStat()

		// Start /proc/diskstats disk activity collector
		wg.Add(1)
		go CollectDiskStat()

		// Start /proc/net/dev network interface device activity collector
		wg.Add(1)
		go CollectNetDevStat()

		// Start /proc/net/snmp IP/ICMP/TCP/UDP activity collector
		wg.Add(1)
		go CollectSnmpStat()

		// Start /proc/net/snmp IP/ICMP/TCP/UDP activity collector
		wg.Add(1)
		go CollectMemStat()

		// Start /proc/net/snmp IP/ICMP/TCP/UDP activity collector
		wg.Add(1)
		go CollectVMStat()

		// Start /proc/mounts filesystem usagecollector
		wg.Add(1)
		go CollectFSStat()

		// Start /proc/sys/fs/file-nr and inode-nr open files and inodes collector
		wg.Add(1)
		go CollectFileStat()

		// Start /proc/<pid>/stat and /proc/<pid>/io collector
		wg.Add(1)
		go CollectProcPidStat()

		// Start MySQL collector
		wg.Add(1)
		go CollectMySQLStat()

		// Start Postgres collector
		wg.Add(1)
		go CollectPGStat()

		// Start nf_conntrack collector
		wg.Add(1)
		go CollectConntrackStat()
	}

	for i := range WEBLOGS {
		wg.Add(1)
		go CollectWebLogStat(WEBLOGS[i].service, WEBLOGS[i].logfile, WEBLOGS[i].format, WEBLOGS[i].url_format, WEBLOGS[i].upstream_time)
		go CollectWebserverStat(WEBLOGS[i].service, WEBLOGS[i].status_url, WEBLOGS[i].status_format)
	}

	// Start SNMP collectors
	wg.Add(1)
	go CollectSnmp()

	// Start metrics sender - this never returns
	wg.Add(1)
	if CARBONURL != "" {
		go MetricsSenderCarbon()
	} else {
		go MetricsSenderJSON()
	}

	// Start profiling web server
	wg.Add(1)
	go ProfilingServer()

	// Wait for everything to finish in case of errors then close logger goroutine
	wg.Wait()
	log.Close()
}
