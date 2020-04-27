//  select datname,
//         case state
//             when 'idle' then 'idle'
//             when 'active' then 'active'
//             when 'idle in transaction' then 'idle-xact'
//             when 'idle in transaction (aborted)' then 'idle-xact-aborted'
//             when 'fastpath function call' then 'fastpath'
//             when 'disabled' then 'disabled'
//             else 'other'
//         end as state,
//         count(*) as count
// from pg_stat_activity
// group by datname, state;

package main

import (
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"kuvasz-agent/log"
)

type pgmetric struct {
	name    string
	isgauge bool
}

type pgmetricvalue struct {
	isgauge bool
	value   uint64
}

type PGStat struct {
	metrics map[string]pgmetricvalue
}

var pg_translation map[string]pgmetric
var db *sqlx.DB
var re *regexp.Regexp

func init_pg_translation() {
	pg_translation = map[string]pgmetric{
		// connections
		"conn_cur":  {"connections.current", true},
		"conn_max":  {"connections.max", true},
		"conn_util": {"connections.%util", true},
		// pg_stat_bgwriter
		"checkpoints_timed":     {"bg_writer.checkpoints.scheduled", false},
		"checkpoints_req":       {"bg_writer.checkpoints.requested", false},
		"checkpoint_write_time": {"bg_writer.checkpoints.write_time", false},
		"checkpoint_sync_time":  {"bg_writer.checkpoints.sync_time", false},
		"buffers_checkpoint":    {"bg_writer.buffers.checkpoint", false},
		"buffers_clean":         {"bg_writer.buffers.clean", false},
		"maxwritten_clean":      {"bg_writer.buffers.maxwritten", false},
		"buffers_backend":       {"bg_writer.buffers.backend", false},
		"buffers_backend_fsync": {"bg_writer.buffers.backend_fsync", false},
		"buffers_alloc":         {"bg_writer.buffers.alloc", false},
		// pg_stat_database
		"numbackends":    {"backends", true},
		"xact_commit":    {"txn.commit", false},
		"xact_rollback":  {"txn.rollback", false},
		"blks_read":      {"blocks.read.total", false},
		"blks_hit":       {"blocks.read.cache", false},
		"blk_read_time":  {"blocks.read.time", false},
		"blk_write_time": {"blocks.write.time", false},
		"tup_returned":   {"rows.select", false},
		"tup_fetched":    {"rows.fetch", false},
		"tup_inserted":   {"rows.insert", false},
		"tup_updated":    {"rows.update", false},
		"tup_deleted":    {"rows.delete", false},
		"conflicts":      {"conflicts", false},
		"deadlocks":      {"deadlocks", false},
		"temp_files":     {"temp.files_created", false},
		"temp_bytes":     {"temp.bytes_written", false},
		// pg_stat_user_tables
		"tablesize":           {"tablesize", true},
		"indexsize":           {"indexsize", true},
		"totalsize":           {"totalsize", true},
		"seq_scan":            {"seq_scan", false},
		"seq_tup_read":        {"seq_tuples_read", false},
		"idx_scan":            {"index_scan", false},
		"idx_tup_read":        {"index_rows_read", false},
		"idx_tup_fetch":       {"index_rows_fetch", false},
		"n_tup_ins":           {"rows.insert", false},
		"n_tup_upd":           {"rows.update", false},
		"n_tup_del":           {"rows.delete", false},
		"n_tup_hot_upd":       {"rows.hot_update", false},
		"n_live_tup":          {"rows.live", true},
		"n_dead_tup":          {"rows.dead", true},
		"n_mod_since_analyze": {"analyze.modifications_since", true},
		"last_analyze":        {"analyze.time_since", true},
		"analyze_count":       {"analyze.count", false},
		"last_autoanalyze":    {"autoanalyze.time_since", true},
		"autoanalyze_count":   {"autoanalyze.count", false},
		"last_vacuum":         {"vacuum.time_since", true},
		"vacuum_count":        {"vacuum.count", false},
		"last_autovacuum":     {"autovacuum.time_since", true},
		"autovacuum_count":    {"autovacuum.count", false},
		// pg_statio_user_tables
		"heap_blks_read":  {"heap.blockread", false},
		"heap_blks_hit":   {"heap.blockhit", false},
		"heap_blks_hr":    {"heap.%hit", true},
		"idx_blks_read":   {"index.blockread", false},
		"idx_blks_hit":    {"index.blockhit", false},
		"idx_blks_hr":     {"index.%hit", false},
		"toast_blks_read": {"toast.blockread", false},
		"toast_blks_hit":  {"toast.blockhit", false},
		"toast_blks_hr":   {"toast.%hit", false},
		"tidx_blks_read":  {"toastindex.blockread", false},
		"tidx_blks_hit":   {"toastindex.blockhit", false},
		"tidx_blks_hr":    {"toastindex.%hit", false},
	}
}

func conv(unk interface{}) uint64 {
	switch i := unk.(type) {
	case float64:
		return uint64(i)
	case float32:
		return uint64(i)
	case int64:
		return uint64(i)
	case uint64:
		return i
	case time.Time:
		return uint64(time.Since(unk.(time.Time)).Seconds())
	default:
		return 0
	}
}

