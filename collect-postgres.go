package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/kuvasz-io/kuvasz-agent/log"
)

type pgmetric struct {
	name    string
	isgauge bool
}

type pgmetricvalue struct {
	isgauge bool
	value   any
	ts      int64
}

type PGStat struct {
	metrics map[string]pgmetricvalue
}

var pg_translation map[string]pgmetric
var db *sqlx.DB
var re *regexp.Regexp
var pgVersion int

func init_pg_translation() {
	pg_translation = map[string]pgmetric{
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
		// pg_stat_wal
		"wal_records":      {"wal.records", false},
		"wal_fpi":          {"wal.fullpageimages", false},
		"wal_bytes":        {"wal.bytes", false},
		"wal_buffers_full": {"wal.buffers-full", false},
		"wal_write":        {"wal.write.count", false},
		"wal_write_time":   {"wal.write.time", false},
		"wal_sync":         {"wal.sync.count", false},
		"wal_sync_time":    {"wal.sync.time", false},
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
		"analyze_count":       {"analyze.count", true},
		"last_autoanalyze":    {"autoanalyze.time_since", true},
		"autoanalyze_count":   {"autoanalyze.count", true},
		"last_vacuum":         {"vacuum.time_since", true},
		"vacuum_count":        {"vacuum.count", true},
		"last_autovacuum":     {"autovacuum.time_since", true},
		"autovacuum_count":    {"autovacuum.count", true},
		// pg_statio_user_tables
		"heap_blks_read":  {"heap.blockread", false},
		"heap_blks_hit":   {"heap.blockhit", false},
		"heap_blks_hr":    {"heap.%hit", true},
		"idx_blks_read":   {"index.blockread", false},
		"idx_blks_hit":    {"index.blockhit", false},
		"idx_blks_hr":     {"index.%hit", true},
		"toast_blks_read": {"toast.blockread", false},
		"toast_blks_hit":  {"toast.blockhit", false},
		"toast_blks_hr":   {"toast.%hit", true},
		"tidx_blks_read":  {"toastindex.blockread", false},
		"tidx_blks_hit":   {"toastindex.blockhit", false},
		"tidx_blks_hr":    {"toastindex.%hit", true},
		// pg_replication_slots
		"active":              {"replication.active", true},
		"sent_lag_bytes":      {"replication.sent_lag_bytes", true},
		"write_lag_bytes":     {"replication.write_lag_bytes", true},
		"flush_lag_bytes":     {"replication.flush_lag_bytes", true},
		"replay_lag_bytes":    {"replication.replay_lag_bytes", true},
		"confirmed_lag_bytes": {"replication.replay_lag_bytes", true},
		"write_lag":           {"replication.write_lag", true},
		"flush_lag":           {"replication.flush_lag", true},
		"replay_lag":          {"replication.replay_lag", true},
		// pg_stat_statements
		"calls":               {"calls", false},
		"rows":                {"rows", false},
		"total_time":          {"total_time", false},
		"shared_blks_hit":     {"shared_blocks.hit", false},
		"shared_blks_read":    {"shared_blocks.read", false},
		"shared_blks_dirtied": {"shared_blocks.dirtied", false},
		"shared_blks_written": {"shared_blocks.written", false},
		"local_blks_hit":      {"local_blocks.hit", false},
		"local_blks_read":     {"local_blocks.read", false},
		"local_blks_dirtied":  {"local_blocks.dirtied", false},
		"local_blks_written":  {"local_blocks.written", false},
		"temp_blks_read":      {"temp_blocks.read", false},
		"temp_blks_written":   {"temp_blocks.written", false},
	}
}

func conv(unk interface{}) uint64 {
	if unk == nil {
		return 0
	}
	switch i := unk.(type) {
	case float64:
		return uint64(i)
	case float32:
		return uint64(i)
	case int64:
		return uint64(i)
	case uint64:
		return i
	case []uint8:
		x, _ := strconv.ParseUint(string(i), 10, 64)
		return x
	case time.Time:
		return uint64(time.Since(unk.(time.Time)).Seconds())
	case bool:
		if i {
			return 1
		} else {
			return 0
		}
	default:
		log.Error(3, "Converting unknown type: %#v (%T)", unk, unk)
		return 0
	}
}

func tofloat32(unk interface{}) float32 {
	if unk == nil {
		return 0
	}
	switch i := unk.(type) {
	case float64:
		return float32(i)
	case float32:
		return i
	case int:
		return float32(i)
	case int64:
		return float32(i)
	case uint64:
		return float32(i)
	default:
		log.Error(3, "Converting unknown type: %#v (%T)", unk, unk)
		return 0
	}
}

