package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const proxyImage = "basecamp/kamal-proxy"

const (
	DefaultHTTPPort    = 80
	DefaultHTTPSPort   = 443
	DefaultMetricsPort = 1318
)

type ProxySettings struct {
	HTTPPort    int `json:"hp"`
	HTTPSPort   int `json:"hsp"`
	MetricsPort int `json:"mp"`
}

func UnmarshalProxySettings(s string) (ProxySettings, error) {
	var settings ProxySettings
	err := json.Unmarshal([]byte(s), &settings)
	return settings, err
}

func (s ProxySettings) Marshal() string {
	b, _ := json.Marshal(s)
	return string(b)
}

type DeployOptions struct {
	AppName string
	Target  string
	Host    string
	TLS     bool
}

type Proxy struct {
	namespace *Namespace
	Settings  *ProxySettings
}

func NewProxy(ns *Namespace) *Proxy {
	return &Proxy{namespace: ns}
}

func (p *Proxy) Boot(ctx context.Context, settings ProxySettings) error {
	if settings.HTTPPort == 0 {
		settings.HTTPPort = DefaultHTTPPort
	}
	if settings.HTTPSPort == 0 {
		settings.HTTPSPort = DefaultHTTPSPort
	}
	if settings.MetricsPort == 0 {
		settings.MetricsPort = DefaultMetricsPort
	}

	reader, err := p.namespace.client.ImagePull(ctx, proxyImage, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling proxy image: %w", err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)

	containerName := p.namespace.name + "-proxy"
	metricsPortTCP := nat.Port(fmt.Sprintf("%d/tcp", settings.MetricsPort))

	resp, err := p.namespace.client.ContainerCreate(ctx,
		&container.Config{
			Image: proxyImage,
			Cmd:   []string{"kamal-proxy", "run", "--metrics-port", fmt.Sprintf("%d", settings.MetricsPort)},
			Labels: map[string]string{
				"amar": settings.Marshal(),
			},
			ExposedPorts: nat.PortSet{
				"80/tcp":       struct{}{},
				"443/tcp":      struct{}{},
				metricsPortTCP: struct{}{},
			},
		},
		&container.HostConfig{
			PortBindings: nat.PortMap{
				"80/tcp":       []nat.PortBinding{{HostPort: fmt.Sprintf("%d", settings.HTTPPort)}},
				"443/tcp":      []nat.PortBinding{{HostPort: fmt.Sprintf("%d", settings.HTTPSPort)}},
				metricsPortTCP: []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: fmt.Sprintf("%d", settings.MetricsPort)}},
			},
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyAlways},
			LogConfig:     ContainerLogConfig(),
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: p.namespace.name + "-proxy",
					Target: "/home/kamal-proxy/.config/kamal-proxy",
				},
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				p.namespace.name: {},
			},
		},
		nil,
		containerName,
	)
	if err != nil {
		return fmt.Errorf("creating proxy container: %w", err)
	}

	if err := p.namespace.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting proxy container: %w", err)
	}

	p.Settings = &settings
	return nil
}

func (p *Proxy) Destroy(ctx context.Context, destroyVolumes bool) error {
	containerName := p.namespace.name + "-proxy"

	if err := p.namespace.client.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true}); err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("removing proxy: %w", err)
		}
	}

	if destroyVolumes {
		volumeName := p.namespace.name + "-proxy"
		if err := p.namespace.client.VolumeRemove(ctx, volumeName, true); err != nil {
			if !errdefs.IsNotFound(err) {
				return fmt.Errorf("removing proxy volume: %w", err)
			}
		}
	}

	p.Settings = nil
	return nil
}

func (p *Proxy) Exec(ctx context.Context, cmd []string) error {
	_, err := p.ExecOutput(ctx, cmd)
	return err
}

func (p *Proxy) Deploy(ctx context.Context, opts DeployOptions) error {
	args := []string{"kamal-proxy", "deploy", opts.AppName, "--target", opts.Target}

	if opts.Host != "" {
		args = append(args, "--host", opts.Host)
	}

	if opts.TLS {
		args = append(args, "--tls")
	}

	return p.Exec(ctx, args)
}

func (p *Proxy) ExecOutput(ctx context.Context, cmd []string) (string, error) {
	containerName := p.namespace.name + "-proxy"
	execResp, err := p.namespace.client.ContainerExecCreate(ctx, containerName, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("creating exec: %w", err)
	}

	resp, err := p.namespace.client.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("attaching exec: %w", err)
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return "", fmt.Errorf("reading exec output: %w", err)
	}

	inspect, err := p.namespace.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return "", fmt.Errorf("inspecting exec: %w", err)
	}
	if inspect.ExitCode != 0 {
		return stdout.String() + stderr.String(), fmt.Errorf("exec failed with exit code %d", inspect.ExitCode)
	}

	return stdout.String(), nil
}
