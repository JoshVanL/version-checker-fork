package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/jetstack/version-checker/pkg/api"
	"github.com/jetstack/version-checker/pkg/client/acr"
	"github.com/jetstack/version-checker/pkg/client/docker"
	"github.com/jetstack/version-checker/pkg/client/ecr"
	"github.com/jetstack/version-checker/pkg/client/gcr"
	"github.com/jetstack/version-checker/pkg/client/quay"
	"github.com/jetstack/version-checker/pkg/client/selfhosted"
)

// ImageClient represents a image registry client that can list available tags
// for image URLs.
type ImageClient interface {
	// IsHost will return true if this client is appropriate for the given
	// host.
	IsHost(host string) bool

	// RepoImage will return the registries repository and image, from a given
	// URL path.
	RepoImageFromPath(path string) (string, string)

	// Tags will return the available tags for the given host, repo, and image
	// using that client.
	Tags(ctx context.Context, host, repo, image string) ([]api.ImageTag, error)
}

// Client is a container image registry client to list tags of given image
// URLs.
type Client struct {
	clients        []ImageClient
	fallbackClient ImageClient
}

// Options used to configure client authentication.
type Options struct {
	ACR        acr.Options
	ECR        ecr.Options
	GCR        gcr.Options
	Docker     docker.Options
	Quay       quay.Options
	Selfhosted selfhosted.Options
}

func New(ctx context.Context, log *logrus.Entry, opts Options) (*Client, error) {
	acrClient, err := acr.New(opts.ACR)
	if err != nil {
		return nil, fmt.Errorf("failed to create acr client: %s", err)
	}
	dockerClient, err := docker.New(ctx, opts.Docker)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %s", err)
	}

	selfhostedClient, err := selfhosted.New(ctx, log, opts.Selfhosted)
	if err != nil {
		return nil, fmt.Errorf("failed to create selfhosted client: %s", err)
	}

	fallbackClient, err := selfhosted.New(ctx, log, selfhosted.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create fallback client: %s", err)
	}

	return &Client{
		clients: []ImageClient{
			acrClient,
			ecr.New(opts.ECR),
			dockerClient,
			gcr.New(opts.GCR),
			quay.New(opts.Quay),
			selfhostedClient,
		},
		fallbackClient: fallbackClient,
	}, nil
}

// Tags returns the full list of image tags available, for a given image URL.
func (c *Client) Tags(ctx context.Context, imageURL string) ([]api.ImageTag, error) {
	client, host, path := c.fromImageURL(imageURL)
	repo, image := client.RepoImageFromPath(path)
	return client.Tags(ctx, host, repo, image)
}

// fromImageURL will return the appropriate registry client for a given
// image URL, and the host + path to search
func (c *Client) fromImageURL(imageURL string) (ImageClient, string, string) {
	var host, path string

	if strings.Contains(imageURL, ".") || strings.Contains(imageURL, ":") {
		split := strings.SplitN(imageURL, "/", 2)
		if len(split) < 2 {
			path = imageURL
		} else {
			host, path = split[0], split[1]
		}
	} else {
		path = imageURL
	}

	for _, client := range c.clients {
		if client.IsHost(host) {
			return client, host, path
		}
	}

	// fall back to docker with no path split
	return c.fallbackClient, "", imageURL
}
