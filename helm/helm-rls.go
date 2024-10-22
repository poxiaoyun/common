package helm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"golang.org/x/exp/slices"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type ReleaseManager struct {
	Config *rest.Config
}

func NewHelmConfig(ctx context.Context, namespace string, cfg *rest.Config) (*action.Configuration, error) {
	baselog := klog.FromContext(ctx)
	logfunc := func(format string, v ...interface{}) {
		baselog.Info(fmt.Sprintf(format, v...))
	}

	cligetter := genericclioptions.NewConfigFlags(true)
	cligetter.WrapConfigFn = func(*rest.Config) *rest.Config {
		return cfg
	}

	config := &action.Configuration{}
	config.Init(cligetter, namespace, "", logfunc) // release storage namespace
	if kc, ok := config.KubeClient.(*kube.Client); ok {
		kc.Namespace = namespace // install to namespace
	}
	return config, nil
}

func TemplateChartFromPath(ctx context.Context, rlsname, namespace string, chartPath string, values map[string]any) (*release.Release, error) {
	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("load chart: %w", err)
	}
	return TemplateChart(ctx, rlsname, namespace, chart, values)
}

func TemplateChart(ctx context.Context, rlsname, namespace string, chart *chart.Chart, values map[string]any) (*release.Release, error) {
	install := action.NewInstall(&action.Configuration{})
	install.ReleaseName, install.Namespace = rlsname, namespace
	install.DryRun, install.DisableHooks, install.ClientOnly = true, true, true
	return install.RunWithContext(ctx, chart, values)
}

type ApplyChartOptions struct {
	FileOverrides  map[string][]byte
	Values         map[string]any
	Auth           AuthOptions
	PostRenderFunc PostRenderFunc
}

func ApplyChartFromImage(ctx context.Context, cfg *rest.Config, image, version string,
	rlsname, rlsnamespace string, options ApplyChartOptions,
) (*release.Release, bool, error) {
	if !strings.HasPrefix(image, "oci://") {
		image = "oci://" + image
	}
	return ApplyChartFromRepo(ctx, cfg, image, "", version, rlsname, rlsnamespace, options)
}

func ApplyChartFromRepo(ctx context.Context, cfg *rest.Config,
	repository, chart, version string, rlsname, rlsnamespace string, options ApplyChartOptions,
) (*release.Release, bool, error) {
	chartpath, err := DownloadWithCache(ctx, repository, chart, version, DownloadOptions{Auth: options.Auth})
	if err != nil {
		return nil, false, err
	}
	return ApplyChartFromFile(ctx, cfg, chartpath, rlsname, rlsnamespace, options)
}

// ApplyChartFromFile applies a chart from a file and returns the release and whether it was created or upgraded.
func ApplyChartFromFile(ctx context.Context,
	cfg *rest.Config, chartPath string, rlsname, rlsnamespace string, options ApplyChartOptions,
) (*release.Release, bool, error) {
	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, false, fmt.Errorf("load chart: %w", err)
	}
	return ApplyChart(ctx, cfg, chart, rlsname, rlsnamespace, options)
}

func ApplyChartFromArchive(ctx context.Context,
	cfg *rest.Config, chartdata io.Reader, rlsname, rlsnamespace string, options ApplyChartOptions,
) (*release.Release, bool, error) {
	chart, err := loader.LoadArchive(chartdata)
	if err != nil {
		return nil, false, fmt.Errorf("load chart: %w", err)
	}
	return ApplyChart(ctx, cfg, chart, rlsname, rlsnamespace, options)
}

