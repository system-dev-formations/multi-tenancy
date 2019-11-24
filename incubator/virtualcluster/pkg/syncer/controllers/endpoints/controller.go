/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package endpoints

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	ctrl "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type controller struct {
	endpointClient                  v1core.EndpointsGetter
	multiClusterEndpointsController *sc.MultiClusterController
}

func Register(
	endpointsClient v1core.EndpointsGetter,
	endpointsInformer coreinformers.EndpointsInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		endpointClient: endpointsClient,
	}

	options := sc.Options{Reconciler: c}
	multiClusterEndpointsController, err := sc.NewController("tenant-masters-endpoints-controller", &v1.Endpoints{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster endpoints controller %v", err)
		return
	}
	c.multiClusterEndpointsController = multiClusterEndpointsController

	controllerManager.AddController(c)
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return c.multiClusterEndpointsController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile endpoints %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileEndpointsCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Endpoints))
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcileEndpointsUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Endpoints))
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileEndpointsRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Endpoints))
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileEndpointsCreate(cluster, namespace, name string, ep *v1.Endpoints) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, ep)
	if err != nil {
		return err
	}

	pEndpoints := newObj.(*v1.Endpoints)

	_, err = c.endpointClient.Endpoints(targetNamespace).Create(pEndpoints)
	if errors.IsAlreadyExists(err) {
		klog.Infof("endpoints %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcileEndpointsUpdate(cluster, namespace, name string, ep *v1.Endpoints) error {
	return nil
}

func (c *controller) reconcileEndpointsRemove(cluster, namespace, name string, ep *v1.Endpoints) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.endpointClient.Endpoints(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("endpoints %s/%s of cluster not found in super master", namespace, name)
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster ctrl.ClusterInterface) {
	klog.Infof("tenant-masters-endpoints-controller watch cluster %s for endpoints resource", cluster.GetClusterName())
	err := c.multiClusterEndpointsController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s endpoints event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster ctrl.ClusterInterface) {
	klog.Infof("tenant-masters-endpoints-controller stop watching cluster %s for endpoints resource", cluster.GetClusterName())
	c.multiClusterEndpointsController.TeardownClusterResource(cluster)
}
