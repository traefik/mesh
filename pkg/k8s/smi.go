package k8s

import (
	"errors"
	"fmt"
	"strings"

	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha2"
	specs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// CheckSMIVersion checks if the SMI CRDs versions installed match the supported versions.
func CheckSMIVersion(client kubernetes.Interface, aclEnabled bool) error {
	serverGroups, err := client.Discovery().ServerGroups()
	if err != nil {
		return fmt.Errorf("unable to list kubernetes server groups: %w", err)
	}

	requiredGroups := []schema.GroupVersion{
		split.SchemeGroupVersion,
		specs.SchemeGroupVersion,
	}

	if aclEnabled {
		requiredGroups = append(requiredGroups, access.SchemeGroupVersion)
	}

	var errs []string

	for _, requiredGroup := range requiredGroups {
		var version string

		for _, group := range serverGroups.Groups {
			if requiredGroup.Group == group.Name {
				version = group.PreferredVersion.Version

				break
			}
		}

		if version == "" {
			errs = append(errs, fmt.Sprintf("unable to find group %q version %q", requiredGroup.Group, requiredGroup.Version))
		} else if version != requiredGroup.Version {
			errs = append(errs, fmt.Sprintf("unable to find group %q version %q, got %q", requiredGroup.Group, requiredGroup.Version, version))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}
