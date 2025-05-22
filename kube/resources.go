package kube

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func MaxResourceList(total corev1.ResourceList, add corev1.ResourceList) {
	ResourceListCollect(total, add, func(_ corev1.ResourceName, into *resource.Quantity, val resource.Quantity) {
		if into.Cmp(val) < 0 {
			*into = val
		}
	})
}

func AddResourceList(total corev1.ResourceList, add corev1.ResourceList) {
	ResourceListCollect(total, add, func(_ corev1.ResourceName, into *resource.Quantity, val resource.Quantity) {
		into.Add(val)
	})
}

func SubResourceList(total corev1.ResourceList, sub corev1.ResourceList) {
	ResourceListCollect(total, sub, func(_ corev1.ResourceName, into *resource.Quantity, val resource.Quantity) {
		into.Sub(val)
	})
}

func MultiplyResourceList(total corev1.ResourceList, factor int64) {
	ResourceListCollect(total, total, func(_ corev1.ResourceName, into *resource.Quantity, _ resource.Quantity) {
		into.Mul(factor)
	})
}

type ResourceListCollectFunc func(corev1.ResourceName, *resource.Quantity, resource.Quantity)

func ResourceListCollect(into, vals corev1.ResourceList, collect ResourceListCollectFunc) corev1.ResourceList {
	for resourceName, quantity := range vals {
		lastQuantity := into[resourceName].DeepCopy()
		collect(resourceName, &lastQuantity, quantity)
		into[resourceName] = lastQuantity
	}
	return into
}
