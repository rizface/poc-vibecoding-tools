package container

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	dockerClient "github.com/moby/moby/client"
)

type DockerClient struct {
	client *dockerClient.Client
}

func NewDockerClient(client *dockerClient.Client) *DockerClient {
	return &DockerClient{
		client: client,
	}
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

func (dc *DockerClient) CreateNginxSandboxContainer(ctx context.Context, input CreateNginxSandboxContainerInput) (CreateNginxSandboxContainerOutput, error) {
	port := network.MustParsePort("80/tcp")
	host := fmt.Sprintf("%s.traefik.me", getHost())
	hostLabel := fmt.Sprintf("Host(`%s`)", host)

	resp, err := dc.client.ContainerCreate(ctx, dockerClient.ContainerCreateOptions{
		Name: input.Name,
		Config: &container.Config{
			Image:        "nginx:latest",
			ExposedPorts: network.PortSet{port: struct{}{}},
			Labels: map[string]string{
				"traefik.enable": "true",
				fmt.Sprintf("traefik.http.routers.%s.rule", input.Name):        hostLabel,
				fmt.Sprintf("traefik.http.routers.%s.entrypoints", input.Name): "web",
			},
		},
		HostConfig: &container.HostConfig{
			PortBindings: network.PortMap{
				port: []network.PortBinding{
					{HostIP: netip.IPv4Unspecified(), HostPort: ""},
				},
			},
			Binds: input.Binds,
		},
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				"code_generation": {},
			},
		},
	})
	if err != nil {
		return CreateNginxSandboxContainerOutput{}, err
	}

	if _, err = dc.client.ContainerStart(ctx, resp.ID, dockerClient.ContainerStartOptions{}); err != nil {
		_, errRemoveContainer := dc.client.ContainerRemove(ctx, resp.ID, dockerClient.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   true,
			Force:         true,
		})
		if errRemoveContainer != nil {
			fmt.Printf("[ERROR]: failed remove container %s; %s \n", resp.ID, errRemoveContainer.Error())
		}

		return CreateNginxSandboxContainerOutput{}, err
	}

	info, err := dc.client.ContainerInspect(ctx, resp.ID, dockerClient.ContainerInspectOptions{})
	if err != nil {
		_, errRemoveContainer := dc.client.ContainerRemove(ctx, resp.ID, dockerClient.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   true,
			Force:         true,
		})
		if errRemoveContainer != nil {
			fmt.Printf("[ERROR]: failed remove container %s; %s \n", resp.ID, errRemoveContainer.Error())
		}

		return CreateNginxSandboxContainerOutput{}, err
	}

	fmt.Println(info.Container.NetworkSettings.Ports)

	return CreateNginxSandboxContainerOutput{
		ID:   resp.ID,
		Port: info.Container.NetworkSettings.Ports[port][0].HostPort,
		Host: host,
	}, nil
}

func (dc *DockerClient) RemoveContainer(ctx context.Context, input RemoveContainerInput) error {
	_, err := dc.client.ContainerRemove(ctx, input.ID, dockerClient.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})

	return err
}