func ApplyChart(ctx context.Context,
	cfg *rest.Config, chart *chart.Chart,
	rlsname, rlsnamespace string,
	options ApplyChartOptions,
) (*release.Release, bool, error) {
	log := klog.FromContext(ctx).WithValues("name", rlsname, "namespace", rlsnamespace)
	// apply files
	if len(options.FileOverrides) > 0 {
		newchart, err := LoadChartWithFileOverride(chart, options.FileOverrides)
		if err != nil {
			return nil, false, fmt.Errorf("override files: %w", err)
		}
		chart = newchart
	}
	values := options.Values
	if rlsname == "" {
		rlsname = chart.Name()
	}
	helmcfg, err := NewHelmConfig(ctx, rlsnamespace, cfg)
	if err != nil {
		return nil, false, err
	}
	existRelease, err := action.NewGet(helmcfg).Run(rlsname)
	if err != nil {
		if !errors.Is(err, driver.ErrReleaseNotFound) {
			return nil, false, err
		}
		// not install, install it now
		log.Info("installing", "values", values)
		install := action.NewInstall(helmcfg)
		install.ReleaseName, install.Namespace = rlsname, rlsnamespace
		install.CreateNamespace = true
		install.PostRenderer = options.PostRenderFunc
		install.EnableDNS = true
		rls, err := install.RunWithContext(ctx, chart, values)
		if err != nil {
			return nil, false, err
		}
		return rls, true, nil
	}
	// check should upgrade
	if existRelease.Info.Status == release.StatusDeployed &&
		existRelease.Chart.Metadata.Version == chart.Metadata.Version &&
		equalMapValues(existRelease.Config, values) {
		log.Info("already uptodate", "values", values)
		return existRelease, false, nil
	}
	log.Info("upgrading", "old", existRelease.Config, "new", values)
	client := action.NewUpgrade(helmcfg)
	client.Namespace = rlsnamespace
	client.ResetValues = true
	client.PostRenderer = options.PostRenderFunc
	client.EnableDNS = true
	client.MaxHistory = 5
	rls, err := client.RunWithContext(ctx, rlsname, chart, values)
	if err != nil {
		return nil, false, err
	}
	return rls, true, nil
}

func LoadChartWithFileOverride(cht *chart.Chart, files map[string][]byte) (*chart.Chart, error) {
	// override files
	bufferdFiles := []*loader.BufferedFile{}
	for _, f := range cht.Raw {
		if content, ok := files[f.Name]; ok {
			bufferdFiles = append(bufferdFiles, &loader.BufferedFile{Name: f.Name, Data: content})
			delete(files, f.Name)
		} else {
			bufferdFiles = append(bufferdFiles, &loader.BufferedFile{Name: f.Name, Data: f.Data})
		}
	}
	for name, content := range files {
		bufferdFiles = append(bufferdFiles, &loader.BufferedFile{Name: name, Data: content})
	}
	return loader.LoadFiles(bufferdFiles)
}

func removeHistories(_ context.Context, storage *storage.Storage, name string, max int) error {
	rlss, err := storage.History(name)
	if err != nil {
		return err
	}
	if max <= 0 {
		max = 1
	}

	// newest to old
	slices.SortFunc(rlss, func(a, b *release.Release) int {
		if a.Version > b.Version {
			return 1
		} else if a.Version < b.Version {
			return -1
		} else {
			return 0
		}
	})

	var lastDeployed *release.Release
	toDelete := []*release.Release{}
	for _, rls := range rlss {
		if rls.Info.Status == release.StatusDeployed && lastDeployed == nil {
			lastDeployed = rls
			continue
		}
		// once we have enough releases to delete to reach the max, stop
		// all - deleted = max
		if len(rlss)-len(toDelete) == max {
			break
		}
		toDelete = append(toDelete, rls)
	}
	for _, todel := range toDelete {
		if _, err := storage.Delete(todel.Name, todel.Version); err != nil {
			return err
		}
	}
	return nil
}

func equalMapValues(a, b map[string]interface{}) bool {
	return (len(a) == 0 && len(b) == 0) || reflect.DeepEqual(a, b)
}

func RemoveChart(ctx context.Context, cfg *rest.Config, rlsname, namespace string) (*release.Release, error) {
	log := klog.FromContext(ctx)
	helmcfg, err := NewHelmConfig(ctx, namespace, cfg)
	if err != nil {
		return nil, err
	}
	exist, err := action.NewGet(helmcfg).Run(rlsname)
	if err != nil {
		if !errors.Is(err, driver.ErrReleaseNotFound) {
			return nil, err
		}
		return nil, nil
	}
	log.Info("uninstalling")
	uninstall := action.NewUninstall(helmcfg)
	uninstalledRelease, err := uninstall.Run(exist.Name)
	if err != nil {
		return nil, err
	}
	return uninstalledRelease.Release, nil
}

type PostRenderFunc func(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error)

func (f PostRenderFunc) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	return f(renderedManifests)
}
