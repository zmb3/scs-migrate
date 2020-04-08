package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/fatih/color"
	"github.com/gosuri/uiprogress"
	"github.com/mattn/go-isatty"
)

const (
	CircuitBreaker2Label = "p-circuit-breaker-dashboard"

	ConfigServer2Label = "p-config-server"
	ConfigServer3Label = "p.config-server"

	Eureka2Label = "p-service-registry"
	Eureka3Label = "p.service-registry"
)

const (
	ErrorIcon   = "❌"
	WarningIcon = "⚠️"
	SafeIcon    = "✅"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type checker struct {
	client         *cfclient.Client
	appsByGUID     map[string]cfclient.App
	orgNamesByGUID map[string]string
}

type usage struct {
	serviceInstanceName string
	org                 string
	space               string
	boundApps           int
	appNames            []string

	// Config parameters in SCS 2.X and below that
	// may be incompatible with SCS 3.X
	usesOldGitRepos  bool
	usesEncryptKey   bool
	usesOldComposite bool
}

func (c *checker) populateOrgsAndApps() error {
	orgs, err := c.client.ListOrgs()
	if err != nil {
		return fmt.Errorf("could not list orgs: %w", err)
	}
	c.orgNamesByGUID = make(map[string]string)
	for _, org := range orgs {
		c.orgNamesByGUID[org.Guid] = org.Name
	}

	// TODO: fetch apps lazily, as there could be a lot
	c.appsByGUID = make(map[string]cfclient.App)
	apps, err := c.client.ListApps()
	if err != nil {
		return fmt.Errorf("could not list apps: %w", err)
	}
	for _, app := range apps {
		c.appsByGUID[app.Guid] = app
	}
	return nil
}

func servicesByLabel(label string, summaries []cfclient.ServiceSummary) []cfclient.ServiceSummary {
	var result []cfclient.ServiceSummary
	for i := range summaries {
		if summaries[i].ServicePlan.Service.Label == label {
			result = append(result, summaries[i])
		}
	}
	return result
}

func (c *checker) getAppsForServiceInstance(guid string) ([]string, error) {
	var apps []string
	q := url.Values{}
	q.Set("q", "service_instance_guid:"+guid)
	bindings, err := c.client.ListServiceBindingsByQuery(q)
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		// TODO: this app might be in a different org/space
		// if the service instance is shared, make it easier to identify
		apps = append(apps, c.appsByGUID[binding.AppGuid].Name)
	}
	return apps, nil
}

// commandLineURL converts a config server dashboard URL
// into a URL that's accessible with a UAA token
func commandLineURL(dashboardURL string) string {
	return strings.Replace(
		strings.Replace(dashboardURL, "dashboard", "cli", -1),
		ConfigServer2Label, "instances", -1) + "/parameters"
}

