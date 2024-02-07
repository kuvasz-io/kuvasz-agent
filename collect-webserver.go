package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kuvasz-io/kuvasz-agent/log"
	"github.com/satyrius/gonx"
)

type WebserverStat struct {
	active   uint64
	waiting  uint64
	reading  uint64
	writing  uint64
	accepts  uint64
	handles  uint64
	requests uint64
}

func parseuint(rec *gonx.Entry, field string) uint64 {
	s, err := rec.Field(field)
	if err != nil {
		log.Info("[WEBSERVER] Can't find field %s in %v: %s", field, rec, err.Error())
		return 0
	}
	u, _ := strconv.ParseUint(s, 10, 64)
	return u
}

func ReadWebserverStat(name string, status_url string, webserverstat *WebserverStat, p *gonx.Parser) error {
	request, err := http.NewRequest("GET", status_url, nil)
	if err != nil {
		log.Error(3, "[WEBSERVER] [%s] Can't create request object %s", name, err.Error())
		return err
	}

	client := http.Client{}
	rawResponse, err := client.Do(request)
	if err != nil {
		log.Error(3, "[WEBSERVER] [%s] Can't fetch status from %s:  %s", name, status_url, err.Error())
		return err
	}
	log.Debug("[WEBSERVER] [%s] Response: %+v", name, rawResponse)

	rawResponseBody, err := io.ReadAll(rawResponse.Body)
	if err != nil {
		log.Error(3, "[WEBSERVER] [%s] Can't read response body from %s: %s", name, status_url, err.Error())
		return err
	}
	response := strings.ReplaceAll(string(rawResponseBody), "\n", " ")
	response = strings.TrimSpace(response)
	log.Debug("[WEBSERVER] [%s] Response: %s", name, response)

	if rawResponse.StatusCode != 200 {
		log.Error(3, "[WEBSERVER] [%s] Failed to get status page from %s: %+v", name, status_url, rawResponse)
		return nil
	}

	rec, err := p.ParseString(response)
	if err != nil {
		log.Error(3, "[WEBSERVER] [%s] Can't parse status: %s", name, err)
		return err
	}
	log.Trace("[WEBSERVER] [%s] Parsed status: %+v", name, rec)
	webserverstat.accepts = parseuint(rec, "accepts")
	webserverstat.active = parseuint(rec, "active")
	webserverstat.handles = parseuint(rec, "handles")
	webserverstat.reading = parseuint(rec, "reading")
	webserverstat.requests = parseuint(rec, "requests")
	webserverstat.waiting = parseuint(rec, "waiting")
	webserverstat.writing = parseuint(rec, "writing")

	log.Trace("[WEBSERVER] [%s] Web server status: %+v", name, webserverstat)
	return nil
}

func CollectWebserverStat(name string, status_url string, status_format string) {
	var OldWebserverStat, NewWebserverStat WebserverStat
	var err error
	var m Metrics

	if status_url == "" {
		log.Debug("[WEBSERVER] [%s] No web server status url defined, collection disabled", name)
		return
	}

	log.Debug("[WEBSERVER] [%s] Compiling status format: %s", name, status_format)
	p := gonx.NewParser(status_format)
	if p == nil {
		log.Error(3, "[WEBSERVER] [%s] Can't compile status format: %s", name, status_format)
		return
	}

	log.Debug("[WEBSERVER] [%s] Starting web server monitor at %s", name, status_url)
	err = ReadWebserverStat(name, status_url, &OldWebserverStat, p)
	if err != nil {
		log.Info("[WEBSERVER] Error getting initial stats, ignoring")
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		err = ReadWebserverStat(name, status_url, &NewWebserverStat, p)
		if err != nil {
			log.Info("[WEBSERVER] Error getting web server stat, ignoring")
			continue
		}
		m = send_metric(m, name+".active", float32(NewWebserverStat.active))
		m = send_metric(m, name+".connections.waiting", float32(NewWebserverStat.waiting))
		m = send_metric(m, name+".connections.reading", float32(NewWebserverStat.reading))
		m = send_metric(m, name+".connections.writing", float32(NewWebserverStat.writing))
		m = send_metric(m, name+".accepts", float32(NewWebserverStat.accepts-OldWebserverStat.accepts)/float32(DELTA))
		m = send_metric(m, name+".handles", float32(NewWebserverStat.handles-OldWebserverStat.handles)/float32(DELTA))
		m = send_metric(m, name+".requests", float32(NewWebserverStat.requests-OldWebserverStat.requests)/float32(DELTA))
		OldWebserverStat = NewWebserverStat
		metricschannel <- m
		m = nil
	}
}