func ReadPGTableStat(dbname string, pgstat *PGStat) error {
	dsn := strings.Replace(POSTGRES_DB_DSN, "$$", dbname, 1)
	log.Trace("[POSTGRES] [%s] Open connection using dsn: %s", dbname, dsn)

	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		log.Error(3, "Can't open Postgres connection to DSN %s: %s", dsn, err)
		return err
	}
	db.Ping()
	defer db.Close()

	log.Debug("[POSTGRES] [%s] Read pg_stat_user_tables join pg_statio_user_tables", dbname)
	rows, err := db.Queryx(`select *, 
								pg_table_size(p.relid) as tablesize, pg_indexes_size(p.relid) as indexsize, pg_total_relation_size(p.relid) as totalsize,
								heap_blks_hit, heap_blks_read, 100*heap_blks_hit / nullif((heap_blks_hit  + heap_blks_read), 0) as heap_blks_hr,
								idx_blks_hit, idx_blks_read, 100*idx_blks_hit  / nullif((idx_blks_hit   + idx_blks_read),  0) as idx_blks_hr,
								toast_blks_hit, toast_blks_read, 100*toast_blks_hit / nullif((toast_blks_hit + toast_blks_read),0) as toast_blks_hr,
								tidx_blks_hit, tidx_blks_read, 100*tidx_blks_hit / nullif((tidx_blks_hit  + tidx_blks_read), 0) as tidx_blks_hr
							from pg_stat_user_tables p inner join pg_statio_user_tables q on p.relid=q.relid;`)
	if err != nil {
		log.Error(3, "[POSTGRES] [%s] Can't execute select * from pg_stat_user_tables...: %s", dbname, err)
		return err
	}

	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [%s] Can't map pg_stat_user_tables: %s", dbname, err)
			break
		}
		tablename := string(results["relname"].([]uint8))
		log.Trace("[POSTGRES] [%s.%s] Scanning pg_stat_user_tables join pg_statio_user_tables", dbname, tablename)
		for k, v := range results {
			pgmetric, ok := pg_translation[k]
			val := conv(v)
			if ok && val != 0 {
				name := "database." + dbname + ".table." + tablename + "." + pgmetric.name
				log.Trace("[POSTGRES] [%s.%s] %s -> %s = %d", dbname, tablename, k, name, val)
				pgstat.metrics[name] = pgmetricvalue{pgmetric.isgauge, val}
			} else {
				log.Trace("[POSTGRES] [%s.%s] %s skipped", dbname, tablename, k)
			}
		}
	}

	log.Debug("[POSTGRES] [%s] Read pg_stat_user_indexes", dbname)
	rows, err = db.Queryx(`select * from pg_stat_user_indexes;`)
	if err != nil {
		log.Error(3, "[POSTGRES] [%s] Can't execute select * from pg_stat_user_indexes...: %s", dbname, err)
		return err
	}

	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [%s] Can't map pg_stat_user_tables: %s", dbname, err)
			break
		}
		tablename := string(results["relname"].([]uint8))
		indexname := string(results["indexrelname"].([]uint8))
		log.Trace("[POSTGRES] [%s.%s] Scanning pg_stat_user_indexes", dbname, tablename)
		for k, v := range results {
			pgmetric, ok := pg_translation[k]
			val := conv(v)
			if ok && val != 0 {
				name := "database." + dbname + ".table." + tablename + ".index." + indexname + "." + pgmetric.name
				log.Trace("[POSTGRES] [%s.%s.%s] %s -> %s = %d", dbname, tablename, indexname, k, name, val)
				pgstat.metrics[name] = pgmetricvalue{pgmetric.isgauge, val}
			} else {
				log.Trace("[POSTGRES] [%s.%s] %s skipped", dbname, tablename, k)
			}
		}
	}

	return nil
}