func ReadPGStatActivity(pgstat *PGStat) error {
	log.Debug("[POSTGRES] Read pg_stat_activity")
	rows, err := db.Queryx(`select 'database.' || datname || '.activity.' || coalesce(usename,'postgres') || '.' ||
									case when application_name = '' then 
										regexp_replace(backend_type, '[^a-zA-Z0-9\-]','-', 'g') 
									     else regexp_replace(application_name, '[^a-zA-Z0-9\-]','-', 'g') 
									end || '.' ||
									case state
										when 'idle' then 'idle'
										when 'active' then 'active'
										when 'idle in transaction' then 'idle-in-xact'
										when 'idle in transaction (aborted)' then 'idle-in-xact-aborted'
										when 'fastpath function call' then 'fastpath'
										when 'disabled' then 'disabled'
										else 'other'
									end || '.' ||
							       coalesce(wait_event_type, 'None') || '.' ||
								   coalesce(wait_event, 'None') as metric,
								   count(*) as counts
	                        from pg_stat_activity
							where datname is not null
							group by datname, usename, application_name, state, wait_event_type, wait_event, backend_type`)
	if err != nil {
		log.Error(3, "[POSTGRES] [pg_stat_activity] Can't execute select * from pg_stat_activity: %s", err)
		return err
	}

	defer rows.Close()
	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_stat_activity] Can't map pg_stat_activity: ", err)
			break
		}
		log.Trace("[POSTGRES] [pg_stat_activity] Read row: %+v", results)
		metric := results["metric"].(string)
		log.Trace("[POSTGRES] [pg_stat_activity] %s = %d", metric, conv(results["counts"]))
		pgstat.metrics[metric] = pgmetricvalue{true, conv(results["counts"]), time.Now().Unix()}
	}

	return nil
}

func ReadPGBackends(pgstat *PGStat) error {
	var backendType string
	var count uint64
	log.Debug("[POSTGRES] Read pg_stat_activity backends")
	rows, err := db.Queryx(`select backend_type, count(*) from pg_stat_activity group by backend_type;`)
	if err != nil {
		log.Error(3, "[POSTGRES] [pg_stat_activity] Can't execute select backend_type from pg_stat_activity: %s", err)
		return err
	}

	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&backendType, &count)
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_stat_activity] Can't scan pg_stat_activity backends: ", err)
			break
		}
		log.Trace("[POSTGRES] [pg_stat_activity] Backend type: %s, count: %d", backendType, count)
		pgstat.metrics["backend."+backendType] = pgmetricvalue{true, count, time.Now().Unix()}
	}

	return nil
}

func ReadPGStatTables(dbname string, pgstat *PGStat) error {
	dsn := strings.Replace(POSTGRES_DB_DSN, "$$", dbname, 1)
	log.Trace("[POSTGRES] [%s] Open connection using dsn: %s", dbname, dsn)

	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		log.Error(3, "Can't open Postgres connection to DSN %s: %s", dsn, err)
		return err
	}
	err = db.Ping()
	if err != nil {
		log.Error(3, "Can't Ping Postgres connection to DSN %s: %s", dsn, err)
		return err
	}
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
			if ok {
				name := "database." + dbname + ".table." + tablename + "." + pgmetric.name
				log.Trace("[POSTGRES] [%s.%s] %s -> %s = %d", dbname, tablename, k, name, conv(v))
				pgstat.metrics[name] = pgmetricvalue{pgmetric.isgauge, conv(v), time.Now().Unix()}
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
			if ok {
				name := "database." + dbname + ".table." + tablename + ".index." + indexname + "." + pgmetric.name
				log.Trace("[POSTGRES] [%s.%s.%s] %s -> %s = %d", dbname, tablename, indexname, k, name, conv(v))
				pgstat.metrics[name] = pgmetricvalue{pgmetric.isgauge, conv(v), time.Now().Unix()}
			} else {
				log.Trace("[POSTGRES] [%s.%s] %s skipped", dbname, tablename, k)
			}
		}
	}

	return nil
}

func ReadPGStatBgwriter(pgstat *PGStat) error {
	log.Debug("[POSTGRES] Read pg_stat_bgwriter")
	rows, err := db.Queryx("select * from pg_stat_bgwriter")
	if err != nil {
		log.Error(3, "[POSTGRES] [bgwriter] Can't execute select * from pg_stat_bgwriter: %s", err)
		return err
	}

	defer rows.Close()
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
				pgstat.metrics[pgmetric.name] = pgmetricvalue{pgmetric.isgauge, conv(v), time.Now().Unix()}
			} else {
				log.Trace("[POSTGRES] [bgwriter] %s skipped", k)
			}
		}
	}

	return nil
}

