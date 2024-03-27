package collector_manager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/stackrox/collector/integration-tests/pkg/config"
	"github.com/stackrox/collector/integration-tests/pkg/executor"
	"golang.org/x/exp/maps"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TEST_NAMESPACE = "collector-tests"
)

type K8sCollectorManager struct {
	executor     executor.Executor
	volumeMounts []coreV1.VolumeMount
	volumes      []coreV1.Volume
	env          []coreV1.EnvVar
	config       map[string]any

	testName string
}

func newK8sManager(e executor.Executor, name string) *K8sCollectorManager {
	collectorOptions := config.CollectorInfo()
	collectionMethod := config.CollectionMethod()

	collectorConfig := map[string]any{
		"logLevel":       collectorOptions.LogLevel,
		"turnOffScrape":  true,
		"scrapeInterval": 2,
	}

	env := []coreV1.EnvVar{
		{Name: "GRPC_SERVER", Value: "tester-svc:9999"},
		{Name: "COLLECTION_METHOD", Value: collectionMethod},
		{Name: "COLLECTOR_PRE_ARGUMENTS", Value: collectorOptions.PreArguments},
		{Name: "ENABLE_CORE_DUMP", Value: "false"},
	}

	if !collectorOptions.Offline {
		env = append(env, coreV1.EnvVar{Name: "MODULE_DOWNLOAD_BASE_URL", Value: "https://collector-modules.stackrox.io/612dd2ee06b660e728292de9393e18c81a88f347ec52a39207c5166b5302b656"})
	}

	propagationHostToContainer := coreV1.MountPropagationHostToContainer
	mounts := []coreV1.VolumeMount{
		{Name: "proc-ro", ReadOnly: true, MountPath: "/host/proc", MountPropagation: &propagationHostToContainer},
		{Name: "etc-ro", ReadOnly: true, MountPath: "/host/etc", MountPropagation: &propagationHostToContainer},
		{Name: "usr-ro", ReadOnly: true, MountPath: "/host/usr/lib", MountPropagation: &propagationHostToContainer},
		{Name: "sys-ro", ReadOnly: true, MountPath: "/host/sys/kernel/debug", MountPropagation: &propagationHostToContainer},
		{Name: "var-rw", ReadOnly: false, MountPath: "/host/var", MountPropagation: &propagationHostToContainer},
		{Name: "run-rw", ReadOnly: false, MountPath: "/host/run", MountPropagation: &propagationHostToContainer},
		{Name: "tmp", ReadOnly: false, MountPath: "/tmp", MountPropagation: &propagationHostToContainer},
		{Name: "module", ReadOnly: false, MountPath: "/module"},
	}

	volumes := []coreV1.Volume{
		{Name: "proc-ro", VolumeSource: coreV1.VolumeSource{HostPath: &coreV1.HostPathVolumeSource{Path: "/proc"}}},
		{Name: "etc-ro", VolumeSource: coreV1.VolumeSource{HostPath: &coreV1.HostPathVolumeSource{Path: "/etc"}}},
		{Name: "usr-ro", VolumeSource: coreV1.VolumeSource{HostPath: &coreV1.HostPathVolumeSource{Path: "/usr/lib"}}},
		{Name: "sys-ro", VolumeSource: coreV1.VolumeSource{HostPath: &coreV1.HostPathVolumeSource{Path: "/sys/kernel/debug"}}},
		{Name: "var-rw", VolumeSource: coreV1.VolumeSource{HostPath: &coreV1.HostPathVolumeSource{Path: "/var"}}},
		{Name: "run-rw", VolumeSource: coreV1.VolumeSource{HostPath: &coreV1.HostPathVolumeSource{Path: "/run"}}},
		{Name: "tmp", VolumeSource: coreV1.VolumeSource{HostPath: &coreV1.HostPathVolumeSource{Path: "/tmp"}}},
		{Name: "module", VolumeSource: coreV1.VolumeSource{EmptyDir: &coreV1.EmptyDirVolumeSource{}}},
	}

	return &K8sCollectorManager{
		executor:     e,
		volumeMounts: mounts,
		volumes:      volumes,
		env:          env,
		config:       collectorConfig,
		testName:     name,
	}
}

