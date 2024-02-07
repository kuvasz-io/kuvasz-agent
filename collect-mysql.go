package main

import (
	"database/sql"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kuvasz-io/kuvasz-agent/log"
)

type mymetric struct {
	name    string
	isgauge bool
}

type MySQLStat map[string]uint64

var translation map[string]mymetric

func initTranslation() {
	translation = map[string]mymetric{
		"Questions":                         {"queries.total", false},
		"Com_select":                        {"queries.select", false},
		"Com_insert":                        {"queries.insert", false},
		"Com_update":                        {"queries.update", false},
		"Com_delete":                        {"queries.delete", false},
		"Bytes_received":                    {"conn.rx-bytes", false},
		"Bytes_sent":                        {"conn.tx-bytes", false},
		"Connections":                       {"conn.attempts", false},
		"Aborted_clients":                   {"conn.abortedclients", false},
		"Aborted_connects":                  {"conn.abortedconnects", false},
		"Threads_cached":                    {"conn.cached-threads", true},
		"Threads_connected":                 {"conn.connectedclients", true},
		"Threads_running":                   {"conn.runningclients", true},
		"max_connections":                   {"conn.max", true},
		"Handler_commit":                    {"handler.commit", false},
		"Handler_delete":                    {"handler.delete", false},
		"Handler_prepare":                   {"handler.prepare", false},
		"Handler_read_first":                {"handler.read-first", false},
		"Handler_read_key":                  {"handler.read-key", false},
		"Handler_read_last":                 {"handler.read-last", false},
		"Handler_read_next":                 {"handler.read-next", false},
		"Handler_read_prev":                 {"handler.read-prev", false},
		"Handler_read_rnd":                  {"handler.read-rnd", false},
		"Handler_read_rnd_next":             {"handler.read-rndnxt", false},
		"Handler_rollback":                  {"handler.rollback", false},
		"Handler_update":                    {"handler.savepoint", false},
		"Handler_write":                     {"handler.write", false},
		"Innodb_buffer_pool_pages_data":     {"innodbpool.pg-used", true},
		"Innodb_buffer_pool_pages_dirty":    {"innodbpool.pg-dirty", true},
		"Innodb_buffer_pool_pages_total":    {"innodbpool.pg-total", true},
		"Innodb_buffer_pool_read_requests":  {"innodbpool.read-req", false},
		"Innodb_buffer_pool_reads":          {"innodbpool.read-disk", false},
		"Innodb_buffer_pool_write_requests": {"innodbpool.write-req", false},
		"Innodb_page_size":                  {"innodb.pg-size", true},
		"Innodb_data_read":                  {"innodb.read-bytes", false},
		"Innodb_data_reads":                 {"innodb.read-req", false},
		"Innodb_data_written":               {"innodb.write-bytes", false},
		"Innodb_data_writes":                {"innodb.write-req", false},
		"Innodb_row_lock_current_waits":     {"innodblock.waiters", true},
		"Innodb_row_lock_time":              {"innodblock.waittime", false},
		"Innodb_row_lock_waits":             {"innodblock.waitnumber", false},
		"Innodb_rows_read":                  {"innodbrows.read", false},
		"Innodb_rows_inserted":              {"innodbrows.insert", false},
		"Innodb_rows_updated":               {"innodbrows.update", false},
		"Innodb_rows_deleted":               {"innodbrows.delete", false},
	}
}

func ReadMySQLStat(sqlCmd string, mysqlstat MySQLStat) error {
	db, err := sql.Open("mysql", MYSQL_DSN)
	if err != nil {
		log.Error(3, "Can't open MySQL connection to DSN %s: %s", MYSQL_DSN, err)
		return err
	}
	defer db.Close()

	rows, err := db.Query(sqlCmd)
	if err != nil {
		log.Error(3, "Can't execute SHOW GLOBAL STATUS: %s", err)
		return err
	}

	for rows.Next() {
		var name, value string
		err = rows.Scan(&name, &value)
		if err != nil {
			log.Error(3, "Can't read row: %s", err)
			return err
		}
		mymetric, ok := translation[name]
		if ok {
			log.Trace("[MYSQL] %s -> %s = %s", name, mymetric.name, value)
			mysqlstat[name], _ = strconv.ParseUint(value, 10, 64)
		} else {
			log.Trace("[MYSQL] %s = %s skipped", name, value)
		}
	}
	db.Close()
	return nil
}

func CollectMySQLStat() {
	var OldMySQLStat, NewMySQLStat, TempMySQLStat MySQLStat
	var err error
	var m Metrics

	if MYSQL_DSN == "" {
		log.Debug("MySQL DSN empty, collection disabled")
		return
	}

	initTranslation()
	OldMySQLStat = make(MySQLStat)
	NewMySQLStat = make(MySQLStat)

	err = ReadMySQLStat("SHOW GLOBAL STATUS", OldMySQLStat)
	if err != nil {
		return
	}
	err = ReadMySQLStat("SHOW GLOBAL VARIABLES", OldMySQLStat)
	if err != nil {
		return
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		err = ReadMySQLStat("SHOW GLOBAL STATUS", NewMySQLStat)
		if err != nil {
			return
		}
		err = ReadMySQLStat("SHOW GLOBAL VARIABLES", NewMySQLStat)
		if err != nil {
			return
		}
		for k, v := range NewMySQLStat {
			t := translation[k]
			if t.isgauge {
				m = send_metric(m, "mysql."+t.name, float32(v))
			} else {
				log.Trace("[MYSQL] %s -> %s, old = %d, new=%d", k, t.name, v, OldMySQLStat[k])
				m = send_metric(m, "mysql."+t.name, float32(v-OldMySQLStat[k])/float32(DELTA))
			}
		}
		m = send_metric(m, "mysql.conn-%used", 100.0*float32(NewMySQLStat["Threads_connected"])/float32(NewMySQLStat["max_connections"]))
		m = send_metric(m, "mysql.innodbpool-%used", 100.0*float32(NewMySQLStat["Innodb_buffer_pool_pages_data"])/float32(NewMySQLStat["Innodb_buffer_pool_pages_total"]))
		TempMySQLStat = NewMySQLStat
		NewMySQLStat = OldMySQLStat
		OldMySQLStat = TempMySQLStat
		metricschannel <- m
		m = nil
	}
}
