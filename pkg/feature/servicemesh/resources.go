package servicemesh

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/feature"
)

// MeshRefs stores service mesh configuration in the config map, so it can
// be easily accessed by other components which rely on this information.
func MeshRefs(ctx context.Context, f *feature.Feature) error {
	meshConfig, err := FeatureData.ControlPlane.Extract(f)
	if err != nil {
		return fmt.Errorf("failed to get control plane struct: %w", err)
	}
	namespace := f.TargetNamespace

	data := map[string]string{
		"CONTROL_PLANE_NAME": meshConfig.Name,
		"MESH_NAMESPACE":     meshConfig.Namespace,
	}

	return cluster.CreateOrUpdateConfigMap(
		ctx,
		f.Client,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConfigMapMeshRef,
				Namespace: namespace,
			},
			Data: data,
		},
		feature.OwnedBy(f),
	)
}

// AuthRefs stores authorization configuration in the config map, so it can
// be easily accessed by other components which rely on this information.
func AuthRefs(ctx context.Context, f *feature.Feature) error {
	targetNamespace := f.TargetNamespace
	auth, err := FeatureData.Authorization.Extract(f)
	if err != nil {
		return fmt.Errorf("could not get auth from feature: %w", err)
	}

	audiences := auth.Audiences
	audiencesList := ""
	if audiences != nil && len(*audiences) > 0 {
		audiencesList = strings.Join(*audiences, ",")
	}
	data := map[string]string{
		"AUTH_AUDIENCE":   audiencesList,
		"AUTH_PROVIDER":   auth.ProviderName,
		"AUTHORINO_LABEL": auth.AuthConfigSelector,
	}

	return cluster.CreateOrUpdateConfigMap(
		ctx,
		f.Client,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConfigMapAuthRef,
				Namespace: targetNamespace,
			},
			Data: data,
		},
		feature.OwnedBy(f),
	)
}