func ReadPGStatDatabase(pgstat *PGStat) error {
	log.Debug("[POSTGRES] Read pg_stat_database")
	rows, err := db.Queryx("select * from pg_stat_database")
	if err != nil {
		log.Error(3, "[POSTGRES] [pg_stat_database] Can't execute select * from pg_stat_database: %s", err)
		return err
	}

	defer rows.Close()
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
				pgstat.metrics[name] = pgmetricvalue{pgmetric.isgauge, conv(v), time.Now().Unix()}
			} else {
				log.Trace("[POSTGRES] [%s] %s skipped", dbname, k)
			}
		}
		// table stats
		if POSTGRES_DB_DSN != "" {
			err = ReadPGStatTables(dbname, pgstat)
			if err != nil {
				log.Error(3, "[POSTGRES] [%s] Can't read postgres table stats, try later", dbname)
				continue
			}
		}
	}

	return nil
}

func ReadPGDatabase(pgstat *PGStat) error {
	log.Debug("[POSTGRES] Read pg_database info")
	rows, err := db.Queryx("select pg_database.datname, pg_database_size(pg_database.datname) as size, age(datfrozenxid) as age FROM pg_database")
	if err != nil {
		log.Error(3, "[POSTGRES] [pg_database] Can't execute select pg_database...: %s", err)
		return err
	}

	defer rows.Close()
	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_database_size] Can't map pg_database_size: ", err)
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
		log.Trace("[POSTGRES] [pg_database_size] Scanning pg_database for %s", dbname)
		name := "database." + dbname
		pgstat.metrics[name+".size"] = pgmetricvalue{true, conv(results["size"]), time.Now().Unix()}
		pgstat.metrics[name+".age"] = pgmetricvalue{true, conv(results["age"]), time.Now().Unix()}
	}
	return nil
}

func ReadPGStatWAL(pgstat *PGStat) error {
	if pgVersion >= 14 {
		log.Debug("[POSTGRES] Read pg_stat_wal")
		rows, err := db.Queryx("select * from pg_stat_wal")
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_stat_wal] Can't execute select pg_stat_wal()...: %s", err)
			return err
		}
		defer rows.Close()
		for rows.Next() {
			results := make(map[string]interface{})
			err = rows.MapScan(results)
			if err != nil {
				log.Error(3, "[POSTGRES] [pg_stat_wal] Can't map pg_stat_wal: ", err)
				break
			}
			for k, v := range results {
				pgmetric, ok := pg_translation[k]
				if ok {
					log.Trace("[POSTGRES] [pg_stat_wal] %s -> %s = %d", k, pgmetric.name, conv(v))
					pgstat.metrics[pgmetric.name] = pgmetricvalue{pgmetric.isgauge, conv(v), time.Now().Unix()}
				} else {
					log.Trace("[POSTGRES] [pg_stat_wal] %s skipped", k)
				}
			}
		}
	}
	if pgVersion >= 10 {
		log.Debug("[POSTGRES] Read pg_ls_waldir")
		rows, err := db.Queryx("select count(*) as count, sum(size) as size from pg_ls_waldir() where length(name)=24;")
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_ls_waldir] Can't execute select pg_ls_waldir()...: %s", err)
			return err
		}

		defer rows.Close()
		for rows.Next() {
			results := make(map[string]interface{})
			err = rows.MapScan(results)
			if err != nil {
				log.Error(3, "[POSTGRES] [pg_ls_waldir] Can't map pg_ls_waldir: ", err)
				break
			}
			count := conv(results["count"])
			size := conv(results["size"])
			log.Trace("[POSTGRES] [pg_ls_waldir] count = %d, size = %d", count, size)
			pgstat.metrics["wal.count"] = pgmetricvalue{true, count, time.Now().Unix()}
			pgstat.metrics["wal.size"] = pgmetricvalue{true, size, time.Now().Unix()}
		}
	}
	return nil
}

