/*
2021 © Postgres.ai
*/

package cont

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldStopInternalProcess(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		expected bool
	}{
		{name: "sync label stops process", label: DBLabSyncLabel, expected: true},
		{name: "promote label does not stop", label: DBLabPromoteLabel, expected: false},
		{name: "patch label does not stop", label: DBLabPatchLabel, expected: false},
		{name: "dump label does not stop", label: DBLabDumpLabel, expected: false},
		{name: "restore label does not stop", label: DBLabRestoreLabel, expected: false},
		{name: "embedded ui label does not stop", label: DBLabEmbeddedUILabel, expected: false},
		{name: "foundation label does not stop", label: DBLabFoundationLabel, expected: false},
		{name: "rename label does not stop", label: DBLabRenameLabel, expected: false},
		{name: "empty label does not stop", label: "", expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, shouldStopInternalProcess(tc.label))
		})
	}
}

func TestGetContainerName(t *testing.T) {
	tests := []struct {
		name     string
		names    []string
		expected string
	}{
		{name: "single name", names: []string{"/my_container"}, expected: "/my_container"},
		{name: "multiple names", names: []string{"/name1", "/name2"}, expected: "/name1, /name2"},
		{name: "no names", names: nil, expected: ""},
		{name: "empty names", names: []string{}, expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := container.Summary{Names: tc.names}
			assert.Equal(t, tc.expected, getContainerName(c))
		})
	}
}

