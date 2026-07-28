/*
2021 © Postgres.ai
*/

package cont

import (
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/moby/moby/api/types/container"
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

func TestResourceOptions(t *testing.T) {
	testCases := []struct {
		configOptions  map[string]interface{}
		expectedConfig *container.HostConfig
	}{
		{
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
			},
		},
		{
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
			},
		},
	}

	for _, tc := range testCases {
		hostConfig, err := ResourceOptions(tc.configOptions)
		require.Nil(t, err)

		assert.Equal(t, tc.expectedConfig.Resources, hostConfig.Resources)
		assert.Equal(t, tc.expectedConfig.ShmSize, hostConfig.ShmSize)
	}
}
