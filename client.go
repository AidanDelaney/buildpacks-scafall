package pack

import (
	"context"
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	dockerClient "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/logging"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_image_fetcher.go github.com/buildpacks/pack ImageFetcher

// ImageFetcher is an interface representing the ability to fetch images.
type ImageFetcher interface {
	// Fetch fetches an image by resolving it both remotely and locally depending on provided parameters.
	// If daemon is true, it will look return a `local.Image`. Pull, applicable only when daemon is true, will
	// attempt to pull a remote image first.
	Fetch(ctx context.Context, name string, daemon, pull bool) (imgutil.Image, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_downloader.go github.com/buildpacks/pack Downloader

// Downloader is an interface for collecting both remote and local assets
type Downloader interface {

	// Download collects both local and remote assets so that they are all
	// readable.
	Download(ctx context.Context, pathOrURI string) (blob.Blob, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_image_factory.go github.com/buildpacks/pack ImageFactory

// ImageFactory is an interface representing the ability to create a new OCI image.
type ImageFactory interface {

	// NewImage initialize of an image object with required settings so that it
	// can be saved
	NewImage(repoName string, local bool) (imgutil.Image, error)
}

// Client defines the parameters needed to run a build, and produce an image.
type Client struct {
	// Logger used for all client output
	logger       logging.Logger

	// Utility to pull images either locally or remotely.
	imageFetcher ImageFetcher

	// used to gather buildpacks from both remote urls, or local sources
	downloader   Downloader

	// object responsible for executing all lifecycle phases
	lifecycle    Lifecycle

	// client used to interact with local and remote registries
	docker       dockerClient.CommonAPIClient

	imageFactory ImageFactory

	// enable experimental features
	experimental bool
}

// functions that mutate some setting on the client.
type ClientOption func(c *Client)

// WithLogger supply your own logger.
func WithLogger(l logging.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// WithImageFactory supply your own image factory.
func WithImageFactory(f ImageFactory) ClientOption {
	return func(c *Client) {
		c.imageFactory = f
	}
}

// WithFetcher supply your own fetcher.
func WithFetcher(f ImageFetcher) ClientOption {
	return func(c *Client) {
		c.imageFetcher = f
	}
}

// WithDownloader supply your own downloader.
func WithDownloader(d Downloader) ClientOption {
	return func(c *Client) {
		c.downloader = d
	}
}

// WithCacheDir supply your own cache directory.
//
// Deprecated: use WithDownloader instead.
func WithCacheDir(path string) ClientOption {
	return func(c *Client) {
		c.downloader = blob.NewDownloader(c.logger, path)
	}
}

// WithDockerClient supply your own docker client.
func WithDockerClient(docker dockerClient.CommonAPIClient) ClientOption {
	return func(c *Client) {
		c.docker = docker
	}
}

// WithExperimental sets whether experimental features should be enabled
func WithExperimental(experimental bool) ClientOption {
	return func(c *Client) {
		c.experimental = experimental
	}
}

// NewClient and returns a Client with the specified options.
func NewClient(opts ...ClientOption) (*Client, error) {
	var client Client

	for _, opt := range opts {
		opt(&client)
	}

	if client.logger == nil {
		client.logger = logging.New(os.Stderr)
	}

	if client.docker == nil {
		var err error
		client.docker, err = dockerClient.NewClientWithOpts(
			dockerClient.FromEnv,
			dockerClient.WithVersion("1.38"),
		)
		if err != nil {
			return nil, errors.Wrap(err, "creating docker client")
		}
	}

	if client.downloader == nil {
		packHome, err := config.PackHome()
		if err != nil {
			return nil, errors.Wrap(err, "getting pack home")
		}
		client.downloader = blob.NewDownloader(client.logger, filepath.Join(packHome, "download-cache"))
	}

	if client.imageFetcher == nil {
		client.imageFetcher = image.NewFetcher(client.logger, client.docker)
	}

	if client.imageFactory == nil {
		client.imageFactory = image.NewFactory(client.docker, authn.DefaultKeychain)
	}

	client.lifecycle = build.NewLifecycle(client.docker, client.logger)

	return &client, nil
}
