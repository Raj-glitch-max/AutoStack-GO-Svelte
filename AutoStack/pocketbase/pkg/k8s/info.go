package k8s

import "fmt"

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func GetClusterInfo() (interface{}, error) {
	if Clientset == nil {
		return nil, fmt.Errorf("kubernetes disabled: no client (local runtime mode)")
	}

	// get storage classes
	sc, err := Clientset.StorageV1().StorageClasses().List(Ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// get ingress classes
	ic, err := Clientset.NetworkingV1().IngressClasses().List(Ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// get names of storage classes
	storageClasses := []string{}
	for _, s := range sc.Items {
		storageClasses = append(storageClasses, s.Name)
	}

	// get names of ingress classes
	ingressClasses := []string{}
	for _, i := range ic.Items {
		ingressClasses = append(ingressClasses, i.Name)
	}

	clusterInfo := map[string]interface{}{
		"storageClasses": storageClasses,
		"ingressClasses": ingressClasses,
	}

	return clusterInfo, nil
}
