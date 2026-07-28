//go:build integration
// +build integration

package snapshot

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestDatabaseRename(t *testing.T) {
	ctx := context.Background()

	logStrategy := wait.NewLogStrategy("database system is ready to accept connections")
	logStrategy.Occurrence = 2

	req := testcontainers.ContainerRequest{
		Name:         "pg_rename_test",
		Image:        "postgres:17",
		ExposedPorts: []string{port},
		WaitingFor: wait.ForAll(
			logStrategy,
			wait.ForLog("PostgreSQL init process complete; ready for start up."),
		),
		Env: map[string]string{
			"POSTGRES_PASSWORD": testPassword,
			"PGDATA":            pgdata,
		},
	}

	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	defer func() { _ = pgContainer.Terminate(ctx) }()

	// create test databases.
	code, _, err := pgContainer.Exec(ctx, []string{
		"psql", "-U", user, "-d", dbname, "-XAtc", "CREATE DATABASE test_prod",
	})
	require.NoError(t, err)
	require.Equal(t, 0, code)

	code, _, err = pgContainer.Exec(ctx, []string{
		"psql", "-U", user, "-d", dbname, "-XAtc", "CREATE DATABASE another_db",
	})
	require.NoError(t, err)
	require.Equal(t, 0, code)

	// run rename using the command format produced by buildRenameCommand.
	renameCmd := buildRenameCommand(user, dbname, "test_prod", "test_dblab")
	code, _, err = pgContainer.Exec(ctx, renameCmd)
	require.NoError(t, err)
	require.Equal(t, 0, code, "ALTER DATABASE RENAME should succeed")

	// verify: test_prod renamed to test_dblab, another_db untouched.
	code, reader, err := pgContainer.Exec(ctx, []string{
		"psql", "-U", user, "-d", dbname, "-XAtc",
		"SELECT datname FROM pg_database WHERE datname IN ('test_prod', 'test_dblab', 'another_db') ORDER BY datname",
	})
	require.NoError(t, err)
	require.Equal(t, 0, code)

	var outBuf, errBuf bytes.Buffer
	_, err = stdcopy.StdCopy(&outBuf, &errBuf, reader)
	require.NoError(t, err)

	if errBuf.Len() > 0 {
		t.Log("stderr: ", errBuf.String())
	}

	output := outBuf.String()

	assert.Contains(t, output, "another_db", "another_db should still exist")
	assert.Contains(t, output, "test_dblab", "test_prod should have been renamed to test_dblab")
	assert.False(t, strings.Contains(output, "test_prod"), "test_prod should no longer exist")

	// verify rename of connection database is rejected.
	require.Error(t, validateDatabaseRenames(map[string]string{dbname: "renamed"}, dbname))
}
