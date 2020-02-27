package kubegc

import (
	"log"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	selection "k8s.io/apimachinery/pkg/selection"
	discovery "k8s.io/client-go/discovery"
	dynamic "k8s.io/client-go/dynamic"
	rest "k8s.io/client-go/rest"
)

// KubeGC interface
type KubeGC interface {
	Clean(bool) error
}

type kubeGC struct {
	config        *rest.Config
	labelSelector string
	matchFilter   labels.Selector
}

type orphanResource struct {
	gv        schema.GroupVersion
	kind      string
	resource  string
	namespace string
	name      string
}

// NewKubeGC creates new kubeGC instance
func NewKubeGC(config *rest.Config, labelSelector string, annotationFilter string) (KubeGC, error) {

	matchFilter, err := labels.Parse(annotationFilter)
	if err != nil {
		return nil, err
	}

	requirements, _ := matchFilter.Requirements()
	for _, req := range requirements {
		addReq, err := labels.NewRequirement(req.Key(), selection.Exists, []string{})
		if err != nil {
			return nil, err
		}
		matchFilter = matchFilter.Add(*addReq)
	}

	return &kubeGC{
		config:        config,
		labelSelector: labelSelector,
		matchFilter:   matchFilter,
	}, nil
}

func (gc *kubeGC) Clean(dryRun bool) error {
	dynamicClient, err := dynamic.NewForConfig(gc.config)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(gc.config)
	if err != nil {
		return err
	}

	serverGroups, err := discoveryClient.ServerGroups()
	if err != nil {
		return err
	}

	var orphans = []orphanResource{}

	for _, group := range serverGroups.Groups {
		srs, err := discoveryClient.ServerResourcesForGroupVersion(group.PreferredVersion.GroupVersion)
		if err != nil {
			log.Print(err)
			continue
		}
		gv, _ := schema.ParseGroupVersion(group.PreferredVersion.GroupVersion)
		for _, apiResource := range srs.APIResources {
			list, err := dynamicClient.
				Resource(
					schema.GroupVersionResource{
						Group:    gv.Group,
						Version:  gv.Version,
						Resource: apiResource.Name,
					},
				).
				List(
					metav1.ListOptions{
						LabelSelector: gc.labelSelector,
					},
				)
			if err != nil {
				//log.Print(err)
				continue
			}
			for _, r := range list.Items {
				if len(r.GetOwnerReferences()) > 0 {
					continue
				}
				annotations := labels.Set(r.GetAnnotations())
				if gc.matchFilter.Matches(annotations) {
					orphans = append(orphans, orphanResource{
						gv:        gv,
						resource:  apiResource.Name,
						kind:      apiResource.Kind,
						namespace: r.GetNamespace(),
						name:      r.GetName(),
					})
				}
			}
		}
	}

	// Sort resources list: non-namespaced, namespaced, namespaces
	sort.Slice(orphans, func(i, j int) bool {
		if orphans[i].kind == orphans[j].kind {
			return false
		}
		if orphans[j].kind == "Namespace" {
			return true
		}
		if orphans[i].namespace == "" && orphans[j].namespace != "" {
			return true
		}
		return false
	})

	// Do cleanup
	logPrefix := ""
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}
	if dryRun {
		deleteOptions.DryRun = []string{metav1.DryRunAll}
		logPrefix = "(dry-run) "
	}

	for _, o := range orphans {
		result := "OK"

		err = dynamicClient.
			Resource(o.gv.WithResource(o.resource)).
			Namespace(o.namespace).
			Delete(o.name, deleteOptions)
		if err != nil {
			result = err.Error()
		}

		if o.namespace == "" {
			log.Printf("%sdelete %s/%s... %s", logPrefix, o.resource, o.name, result)
		} else {
			log.Printf("%sdelete %s/%s in namespace %s... %s", logPrefix, o.resource, o.name, o.namespace, result)
		}
	}

	return nil
}
