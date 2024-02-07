package main

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hpcloud/tail"
	"github.com/kuvasz-io/kuvasz-agent/log"
	"github.com/satyrius/gonx"
)

type ReqStat struct {
	prefix        string
	shortprefix   string
	ts            int64
	websocket     bool
	req           uint64
	io_rx         uint64
	io_tx         uint64
	t_us_connect  uint64
	t_us_headers  uint64
	t_us_response uint64
	t_time        uint64
}

func parseint(s string, err error) uint64 {
	var i uint64
	if err != nil {
		log.Error(3, "[WEBLOG] Can't read field: %s", s, err)
		return 0
	}
	if s == `"-"` || s == "-" {
		return 0
	}
	i, err = strconv.ParseUint(s, 10, 64)
	if err != nil {
		log.Error(3, "[WEBLOG] Can't parse int %s: %s", s, err)
		return 0
	}
	return i
}

func parsems(s string, err error) uint64 {
	var f float64
	if err != nil {
		log.Error(3, "[WEBLOG] Can't read field: %s", s, err)
		return 0
	}
	if s == `"-"` || s == "-" {
		return 0
	}
	f, err = strconv.ParseFloat(s, 64)
	if err != nil {
		log.Error(3, "[WEBLOG] Can't parse ms %s: %s", s, err)
		return 0
	}
	return uint64(f * 1000.0)
}

func extractURL(re *regexp.Regexp, u string) string {
	var url string

	found := re.FindStringSubmatch(u)
	if len(found) != 2 {
		return "other"
	}
	url = found[1]
	if url == "" {
		return "other"
	}
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	url = strings.ReplaceAll(url, "/", "-")
	return url
}

func ParseRecord(service string, rec *gonx.Entry, urlre *regexp.Regexp, upstream_time int) (ReqStat, error) {
	var w ReqStat
	var method string
	var resp string
	var request string
	var url string

	log.Debug("[WEBLOG] [%s] Parsed entry: %+v", service, rec)
	ts_string, err := rec.Field("time_local")
	if err != nil {
		log.Error(3, "[WEBLOG] [%s] Can't find timestamp field in %v: %s", service, rec, err)
		return w, err
	}
	ts_time, err := time.Parse("02/Jan/2006:15:04:05 -0700", ts_string)
	if err != nil {
		log.Error(3, "[WEBLOG] [%s] Can't parse time field %s: %s", service, ts_string, err)
		return w, err
	}
	w.ts = ts_time.Unix()
	w.req = 1
	w.io_rx = parseint(rec.Field("request_length"))
	w.io_tx = parseint(rec.Field("body_bytes_sent"))
	w.websocket = false
	request, err = rec.Field("request")
	if err != nil {
		log.Error(3, "[WEBLOG] [%s] Can't parse find request field in %v: %s", service, rec, err)
		return w, err
	}

	m := strings.Fields(request)
	if len(m) != 3 {
		log.Error(3, "[WEBLOG] [%s] Can't parse find request field in '%s': found %d fields", service, request, len(m))
		return w, errors.New("invalid request field")
	}
	switch m[0] {
	case "GET":
		method = "get"
	case "POST":
		method = "post"
	case "PUT":
		method = "put"
	case "PATCH":
		method = "patch"
	case "DELETE":
		method = "delete"
	case "OPTIONS":
		method = "options"
	default:
		method = "other"
	}

	url = extractURL(urlre, m[1])

	stat, err := rec.Field("status")
	if err != nil {
		log.Error(3, "[WEBLOG] [%s] Can't parse find status field in %v: %s", service, rec, err)
		return w, err
	}
	switch stat[0] {
	case '1':
		w.websocket = true
		resp = "1xx"
	case '2':
		resp = "2xx"
	case '3':
		resp = "3xx"
	case '4':
		resp = "4xx"
	case '5':
		resp = "5xx"
	default:
		resp = "0xx"
	}

	w.prefix = fmt.Sprintf("%s.%s.%s", url, method, resp)
	w.shortprefix = fmt.Sprintf("%s.%s", url, method)
	w.t_time = parsems(rec.Field("request_time"))
	if upstream_time == 1 {
		w.t_us_connect = parsems(rec.Field("upstream_connect_time"))
		w.t_us_headers = parsems(rec.Field("upstream_header_time"))
		w.t_us_response = parsems(rec.Field("upstream_response_time"))
	}
	log.Trace("[WEBLOG] [%s] metrics: %+v", service, w)
	return w, nil
}

func append_rate(m Metrics, name string, ts int64, value uint64) Metrics {
	var metric Metric
	metric.Name = name
	metric.Value = float32(value) / float32(DELTA)
	metric.Ts = ts
	m = append(m, metric)
	return m
}

