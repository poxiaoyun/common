package helm

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"syscall"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/klog/v2"
)

const (
	DefaultFileMode      os.FileMode = 0o644
	DefaultDirectoryMode os.FileMode = 0o755
)

type DownloadOptions struct {
	Cachedir string
	Auth     AuthOptions
}

type AuthOptions struct {
	Username string
	Password string
	Token    string
}

// Download helm chart into cachedir saved as {name}-{version}.tgz file.
// the repo can be oci format. eg: oci://registry.example.com/library/my-chart
// or http/https format, eg: http://example.com/charts
func DownloadWithCache(ctx context.Context, repo, name, version string, options DownloadOptions) (string, error) {
	u, err := url.Parse(repo)
	if err != nil {
		return "", err
	}
	cachedir := options.Cachedir
	if cachedir == "" {
		cachedir = filepath.Join(os.TempDir(), "helm-cache")
	}
	perhostdir := filepath.Join(cachedir, u.Host, u.Path)
	if registry.IsOCI(repo) {
		// oci repo contains chart name
		// example: eaxample.com/library/my-chart
		perhostdir, name = path.Split(perhostdir)
	}
	cachefilename := filepath.Join(perhostdir, name+"-"+version+".tgz")
	if version == "" {
		// use 0.0.0 as a placeholder
		cachefilename = filepath.Join(perhostdir, name+"-"+"0.0.0"+".tgz")
	}
	// check exists
	if _, err := os.Stat(cachefilename); err == nil {
		return cachefilename, nil
	}
	chartPath, err := DownloadChart(ctx, repo, name, version, perhostdir, options)
	if err != nil {
		return "", err
	}
	if chartPath == cachefilename {
		return chartPath, nil
	}
	os.MkdirAll(filepath.Dir(cachefilename), DefaultDirectoryMode)
	return cachefilename, RenameFile(chartPath, cachefilename)
}

func RenameFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	terr, ok := err.(*os.LinkError)
	if !ok {
		return err
	}
	if terr.Err != syscall.EXDEV {
		return err
	}
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// nolint: nosnakecase
	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// name is the name of the chart
// repo is the url of the chart repository,eg: http://charts.example.com
// if repopath is not empty,download it from repo and set chartNameOrPath to repo/repopath.
// LoadChart loads the chart from the repository
func UpdateChart(ctx context.Context, chartPath string) (*chart.Chart, error) {
	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, err
	}
	// dependencies update
	if err := action.CheckDependencies(chart, chart.Metadata.Dependencies); err != nil {
		settings := cli.New()
		man := &downloader.Manager{
			Out:              log.Default().Writer(),
			ChartPath:        chartPath,
			SkipUpdate:       false,
			Getters:          getter.All(settings),
			RepositoryConfig: settings.RepositoryConfig,
			RepositoryCache:  settings.RepositoryCache,
			Debug:            settings.Debug,
		}
		if err := man.Update(); err != nil {
			return nil, err
		}
		chart, err = loader.Load(chartPath)
		if err != nil {
			return nil, err
		}
	}
	return chart, nil
}

func DownloadChart(ctx context.Context, repourl, name, version string, into string, options DownloadOptions) (string, error) {
	logwriter := &LogWriter{Logger: klog.FromContext(ctx)}
	settings := cli.New()
	dl := downloader.ChartDownloader{
		Out:              logwriter,
		Getters:          getter.All(settings),
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
		Options: []getter.Option{
			getter.WithInsecureSkipVerifyTLS(true),
			getter.WithPlainHTTP(true),
			getter.WithBasicAuth(options.Auth.Username, options.Auth.Password),
		},
	}
	if registry.IsOCI(repourl) {
		registryClient, err := registry.NewClient(
			registry.ClientOptDebug(settings.Debug),
			registry.ClientOptWriter(logwriter),
			registry.ClientOptCredentialsFile(settings.RegistryConfig),
		)
		if err != nil {
			return "", err
		}
		dl.RegistryClient = registryClient
		dl.Options = append(dl.Options, getter.WithRegistryClient(registryClient))
	} else {
		chartURL, err := repo.FindChartInAuthAndTLSAndPassRepoURL(
			repourl,
			options.Auth.Username, options.Auth.Password, // username password
			name, version,
			"", "", "", // cert key ca
			true, false, // insecureTLS passCredentialsAll
			dl.Getters)
		if err != nil {
			return "", err
		}
		repourl = chartURL
	}
	if err := os.MkdirAll(into, DefaultDirectoryMode); err != nil {
		return "", err
	}
	filename, _, err := dl.DownloadTo(repourl, version, into)
	if err != nil {
		return filename, fmt.Errorf("failed to download %s: %w", name, err)
	}
	return filename, nil
}

var _ io.Writer = &LogWriter{}

type LogWriter struct {
	Logger klog.Logger
}

// Write implements io.Writer.
func (l *LogWriter) Write(p []byte) (n int, err error) {
	l.Logger.Info(string(p))
	return len(p), nil
}