func ReadPGStat(pgstat *PGStat) error {
	log.Debug("[POSTGRES] Read pg_stat_bgwriter")
	rows, err := db.Queryx("select * from pg_stat_bgwriter")
	if err != nil {
		log.Error(3, "[POSTGRES] [bgwriter] Can't execute select * from pg_stat_bgwriter: %s", err)
		return err
	}

	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [bgwriter] Can't map pg_stat_bgwriter: ", err)
			break
		}
		for k, v := range results {
			pgmetric, ok := pg_translation[k]
			if ok {
				log.Trace("[POSTGRES] [bgwriter] %s -> %s = %d", k, pgmetric.name, conv(v))
				pgstat.metrics[pgmetric.name] = pgmetricvalue{pgmetric.isgauge, conv(v)}
			} else {
				log.Trace("[POSTGRES] [bgwriter] %s skipped", k)
			}
		}
	}
	rows.Close()

	log.Debug("[POSTGRES] Read pg_stat_database")
	rows, err = db.Queryx("select * from pg_stat_database")
	if err != nil {
		log.Error(3, "[POSTGRES] [pg_stat_database] Can't execute select * from pg_stat_database: %s", err)
		return err
	}

	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_stat_database] Can't map pg_stat_database: ", err)
			break
		}
		r := results["datname"]
		if r == nil {
			log.Trace("[POSTGRES] Skip null database")
			continue
		}
		dbname := string(r.([]uint8))
		if re != nil && re.MatchString(dbname) {
			log.Trace("[POSTGRES] Database %s blackisted, ignoring", dbname)
			continue
		}
		log.Trace("[POSTGRES] [%s] Scanning pg_stat_database", dbname)
		for k, v := range results {
			pgmetric, ok := pg_translation[k]
			if ok {
				name := "database." + dbname + "." + pgmetric.name
				log.Trace("[POSTGRES] [%s] %s -> %s = %d", dbname, k, name, conv(v))
				pgstat.metrics[name] = pgmetricvalue{pgmetric.isgauge, conv(v)}
			} else {
				log.Trace("[POSTGRES] [%s] %s skipped", dbname, k)
			}
		}
		// table stats
		if POSTGRES_DB_DSN != "" {
			err = ReadPGTableStat(dbname, pgstat)
			if err != nil {
				log.Error(3, "[POSTGRES] [%s] Can't read postgres table stats, try later", dbname)
				continue
			}
		}
	}

	return nil
}

func ReadPGReplication() (error, float32) {
	var rec bool
	var delay float32

	log.Trace("[POSTGRES] Find if instance is slave")
	err := db.QueryRow("select pg_is_in_recovery()").Scan(&rec)
	if err != nil {
		log.Error(3, "Can't execute select pg_is_in_recovery(): %s", err)
		return err, 0
	}

	if !rec {
		log.Trace("[POSTGRES] Instance is not a slave")
		return nil, 0
	}

	err = db.QueryRow("SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))").Scan(&delay)
	if err != nil {
		log.Error(3, "Can't extract replication delay: %s", err)
		return err, 0
	}
	log.Trace("[POSTGRES] Replication delay = %f", delay)
	return nil, delay
}

func CollectPGStat() {
	var OldPGStat, NewPGStat, TempPGStat PGStat
	var err error
	var m Metrics
	var delay float32
	var curr, max float32
	var util float32

	defer wg.Done()

	if POSTGRES_DSN == "" {
		log.Debug("Postgres DSN empty, collection disabled")
		return
	}

	log.Trace("[POSTGRES] Initialize translation data")
	init_pg_translation()

	log.Trace("[POSTGRES] Compile blacklist")
	re, err = regexp.Compile(POSTGRES_DB_BLACKLIST)
	if err != nil {
		log.Error(3, "[POSTGRES] Cannot compile blacklist regex %s: %s. Ignoring blacklist.", POSTGRES_DB_BLACKLIST, err)
		re = nil
	}

	log.Trace("[POSTGRES] Open database connection")
	db, err = sqlx.Open("postgres", POSTGRES_DSN)
	if err != nil {
		log.Error(3, "Can't open Postgres connection to DSN %s: %s", POSTGRES_DSN, err)
		return
	}
	db.Ping()
	defer db.Close()

	OldPGStat.metrics = make(map[string]pgmetricvalue)
	NewPGStat.metrics = make(map[string]pgmetricvalue)
	err = ReadPGStat(&OldPGStat)
	if err != nil {
		return
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		// Collect global and database specific metrics
		err = ReadPGStat(&NewPGStat)
		if err != nil {
			log.Error(3, "Can't read postgres stats, try later")
			continue
		}
		// Send stats
		for k, v := range NewPGStat.metrics {
			if v.isgauge {
				m = send_metric(m, "postgres."+k, float32(v.value))
			} else {
				log.Trace("[POSTGRES] %s, old = %d, new=%d", k, v.value, OldPGStat.metrics[k].value)
				m = send_metric(m, "postgres."+k, float32(v.value-OldPGStat.metrics[k].value)/float32(DELTA))
			}
		}
		// Collect replication lag
		err, delay = ReadPGReplication()
		if err != nil {
			log.Error(3, "Can't read replication status, try later")
		}
		// Send replication lag
		m = send_metric(m, "postgres.replication.lag", delay)

		// Collect connections
		log.Debug("[POSTGRES] Read connections")
		err := db.QueryRow(`select sum(numbackends) as conn_cur, 
		                      (SELECT setting::float FROM pg_settings WHERE name = 'max_connections') as conn_max,
		                      sum(numbackends) / (SELECT setting::float FROM pg_settings WHERE name = 'max_connections') as conn_util
		                from pg_stat_database;`).Scan(&curr, &max, &util)
		if err != nil {
			log.Error(3, "Can't get number of connections: %s", err)
		} else {
			m = send_metric(m, "postgres."+pg_translation["conn_cur"].name, curr)
			m = send_metric(m, "postgres."+pg_translation["conn_max"].name, max)
			m = send_metric(m, "postgres."+pg_translation["conn_util"].name, util)
		}
		TempPGStat = NewPGStat
		NewPGStat = OldPGStat
		OldPGStat = TempPGStat
		metricschannel <- m
		m = nil
	}
}