func getConfigServerParameters(c *cfclient.Client, summary *cfclient.ServiceSummary) (map[string]interface{}, error) {
	req, _ := http.NewRequest(http.MethodGet, commandLineURL(summary.DashboardURL), nil)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func main() {
	api := flag.String("api", "", "API endpoint. Required. ex: https://api.sys.foo.com")
	user := flag.String("user", "", "Cloud Foundry API user. Required")
	password := flag.String("password", "", "Cloud Foundry API Password.  May also be provided via the $PASSWORD variable")
	insecure := flag.Bool("insecure", false, "do not validate TLS connections")
	flag.Parse()

	for _, arg := range flag.Args() {
		if arg == "version" {
			fmt.Printf("%v, commit %v, built at %v\n", version, commit, date)
			return
		}
	}

	if len(*password) == 0 {
		*password = os.Getenv("PASSWORD")
	}

	if len(*api) == 0 || len(*user) == 0 || len(*password) == 0 {
		log.Fatal("the api, user, and password flags are required, please try again")
	}

	client, err := cfclient.NewClient(&cfclient.Config{
		ApiAddress:        *api,
		Username:          *user,
		Password:          *password,
		SkipSslValidation: *insecure,
	})
	if err != nil {
		log.Fatalf("could not create API client: %v", err)
	}

	c := &checker{client: client}
	if err := c.populateOrgsAndApps(); err != nil {
		log.Fatal(err)
	}

	isTerminal := isatty.IsTerminal(os.Stdout.Fd())
	if !isTerminal {
		color.NoColor = true
	}

	spaces, err := client.ListSpaces()
	if err != nil {
		log.Fatal(err)
	}

	prog := uiprogress.New()
	prog.SetOut(os.Stderr)
	bar := prog.AddBar(len(spaces) - 1).AppendCompleted()
	prog.Start()

	var hystrixUsages, configServerUsages, serviceRegistryUsages []usage
	var errors []error
	for i, space := range spaces {
		bar.Set(i)
		summary, err := space.Summary()
		if err != nil {
			errors = append(errors, fmt.Errorf("couldn't get space summary for %s: %w", space.Guid, err))
			continue
		}

		hystrixDashboards := servicesByLabel(CircuitBreaker2Label, summary.Services)
		errors = append(errors, c.buildUsages(&space, hystrixDashboards, &hystrixUsages)...)

		configServers := servicesByLabel(ConfigServer2Label, summary.Services)
		errors = append(errors, c.buildUsages(&space, configServers, &configServerUsages)...)

		serviceRegistries := servicesByLabel(Eureka2Label, summary.Services)
		errors = append(errors, c.buildUsages(&space, serviceRegistries, &serviceRegistryUsages)...)
	}

	prog.Stop()
	fmt.Println()

	if l := len(errors); l > 0 {
		fmt.Printf("Encountered %d errors:\n", l)
		for _, err := range errors {
			fmt.Println(err)
		}
		fmt.Println()
	}

	if len(hystrixUsages) > 0 {
		color.Yellow(WarningIcon + "  The Circuit Breaker Dashboard is unavailable in SCS 3.x - the following services cannot be migrated:")
		fmt.Println("See", color.BlueString("tanzu.vmware.com/content/practitioners/replacing-the-spring-cloud-services-circuit-breaker-dashboard"))
		printUsages(hystrixUsages)
	} else {
		fmt.Println(SafeIcon, " no usages of deprecated Circuit Breaker Dashboard")
	}
	fmt.Println()

	if len(configServerUsages) > 0 {
		color.Green(SafeIcon + " Config Server instances to be migrated")
		printUsages(configServerUsages)
		fmt.Println()
	} else {
		fmt.Println(SafeIcon, " no Config Server instances to migrate")
	}

	if len(serviceRegistryUsages) > 0 {
		color.Green(SafeIcon + " Service Registry instances to be migrated")
		color.Yellow(WarningIcon + "  Note: not available in SCS 3.0, you must run SCS 3.1+")
		printUsages(serviceRegistryUsages)
	} else {
		fmt.Println(SafeIcon, " no Service Registry instances to be migrated")
	}
}

func (c *checker) buildUsages(space *cfclient.Space, summaries []cfclient.ServiceSummary, usages *[]usage) []error {
	var errors []error
	for _, service := range summaries {
		var apps []string
		if service.BoundAppCount > 0 {
			// TODO: include org/space and not just the app name
			apps, _ = c.getAppsForServiceInstance(service.Guid)
		}

		// check for incompatible config params (config server only)
		var usesOldGitRepos, usesEncryptKey, usesComposite bool
		if service.ServicePlan.Service.Label == ConfigServer2Label {
			config, err := getConfigServerParameters(c.client, &service)
			if err != nil {
				errors = append(errors, fmt.Errorf("couldn't get config parameters for service %s: %w", service.Guid, err))
			} else {
				usesOldGitRepos, usesEncryptKey, usesComposite = validateConfigServerParams(config)
			}
		}

		*usages = append(*usages, usage{
			space:               space.Name,
			org:                 c.orgNamesByGUID[space.OrganizationGuid],
			serviceInstanceName: service.Name,
			boundApps:           service.BoundAppCount,
			appNames:            apps,

			usesOldGitRepos:  usesOldGitRepos,
			usesEncryptKey:   usesEncryptKey,
			usesOldComposite: usesComposite,
		})
	}
	return errors
}

func validateConfigServerParams(params map[string]interface{}) (usesGitRepos, usesEncryptKey, usesComposite bool) {
	// SCS 3.x does not support multiple repositories via
	// { "git" { "repos": [] } }
	if git, ok := params["git"]; ok {
		if gitm, ok := git.(map[string]interface{}); ok {
			_, usesGitRepos = gitm["repos"]
		}
	}
	// encrypt key not available in SCS 3.0.0, added in SCS 3.1.6
	if encrypt, ok := params["encrypt"]; ok {
		if encryptm, ok := encrypt.(map[string]interface{}); ok {
			_, usesEncryptKey = encryptm["key"]
		}
	}
	if composite, ok := params["composite"]; ok {
		if composites, ok := composite.([]interface{}); ok {
			// SCS 2.x composite entries use { "git": {...} } or { "vault": {...} }
			// but SCS 3.x entries have { "type": "git" ...} or { "type": "vault"}
			for _, entry := range composites {
				if obj, ok := entry.(map[string]interface{}); ok {
					_, usesCompositeGit := obj["git"]
					_, usesCompositeVault := obj["vault"]
					usesComposite = usesComposite || usesCompositeGit || usesCompositeVault
				}
			}
		}
	}
	return
}

func printUsages(usages []usage) {
	for _, usage := range usages {
		fmt.Printf("- %s/%s/%s (%d apps)\n", usage.org, usage.space, usage.serviceInstanceName, usage.boundApps)
		if usage.usesEncryptKey {
			color.Yellow(" %s  This service uses \"encrypt.key\", which is only available in SCS 3.1.6 and later.", WarningIcon)
		}
		if usage.usesOldComposite {
			color.Yellow(" %s  This service uses composite backend which require a new configuration format in SCS 3.x.", WarningIcon)
		}
		if usage.usesOldGitRepos {
			color.Red(" %s This service uses \"git.repos\", which is not supported in SCS 3.x", ErrorIcon)
		}
		for _, app := range usage.appNames {
			fmt.Println("  -", app)
		}
	}
}
