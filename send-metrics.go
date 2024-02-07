package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

type Metric struct {
	Name  string
	Value float32
	Ts    int64
}

type Metrics []Metric

// func send_metric_uint64_counter(m Metrics, name string, new uint64, old uint64) Metrics {
// 	return send_metric(m, name, float32(new-old)/float32(DELTA))
// }

func send_metric_uint64_delta(m Metrics, name string, new uint64) Metrics {
	return send_metric(m, name, float32(new)/float32(DELTA))
}

func send_metric_uint64_gauge(m Metrics, name string, new uint64) Metrics {
	return send_metric(m, name, float32(new))
}

func send_metric(m Metrics, name string, value float32) Metrics {
	var metric Metric

	log.Trace("Sending to handler %-30s = %f", name, value)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, " ", "-")
	metric.Name = PREFIX + name
	metric.Value = value
	metric.Ts = time.Now().Unix()
	m = append(m, metric)
	return m
}

func send_now(name string, value float32) {
	var metric Metric
	var m []Metric

	log.Trace("Sending to handler %-30s = %f", name, value)
	metric.Name = PREFIX + name
	metric.Value = value
	metric.Ts = time.Now().Unix()
	m = append(m, metric)
	metricschannel <- m
}

func MetricsSenderJSON() {
	var m Metrics
	var jsonmessage []byte
	var err error

	for {
		m = <-metricschannel
		log.Trace("%v", m)
		jsonmessage, err = json.Marshal(m)
		if err != nil {
			log.Error(3, "Can't marshal metrics: %s", m, err)
			continue
		}
		req, err := http.NewRequest("POST", APIURL+APIKEY, bytes.NewBuffer(jsonmessage))
		if err != nil {
			log.Error(3, "Can't POST message to API: %s", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Error(3, "Can't connect to API %s: %s", APIURL, err)
			continue
		}
		log.Trace("Response status: %s", resp.Status)
		body, _ := io.ReadAll(resp.Body)
		log.Trace("Response body: %s", string(body))
		resp.Body.Close()
	}
}

func MetricsSenderCarbon() {
	var m Metrics

	for {
		m = <-metricschannel
		log.Trace("%v", m)
		conn, err := net.Dial("tcp", CARBONURL)
		if err != nil {
			log.Error(3, "Can't connect to API at %s", CARBONURL)
			continue
		}
		for _, metric := range m {
			fmt.Fprintf(conn, "%s %f %d\n", metric.Name, metric.Value, metric.Ts)
		}
		conn.Close()
	}
}