func TestBuildHostConfigIncludesConfiguredVolume(t *testing.T) {
	const dataDir = "/var/lib/dblab"

	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"Mounts":[
			{"Type":"bind","Source":"/var/lib/dblab","Destination":"/var/lib/dblab","RW":true},
			{"Type":"bind","Source":"/etc/dblab","Destination":"/etc/dblab","RW":false}
		]}`))
		require.NoError(t, err)
	}))
	defer dockerServer.Close()

	dockerClient, err := client.New(client.WithHost(dockerServer.URL), client.WithAPIVersion("1.47"))
	require.NoError(t, err)
	defer func() { require.NoError(t, dockerClient.Close()) }()

	hostConfig, err := BuildHostConfig(context.Background(), dockerClient, dataDir, map[string]interface{}{
		"volume": "/etc/dblab/certs:/var/lib/postgresql/cert:ro",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"/etc/dblab/certs:/var/lib/postgresql/cert:ro"}, hostConfig.Binds)

	targets := make([]string, 0, len(hostConfig.Mounts))
	for _, m := range hostConfig.Mounts {
		targets = append(targets, m.Target)
	}

	assert.Contains(t, targets, dataDir, "configured binds must not replace inherited mounts")
}

func TestResolveMountConflicts(t *testing.T) {
	const dataDir = "/var/lib/dblab"

	inherited := []mount.Mount{
		{Type: mount.TypeBind, Source: dataDir, Target: dataDir},
		{Type: mount.TypeBind, Source: "/etc/dblab/certs", Target: "/var/lib/postgresql/cert", ReadOnly: true},
	}

	testCases := []struct {
		name            string
		binds           []string
		expectedBinds   []string
		expectedTargets []string
	}{
		{
			name:            "no binds keep every inherited mount",
			binds:           nil,
			expectedBinds:   nil,
			expectedTargets: []string{dataDir, "/var/lib/postgresql/cert"},
		},
		{
			name:            "bind coexists with inherited mounts",
			binds:           []string{"/etc/dblab/pgpass:/var/lib/postgresql/.pgpass:ro"},
			expectedBinds:   []string{"/etc/dblab/pgpass:/var/lib/postgresql/.pgpass:ro"},
			expectedTargets: []string{dataDir, "/var/lib/postgresql/cert"},
		},
		{
			name:            "bind overrides the inherited mount with the same destination",
			binds:           []string{"/etc/dblab/certs:/var/lib/postgresql/cert/:ro"},
			expectedBinds:   []string{"/etc/dblab/certs:/var/lib/postgresql/cert/:ro"},
			expectedTargets: []string{dataDir},
		},
		{
			name:            "bind shadowing the data directory is skipped",
			binds:           []string{"/mnt/other:" + dataDir},
			expectedBinds:   []string{},
			expectedTargets: []string{dataDir, "/var/lib/postgresql/cert"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			binds, mounts := resolveMountConflicts(tc.binds, inherited, dataDir)

			targets := make([]string, 0, len(mounts))
			for _, m := range mounts {
				targets = append(targets, m.Target)
			}

			assert.Equal(t, tc.expectedBinds, binds)
			assert.Equal(t, tc.expectedTargets, targets)
		})
	}
}

func TestResourceOptions(t *testing.T) {
	testCases := []struct {
		name           string
		configOptions  map[string]interface{}
		expectedConfig *container.HostConfig
	}{
		{
			name: "hyphenated resource names and single volume",
			configOptions: map[string]interface{}{
				"memory":             100000,
				"memory-swappiness":  50,
				"memory-reservation": 3000,
				"memory-swap":        "100M",
				"shm-size":           "64m",
				"oom-kill-disable":   true,

				"cpu-period":  100000,
				"cpu-quota":   100000,
				"cpuset-cpus": "1",
				"cpu-shares":  1024,

				"blkio-weight":  150,
				"oom-score-adj": 10,
				"volume":        "/etc/dblab/certs:/var/lib/postgresql/cert:ro",
			},
			expectedConfig: &container.HostConfig{
				Resources: container.Resources{
					Memory:            100000,
					MemorySwappiness:  pointer.ToInt64(50),
					MemoryReservation: 3000,
					MemorySwap:        104857600,
					OomKillDisable:    pointer.ToBool(true),
					CpusetCpus:        "1",
					CPUPeriod:         100000,
					CPUQuota:          100000,
					CPUShares:         1024,
					BlkioWeight:       150,
				},
				OomScoreAdj: 10,
				ShmSize:     67108864,
				Binds:       []string{"/etc/dblab/certs:/var/lib/postgresql/cert:ro"},
			},
		},
		{
			name: "normalized resource names and a named volume",
			configOptions: map[string]interface{}{
				"memory":            100000,
				"memoryswappiness":  50,
				"memoryreservation": 3000,
				"memoryswap":        "100M",
				"shmsize":           "1gb",
				"oomkilldisable":    true,

				"cpuperiod":  100000,
				"cpuquota":   100000,
				"cpusetcpus": "1",
				"cpushares":  1024,

				"blkioweight": 150,
				"oomscoreadj": 10,
				"volume":      "dblab-config:/etc/dblab:ro",
			},
			expectedConfig: &container.HostConfig{
				Resources: container.Resources{
					Memory:            100000,
					MemorySwappiness:  pointer.ToInt64(50),
					MemoryReservation: 3000,
					MemorySwap:        104857600,
					OomKillDisable:    pointer.ToBool(true),
					CpusetCpus:        "1",
					CPUPeriod:         100000,
					CPUQuota:          100000,
					CPUShares:         1024,
					BlkioWeight:       150,
				},
				OomScoreAdj: 10,
				ShmSize:     1073741824,
				Binds:       []string{"dblab-config:/etc/dblab:ro"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hostConfig, err := ResourceOptions(tc.configOptions)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedConfig.Resources, hostConfig.Resources)
			assert.Equal(t, tc.expectedConfig.ShmSize, hostConfig.ShmSize)
			assert.Equal(t, tc.expectedConfig.Binds, hostConfig.Binds)
		})
	}
}

func TestResourceOptionsRejectsInvalidVolumes(t *testing.T) {
	testCases := []struct {
		name   string
		volume interface{}
	}{
		{name: "scalar is not a string", volume: 42},
		{name: "list is not a string", volume: []interface{}{"/etc/dblab/certs:/var/lib/postgresql/cert:ro"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResourceOptions(map[string]interface{}{"volume": tc.volume})
			require.ErrorContains(t, err, "container configuration volume must be a string")
		})
	}
}
