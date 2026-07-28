/*
2021 © Postgres.ai
*/

// Package pgtool provides tools to run PostgreSQL-specific commands.
package pgtool

import (
	"context"
	"fmt"
	"os"

	"github.com/moby/moby/client"
	"github.com/pkg/errors"
)

// ReadControlData reads a control data file.
func ReadControlData(ctx context.Context, d *client.Client, contID, dataDir string, pgVersion float64) (client.HijackedResponse, error) {
	controlDataCmd, err := d.ExecCreate(ctx, contID, pgControlDataConfig(dataDir, pgVersion))

	if err != nil {
		return client.HijackedResponse{}, errors.Wrap(err, "failed to create an exec command")
	}

	attachResponse, err := d.ExecAttach(ctx, controlDataCmd.ID, client.ExecAttachOptions{})
	if err != nil {
		return client.HijackedResponse{}, errors.Wrap(err, "failed to attach to the exec command")
	}

	return attachResponse.HijackedResponse, nil
}

func pgControlDataConfig(pgDataDir string, pgVersion float64) client.ExecCreateOptions {
	command := fmt.Sprintf("/usr/lib/postgresql/%g/bin/pg_controldata", pgVersion)

	return client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{command, "-D", pgDataDir},
		Env:          os.Environ(),
	}
}
