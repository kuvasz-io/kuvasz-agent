package main

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/kuvasz-io/kuvasz-agent/log"
)

const MAX_OID = 50

type Oid struct {
	Oid  string `json:"oid"`
	Name string `json:"name"`
}

type SnmpTarget struct {
	Target    string `json:"target"`
	Port      uint16 `json:"port"`
	Version   int    `json:"version"`
	Community string `json:"community"`
	Timeout   int    `json:"timeout"`
	Retries   int    `json:"retries"`
	Oids      []Oid  `json:"oids"`
}

type Counter struct {
	Value int64
	Type  int
}

type Counters [MAX_OID]Counter

var (
	snmptargets []SnmpTarget
)

func CollectSnmpOids(g *gosnmp.GoSNMP, oids []string, counters *Counters) error {
	result, err := g.Get(oids) // Get() accepts up to g.MAX_OIDS
	if err != nil {
		log.Error(3, "[POLLER-SNMP] Can't get oids %v: %s", oids, err)
		return err
	}

	for i, variable := range result.Variables {
		log.Trace("[POLLER-SNMP] %d: oid: %s, Type: %v, value = %v ", i, variable.Name, variable.Type, variable.Value)

		switch variable.Type {
		case gosnmp.Counter32:
			counters[i].Value = int64(variable.Value.(uint))
			counters[i].Type = 0
		case gosnmp.Counter64:
			counters[i].Value = int64(variable.Value.(uint64))
			counters[i].Type = 1
		case gosnmp.Gauge32, gosnmp.Uinteger32:
			counters[i].Value = int64(variable.Value.(uint))
			counters[i].Type = 2
		case gosnmp.Integer:
			counters[i].Value = int64(variable.Value.(int))
			counters[i].Type = 2
		default:
			log.Info("Invalid type %v for oid %s", variable.Type, variable.Name)
		}
	}
	return nil
}

func CollectSnmpTarget(snmptarget SnmpTarget) {
	var oids []string
	var m Metrics
	var g gosnmp.GoSNMP
	var oldcounters, newcounters Counters

	log.Trace("[POLLER-SNMP] Collecting target %v", snmptarget)
	g.Target = snmptarget.Target
	if snmptarget.Port != 0 {
		g.Port = snmptarget.Port
	} else {
		g.Port = 161
	}
	if (snmptarget.Version == 0) || (snmptarget.Version == 2) {
		g.Version = gosnmp.Version2c
	} else if snmptarget.Version == 1 {
		g.Version = gosnmp.Version1
	}
	if snmptarget.Community != "" {
		g.Community = snmptarget.Community
	} else {
		g.Community = "public"
	}
	if snmptarget.Timeout != 0 {
		g.Timeout = time.Duration(snmptarget.Timeout) * time.Millisecond
	} else {
		g.Timeout = time.Duration(100) * time.Millisecond
	}
	if snmptarget.Retries != 0 {
		g.Retries = snmptarget.Retries
	} else {
		g.Retries = 1
	}
	err := g.Connect()
	if err != nil {
		log.Error(3, "[POLLER-SNMP] Can't connect to target %s: %s", g.Target, err)
		return
	}
	defer g.Conn.Close()
	for i := range snmptarget.Oids {
		oids = append(oids, snmptarget.Oids[i].Oid)
	}
	for {
		err = CollectSnmpOids(&g, oids, &oldcounters)
		if err != nil {
			log.Error(3, "[POLLER-SNMP] Cannot get initial counters for target %s: %s", snmptarget.Target, err)
			time.Sleep(time.Duration(DELTA) * time.Second)
			continue
		}
		for {
			time.Sleep(time.Duration(DELTA) * time.Second)
			err := CollectSnmpOids(&g, oids, &newcounters)
			if err != nil {
				log.Error(3, "[POLLER-SNMP] Cannot get counters: %s", err)
				break
			}
			for i, oid := range snmptarget.Oids {
				switch oldcounters[i].Type {
				case 0:
					if newcounters[i].Value < oldcounters[i].Value {
						m = send_metric(m, snmptarget.Target+"."+oid.Name, float32(4294967295-oldcounters[i].Value+newcounters[i].Value)/float32(DELTA))
					} else {
						m = send_metric(m, snmptarget.Target+"."+oid.Name, float32(newcounters[i].Value-oldcounters[i].Value)/float32(DELTA))
					}
				case 1:
					m = send_metric(m, snmptarget.Target+"."+oid.Name, float32(newcounters[i].Value-oldcounters[i].Value)/float32(DELTA))
				case 2:
					m = send_metric(m, snmptarget.Target+"."+oid.Name, float32(newcounters[i].Value))
				}
			}
			oldcounters = newcounters
			log.Debug("[POLLER-SNMP] ----------------")
			metricschannel <- m
			m = nil
			if TEST_ENABLED {
				return
			}
		}
	}
}

func CollectSnmp() {
	f, err := os.Open(SNMP_JSON_DIR)
	if err != nil {
		log.Error(3, "Can't open SNMP targets directory %s: %s", SNMP_JSON_DIR, err)
		return
	}
	list, err := f.Readdir(-1)
	if err != nil {
		log.Error(3, "Can't read SNMP targets directory %s: %s", SNMP_JSON_DIR, err)
		f.Close()
		return
	}
	f.Close()
	for i := range list {
		if list[i].IsDir() {
			continue
		}
		if !strings.HasSuffix(list[i].Name(), ".json") {
			continue
		}
		body, err := os.ReadFile(SNMP_JSON_DIR + "/" + list[i].Name())
		if err != nil {
			log.Error(3, "[POLLER-SNMP] Can't read SNMP targets file %s: %s", list[i].Name(), err)
			continue
		}
		err = json.Unmarshal(body, &snmptargets)
		if err != nil {
			log.Info("[POLLER-SNMP] Can't process json: %s, error: %s", body, err)
			return
		}

		for _, snmptarget := range snmptargets {
			go CollectSnmpTarget(snmptarget)
		}
	}
}
