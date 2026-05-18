package k8s

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/janlauber/one-click/pkg/env"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

var (
	Clientset       *kubernetes.Clientset
	DynamicClient   dynamic.Interface
	DiscoveryClient *discovery.DiscoveryClient
	MetricsClient   metricsv.Interface
	Kubeconfig      *rest.Config
	Ctx             context.Context
)

// Init wires the Kubernetes clients.
//
// Required behavior (minimal, safe):
//  1) If KUBERNETES_SERVICE_HOST/PORT exist, use in-cluster config (strict).
//  2) Else if KUBECONFIG exists, use it.
//  3) Else try --kubeconfig flag (defaults to LOCAL_KUBECONFIG_FILE / ~/.kube/config).
//  4) Else run in LOCAL RUNTIME MODE with Kubernetes disabled.
//
// In local runtime mode we DO NOT exit the process. K8s-dependent handlers
// should handle nil clients and return a clear error.
func Init() {
	Ctx = context.Background()

	// Flags must only be parsed once; guard so repeated Init() calls won't panic.
	var kubeconfigFlag string
	if !flag.Parsed() {
		flag.StringVar(&kubeconfigFlag, "kubeconfig", env.Config.LocalKubeConfigFile, "(optional) absolute path to the kubeconfig file")
		flag.Parse()
	}

	strictCluster := hasInClusterEnv()
	log.Printf("[runtime] local=%v", env.Config.Local)

	var (
		cfg *rest.Config
		err error
	)

	if strictCluster {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			log.Fatalf("Failed to get in-cluster config: %v", err)
		}
		log.Printf("[k8s] mode=in-cluster host=%s", safeHost(cfg))
	} else {
		kcfg := os.Getenv("KUBECONFIG")
		if kcfg == "" {
			kcfg = kubeconfigFlag
		}
		kcfg = expandHome(kcfg)

		if kcfg != "" {
			cfg, err = clientcmd.BuildConfigFromFlags("", kcfg)
			if err != nil {
				log.Printf("[k8s] kubeconfig load failed path=%q err=%v", kcfg, err)
				cfg = nil
			} else {
				log.Printf("[k8s] mode=kubeconfig path=%q host=%s", kcfg, safeHost(cfg))
			}
		}

		if cfg == nil {
			log.Printf("[k8s] no cluster config found; cluster watchers/controllers disabled (local runtime mode)")
			disableClients()
			return
		}
	}

	Kubeconfig = cfg

	var err2 error
	DiscoveryClient, err2 = discovery.NewDiscoveryClientForConfig(Kubeconfig)
	if err2 != nil {
		if strictCluster {
			log.Fatalf("Failed to create discovery client: %v", err2)
		}
		log.Printf("[k8s] discovery client init failed (%v) — disabling K8s", err2)
		disableClients()
		return
	}

	MetricsClient, err2 = metricsv.NewForConfig(Kubeconfig)
	if err2 != nil {
		if strictCluster {
			log.Fatalf("Failed to create metrics client: %v", err2)
		}
		log.Printf("[k8s] metrics client init failed (%v) — continuing without metrics", err2)
		MetricsClient = nil
	}

	Clientset, err2 = kubernetes.NewForConfig(Kubeconfig)
	if err2 != nil {
		if strictCluster {
			log.Fatalf("Failed to create Kubernetes clientset: %v", err2)
		}
		log.Printf("[k8s] clientset init failed (%v) — disabling K8s", err2)
		disableClients()
		return
	}

	DynamicClient, err2 = dynamic.NewForConfig(Kubeconfig)
	if err2 != nil {
		if strictCluster {
			log.Fatalf("Failed to create dynamic client: %v", err2)
		}
		log.Printf("[k8s] dynamic client init failed (%v) — continuing without dynamic client", err2)
		DynamicClient = nil
	}
}

func hasInClusterEnv() bool {
	return os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != ""
}

func expandHome(p string) string {
	if p == "" {
		return ""
	}
	if len(p) >= 2 && p[:2] == "~/" {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

func safeHost(cfg *rest.Config) string {
	if cfg == nil || cfg.Host == "" {
		return "unknown"
	}
	return cfg.Host
}

func disableClients() {
	Kubeconfig = nil
	Clientset = nil
	DynamicClient = nil
	DiscoveryClient = nil
	MetricsClient = nil
}

func GetClusterVersion() (string, error) {
	if DiscoveryClient != nil {
		clusterVersion, err := DiscoveryClient.ServerVersion()
		if err != nil {
			return "", err
		}
		return clusterVersion.GitVersion, nil
	}

	return "unknown", nil
}

func GetClusterApi() (string, error) {
	var clusterName string

	if Kubeconfig != nil {
		clusterName = Kubeconfig.Host
	} else {
		clusterName = "unknown"
	}

	return clusterName, nil
}
