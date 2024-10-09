package helm

import (
	"context"
	"testing"
)

func TestDownloadChart(t *testing.T) {
	type args struct {
		in0     context.Context
		repourl string
		name    string
		version string
		into    string
		options DownloadOptions
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "download-chart",
			args: args{
				in0:     context.Background(),
				repourl: "https://charts.bitnami.com/bitnami",
				name:    "postgresql",
				version: "12.2.8",
				into:    "tmp",
				options: DownloadOptions{},
			},
			want:    "tmp/postgresql-12.2.8.tgz",
			wantErr: false,
		},
		{
			name: "download-oci",
			args: args{
				in0:     context.Background(),
				repourl: "oci://registry-1.docker.io/bitnamicharts/postgresql",
				name:    "postgresql",
				version: "15.5.0",
				into:    "tmp",
				options: DownloadOptions{},
			},
			want:    "tmp/postgresql-15.5.0.tgz",
			wantErr: false,
		},
		{
			name: "download-oci-no-version",
			args: args{
				in0:     context.Background(),
				repourl: "oci://registry-1.docker.io/bitnamicharts/postgresql",
				name:    "postgresql",
				into:    "tmp",
				options: DownloadOptions{},
			},
			want:    "tmp/postgresql-15.5.0.tgz",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DownloadChart(tt.args.in0, tt.args.repourl, tt.args.name, tt.args.version, tt.args.into, tt.args.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("DownloadChart() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DownloadChart() = %v, want %v", got, tt.want)
			}
		})
	}
}