func append_ratio(m Metrics, name string, v ReqStat, reqTotal int) Metrics {
	var metric Metric
	metric.Name = name
	metric.Value = float32(v.req) / float32(reqTotal)
	metric.Ts = v.ts
	m = append(m, metric)
	// fmt.Printf("%s: req=%d, total=%d, ratio=%f\n", name, v.req, ReqTotal, metric.Value)
	return m
}

func append_latency(m Metrics, name string, ts int64, value uint64, reqs uint64) Metrics {
	var metric Metric
	metric.Name = name
	metric.Value = float32(value) / float32(reqs)
	metric.Ts = ts
	m = append(m, metric)
	return m
}

func CollectWebLogStat(service string, filename string, format string, url_format string, upstream_time int) {
	var w ReqStat
	var t *tail.Tail
	var last_ts int64
	var m Metrics
	var webmetrics map[string]ReqStat
	var reqmetrics map[string]int
	var err error
	var urlre *regexp.Regexp

	urlre, err = regexp.Compile(url_format)
	if err != nil {
		log.Error(3, "[WEBLOG] [%s] Can't compile url regexp %s: %s", service, url_format, err)
		return
	}
	p := gonx.NewParser(format)
	log.Debug("[WEBLOG] [%s] Opening logfile %s with format %s", service, filename, format)
	t, err = tail.TailFile(filename, tail.Config{ReOpen: true, Follow: true, Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}})
	if err != nil {
		log.Error(3, "[WEBLOG] [%s] Can't open logfile %s: %s", service, filename, err)
		return
	}
	for l := range t.Lines {
		line := l.Text
		// fmt.Printf(".")
		log.Debug("[WEBLOG] [%s] %s", service, line)
		rec, err := p.ParseString(line)
		if err != nil {
			log.Error(3, "[WEBLOG] [%s] Can't read log entry: %s", service, err)
			continue
		}
		w, err = ParseRecord(service, rec, urlre, upstream_time)
		if err != nil {
			log.Error(3, "[WEBLOG] [%s] Can't parse web entry: %s", service, err)
			continue
		}
		if last_ts == 0 {
			log.Trace("[WEBLOG] [%s] Got first log record", service)
			last_ts = w.ts
			webmetrics = make(map[string]ReqStat)
			webmetrics[w.prefix] = w
			reqmetrics = make(map[string]int)
			reqmetrics[w.shortprefix] = 1
			continue
		}
		if (w.ts - last_ts) > int64(DELTA) {
			// calculate and send metrics
			for k, v := range webmetrics {
				m = append_rate(m, PREFIX+service+".url."+v.prefix+".req", v.ts, v.req)
				m = append_ratio(m, PREFIX+service+".url."+v.prefix+".ratio", v, reqmetrics[v.shortprefix])
				m = append_rate(m, PREFIX+service+".url."+v.prefix+".bytes_in", v.ts, v.io_rx)
				m = append_rate(m, PREFIX+service+".url."+v.prefix+".bytes_out", v.ts, v.io_tx)
				m = append_latency(m, PREFIX+service+".url."+v.prefix+".rt", v.ts, v.t_time, v.req)
				if upstream_time == 1 {
					m = append_latency(m, PREFIX+service+".url."+v.prefix+".upstream_rt", v.ts, v.t_us_response, v.req)
					m = append_latency(m, PREFIX+service+".url."+v.prefix+".upstream_connect", v.ts, v.t_us_connect, v.req)
					m = append_latency(m, PREFIX+service+".url."+v.prefix+".upstream_headers", v.ts, v.t_us_headers, v.req)
					m = append_latency(m, PREFIX+service+".url."+v.prefix+".proxy_latency", v.ts, v.t_time-v.t_us_response, v.req)
				}
				log.Trace("[WEBLOG] [%s] k=%v, v=%v", service, k, v)
			}
			metricschannel <- m
			m = nil
			last_ts = w.ts
			webmetrics = make(map[string]ReqStat)
			webmetrics[w.prefix] = w
			reqmetrics = make(map[string]int)
			reqmetrics[w.shortprefix] = 1
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if o, ok := webmetrics[w.prefix]; ok {
			log.Trace("[WEBLOG] [%s] Accumulating metric: %s, %v", service, w.prefix, w)
			o.req += 1
			o.io_rx += w.io_rx
			o.io_tx += w.io_tx
			o.t_us_connect += w.t_us_connect
			o.t_us_headers += w.t_us_headers
			o.t_us_response += w.t_us_response
			o.t_time += w.t_time
			webmetrics[o.prefix] = o
			reqmetrics[o.shortprefix]++
		} else {
			log.Trace("[WEBLOG] [%s] Starting new metric: %s, %v", service, w.prefix, w)
			webmetrics[w.prefix] = w
			reqmetrics[w.shortprefix]++
		}
	}
}