func ReadPGLocks(pgstat *PGStat) error {
	log.Debug("[POSTGRES] Read pg_locks")
	rows, err := db.Queryx(`select d.datname, coalesce(c.relname, 'none') rel_name, locktype, mode, granted, count(*) counts
							from pg_locks l 
								inner join pg_database d on l.database = d.oid
								left join pg_class c on relation=c.oid
							where relname  not like 'pg_%' or c.relname is null
							group by d.datname, rel_name, locktype, mode, granted;`)
	if err != nil {
		log.Error(3, "[POSTGRES] [pg_locks] Can't execute select pg_locks...: %s", err)
		return err
	}

	defer rows.Close()
	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_locks] Can't map pg_locks: ", err)
			break
		}
		dbname := string(results["datname"].([]uint8))
		if re != nil && re.MatchString(dbname) {
			log.Trace("[POSTGRES] Database %s blackisted, ignoring", dbname)
			continue
		}
		relname := string(results["rel_name"].([]uint8))
		locktype := string(results["locktype"].(string))
		mode := string(results["mode"].(string))
		granted := ""
		if conv(results["granted"]) == 1 {
			granted = "granted"
		} else {
			granted = "notgranted"
		}
		name := "database." + dbname + ".table." + relname + ".locks." + locktype + "." + mode + "." + granted
		v := results["counts"]
		log.Trace("[POSTGRES] [pg_locks] %s = %d", name, conv(v))
		pgstat.metrics[name] = pgmetricvalue{true, conv(v), time.Now().Unix()}
	}
	return nil
}

func ReadPGStatStatements(pgstat *PGStat) error {
	var q string

	log.Debug("[POSTGRES] Read pg_stat_statements")
	if pgVersion < 13 {
		q = `
				SELECT  db.datname as database, substring(md5(dbid::text || userid::text || queryid::text) for 8) as qid, 
						calls,
						total_time, 
						rows, 
						shared_blks_hit, shared_blks_read, shared_blks_dirtied, shared_blks_written, 
						local_blks_hit, local_blks_read, local_blks_dirtied, local_blks_written, 
						temp_blks_read, temp_blks_written, 
						blk_read_time, blk_write_time
				FROM    		   pg_stat_statements st 
						INNER JOIN pg_database db ON db.oid=st.dbid 
				ORDER BY 4 DESC
				LIMIT 100;`
	} else {
		q = `
				SELECT  db.datname as database, substring(md5(dbid::text || userid::text || queryid::text) for 8) as qid, 
						calls,
						total_exec_time+total_plan_time as total_time, 
						rows, 
						shared_blks_hit, shared_blks_read, shared_blks_dirtied, shared_blks_written, 
						local_blks_hit, local_blks_read, local_blks_dirtied, local_blks_written, 
						temp_blks_read, temp_blks_written, 
						blk_read_time, blk_write_time,
						wal_records, wal_fpi, wal_bytes
				FROM    		   pg_stat_statements st 
						INNER JOIN pg_database db ON db.oid=st.dbid 
				ORDER BY 4 DESC
				LIMIT 100;`
	}
	rows, err := db.Queryx(q)
	if err != nil {
		log.Error(3, "[POSTGRES] [pg_stat_statements] Can't execute pg_stat_statements: %s", err)
		log.Error(3, "Query = %s", q)
		return err
	}

	defer rows.Close()
	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_stat_statements] Can't map pg_stat_statements: ", err)
			break
		}
		r := results["database"]
		if r == nil {
			log.Error(3, "[POSTGRES] [pg_stat_statements] Null database name")
			break
		}
		dbName := string(r.([]uint8))

		r = results["qid"]
		if r == nil {
			log.Error(3, "[POSTGRES] [pg_stat_statements] Null query_id")
			break
		}
		qid := r.(string)

		log.Trace("[POSTGRES] [%s] [%s] Scanning pg_stat_statements", dbName, qid)
		for k, v := range results {
			pgmetric, ok := pg_translation[k]
			if ok {
				name := "database." + dbName + ".statement." + qid + "." + pgmetric.name
				log.Trace("[POSTGRES] [%s] [%s] %s -> %s = %d", dbName, qid, k, name, conv(v))
				pgstat.metrics[name] = pgmetricvalue{pgmetric.isgauge, conv(v), time.Now().Unix()}
			} else {
				log.Trace("[POSTGRES] [%s] %s skipped", dbName, k)
			}
		}
	}

	return nil
}

