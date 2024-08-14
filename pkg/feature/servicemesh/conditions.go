package servicemesh

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/feature"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
)

const (
	interval = 2 * time.Second
	duration = 5 * time.Minute
)

// EnsureAuthNamespaceExists creates a namespace for the Authorization provider and set ownership so it will be garbage collected when the operator is uninstalled.
func EnsureAuthNamespaceExists(ctx context.Context, f *feature.Feature) error {
	authz, err := FeatureData.Authorization.Extract(f)
	if err != nil {
		return fmt.Errorf("could not get auth from feature: %w", err)
	}

	_, err = cluster.CreateNamespace(ctx, f.Client, authz.Namespace, feature.OwnedBy(f), cluster.WithLabels(labels.ODH.OwnedNamespace, "true"))
	return err
}

func WaitForServiceMeshMember(namespace string) feature.Action {
	return func(ctx context.Context, f *feature.Feature) error {
		gvk := schema.GroupVersionKind{
			Version: "maistra.io/v1",
			Kind:    "ServiceMeshMember",
		}
		f.Log.Info("waiting for resource to be created", "namespace", namespace, "resource", gvk)

		return wait.PollUntilContextTimeout(ctx, interval, duration, false, func(ctx context.Context) (bool, error) {
			smm := &unstructured.Unstructured{}
			smm.SetGroupVersionKind(gvk)

			err := f.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "default"}, smm)
			if err != nil {
				f.Log.Error(err, "failed waiting for resource", "namespace", namespace, "resource", gvk)

				return false, err
			}

			conditions, found, err := unstructured.NestedSlice(smm.Object, "status", "conditions")
			if err != nil {
				return false, err
			}
			if !found {
				return false, nil
			}
			for _, condition := range conditions {
				if cond, ok := condition.(map[string]interface{}); ok {
					conType, _, _ := unstructured.NestedString(cond, "type")
					conStatus, _, _ := unstructured.NestedString(cond, "status")
					if conType == "Ready" && conStatus == "True" {
						return true, nil
					}
				}
			}
			return false, nil
		})
	}
}

func EnsureServiceMeshOperatorInstalled(ctx context.Context, f *feature.Feature) error {
	if err := feature.EnsureOperatorIsInstalled("servicemeshoperator")(ctx, f); err != nil {
		return fmt.Errorf("failed to find the pre-requisite Service Mesh Operator subscription, please ensure Service Mesh Operator is installed. %w", err)
	}

	return nil
}

func EnsureServiceMeshInstalled(ctx context.Context, f *feature.Feature) error {
	if err := EnsureServiceMeshOperatorInstalled(ctx, f); err != nil {
		return err
	}

	if err := WaitForControlPlaneToBeReady(ctx, f); err != nil {
		controlPlane, errGet := FeatureData.ControlPlane.Extract(f)
		if errGet != nil {
			return fmt.Errorf("failed to get control plane struct: %w", err)
		}

		f.Log.Error(err, "failed waiting for control plane being ready", "control-plane", controlPlane.Name, "namespace", controlPlane.Namespace)

		return multierror.Append(err, errors.New("service mesh control plane is not ready")).ErrorOrNil()
	}

	return nil
}

func WaitForControlPlaneToBeReady(ctx context.Context, f *feature.Feature) error {
	controlPlane, err := FeatureData.ControlPlane.Extract(f)
	if err != nil {
		return err
	}

	smcp := controlPlane.Name
	smcpNs := controlPlane.Namespace

	f.Log.Info("waiting for control plane components to be ready", "control-plane", smcp, "namespace", smcpNs, "duration (s)", duration.Seconds())

	return wait.PollUntilContextTimeout(ctx, interval, duration, false, func(ctx context.Context) (bool, error) {
		ready, err := CheckControlPlaneComponentReadiness(ctx, f.Client, smcp, smcpNs)

		if ready {
			f.Log.Info("done waiting for control plane components to be ready", "control-plane", smcp, "namespace", smcpNs)
		}

		return ready, err
	})
}

func CheckControlPlaneComponentReadiness(ctx context.Context, c client.Client, smcpName, smcpNs string) (bool, error) {
	smcpObj := &unstructured.Unstructured{}
	smcpObj.SetGroupVersionKind(gvk.ServiceMeshControlPlane)
	err := c.Get(ctx, client.ObjectKey{
		Namespace: smcpNs,
		Name:      smcpName,
	}, smcpObj)

	if err != nil {
		return false, fmt.Errorf("failed to find Service Mesh Control Plane: %w", err)
	}

	components, found, err := unstructured.NestedMap(smcpObj.Object, "status", "readiness", "components")
	if err != nil || !found {
		return false, fmt.Errorf("status conditions not found or error in parsing of Service Mesh Control Plane: %w", err)
	}

	readyComponents := len(components["ready"].([]interface{}))     //nolint:forcetypeassert
	pendingComponents := len(components["pending"].([]interface{})) //nolint:forcetypeassert
	unreadyComponents := len(components["unready"].([]interface{})) //nolint:forcetypeassert

	return pendingComponents == 0 && unreadyComponents == 0 && readyComponents > 0, nil
}
