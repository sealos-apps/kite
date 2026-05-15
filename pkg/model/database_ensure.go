package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"k8s.io/klog/v2"
)

const (
	postgresDatabaseNotFoundCode  = "3D000"
	postgresDuplicateDatabaseCode = "42P04"
	mysqlUnknownDatabaseNumber    = 1049
	mysqlDatabaseExistsNumber     = 1007
)

func ensureDatabaseExists(dbType, dsn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch dbType {
	case "postgres":
		return ensurePostgresDatabaseExists(ctx, dsn)
	case "mysql":
		return ensureMySQLDatabaseExists(ctx, dsn)
	default:
		return nil
	}
}

func ensurePostgresDatabaseExists(ctx context.Context, dsn string) error {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse postgres dsn: %w", err)
	}
	dbName, ok := explicitPostgresDatabaseName(dsn)
	if !ok {
		return nil
	}
	if dbName == "" {
		return nil
	}

	if err := pingSQL(ctx, "pgx", dsn); err == nil {
		return nil
	} else if !isPostgresDatabaseNotFound(err) {
		return err
	}

	klog.Infof("Postgres database %q does not exist, creating it", dbName)
	if err := createPostgresDatabase(ctx, cfg, quotePostgresIdentifier(dbName)); err != nil {
		if isPostgresDatabaseExists(err) {
			return nil
		}
		return fmt.Errorf("create postgres database %q: %w", dbName, err)
	}
	return nil
}

func ensureMySQLDatabaseExists(ctx context.Context, dsn string) error {
	cfg, err := mysql.ParseDSN(strings.TrimPrefix(dsn, "mysql://"))
	if err != nil {
		return fmt.Errorf("parse mysql dsn: %w", err)
	}
	dbName := cfg.DBName
	if dbName == "" {
		return nil
	}

	targetDSN := ensureMySQLParseTime(cfg).FormatDSN()
	if err := pingSQL(ctx, "mysql", targetDSN); err == nil {
		return nil
	} else if !isMySQLDatabaseNotFound(err) {
		return err
	}

	adminCfg := cfg.Clone()
	adminCfg.DBName = ""
	klog.Infof("MySQL database %q does not exist, creating it", dbName)
	if err := createSQLDatabase(ctx, "mysql", adminCfg.FormatDSN(), quoteMySQLIdentifier(dbName)); err != nil {
		if isMySQLDatabaseExists(err) {
			return nil
		}
		return fmt.Errorf("create mysql database %q: %w", dbName, err)
	}
	return nil
}

func pingSQL(ctx context.Context, driverName, dsn string) error {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.PingContext(ctx)
}

func createSQLDatabase(ctx context.Context, driverName, dsn, quotedName string) error {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "CREATE DATABASE "+quotedName)
	return err
}

func createPostgresDatabase(ctx context.Context, cfg *pgx.ConnConfig, quotedName string) error {
	adminCfg := cfg.Copy()
	adminCfg.Config.Database = "postgres"

	db := stdlib.OpenDB(*adminCfg)
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, "CREATE DATABASE "+quotedName)
	return err
}

func isPostgresDatabaseNotFound(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == postgresDatabaseNotFoundCode
}

func isPostgresDatabaseExists(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == postgresDuplicateDatabaseCode
}

func isMySQLDatabaseNotFound(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlUnknownDatabaseNumber
}

func isMySQLDatabaseExists(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDatabaseExistsNumber
}

func ensureMySQLParseTime(cfg *mysql.Config) *mysql.Config {
	cfg = cfg.Clone()
	cfg.ParseTime = true
	return cfg
}

func quotePostgresIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func quoteMySQLIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func explicitPostgresDatabaseName(dsn string) (string, bool) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return explicitPostgresURLDatabaseName(dsn)
	}
	return explicitPostgresKeywordDatabaseName(dsn)
}

func explicitPostgresURLDatabaseName(dsn string) (string, bool) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", false
	}
	if dbName := strings.TrimLeft(u.Path, "/"); dbName != "" {
		return dbName, true
	}
	query := u.Query()
	if dbName := query.Get("dbname"); dbName != "" {
		return dbName, true
	}
	if dbName := query.Get("database"); dbName != "" {
		return dbName, true
	}
	return "", false
}

func explicitPostgresKeywordDatabaseName(dsn string) (string, bool) {
	for len(dsn) > 0 {
		eqIdx := strings.IndexRune(dsn, '=')
		if eqIdx < 0 {
			return "", false
		}
		key := strings.Trim(dsn[:eqIdx], " \t\n\r\v\f")
		dsn = strings.TrimLeft(dsn[eqIdx+1:], " \t\n\r\v\f")

		value, rest, ok := nextPostgresKeywordValue(dsn)
		if !ok {
			return "", false
		}
		if key == "dbname" || key == "database" {
			return value, true
		}
		dsn = rest
	}
	return "", false
}

func nextPostgresKeywordValue(s string) (string, string, bool) {
	if s == "" {
		return "", "", true
	}
	if s[0] == '\'' {
		var value strings.Builder
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' {
				i++
				if i == len(s) {
					return "", "", false
				}
				value.WriteByte(s[i])
				continue
			}
			if s[i] == '\'' {
				rest := strings.TrimLeft(s[i+1:], " \t\n\r\v\f")
				return value.String(), rest, true
			}
			value.WriteByte(s[i])
		}
		return "", "", false
	}

	end := 0
	var value strings.Builder
	for ; end < len(s); end++ {
		if isPostgresKeywordSpace(s[end]) {
			break
		}
		if s[end] == '\\' {
			end++
			if end == len(s) {
				return "", "", false
			}
		}
		value.WriteByte(s[end])
	}
	rest := ""
	if end < len(s) {
		rest = strings.TrimLeft(s[end+1:], " \t\n\r\v\f")
	}
	return value.String(), rest, true
}

func isPostgresKeywordSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
