package container_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"xiaoshiai.cn/common/testkit/container"
)

func TestMain(m *testing.M) {
	if os.Getenv("CONTAINER_RUNTIME_HELPER") != "" {
		runRuntimeHelper()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestCommandRuntimeOperatesResources(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("get test executable: %v", err)
	}
	logFile := t.TempDir() + "/commands.log"
	t.Setenv("CONTAINER_RUNTIME_HELPER", "1")
	t.Setenv("CONTAINER_RUNTIME_LOG", logFile)
	runtime := container.NewCommandRuntime("example", executable)

	network, err := runtime.CreateNetwork(t.Context(), container.NetworkSpec{Name: "cluster"})
	if err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}
	volume, err := runtime.CreateVolume(t.Context(), container.VolumeSpec{Name: "data"})
	if err != nil {
		t.Fatalf("CreateVolume() error = %v", err)
	}
	target, err := runtime.CreateContainer(t.Context(), container.ContainerSpec{
		Name:    "database",
		Image:   "example:1",
		Command: []string{"serve"},
		Environment: map[string]string{
			"USER":     "tester",
			"PASSWORD": "secret",
		},
		Networks: []container.NetworkAttachment{
			{Network: network},
		},
		Ports: []container.PortMapping{
			{
				ContainerPort: container.Port{
					Number:   1234,
					Protocol: container.ProtocolTCP,
				},
				HostAddress: "127.0.0.1",
			},
		},
		Volumes: []container.VolumeMount{
			{Volume: volume, Target: "/data"},
		},
		Binds: []container.BindMount{
			{Source: "/fixtures", Target: "/fixtures", ReadOnly: true},
		},
	})
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}
	if network != "network-id" || volume != "volume-id" || target != "container-id" {
		t.Fatalf("resource handles = (%q, %q, %q)", network, volume, target)
	}

	info, err := runtime.InspectContainer(t.Context(), target)
	if err != nil {
		t.Fatalf("InspectContainer() error = %v", err)
	}
	if info.State != container.ContainerStateRunning || len(info.Ports) != 1 || len(info.Networks) != 1 {
		t.Fatalf("InspectContainer() = %#v", info)
	}
	if info.Ports[0].HostPort != 49152 || info.Ports[0].ContainerPort.Number != 1234 {
		t.Fatalf("port binding = %#v", info.Ports[0])
	}

	result, err := runtime.Exec(t.Context(), target, []string{"fail"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 7 || string(result.Stderr) != "failed\n" {
		t.Fatalf("Exec() = %#v", result)
	}
	logs, err := runtime.Logs(t.Context(), target)
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if string(logs) != "container logs\n" {
		t.Fatalf("Logs() = %q", logs)
	}

	if err := runtime.DestroyContainer(t.Context(), target); err != nil {
		t.Fatalf("DestroyContainer() error = %v", err)
	}
	if err := runtime.DestroyVolume(t.Context(), volume); err != nil {
		t.Fatalf("DestroyVolume() error = %v", err)
	}
	if err := runtime.DestroyNetwork(t.Context(), network); err != nil {
		t.Fatalf("DestroyNetwork() error = %v", err)
	}

	commands, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	run := findRuntimeCommand(string(commands), "run ")
	for _, expected := range []string{
		"--env PASSWORD=secret --env USER=tester",
		"--network network-id",
		"--publish 127.0.0.1::1234/tcp",
		"--mount type=volume,src=volume-id,dst=/data",
		"--mount type=bind,src=/fixtures,dst=/fixtures,readonly",
		"example:1 serve",
	} {
		if !strings.Contains(run, expected) {
			t.Fatalf("run command %q does not contain %q", run, expected)
		}
	}
}

func findRuntimeCommand(commands string, prefix string) string {
	for line := range strings.SplitSeq(commands, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func runRuntimeHelper() {
	args := os.Args[1:]
	file, err := os.OpenFile(
		os.Getenv("CONTAINER_RUNTIME_LOG"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		panic(err)
	}
	if _, err := fmt.Fprintln(file, strings.Join(args, " ")); err != nil {
		panic(err)
	}
	if err := file.Close(); err != nil {
		panic(err)
	}

	switch {
	case args[0] == "network" && args[1] == "create":
		fmt.Println("network-id")
	case args[0] == "network" && args[1] == "inspect":
		fmt.Println(`{"Id":"network-id","Name":"cluster","Driver":"bridge"}`)
	case args[0] == "volume" && args[1] == "create":
		fmt.Println("volume-id")
	case args[0] == "run":
		fmt.Fprintln(os.Stderr, "image pull progress")
		fmt.Println("container-id")
	case args[0] == "inspect":
		fmt.Println(`{"Id":"container-id","Name":"/database","State":{"Status":"running"},"NetworkSettings":{"Ports":{"1234/tcp":[{"HostIp":"127.0.0.1","HostPort":"49152"}]},"Networks":{"cluster":{"NetworkID":"network-id","IPAddress":"10.0.0.2"}}}}`)
	case args[0] == "exec" && args[len(args)-1] == "fail":
		fmt.Fprintln(os.Stderr, "failed")
		os.Exit(7)
	case args[0] == "logs":
		fmt.Println("container logs")
	}
}
