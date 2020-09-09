package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry-community/go-cfclient"
)

func (c *checker) migrateService(service *cfclient.ServiceSummary, spaceGUID string, params map[string]interface{},
	bindings []cfclient.ServiceBinding) (*cfclient.ServiceInstance, error) {

	if err := c.renameServiceInstance(service.Guid, service.Name+"-old", true); err != nil {
		return nil, fmt.Errorf("couldn't rename service %s: %w", service.Name, err)
	}

	// create new service instance, using original name, and an updated config
	newInstance, err := c.client.CreateServiceInstance(cfclient.ServiceInstanceRequest{
		Name:            service.Name,
		ServicePlanGuid: service.ServicePlan.Guid,
		SpaceGuid:       spaceGUID,
		Parameters:      params,
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't create new service instance %s: %w", service.Name, err)
	}

	// bind apps new new service instance and remove the binding to the old service instance
	for _, binding := range bindings {
		if _, err := c.client.CreateServiceBinding(binding.AppGuid, newInstance.Guid); err != nil {
			// TODO: roll back what we've done so far?
			return nil, fmt.Errorf("couldn't bind app %s to new service instance %s: %w",
				binding.AppGuid, newInstance.Name, err)
		}

		if err := c.client.DeleteServiceBinding(binding.Guid); err != nil {
			return nil, fmt.Errorf("couldn't delete old service binding %s: %w", binding.Guid, err)
		}

		restage, err := c.client.RestageApp(binding.AppGuid)
		if err != nil {
			return nil, fmt.Errorf("couldn't restage app %q: %w", binding.AppGuid, err)
		}

		fmt.Printf("restage app %s:\n", binding.AppGuid)
		json.NewEncoder(os.Stdout).Encode(restage)
	}

	// TODO: wait for apps to restage...

	// delete old service instances
	deleteOldServiceInstances := false // not for now
	if deleteOldServiceInstances {
		recursive := false // to also delete bindings, routes, and service keys (TODO experiment with this)
		if err := c.client.DeleteServiceInstance(service.Guid, recursive, true); err != nil {
			return nil, fmt.Errorf("couldn't delete old service instance %s (%s): %w",
				service.Name, service.Guid, err)
		}
	}

	return &newInstance, nil
}

// renameServiceInstance renames the specified service instance and optionally
// waits for the operation to complete.
func (c *checker) renameServiceInstance(guid string, name string, wait bool) error {
	newConfig := fmt.Sprintf(`{ "name": "%s"}`, name)
	err := c.client.UpdateServiceInstance(guid, strings.NewReader(newConfig), true)
	if err != nil {
		return fmt.Errorf("couldn't rename service %s: %w", guid, err)
	}

	if !wait {
		return nil
	}

	// wait for rename to take effect
	for tries := 0; tries < 3; tries++ {
		time.Sleep(1 * time.Second)
		si, err := c.client.GetServiceInstanceByGuid(guid)
		if err == nil {
			switch si.LastOperation.State {
			case "succeeded":
				break
			case "failed":
				return fmt.Errorf("rename service operation failed for service instance %s", guid)
			default:
			}
		}
		if tries == 2 {
			return fmt.Errorf("rename of service instance %s hasn't completed: last operation = %v",
				guid, si.LastOperation)
		}
	}
	return nil
}

func fixConfigServerParams(params map[string]interface{}) {
	if git, ok := params["git"]; ok {
		if gitm, ok := git.(map[string]interface{}); ok {
			if _, ok = gitm["repos"]; ok {
				// TODO: is there an alternative?
				delete(gitm, "repos")
			}
		}
	}

	if composite, ok := params["composite"]; ok {
		if composites, ok := composite.([]interface{}); ok {
			for _, entry := range composites {
				if obj, ok := entry.(map[string]interface{}); ok {
					if git, ok := obj["git"]; ok {
						if gitm, ok := git.(map[string]interface{}); ok {
							for k, v := range gitm {
								obj[k] = v
							}
							obj["type"] = "git"
							delete(obj, "git")
						}
					}
					if vault, ok := obj["vault"]; ok {
						if vaultm, ok := vault.(map[string]interface{}); ok {
							for k, v := range vaultm {
								obj[k] = v
							}
							obj["type"] = "vault"
							delete(obj, "vault")
						}
					}
				}
			}
		}
	}
}
