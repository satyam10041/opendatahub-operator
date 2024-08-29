package capabilities

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-platform/pkg/platform"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
)

func CreateOrUpdatePlatformRBAC(ctx context.Context, cli client.Client, roleName string,
	objectReferences []platform.ObjectReference, metaOptions ...cluster.MetaOptions) error {
	if _, err := cluster.CreateOrUpdateClusterRole(ctx, cli, roleName, createPolicyRules(objectReferences), metaOptions...); err != nil {
		return fmt.Errorf("failed creating cluster role: %w", err)
	}

	// TODO: this assumes the platform controllers are embedded in the operator and it's the operator ServiceAccount that require the roles
	namespace, errNS := cluster.GetOperatorNamespace()
	if errNS != nil {
		return fmt.Errorf("failed getting operator namespace: %w", errNS)
	}

	subjects, roleRef := createPlatformRoleBinding(roleName, namespace)
	if _, err := cluster.CreateOrUpdateClusterRoleBinding(ctx, cli, roleName, subjects, roleRef, metaOptions...); err != nil {
		return fmt.Errorf("failed creating cluster role binding: %w", err)
	}

	return nil
}

func createPolicyRules(objectReferences []platform.ObjectReference) []rbacv1.PolicyRule {
	apiGroups := make([]string, 0)
	resources := make([]string, 0)
	for _, ref := range objectReferences {
		apiGroups = append(apiGroups, ref.GroupVersionKind.Group)
		resources = append(resources, ref.Resources)
	}

	return []rbacv1.PolicyRule{
		{
			APIGroups: apiGroups,
			Resources: resources,
			Verbs:     []string{"get", "list", "watch", "update", "patch"},
		},
	}
}

func createPlatformRoleBinding(roleName, namespace string) ([]rbacv1.Subject, rbacv1.RoleRef) {
	return []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "opendatahub-operator-controller-manager", // "odh-platform-manager",
				Namespace: namespace,
			},
		},
		rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		}
}

func defineMetaOptions(owner metav1.Object) ([]cluster.MetaOptions, error) {
	var metaOpts []cluster.MetaOptions

	ownerRef, err := cluster.ToOwnerReference(owner)
	if err != nil {
		return nil, fmt.Errorf("failed to create owner reference: %w", err)
	}
	metaOpts = append(metaOpts, cluster.WithOwnerReference(ownerRef))

	return metaOpts, nil
}
