package main

import (
	"flag"
	"log"
	"os"
	"path"

	"me.kubetools/pkg/kubegc"

	clientcmd "k8s.io/client-go/tools/clientcmd"
)

func main() {
	var kubeConfig string
	var labelSelector string
	var filter string
	var dryRun bool

	flag.StringVar(&kubeConfig, "kubeconfig", "", "kubeconfig file")
	flag.StringVar(&labelSelector, "label-selector", "", "label selector of resources to check (required)")
	flag.StringVar(&filter, "filter", "", "annotation selector of resources to keep (required)")
	flag.BoolVar(&dryRun, "dry-run", true, "do not perform clean up (default 'true')")
	flag.Parse()

	if len(labelSelector) == 0 || len(filter) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if len(kubeConfig) == 0 {
		kubeConfig = os.Getenv("KUBECONFIG")
		if len(kubeConfig) == 0 {
			kubeConfigDefaultPath := path.Join(os.Getenv("HOME"), ".kube/config")
			if _, err := os.Stat(kubeConfigDefaultPath); err == nil {
				kubeConfig = kubeConfigDefaultPath
			}
		}
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	config.Burst = 100

	gc, err := kubegc.NewKubeGC(config, labelSelector, filter)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	err = gc.Clean(dryRun)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

}