func (k *K8sCollectorManager) Setup(options *CollectorStartupOptions) error {
	if options == nil {
		// default values
		options = &CollectorStartupOptions{}
	}

	for name, value := range options.Env {
		k.env = replaceOrAppendEnvVar(k.env, coreV1.EnvVar{Name: name, Value: value})
	}

	configJson, err := json.Marshal(k.config)
	if err != nil {
		return err
	}
	k.env = replaceOrAppendEnvVar(k.env, coreV1.EnvVar{Name: "COLLECTOR_CONFIG", Value: string(configJson)})

	if options.Config != nil {
		maps.Copy(k.config, options.Config)
	}

	return nil
}

func (k *K8sCollectorManager) Launch() error {
	objectMeta := metaV1.ObjectMeta{
		Name:      "collector",
		Namespace: TEST_NAMESPACE,
		Labels:    map[string]string{"app": "collector"},
	}

	privileged := true
	container := coreV1.Container{
		Name:            "collector",
		Image:           config.Images().CollectorImage(),
		Ports:           []coreV1.ContainerPort{{ContainerPort: 8080}},
		Env:             k.env,
		VolumeMounts:    k.volumeMounts,
		SecurityContext: &coreV1.SecurityContext{Privileged: &privileged},
	}

	pod := &coreV1.Pod{
		ObjectMeta: objectMeta,
		Spec: coreV1.PodSpec{
			Containers:    []coreV1.Container{container},
			Volumes:       k.volumes,
			RestartPolicy: coreV1.RestartPolicyNever, // if the pod fails, it fails
		},
	}

	executor := k.executor.(*executor.K8sExecutor)
	_, err := executor.CreatePod(TEST_NAMESPACE, pod)
	return err
}

func (k *K8sCollectorManager) TearDown() error {
	isRunning, err := k.IsRunning()
	if err != nil {
		return err
	}

	err = k.captureLogs()
	if err != nil {
		return fmt.Errorf("Failed to get collector logs: %s", err)
	}

	if !isRunning {
		exitCode, err := k.executor.ExitCode(executor.PodFilter{
			Name:      "collector",
			Namespace: TEST_NAMESPACE,
		})
		if err != nil {
			return fmt.Errorf("Failed to get container exit code: %s", err)
		}

		if exitCode != 0 {
			return fmt.Errorf("Collector container has non-zero exit code (%d)", exitCode)
		}
	}

	executor := k.executor.(*executor.K8sExecutor)
	return executor.ClientSet().CoreV1().Pods(TEST_NAMESPACE).Delete(context.Background(), "collector", metaV1.DeleteOptions{})
}

func (k *K8sCollectorManager) IsRunning() (bool, error) {
	executor := k.executor.(*executor.K8sExecutor)
	pod, err := executor.ClientSet().CoreV1().Pods(TEST_NAMESPACE).Get(context.Background(), "collector", metaV1.GetOptions{})
	if err != nil {
		return false, err
	}

	return *pod.Status.ContainerStatuses[0].Started, nil
}

func (k *K8sCollectorManager) ContainerID() string {
	pf := executor.PodFilter{
		Name:      "collector",
		Namespace: TEST_NAMESPACE,
	}

	return k.executor.ContainerID(pf)
}

func replaceOrAppendEnvVar(list []coreV1.EnvVar, newVar coreV1.EnvVar) []coreV1.EnvVar {
	for _, envVar := range list {
		if envVar.Name == newVar.Name {
			envVar.Value = newVar.Value
			return list
		}
	}

	return append(list, newVar)
}

func (k *K8sCollectorManager) captureLogs() error {
	executor := k.executor.(*executor.K8sExecutor)
	req := executor.ClientSet().CoreV1().Pods(TEST_NAMESPACE).GetLogs("collector", &coreV1.PodLogOptions{})
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		return err
	}
	defer podLogs.Close()

	logDirectory := filepath.Join(".", "container-logs", config.VMInfo().Config, config.CollectionMethod())
	os.MkdirAll(logDirectory, os.ModePerm)
	logFilePath := filepath.Join(logDirectory, strings.ReplaceAll(k.testName, "/", "_")+"-collector.log")
	fmt.Printf("Dumping collector logs to %q\n", logFilePath)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	_, err = io.Copy(logFile, podLogs)
	return err
}
