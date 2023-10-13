package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/compose/types"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
)

func (e *DockerEngine) newGraphNode(deploymentID string, pgService string, ipfsService string, pkg *pbsubstreams.Package) (conf types.ServiceConfig, motd string, err error) {

	name := graphServiceName(deploymentID)

	configFolder := filepath.Join(e.dir, deploymentID, "config", "graphnode")
	if err := os.MkdirAll(configFolder, 0755); err != nil {
		return conf, motd, fmt.Errorf("creating folder %q: %w", configFolder, err)
	}

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "graphprotocol/graph-node:v0.32.0",
		Restart:       "on-failure",
		Entrypoint:    []string{"graph-node", "--ipfs=ipfs:5001", "--node-id=index_node_0"},
		Ports: []types.ServicePortConfig{
			{Published: 8000, Target: 8000},
			{Published: 8001, Target: 8001},
			{Published: 8020, Target: 8020},
			{Published: 8030, Target: 8030},
			{Published: 8040, Target: 8040},
		},

		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "./config/graphnode",
				Target: "/opt/subservices/config",
			},
		},
		Links:     []string{pgService + ":postgres", ipfsService + ":ipfs"},
		DependsOn: []string{pgService},
		Environment: map[string]*string{
			"DSN":                       deref("postgres://dev-node:insecure-change-me-in-prod@postgres:5432/dev-node?sslmode=disable"),
			"OUTPUT_MODULE":             &pkg.SinkModule,
			"FIREHOSE_API_TOKEN":        &e.token,
			"SUBSTREAMS_API_TOKEN":      &e.token,
			"GRAPH_NODE_CONFIG":         deref("/opt/subservices/config/graph-node.toml"),
			"GRAPH_LOG":                 deref("debug"),
			"ipfs":                      deref("http://localhost:5001"),
			"GRAPH_MAX_GAS_PER_HANDLER": deref("1_000_000_000_000_000"),
		},
	}

	motd = fmt.Sprintf("Graph-node service, listening on https://localhost:8000 (see the logs with 'docker logs %s')", name)

	pkgContent, err := proto.Marshal(pkg)
	if err != nil {
		return conf, motd, fmt.Errorf("marshalling package: %w", err)
	}

	if err := os.WriteFile(filepath.Join(configFolder, "substreams.spkg"), pkgContent, 0644); err != nil {
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	configFile := []byte(`[general]

[store]
[store.primary]
weight = 1
connection = "$DSN"
pool_size = 10

[deployment]
[[deployment.rule]]
store = "primary"
indexers = [ "index_node_0" ]

[chains]
ingestor = "index_node_0"

[chains.mainnet]
shard = "primary"
provider = [
    { label = "mainnet-firehose", details = { type = "firehose", url = "https://mainnet.eth.streamingfast.io:443", token = "$FIREHOSE_API_TOKEN", features = ["compression", "filters"], conn_pool_size = 1 }},
    { label = "mainnet-substreams", details = { type = "substreams", url = "https://mainnet.eth.streamingfast.io:443", token = "$SUBSTREAMS_API_TOKEN", conn_pool_size = 1 }},
]
`)
	if err := os.WriteFile(filepath.Join(configFolder, "graph-node.toml"), configFile, 0644); err != nil {
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	return conf, motd, nil
}

func graphServiceName(deploymentID string) string {
	return deploymentID + "-graphnode"
}
