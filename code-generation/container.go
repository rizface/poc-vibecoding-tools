package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	dockerClient "github.com/moby/moby/client"
)

var dockerApiClient *dockerClient.Client

type createContainerConfig struct {
	name  string
	binds []string
}

type createContainerResp struct {
	containerId   string
	containerPort string
	host          string
}

func getHost() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	rand.Read(b)
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}

	return string(b)
}

func createContainer(ctx context.Context, cfg createContainerConfig) (createContainerResp, error) {
	port := network.MustParsePort("80/tcp")
	host := fmt.Sprintf("%s.traefik.me", getHost())
	hostLabel := fmt.Sprintf("Host(`%s`)", host)

	resp, err := dockerApiClient.ContainerCreate(ctx, dockerClient.ContainerCreateOptions{
		Name: cfg.name,
		Config: &container.Config{
			Image:        "nginx:latest",
			ExposedPorts: network.PortSet{port: struct{}{}},
			Labels: map[string]string{
				"traefik.enable": "true",
				fmt.Sprintf("traefik.http.routers.%s.rule", cfg.name):        hostLabel,
				fmt.Sprintf("traefik.http.routers.%s.entrypoints", cfg.name): "web",
			},
		},
		HostConfig: &container.HostConfig{
			PortBindings: network.PortMap{
				port: []network.PortBinding{
					{HostIP: netip.IPv4Unspecified(), HostPort: ""},
				},
			},
			Binds: cfg.binds,
		},
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				"code_generation": {},
			},
		},
	})
	if err != nil {
		return createContainerResp{}, err
	}

	if _, err = dockerApiClient.ContainerStart(ctx, resp.ID, dockerClient.ContainerStartOptions{}); err != nil {
		_, errRemoveContainer := dockerApiClient.ContainerRemove(ctx, resp.ID, dockerClient.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   true,
			Force:         true,
		})
		if errRemoveContainer != nil {
			fmt.Printf("[ERROR]: failed remove container %s; %s \n", resp.ID, errRemoveContainer.Error())
		}

		return createContainerResp{}, err
	}

	info, err := dockerApiClient.ContainerInspect(ctx, resp.ID, dockerClient.ContainerInspectOptions{})
	if err != nil {
		_, errRemoveContainer := dockerApiClient.ContainerRemove(ctx, resp.ID, dockerClient.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   true,
			Force:         true,
		})
		if errRemoveContainer != nil {
			fmt.Printf("[ERROR]: failed remove container %s; %s \n", resp.ID, errRemoveContainer.Error())
		}

		return createContainerResp{}, err
	}

	fmt.Println(info.Container.NetworkSettings.Ports)

	return createContainerResp{
		containerId:   resp.ID,
		containerPort: info.Container.NetworkSettings.Ports[port][0].HostPort,
		host:          host,
	}, nil
}
