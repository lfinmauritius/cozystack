/*
Copyright 2026 The Cozystack Authors.

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

package tenantgateway

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	gatewayv1alpha1 "github.com/cozystack/cozystack/api/gateway/v1alpha1"
)

// TestReconcile_GatewayClassChangeRecreatesGateway pins that a class change
// deletes the Gateway (foreground, UID precondition) and creates it fresh on
// the next pass instead of patching spec.gatewayClassName in place: the old
// controller's data plane hangs off the old Gateway and must go with it. A
// pass on an unchanged class must never delete it.
func TestReconcile_GatewayClassChangeRecreatesGateway(t *testing.T) {
	s := newScheme(t)
	tgw := &gatewayv1alpha1.TenantGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "cozystack", Namespace: "tenant-foo", UID: "tgw-uid"},
		Spec: gatewayv1alpha1.TenantGatewaySpec{
			Apex:             "foo.example.com",
			CertMode:         gatewayv1alpha1.CertModeHTTP01,
			GatewayClassName: "cilium",
		},
	}
	var deletes []client.DeleteOptions
	var updatedClasses []gatewayv1.ObjectName
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(tgw).
		WithStatusSubresource(tgw, &gatewayv1.Gateway{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, isGateway := obj.(*gatewayv1.Gateway); isGateway {
					o := client.DeleteOptions{}
					o.ApplyOptions(opts)
					deletes = append(deletes, o)
				}
				return cl.Delete(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if gw, isGateway := obj.(*gatewayv1.Gateway); isGateway {
					updatedClasses = append(updatedClasses, gw.Spec.GatewayClassName)
				}
				return cl.Update(ctx, obj, opts...)
			},
		}).
		Build()

	r := &Reconciler{Client: c, Scheme: s}
	key := types.NamespacedName{Name: "cozystack", Namespace: "tenant-foo"}
	reconcile := func(pass string) ctrl.Result {
		t.Helper()
		res, err := r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: key})
		if err != nil {
			t.Fatalf("%s pass: %v", pass, err)
		}
		return res
	}
	gatewayClass := func() (gatewayv1.ObjectName, error) {
		gw := &gatewayv1.Gateway{}
		err := c.Get(context.TODO(), key, gw)
		return gw.Spec.GatewayClassName, err
	}

	reconcile("initial")
	reconcile("unchanged class")
	if len(deletes) != 0 {
		t.Fatalf("Gateway deleted %d time(s) without a class change", len(deletes))
	}

	cur := &gatewayv1alpha1.TenantGateway{}
	if err := c.Get(context.TODO(), key, cur); err != nil {
		t.Fatalf("get TenantGateway: %v", err)
	}
	cur.Spec.GatewayClassName = "envoy-internal"
	if err := c.Update(context.TODO(), cur); err != nil {
		t.Fatalf("update TenantGateway class: %v", err)
	}

	if res := reconcile("class change"); res.RequeueAfter == 0 {
		t.Errorf("class change pass returned %+v, want a requeue while the old Gateway goes", res)
	}
	if len(deletes) != 1 {
		t.Fatalf("class change deleted the Gateway %d time(s), want 1", len(deletes))
	}
	if p := deletes[0].PropagationPolicy; p == nil || *p != metav1.DeletePropagationForeground {
		t.Errorf("Gateway deleted with propagation %v, want Foreground", p)
	}
	if deletes[0].Preconditions == nil || deletes[0].Preconditions.UID == nil {
		t.Errorf("Gateway deleted without a UID precondition")
	}
	for _, cls := range updatedClasses {
		if cls == "envoy-internal" {
			t.Fatalf("Gateway class patched in place to %q instead of recreated", cls)
		}
	}
	if _, err := gatewayClass(); !apierrors.IsNotFound(err) {
		t.Fatalf("Gateway still present after the class-change pass (err=%v)", err)
	}

	reconcile("recreate")
	cls, err := gatewayClass()
	if err != nil {
		t.Fatalf("Gateway not recreated: %v", err)
	}
	if cls != "envoy-internal" {
		t.Errorf("recreated Gateway class=%q, want envoy-internal", cls)
	}
	if len(deletes) != 1 {
		t.Errorf("recreate pass deleted the Gateway again (%d deletes)", len(deletes))
	}
}
