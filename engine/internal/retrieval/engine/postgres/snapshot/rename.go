/*
2026 © Postgres.ai
*/

package snapshot

import (
	"context"
	"fmt"
	"regexp"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"gitlab.com/postgres-ai/database-lab/v3/internal/diagnostic"
	"gitlab.com/postgres-ai/database-lab/v3/internal/retrieval/engine/postgres/tools"
	"gitlab.com/postgres-ai/database-lab/v3/internal/retrieval/engine/postgres/tools/cont"
	"gitlab.com/postgres-ai/database-lab/v3/internal/retrieval/engine/postgres/tools/health"
	"gitlab.com/postgres-ai/database-lab/v3/pkg/config/global"
	"gitlab.com/postgres-ai/database-lab/v3/pkg/log"
)

const (
	renameContainerPrefix = "dblab_rename_"
	dbNameRules           = "must start with a letter or underscore" +
		" and contain only letters, digits, underscores, and hyphens"
)

var validDBNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)

// renameParams holds parameters for running database renames in a standalone container.
type renameParams struct {
	dockerClient *client.Client
	engineProps  *global.EngineProps
	globalCfg    *global.Config
	dataDir      string
	renames      map[string]string
}

// executeDatabaseRenames runs ALTER DATABASE RENAME statements inside an already-running container.
func executeDatabaseRenames(
	ctx context.Context,
	dockerClient *client.Client,
	containerID, user, connDB string,
	renames map[string]string,
) error {
	for oldName, newName := range renames {
		log.Msg(fmt.Sprintf("Renaming database %q to %q", oldName, newName))

		cmd := buildRenameCommand(user, connDB, oldName, newName)

		output, err := tools.ExecCommandWithOutput(ctx, dockerClient, containerID, client.ExecCreateOptions{Cmd: cmd})
		if err != nil {
			return fmt.Errorf("failed to rename database %q to %q: %w", oldName, newName, err)
		}

		log.Msg("Rename result: ", output)
	}

	return nil
}

// runDatabaseRename renames databases using ALTER DATABASE in a temporary container.
func runDatabaseRename(ctx context.Context, params renameParams) error {
	if len(params.renames) == 0 {
		return nil
	}

	connDB := params.globalCfg.Database.Name()

	if err := validateDatabaseRenames(params.renames, connDB); err != nil {
		return err
	}

	pgVersion, err := tools.DetectPGVersion(params.dataDir)
	if err != nil {
		return fmt.Errorf("failed to detect postgres version: %w", err)
	}

	image := fmt.Sprintf("postgresai/extended-postgres:%g", pgVersion)

	if err := tools.PullImage(ctx, params.dockerClient, image); err != nil {
		return fmt.Errorf("failed to pull image for database rename: %w", err)
	}

	pwd, err := tools.GeneratePassword()
	if err != nil {
		return fmt.Errorf("failed to generate password: %w", err)
	}

	hostConfig, err := cont.BuildHostConfig(ctx, params.dockerClient, params.dataDir, nil)
	if err != nil {
		return fmt.Errorf("failed to build host config: %w", err)
	}

	containerName := renameContainerPrefix + params.engineProps.InstanceID

	containerID, err := tools.CreateContainerIfMissing(ctx, params.dockerClient, containerName,
		&container.Config{
			Labels: map[string]string{
				cont.DBLabControlLabel:    cont.DBLabRenameLabel,
				cont.DBLabInstanceIDLabel: params.engineProps.InstanceID,
				cont.DBLabEngineNameLabel: params.engineProps.ContainerName,
			},
			Env: []string{
				"PGDATA=" + params.dataDir,
				"POSTGRES_PASSWORD=" + pwd,
			},
			Image: image,
			Healthcheck: health.GetConfig(
				params.globalCfg.Database.User(),
				connDB,
			),
		},
		hostConfig,
	)
	if err != nil {
		return fmt.Errorf("failed to create rename container: %w", err)
	}

	defer tools.RemoveContainer(ctx, params.dockerClient, containerID, cont.StopPhysicalTimeout)

	defer func() {
		if err != nil {
			tools.PrintContainerLogs(ctx, params.dockerClient, containerName)
			tools.PrintLastPostgresLogs(ctx, params.dockerClient, containerName, params.dataDir)

			filterArgs := make(client.Filters).Add("label", fmt.Sprintf("%s=%s", cont.DBLabControlLabel, cont.DBLabRenameLabel))

			if diagErr := diagnostic.CollectDiagnostics(ctx, params.dockerClient, filterArgs, containerName, params.dataDir); diagErr != nil {
				log.Err("failed to collect rename container diagnostics", diagErr)
			}
		}
	}()

	log.Msg(fmt.Sprintf("Running rename container: %s. ID: %v", containerName, containerID))

	if _, err = params.dockerClient.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("failed to start rename container: %w", err)
	}

	log.Msg("Waiting for rename container readiness")
	log.Msg(fmt.Sprintf("View logs using the command: %s %s", tools.ViewLogsCmd, containerName))

	if err = tools.CheckContainerReadiness(ctx, params.dockerClient, containerID); err != nil {
		return fmt.Errorf("rename container readiness check failed: %w", err)
	}

	user := params.globalCfg.Database.User()

	if err = executeDatabaseRenames(ctx, params.dockerClient, containerID, user, connDB, params.renames); err != nil {
		return err
	}

	if err = tools.RunCheckpoint(ctx, params.dockerClient, containerID, user, connDB); err != nil {
		return fmt.Errorf("failed to run checkpoint after rename: %w", err)
	}

	if err = tools.StopPostgres(ctx, params.dockerClient, containerID, params.dataDir, tools.DefaultStopTimeout); err != nil {
		log.Msg("failed to stop postgres after rename", err)
	}

	return nil
}

func buildRenameCommand(username, connDB, oldName, newName string) []string {
	return []string{
		"psql",
		"-U", username,
		"-d", connDB,
		"-XAtc", fmt.Sprintf(`ALTER DATABASE "%s" RENAME TO "%s"`, oldName, newName),
	}
}

func validateDatabaseRenames(renames map[string]string, connDB string) error {
	targets := make(map[string]string, len(renames))

	for oldName, newName := range renames {
		if oldName == "" || newName == "" {
			return fmt.Errorf("database rename names must not be empty")
		}

		if !validDBNameRegex.MatchString(oldName) {
			return fmt.Errorf("invalid database name %q: %s", oldName, dbNameRules)
		}

		if !validDBNameRegex.MatchString(newName) {
			return fmt.Errorf("invalid database name %q: %s", newName, dbNameRules)
		}

		if oldName == connDB {
			return fmt.Errorf("cannot rename database %q: it is used as the connection database", oldName)
		}

		if newName == connDB {
			return fmt.Errorf("cannot rename database %q to %q: target name is the connection database", oldName, newName)
		}

		if oldName == newName {
			return fmt.Errorf("cannot rename database %q to itself", oldName)
		}

		if prev, ok := targets[newName]; ok {
			return fmt.Errorf("duplicate rename target %q: both %q and %q rename to the same database", newName, prev, oldName)
		}

		targets[newName] = oldName
	}

	for oldName, newName := range renames {
		if _, ok := renames[newName]; ok {
			return fmt.Errorf("chained rename conflict: %q renames to %q, which is also a rename source", oldName, newName)
		}
	}

	return nil
}
