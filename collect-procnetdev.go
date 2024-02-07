package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

type NetDevUsage struct {
	netdevname     string // device name
	rx_bytes       uint64 // The total number of bytes of data transmitted or received by the interface.
	rx_packets     uint64 // The total number of packets of data transmitted or received by the interface.
	rx_errs        uint64 // The total number of transmit or receive errors detected by the device driver.
	rx_err_drop    uint64 // The total number of packets dropped by the device driver.
	rx_err_fifo    uint64 // The number of FIFO buffer errors.
	rx_err_frame   uint64 // The number of packet framing errors.
	rx_compressed  uint64 // The number of compressed packets transmitted or received by the device driver.
	rx_multicast   uint64 // The number of multicast frames transmitted or received by the device driver.
	tx_bytes       uint64 // The total number of bytes of data transmitted or received by the interface.
	tx_packets     uint64 // The total number of packets of data transmitted or received by the interface.
	tx_errs        uint64 // The total number of transmit or receive errors detected by the device driver.
	tx_err_drop    uint64 // The total number of packets dropped by the device driver.
	tx_err_fifo    uint64 // The number of FIFO buffer errors.
	tx_err_colls   uint64 // The number of collisions detected on the interface.
	tx_err_carrier uint64 // The number of carrier losses detected by the device driver.
	tx_compressed  uint64 // The number of compressed packets transmitted or received by the device driver.
}

type NetDevStat struct {
	num_dev int
	netdev  [MAX_NETDEV]NetDevUsage
}

var (
	oldNetDevStat, newNetDevStat NetDevStat
)

func ReadNetDevStat(netdevstat *NetDevStat) error {
	var line string
	var i int

	re, err := regexp.Compile(NETDEV_BLACKLIST)
	if err != nil {
		log.Error(3, "[NETDEV] Cannot compile blacklist regex %s: %s. Ignoring blacklist.", NETDEV_BLACKLIST, err)
		re = nil
	}

	f, err := os.Open(TEST_PROCROOT + "/proc/net/dev")
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	scanner.Scan() // skip first 2 lines
	scanner.Scan()
	for i = 0; i < MAX_NETDEV; {
		if !scanner.Scan() {
			break
		}
		line = strings.TrimLeft(scanner.Text(), " ")
		log.Debug("[NETDEV] Line = " + line)
		netdev := &netdevstat.netdev[i]
		_, err = fmt.Sscanf(line, "%s %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d",
			&netdev.netdevname,
			&netdev.rx_bytes, &netdev.rx_packets, &netdev.rx_errs, &netdev.rx_err_drop, &netdev.rx_err_fifo, &netdev.rx_err_frame, &netdev.rx_compressed, &netdev.rx_multicast,
			&netdev.tx_bytes, &netdev.tx_packets, &netdev.tx_errs, &netdev.tx_err_drop, &netdev.tx_err_fifo, &netdev.tx_err_colls, &netdev.tx_err_carrier, &netdev.tx_compressed)
		if err != nil {
			return err
		}
		netdev.netdevname = strings.TrimRight(netdev.netdevname, ":")
		if re != nil && re.MatchString(netdev.netdevname) {
			continue
		}
		i++
	}
	netdevstat.num_dev = i - 1

	return nil
}

func CollectNetDevStat() {
	var prefix string
	var m Metrics

	err := ReadNetDevStat(&oldNetDevStat)
	if err != nil {
		log.Error(3, "[NETDEV] Cannot get initial stats: %s", err)
		return
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		err := ReadNetDevStat(&newNetDevStat)
		if err != nil {
			log.Error(3, "[NETDEV] Cannot get stats: %s", err)
			return
		}

		for i := 0; i <= oldNetDevStat.num_dev; i++ {
			new := &newNetDevStat.netdev[i]
			old := &oldNetDevStat.netdev[i]
			prefix = fmt.Sprintf("sar.net.dev.%s.", old.netdevname)
			m = send_metric(m, prefix+"rxkB-s", float32(new.rx_bytes-old.rx_bytes)/float32(DELTA)/1024)
			m = send_metric(m, prefix+"rxpck-s", float32(new.rx_packets-old.rx_packets)/float32(DELTA))
			m = send_metric(m, prefix+"rxerr-s", float32(new.rx_errs-old.rx_errs)/float32(DELTA))
			m = send_metric(m, prefix+"rxdrop-s", float32(new.rx_err_drop-old.rx_err_drop)/float32(DELTA))
			m = send_metric(m, prefix+"rxfifo-s", float32(new.rx_err_fifo-old.rx_err_fifo)/float32(DELTA))
			m = send_metric(m, prefix+"rxfram-s", float32(new.rx_err_frame-old.rx_err_frame)/float32(DELTA))
			m = send_metric(m, prefix+"rxcomp-s", float32(new.rx_compressed-old.rx_compressed)/float32(DELTA))
			m = send_metric(m, prefix+"rxmcst-s", float32(new.rx_multicast-old.rx_multicast)/float32(DELTA))
			m = send_metric(m, prefix+"txkB-s", float32(new.tx_bytes-old.tx_bytes)/float32(DELTA)/1024)
			m = send_metric(m, prefix+"txpck-s", float32(new.tx_packets-old.tx_packets)/float32(DELTA))
			m = send_metric(m, prefix+"txerr-s", float32(new.tx_errs-old.tx_errs)/float32(DELTA))
			m = send_metric(m, prefix+"txdrop-s", float32(new.tx_err_drop-old.tx_err_drop)/float32(DELTA))
			m = send_metric(m, prefix+"txfifo-s", float32(new.tx_err_fifo-old.tx_err_fifo)/float32(DELTA))
			m = send_metric(m, prefix+"colls-s", float32(new.tx_err_colls-old.tx_err_colls)/float32(DELTA))
			m = send_metric(m, prefix+"txcarr-s", float32(new.tx_err_carrier-old.tx_err_carrier)/float32(DELTA))
			m = send_metric(m, prefix+"txcomp-s", float32(new.tx_compressed-old.tx_compressed)/float32(DELTA))
		}
		oldNetDevStat = newNetDevStat
		log.Debug("[NETDEV] ----------------")
		metricschannel <- m
		m = nil
		if TEST_ENABLED {
			return
		}
	}
}