func ReadPGReplicationSlots(pgstat *PGStat) error {
	log.Debug("[POSTGRES] Read pg_replication_slots")
	rows, err := db.Queryx(`select 
                            database, active, 
                            (pg_current_wal_lsn() - sent_lsn)::bigint   as sent_lag_bytes, 
                            (pg_current_wal_lsn() - write_lsn)::bigint  as write_lag_bytes,
                            (pg_current_wal_lsn() - flush_lsn)::bigint  as flush_lag_bytes,
                            (pg_current_wal_lsn() - replay_lsn)::bigint as replay_lag_bytes,
                            (pg_current_wal_lsn() - confirmed_flush_lsn)::bigint as confirmed_lag_bytes,
                            (extract(epoch from write_lag)*1000)::bigint as write_lag,
                            (extract(epoch from flush_lag)*1000)::bigint as flush_lag,
                            (extract(epoch from replay_lag)*1000)::bigint as replay_lag
                        from pg_replication_slots left outer join pg_stat_replication on pid=active_pid;
                        `)
	if err != nil {
		log.Error(3, "[POSTGRES] [pg_replication_slots] Can't execute select from pg_replication_slots: %s", err)
		return err
	}

	defer rows.Close()
	for rows.Next() {
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			log.Error(3, "[POSTGRES] [pg_replication_slots] Can't map pg_replication_slots: ", err)
			break
		}
		r := results["database"]
		if r == nil {
			log.Error(3, "[POSTGRES] [pg_replication_slots] Null database name")
			break
		}
		dbName := string(r.([]uint8))
		log.Trace("[POSTGRES] [%s] Scanning pg_replication_slots", dbName)
		for k, v := range results {
			pgmetric, ok := pg_translation[k]
			if ok {
				name := "database." + dbName + "." + pgmetric.name
				log.Trace("[POSTGRES] [%s] %s -> %s = %d", dbName, k, name, conv(v))
				pgstat.metrics[name] = pgmetricvalue{pgmetric.isgauge, conv(v), time.Now().Unix()}
			} else {
				log.Trace("[POSTGRES] [%s] %s skipped", dbName, k)
			}
		}
	}

	return nil
}

func ReadPGGlobals(pgstat *PGStat) error {
	var uptime, autovacuumFreezeMaxAge, vacuumFreezeMinAge, vacuumFreezeTableAge, autovacuumMaxWorkers int64

	log.Debug("[POSTGRES] Read config and other globals")
	err := db.QueryRow(`select extract('epoch' from now() - pg_postmaster_start_time())::bigint,
							current_setting('autovacuum_freeze_max_age')::bigint,
							current_setting('vacuum_freeze_min_age')::bigint,
							current_setting('vacuum_freeze_table_age')::bigint,
							current_setting('autovacuum_max_workers')::bigint`).
		Scan(&uptime,
			&autovacuumFreezeMaxAge,
			&vacuumFreezeMinAge,
			&vacuumFreezeTableAge,
			&autovacuumMaxWorkers)
	if err != nil {
		log.Error(3, "[POSTGRES] [PGGlobals] Can't execute select from pg_postmaster_start_time() and globals: %s", err)
		return err
	}

	pgstat.metrics["uptime"] = pgmetricvalue{true, conv(uptime), time.Now().Unix()}
	pgstat.metrics["config.autovacuum_freeze_max_age"] = pgmetricvalue{true, conv(autovacuumFreezeMaxAge), time.Now().Unix()}
	pgstat.metrics["config.vacuum_freeze_min_age"] = pgmetricvalue{true, conv(vacuumFreezeMinAge), time.Now().Unix()}
	pgstat.metrics["config.vacuum_freeze_table_age"] = pgmetricvalue{true, conv(vacuumFreezeTableAge), time.Now().Unix()}
	pgstat.metrics["config.autovacuum_max_workers"] = pgmetricvalue{true, conv(autovacuumMaxWorkers), time.Now().Unix()}
	return nil
}

func ReadPGReplication(pgstat *PGStat) error {
	var rec bool
	var delay float32

	log.Trace("[POSTGRES] Find if instance is slave")
	err := db.QueryRow("select pg_is_in_recovery()").Scan(&rec)
	if err != nil {
		log.Error(3, "Can't execute select pg_is_in_recovery(): %s", err)
		return err
	}

	if !rec {
		log.Trace("[POSTGRES] Instance is not a slave")
		pgstat.metrics["replication.lag"] = pgmetricvalue{true, 0, time.Now().Unix()}
		return nil
	}

	err = db.QueryRow(`SELECT CASE WHEN pg_last_wal_receive_lsn() = pg_last_wal_replay_lsn()
                                   THEN 0
                                   ELSE EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))
                              END`).Scan(&delay)
	if err != nil {
		log.Error(3, "Can't extract replication delay: %s", err)
		return err
	}
	log.Trace("[POSTGRES] Replication delay = %f", delay)
	pgstat.metrics["replication.lag"] = pgmetricvalue{true, delay, time.Now().Unix()}
	return nil
}

