package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/compose/types"
	pbgraph "github.com/streamingfast/substreams-sink-subgraph/pb/sf/substreams/sink/subgraph/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

func (e *DockerEngine) newGraphDeploy(deploymentID string, ipfsService string, graphnodeService string, pkg *pbsubstreams.Package, pbsvc *pbgraph.Service) (conf types.ServiceConfig, motd string, err error) {

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

	schemaGraphql := []byte(pbsvc.Schema)
	if err := os.WriteFile(filepath.Join(configFolder, "schema.graphql"), schemaGraphql, 0644); err != nil {
		return conf, motd, fmt.Errorf("writing file: %w", err)
	}

	subgraphYaml := []byte(pbsvc.SubgraphYaml)
	sgyaml := &yaml.Node{}
	yaml.Unmarshal(subgraphYaml, sgyaml)

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
