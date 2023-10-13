package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/compose/types"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

func (e *DockerEngine) newGraphDeploy(deploymentID string, ipfsService string, graphnodeService string, pkg *pbsubstreams.Package) (conf types.ServiceConfig, motd string, err error) {

	name := graphdeployServiceName(deploymentID)

	configFolder := filepath.Join(e.dir, deploymentID, "config", "graphdeploy")
	if err := os.MkdirAll(configFolder, 0755); err != nil {
		return conf, motd, fmt.Errorf("creating folder %q: %w", configFolder, err)
	}

	dataFolder := filepath.Join(e.dir, deploymentID, "data", "graphdeploy")
	if err := os.MkdirAll(dataFolder, 0755); err != nil {
		return conf, motd, fmt.Errorf("creating folder %q: %w", dataFolder, err)
	}

	conf = types.ServiceConfig{
		Name:          name,
		ContainerName: name,
		Image:         "node:20",
		Restart:       "on-failure",
		Entrypoint: []string{
			"/opt/subservices/config/start.sh",
		},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "./data/graphdeploy",
				Target: "/opt/subservices/data",
			},
			{
				Type:   "bind",
				Source: "./config/graphdeploy",
				Target: "/opt/subservices/config",
			},
		},
		Links:     []string{ipfsService + ":ipfs", graphnodeService + ":graphnode"},
		DependsOn: []string{ipfsService, graphnodeService},
	}

	motd = fmt.Sprintf("Graph deploy service (no exposed port). Use 'docker logs %s' to see the logs.", name)

	pkgContent, err := proto.Marshal(pkg)
	if err != nil {
		return conf, motd, fmt.Errorf("marshalling package: %w", err)
	}

	pkgName := pkg.PackageMeta[0].Name
	pkgVersion := pkg.PackageMeta[0].Version

	spkgName := fmt.Sprintf("%s-%s.spkg", pkgName, pkgVersion)

	if err := os.WriteFile(filepath.Join(configFolder, spkgName), pkgContent, 0644); err != nil {
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	//FIXME
	schemaGraphql := []byte(`
type approvals @entity {
    id: ID!
    evt_tx_hash: String!
    evt_index: Int!
    evt_block_time: String!
    evt_block_number: Int!
    approved: String!
    owner: String!
    token_id: BigDecimal!
}
type approval_for_alls @entity {
    id: ID!
    evt_tx_hash: String!
    evt_index: Int!
    evt_block_time: String!
    evt_block_number: Int!
    approved: Boolean!
    operator: String!
    owner: String!
}
type mints @entity {
    id: ID!
    evt_tx_hash: String!
    evt_index: Int!
    evt_block_time: String!
    evt_block_number: Int!
    u_project_id: BigDecimal!
    u_to: String!
    u_token_id: BigDecimal!
}
type transfers @entity {
    id: ID!
    evt_tx_hash: String!
    evt_index: Int!
    evt_block_time: String!
    evt_block_number: Int!
    from: String!
    to: String!
    token_id: BigDecimal!
}
`)
	if err := os.WriteFile(filepath.Join(configFolder, "schema.graphql"), schemaGraphql, 0644); err != nil {
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	//FIXME
	substreamsYaml := []byte(`specVersion: 0.0.6
description: Substreams powered art-blocks
repository: https://github.com/streamingfast/substreams-generated-library
schema:
  file: ./schema.graphql

dataSources:
  - kind: substreams
    name: art_blocks_graph
    network: mainnet
    source:
      package:
        moduleName: graph_out
        file: substreams.spkg
    mapping:
      kind: substreams/graph-entities
      apiVersion: 0.0.5`)

	sgyaml := &yaml.Node{}
	yaml.Unmarshal(substreamsYaml, sgyaml)

	dataSources := getChild(sgyaml.Content[0], "dataSources")
	var found bool
	for _, ds := range dataSources.Content {
		// modify the yaml to contain the right substreams.spkg file name
		file := getChild(ds, "source", "package", "file")
		if file != nil {
			found = true
			file.SetString(spkgName)
		}
	}
	if !found {
		return conf, "", fmt.Errorf("invalid input subgraph.yaml: cannot find the dataSources[].source.package.file to point to the correct file")
	}

	f, err := os.Create(filepath.Join(configFolder, "subgraph.yaml"))
	if err != nil {
		return conf, "", err
	}
	defer f.Close()
	yaml.NewEncoder(f).Encode(sgyaml.Content[0])

	startScript := []byte(fmt.Sprintf(`#!/bin/bash
set -xeu

if [ ! -f /opt/subservices/data/setup-complete ]; then
    cd /opt/subservices/config
    npm install -g @graphprotocol/graph-cli
    graph create -g http://graphnode:8020 %s
    graph deploy %s subgraph.yaml --ipfs=http://ipfs:5001 --node=http://graphnode:8020 --version-label=%s
fi

touch /opt/subservices/data/setup-complete
sleep 999999

`, pkgName, pkgName, pkgVersion))
	if err := os.WriteFile(filepath.Join(configFolder, "start.sh"), startScript, 0755); err != nil {
		fmt.Println("")
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	return conf, motd, nil
}

func graphdeployServiceName(deploymentID string) string {
	return deploymentID + "-graphdeploy"
}

// getChild only follows the first object of a sequence, it does not thoroughly recurse all branches
func getChild(parent *yaml.Node, name ...string) *yaml.Node {
	var foundName bool
	for _, child := range parent.Content {
		if foundName {
			if len(name) == 1 {
				return child
			}

			if child.Kind == yaml.SequenceNode {
				return getChild(child.Content[0], name[1:]...)
			}
			return getChild(child, name[1:]...)
		}
		if child.Value == name[0] {
			foundName = true
		}
	}
	return nil
}