func ReadConnections(pgstat *PGStat) error {
	var curr, max uint64
	var util float32

	log.Debug("[POSTGRES] Read connections")
	err := db.QueryRow(`select sum(numbackends) as conn_cur, 
								  (SELECT setting::float FROM pg_settings WHERE name = 'max_connections') as conn_max,
								  sum(numbackends) / (SELECT setting::float FROM pg_settings WHERE name = 'max_connections') as conn_util
							from pg_stat_database;`).Scan(&curr, &max, &util)
	if err != nil {
		log.Error(3, "Can't get number of connections: %s", err)
		return err
	}
	log.Trace("[POSTGRES] Connections current = %f, max = %f, utilization = %f", curr, max, util)
	pgstat.metrics["connections.current"] = pgmetricvalue{true, curr, time.Now().Unix()}
	pgstat.metrics["connections.max"] = pgmetricvalue{true, max, time.Now().Unix()}
	pgstat.metrics["connections.%util"] = pgmetricvalue{true, util, time.Now().Unix()}
	return nil
}

func ReadPGStat(pgstat *PGStat) error {
	var err error

	err = ReadPGStatBgwriter(pgstat)
	if err != nil {
		return err
	}

	err = ReadPGStatDatabase(pgstat)
	if err != nil {
		return err
	}

	err = ReadPGDatabase(pgstat)
	if err != nil {
		return err
	}

	err = ReadPGStatWAL(pgstat)
	if err != nil {
		return err
	}

	err = ReadPGLocks(pgstat)
	if err != nil {
		return err
	}

	err = ReadPGGlobals(pgstat)
	if err != nil {
		return err
	}

	err = ReadPGStatActivity(pgstat)
	if err != nil {
		return err
	}

	err = ReadPGBackends(pgstat)
	if err != nil {
		return err
	}
	_ = ReadPGStatStatements(pgstat)
	_ = ReadPGReplicationSlots(pgstat)
	_ = ReadPGReplication(pgstat)
	_ = ReadConnections(pgstat)

	return nil
}

func DoCollectPGStat() {
	var OldPGStat, NewPGStat, TempPGStat PGStat
	var err error
	var m Metrics

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
	err = db.Ping()
	if err != nil {
		log.Error(3, "Can't Ping Postgres connection to DSN %s: %s", POSTGRES_DSN, err)
		return
	}
	defer db.Close()

	err = db.QueryRow("SELECT current_setting('server_version_num')::int/10000;").Scan(&pgVersion)
	if err != nil {
		log.Error(3, "Can't read server version: %v", err)
		return
	}
	OldPGStat.metrics = make(map[string]pgmetricvalue)
	NewPGStat.metrics = make(map[string]pgmetricvalue)
	err = ReadPGStat(&OldPGStat)
	if err != nil {
		return
	}
	for {
		time.Sleep(time.Duration(DELTA) * time.Second)
		log.Info("[POSTGRES] Collect stats")
		// Collect global and database specific metrics
		err = ReadPGStat(&NewPGStat)
		if err != nil {
			log.Error(3, "Can't read postgres stats, try later")
			continue
		}
		// Send stats
		for k, v := range NewPGStat.metrics {
			if v.isgauge {
				m = send_metric(m, "postgres."+k, tofloat32(v.value))
			} else {
				ov, ok := OldPGStat.metrics[k]
				ovvalue := conv(ov.value)
				vvalue := conv(v.value)
				if !ok {
					log.Trace("[POSTGRES] Found new metric: %s, storing", k)
				} else {
					log.Trace("[POSTGRES] %s, new=%d, old=%d", k, v.value, ov.value)
					if ovvalue > vvalue {
						log.Debug("[POSTGRES] %s, skipping decreasing counter", k)
					} else {
						m = send_metric(m, "postgres."+k, float32(vvalue-ovvalue)/float32(v.ts-ov.ts))
					}
				}
			}
		}

		TempPGStat = NewPGStat
		NewPGStat = OldPGStat
		OldPGStat = TempPGStat
		metricschannel <- m
		m = nil
	}
}

func CollectPGStat() {
	if POSTGRES_DSN == "" {
		log.Debug("Postgres DSN empty, collection disabled")
		return
	}

	for {
		DoCollectPGStat()
		time.Sleep(5 * time.Second)
	}
}
