package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/cli/cli/compose/types"
)

func (e *DockerEngine) newIPFS(deploymentID string) (conf types.ServiceConfig, motd string, err error) {

	name := fmt.Sprintf("%s-ipfs", deploymentID)
	localPort := uint32(5001) // TODO: assign dynamically

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "ipfs")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return types.ServiceConfig{}, "", fmt.Errorf("creating folder %q: %w", dataFolder, err)
	}

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "ipfs/kubo:v0.23.0",
		Restart:       "on-failure",
		Ports: []types.ServicePortConfig{
			{
				Published: localPort,
				Target:    5001,
			},
		},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "./data/ipfs",
				Target: "/data/ipfs",
			},
		},
		HealthCheck: &types.HealthCheckConfig{
			Test:     []string{"CMD", "nc", "-z", "localhost:5001"},
			Interval: toDuration(time.Second * 3),
			Timeout:  toDuration(time.Second * 2),
			Retries:  deref(uint64(10)),
		},
	}

	motd = fmt.Sprintf("IPFS service %q available at URL: 'http://localhost:%d'",
		name,
		localPort,
	)

	return conf, motd, nil

}
