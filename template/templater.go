// Copyright 2022 The kubegems.io Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package template

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/time"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

type Options struct {
	Config    *rest.Config
	IsUpgrade bool
	DebugOut  io.Writer // DebugOut is used to write debug information when templating
}

func Template(ctx context.Context, name, namespace string, chart *chart.Chart, values map[string]any, options Options) (*release.Release, error) {
	rlsopt := chartutil.ReleaseOptions{
		Name:      name,
		Namespace: namespace,
		IsInstall: !options.IsUpgrade,
	}
	var dc discovery.DiscoveryInterface
	if options.Config != nil {
		cs, err := kubernetes.NewForConfig(options.Config)
		if err != nil {
			return nil, err
		}
		dc = memory.NewMemCacheClient(cs)
	}
	caps := chartutil.DefaultCapabilities
	if dc != nil {
		kubeVersion, err := dc.ServerVersion()
		if err != nil {
			return nil, err
		}
		apiVersions, err := action.GetVersionSet(dc)
		if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
			return nil, fmt.Errorf("could not get apiVersions from Kubernetes: %w", err)
		}
		caps.APIVersions = apiVersions
		caps.KubeVersion = chartutil.KubeVersion{
			Version: kubeVersion.GitVersion,
			Major:   kubeVersion.Major,
			Minor:   kubeVersion.Minor,
		}
	}
	if chartkubeversion := chart.Metadata.KubeVersion; chartkubeversion != "" {
		if !chartutil.IsCompatibleRange(chartkubeversion, caps.KubeVersion.String()) {
			return nil, fmt.Errorf("chart requires kubeVersion: %s which is incompatible with Kubernetes %s", chartkubeversion, caps.KubeVersion.String())
		}
	}
	if err := chartutil.ProcessDependenciesWithMerge(chart, values); err != nil {
		return nil, err
	}
	// render the chart
	valuesToRender, err := chartutil.ToRenderValues(chart, values, rlsopt, caps)
	if err != nil {
		return nil, err
	}
	eng := engine.Engine{}
	if restconfig := options.Config; restconfig != nil {
		eng = engine.New(restconfig)
	}
	eng.EnableDNS = true
	renderdFiles, err := eng.Render(chart, valuesToRender)
	if err != nil {
		return nil, err
	}
	// parse results
	ts := time.Now()
	rls := &release.Release{
		Name:      name,
		Namespace: namespace,
		Chart:     chart,
		Config:    values,
		Info: &release.Info{
			FirstDeployed: ts,
			LastDeployed:  ts,
			Status:        release.StatusUnknown,
		},
		Version: 1,
	}
	// parse notes
	const notesFileSuffix = "NOTES.txt"
	var notesBuffer bytes.Buffer
	for filename, filecontent := range renderdFiles {
		if !strings.HasSuffix(filename, notesFileSuffix) {
			continue
		}
		if filename == path.Join(chart.Name(), "templates", notesFileSuffix) {
			// If buffer contains data, add newline before adding more
			if notesBuffer.Len() > 0 {
				notesBuffer.WriteString("\n")
			}
			notesBuffer.WriteString(filecontent)
		}
		delete(renderdFiles, filename)
	}
	rls.Info.Notes = notesBuffer.String()
	hs, manifests, err := releaseutil.SortManifests(renderdFiles, caps.APIVersions, releaseutil.InstallOrder)
	if err != nil {
		if out := options.DebugOut; out != nil {
			for file, val := range renderdFiles {
				fmt.Fprintf(out, "---\n# Source: %s\n%s\n", file, val)
			}
			fmt.Fprintln(out, "---")
		}
		return nil, err
	}
	rls.Hooks = hs

	out := bytes.NewBuffer(nil)
	for _, crd := range chart.CRDObjects() {
		fmt.Fprintf(out, "---\n# Source: %s\n%s\n", crd.Name, string(crd.File.Data[:]))
	}
	for _, m := range manifests {
		fmt.Fprintf(out, "---\n# Source: %s\n%s\n", m.Name, m.Content)
	}
	rls.Manifest = out.String()
	return rls, nil
}

const chartFileName = "Chart.yaml"

func LoadChart(name, version, path string) (*chart.Chart, error) {
	if version == "" {
		version = "0.0.0"
	}
	absdir, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	absdir += string(filepath.Separator)
	containsChartFile := false
	files := []*loader.BufferedFile{}
	walk := func(name string, fi os.FileInfo, err error) error {
		relfilename := strings.TrimPrefix(name, absdir)
		if relfilename == "" {
			return nil
		}
		if relfilename == chartFileName {
			containsChartFile = true
		}
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files = append(files, &loader.BufferedFile{Name: relfilename, Data: data})
		return nil
	}
	if err = filepath.Walk(absdir, walk); err != nil {
		return nil, err
	}
	if !containsChartFile {
		chartfile := chart.Metadata{
			APIVersion: chart.APIVersionV2,
			Name:       name,
			Version:    version,
		}
		chartfilecontent, err := yaml.Marshal(chartfile)
		if err != nil {
			return nil, err
		}
		files = append(files, &loader.BufferedFile{Name: chartFileName, Data: chartfilecontent})
	}
	return loader.LoadFiles(files)
}
