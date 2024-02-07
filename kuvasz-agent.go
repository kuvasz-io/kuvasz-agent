package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuvasz-io/kuvasz-agent/log"
	"github.com/kuvasz-io/kuvasz-agent/util"
	ini "gopkg.in/ini.v1"
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
	upstream_time int
}

type Header struct {
	key   string
	value string
}
type Cert struct {
	sites map[string]interface{}
}

var (
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
			err := os.MkdirAll(filepath.Dir(logPath), os.ModePerm)
			if err != nil {
				log.Fatal(4, "Fail to create log directory %s: %v", filepath.Dir(logPath), err)
			}
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
	_ = http.ListenAndServe("localhost:6060", nil)
}

func sep(r rune) bool {
	switch r {
	case ',', ' ':
		return true
	}
	return false
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

	options := ini.LoadOptions{
		IgnoreInlineComment:     false,
		Insensitive:             true,
		SkipUnrecognizableLines: true,
		AllowShadows:            true,
		AllowNonUniqueSections:  true,
	}
	Cfg, err = ini.LoadSources(options, configfile)
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
	w := strings.FieldsFunc(services, sep)
	for i := range w {
		WEBLOGS = append(WEBLOGS, WebLog{
			service:       w[i],
			logfile:       Cfg.Section("web." + w[i]).Key("logfile").MustString(""),
			format:        Cfg.Section("web." + w[i]).Key("format").MustString(""),
			url_format:    Cfg.Section("web." + w[i]).Key("url_format").MustString("/([^? /]*)"),
			status_url:    Cfg.Section("web." + w[i]).Key("status_url").MustString(""),
			status_format: strings.ReplaceAll(Cfg.Section("web."+w[i]).Key("status_format").MustString(""), "\\n", "\n"),
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

	// Start /proc/stat CPU and Tasks collector
	go CollectProcStat()

	// Start /proc/diskstats disk activity collector
	go CollectDiskStat()

	// Start /proc/net/dev network interface device activity collector
	go CollectNetDevStat()

	// Start /proc/net/snmp IP/ICMP/TCP/UDP activity collector
	go CollectSnmpStat()

	// Start /proc/net/snmp IP/ICMP/TCP/UDP activity collector
	go CollectMemStat()

	// Start /proc/net/snmp IP/ICMP/TCP/UDP activity collector
	go CollectVMStat()

	// Start /proc/mounts filesystem usagecollector
	go CollectFSStat()

	// Start /proc/sys/fs/file-nr and inode-nr open files and inodes collector
	go CollectFileStat()

	// Start /proc/<pid>/stat and /proc/<pid>/io collector
	go CollectProcPidStat()

	// Start MySQL collector
	go CollectMySQLStat()

	// Start Postgres collector
	go CollectPGStat()

	// Start nf_conntrack collector
	go CollectConntrackStat()

	for i := range WEBLOGS {
		go CollectWebLogStat(WEBLOGS[i].service, WEBLOGS[i].logfile, WEBLOGS[i].format, WEBLOGS[i].url_format, WEBLOGS[i].upstream_time)
		go CollectWebserverStat(WEBLOGS[i].service, WEBLOGS[i].status_url, WEBLOGS[i].status_format)
	}

	cert, err := Cfg.GetSection("cert")
	if err != nil {
		log.Error(3, "Cannot read cert section: %v", err)
	} else {
		log.Debug("Certs: %+v", cert.Keys())
		for k, v := range cert.KeysHash() {
			go CollectCertExpiry(k, v)
		}
	}

	pingsection, err := Cfg.GetSection("ping")
	if err != nil {
		log.Error(3, "Cannot read ping section: %v", err)
	} else {
		pingsections := pingsection.ChildSections()
		for i := range pingsections {
			var h []Header
			url := pingsections[i].Key("url").MustString("")
			headers := pingsections[i].Key("header").ValueWithShadows()
			log.Debug("%s, headers=%v", pingsections[i].Name(), headers)
			for i := range headers {
				sh := strings.Split(headers[i], ":")
				if len(sh) != 2 {
					log.Error(3, "ignoring header: %s", headers[i])
					continue
				}
				h = append(h, Header{strings.TrimSpace(sh[0]), strings.TrimSpace(sh[1])})
				log.Debug("Adding header: %s: %s", strings.TrimSpace(sh[0]), strings.TrimSpace(sh[1]))
			}
			log.Debug("headers=%v", h)
			go CollectPing(strings.TrimPrefix(pingsections[i].Name(), "ping."), url, h)
		}
	}

	// Start SNMP collectors
	go CollectSnmp()

	// Start metrics sender
	if CARBONURL != "" {
		go MetricsSenderCarbon()
	} else {
		go MetricsSenderJSON()
	}

	// Start profiling web server
	// go ProfilingServer()

	// Wait forever
	select {}
}
