package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestIsPostgresDatabaseNotFound(t *testing.T) {
	require.True(t, isPostgresDatabaseNotFound(&pgconn.PgError{Code: postgresDatabaseNotFoundCode}))
	require.True(t, isPostgresDatabaseNotFound(fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: postgresDatabaseNotFoundCode})))
	require.False(t, isPostgresDatabaseNotFound(&pgconn.PgError{Code: "28P01"}))
	require.False(t, isPostgresDatabaseNotFound(errors.New("connection refused")))
}

func TestIsPostgresDatabaseExists(t *testing.T) {
	require.True(t, isPostgresDatabaseExists(&pgconn.PgError{Code: postgresDuplicateDatabaseCode}))
	require.True(t, isPostgresDatabaseExists(fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: postgresDuplicateDatabaseCode})))
	require.False(t, isPostgresDatabaseExists(&pgconn.PgError{Code: "28P01"}))
	require.False(t, isPostgresDatabaseExists(errors.New("connection refused")))
}

func TestIsMySQLDatabaseNotFound(t *testing.T) {
	require.True(t, isMySQLDatabaseNotFound(&mysql.MySQLError{Number: mysqlUnknownDatabaseNumber}))
	require.False(t, isMySQLDatabaseNotFound(&mysql.MySQLError{Number: 1045}))
	require.False(t, isMySQLDatabaseNotFound(errors.New("connection refused")))
}

func TestIsMySQLDatabaseExists(t *testing.T) {
	require.True(t, isMySQLDatabaseExists(&mysql.MySQLError{Number: mysqlDatabaseExistsNumber}))
	require.False(t, isMySQLDatabaseExists(&mysql.MySQLError{Number: 1045}))
	require.False(t, isMySQLDatabaseExists(errors.New("connection refused")))
}

func TestQuotePostgresIdentifier(t *testing.T) {
	require.Equal(t, `"kitedb"`, quotePostgresIdentifier("kitedb"))
	require.Equal(t, `"kite""db"`, quotePostgresIdentifier(`kite"db`))
}

func TestQuoteMySQLIdentifier(t *testing.T) {
	require.Equal(t, "`kitedb`", quoteMySQLIdentifier("kitedb"))
	require.Equal(t, "`kite``db`", quoteMySQLIdentifier("kite`db"))
}

func TestEnsureMySQLParseTime(t *testing.T) {
	cfg, err := mysql.ParseDSN("user:pass@tcp(localhost:3306)/kitedb")
	require.NoError(t, err)

	withParseTime := ensureMySQLParseTime(cfg)
	require.True(t, withParseTime.ParseTime)
	require.Contains(t, withParseTime.FormatDSN(), "parseTime=true")
	require.False(t, cfg.ParseTime)
}

func TestExplicitPostgresDatabaseName(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
		ok   bool
	}{
		{
			name: "url path",
			dsn:  "postgres://user:pass@localhost:5432/kitedb?sslmode=disable",
			want: "kitedb",
			ok:   true,
		},
		{
			name: "url dbname query",
			dsn:  "postgres://user:pass@localhost:5432/?dbname=kitedb",
			want: "kitedb",
			ok:   true,
		},
		{
			name: "keyword dbname",
			dsn:  "host=localhost user=postgres dbname=kitedb sslmode=disable",
			want: "kitedb",
			ok:   true,
		},
		{
			name: "keyword quoted dbname",
			dsn:  "host=localhost user=postgres dbname='kite db' sslmode=disable",
			want: "kite db",
			ok:   true,
		},
		{
			name: "keyword escaped quote",
			dsn:  `host=localhost dbname='kite\'db' sslmode=disable`,
			want: "kite'db",
			ok:   true,
		},
		{
			name: "no explicit database",
			dsn:  "host=localhost user=postgres sslmode=disable",
			want: "",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := explicitPostgresDatabaseName(tt.dsn)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}
