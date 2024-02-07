package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

type properties struct {
	prop_name  []string
	prop_value []uint64
}

type SnmpStat struct {
	ip_prop       properties
	icmp_prop     properties
	icmp_msg_prop properties
	tcp_prop      properties
	udp_prop      properties
	udplite_prop  properties
}

var (
	oldSnmpStat, newSnmpStat SnmpStat
)

func readProperties(scanner *bufio.Scanner, tag string, prop *properties) {
	var line string
	var i int

	scanner.Scan()
	line = scanner.Text()
	log.Debug("[SNMP] Line=%s", line)
	prop.prop_name = strings.Split(line, " ")
	if prop.prop_name[0] != tag {
		log.Error(3, "[SNMP] /proc/net/snmp format error, expecting %s, found %s", tag, prop.prop_name[0])
		return
	}
	scanner.Scan()
	line = scanner.Text()
	log.Debug("[SNMP] Line=%s", line)
	prop_values := strings.Split(line, " ")
	if len(prop_values) != len(prop.prop_name) {
		log.Error(3, "[SNMP] /proc/net/snmp format error, expecting %d values, found %s", len(prop.prop_name), len(prop_values))
		return
	}
	if prop_values[0] != tag {
		log.Error(3, "[SNMP] /proc/net/snmp format error, expecting %s, found %s", tag, prop.prop_name[0])
		return
	}
	prop.prop_value = make([]uint64, len(prop.prop_name), cap(prop.prop_name))
	for i = range prop_values {
		prop.prop_value[i], _ = strconv.ParseUint(prop_values[i], 10, 64)
		log.Trace("[SNMP] %s=%d", prop.prop_name[i], prop.prop_value[i])
	}
}

func ReadSnmpStat(snmpstat *SnmpStat) error {
	f, err := os.Open(TEST_PROCROOT + "/proc/net/snmp")
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	readProperties(scanner, "Ip:", &snmpstat.ip_prop)
	readProperties(scanner, "Icmp:", &snmpstat.icmp_prop)
	readProperties(scanner, "IcmpMsg:", &snmpstat.icmp_msg_prop)
	readProperties(scanner, "Tcp:", &snmpstat.tcp_prop)
	readProperties(scanner, "Udp:", &snmpstat.udp_prop)
	readProperties(scanner, "UdpLite:", &snmpstat.udplite_prop)
	return nil
}

func SendSnmpStats(prefix string, old *properties, new *properties, snmpmap *map[string]bool) {
	var v float32
	var m Metrics = nil

	for i := 1; i < len(old.prop_name); i++ {
		iscounter, found := (*snmpmap)[old.prop_name[i]]
		if !found {
			continue
		}
		if iscounter { // counter - send delta
			v = float32(new.prop_value[i]-old.prop_value[i]) / float32(DELTA)
		} else { // gauge - send latest value
			v = float32(new.prop_value[i])
		}
		m = send_metric(m, prefix+old.prop_name[i], v)
	}
	metricschannel <- m
}

func CollectSnmpStat() {
	ipsnmpmap := map[string]bool{
		"InReceives":      true,
		"InHdrErrors":     true,
		"InAddrErrors":    true,
		"ForwDatagrams":   true,
		"InUnknownProtos": true,
		"InDiscards":      true,
		"InDelivers":      true,
		"OutRequests":     true,
		"OutDiscards":     true,
		"OutNoRoutes":     true,
		"ReasmTimeout":    false,
		"ReasmReqds":      true,
		"ReasmOKs":        true,
		"ReasmFails":      true,
		"FragOKs":         true,
		"FragFails":       true,
		"FragCreates":     true,
	}

	icmpsnmpmap := map[string]bool{
		"InMsgs":           true,
		"InErrors":         true,
		"InDestUnreachs":   true,
		"InTimeExcds":      true,
		"InParmProbs":      true,
		"InSrcQuenchs":     true,
		"InRedirects":      true,
		"InEchos":          true,
		"InEchoReps":       true,
		"InTimestamps":     true,
		"InTimestampReps":  true,
		"InAddrMasks":      true,
		"InAddrMaskReps":   true,
		"OutMsgs":          true,
		"OutErrors":        true,
		"OutDestUnreachs":  true,
		"OutTimeExcds":     true,
		"OutParmProbs":     true,
		"OutSrcQuenchs":    true,
		"OutRedirects":     true,
		"OutEchos":         true,
		"OutEchoReps":      true,
		"OutTimestamps":    true,
		"OutTimestampReps": true,
		"OutAddrMasks":     true,
		"OutAddrMaskReps":  true,
	}

	tcpsnmpmap := map[string]bool{
		"ActiveOpens":  true,
		"PassiveOpens": true,
		"AttemptFails": true,
		"EstabResets":  true,
		"CurrEstab":    false,
		"InSegs":       true,
		"OutSegs":      true,
		"RetransSegs":  true,
		"InErrs":       true,
		"OutRsts":      true,
	}

	udpsnmpmap := map[string]bool{
		"InDatagrams":  true,
		"NoPorts":      true,
		"InErrors":     true,
		"OutDatagrams": true,
		"RcvbufErrors": true,
		"SndbufErrors": true,
	}

	err := ReadSnmpStat(&oldSnmpStat)
	if err != nil {
		log.Error(3, "[SNMP] Cannot get initial stats: %s", err)
		return
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		err := ReadSnmpStat(&newSnmpStat)
		if err != nil {
			log.Error(3, "[SNMP] Cannot get stats: %s", err)
			return
		}
		SendSnmpStats("sar.net.ip.", &oldSnmpStat.ip_prop, &newSnmpStat.ip_prop, &ipsnmpmap)
		SendSnmpStats("sar.net.icmp.", &oldSnmpStat.icmp_prop, &newSnmpStat.icmp_prop, &icmpsnmpmap)
		SendSnmpStats("sar.net.tcp.", &oldSnmpStat.tcp_prop, &newSnmpStat.tcp_prop, &tcpsnmpmap)
		SendSnmpStats("sar.net.udp.", &oldSnmpStat.udp_prop, &newSnmpStat.udp_prop, &udpsnmpmap)
		log.Debug("[SNMP] ----------------")
		oldSnmpStat = newSnmpStat
		if TEST_ENABLED {
			return
		}
	}
}
