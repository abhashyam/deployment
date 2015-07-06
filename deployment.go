package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"gopkg.in/rightscale/rsc.v2/cm15"
	"gopkg.in/rightscale/rsc.v2/rsapi"
)

type Input struct {
	Name, Value string
}

type Server struct {
	Name, Template                string
	CurrentInstance, NextInstance []Input
	Locked                        bool
}

type ServerArray struct {
	Name, Template                string
	CurrentInstance, NextInstance []Input
	Locked                        bool
}

type RightScript struct {
	Name      string
	Revision  int
	UpdatedAt *cm15.RubyTime
}

type Recipe struct {
	Name, Cookbook string
	Revision       string
	Frozen         bool
	FrozenAt       *cm15.RubyTime
	UpdatedAt      *cm15.RubyTime
}

type ServerTemplate struct {
	Name         string
	Revision     int
	RightScripts []RightScript
	Recipes      []Recipe
}

type Deployment struct {
	Name               string
	ServersNumber      int
	ServerArraysNumber int
	Servers            []Server
	ServerArrays       []ServerArray
	Inputs             []Input
	ServerTemplates    []ServerTemplate
}

var templates map[string]string

func extractHref(links []map[string]string, rel string) string {
	for _, linkMap := range links {
		if linkMap["rel"] == rel {
			return linkMap["href"]
		} else {
			continue
		}
	}
	return ""
}

func inputsRetrieve(client *cm15.Api, inputsLocator string) []Input {
	locator := client.InputLocator(inputsLocator)
	inputs, err := locator.Index(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find inputs: %s", err)
	}
	var inputsRetrieved = make([]Input, len(inputs))
	for index, input := range inputs {
		inputsRetrieved[index] = Input{Name: input.Name, Value: input.Value}
	}
	return inputsRetrieved
}

func templateRetrieve(client *cm15.Api, templateLocator string) *cm15.ServerTemplate {
	locator := client.ServerTemplateLocator(templateLocator)
	template, err := locator.Show(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find server template: %s", err)
	}
	return template
}

func cookbooksRetrieve(client *cm15.Api, cookbookLocator string) *cm15.Cookbook {
	locator := client.CookbookLocator(cookbookLocator)
	cookbook, err := locator.Show(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find cookbook: %s", err)
	}
	return cookbook
}

func cookbookAttachmentsRetrieve(client *cm15.Api, cookbookAttachmentsLocator string) []*cm15.CookbookAttachment {
	locator := client.CookbookAttachmentLocator(cookbookAttachmentsLocator)
	cookbookAttachments, err := locator.Index(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find cookbook attachments: %s", err)
	}
	return cookbookAttachments
}

func extractCookbooks(client *cm15.Api, cookbookAttachments []*cm15.CookbookAttachment) []*cm15.Cookbook {
	cookbooks := make([]*cm15.Cookbook, len(cookbookAttachments))
	for i := 0; i < len(cookbookAttachments); i++ {
		cookbooks[i] = cookbooksRetrieve(client, extractHref(cookbookAttachments[i].Links, "cookbook"))
	}
	return cookbooks
}

func extractRightScript(client *cm15.Api, rightscriptLocator string) RightScript {
	var rs RightScript
	locator := client.RightScriptLocator(rightscriptLocator)
	rightScript, err := locator.Show()
	if err != nil {
		fmt.Println("failed to find right script: %s", err)
	} else {
		rs = RightScript{
			Name:      rightScript.Name,
			Revision:  rightScript.Revision,
			UpdatedAt: rightScript.UpdatedAt,
		}
	}
	return rs
}

func extractRecipe(recipeName string, cookbooks []*cm15.Cookbook) Recipe {
	var recipe Recipe
	cookbookName := strings.Split(recipeName, "::")[0]
	i := 0
	for ; i < len(cookbooks); i++ {
		if cookbooks[i].Name == cookbookName {
			break
		}
	}
	if i != len(cookbooks) {
		recipe = Recipe{
			Name:     recipeName,
			Cookbook: cookbooks[i].Name,
			Revision: cookbooks[i].Version,
		}
		if cookbooks[i].State == "frozen" {
			recipe.Frozen = true
			recipe.UpdatedAt = cookbooks[i].UpdatedAt
			recipe.FrozenAt = cookbooks[i].UpdatedAt
		} else {
			recipe.Frozen = false
			recipe.UpdatedAt = cookbooks[i].UpdatedAt
		}
	}
	return recipe
}

func instanceRetrieve(client *cm15.Api, instanceLocator string) *cm15.Instance {
	locator := client.InstanceLocator(instanceLocator)
	instance, err := locator.Show(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find instance: %s", err)
	}
	return instance
}

func runnableBindingsRetrieve(client *cm15.Api, runnableBindingLocator string) []*cm15.RunnableBinding {
	locator := client.RunnableBindingLocator(runnableBindingLocator)
	runnableBindings, err := locator.Index(rsapi.ApiParams{})
	if err != nil {
		fmt.Printf("failed to find runnable bindings: %s", err)
	}
	return runnableBindings
}

func extractAttachmentsInfo(client *cm15.Api, runnableBindings []*cm15.RunnableBinding, cookbooks []*cm15.Cookbook) ([]RightScript, []Recipe) {
	var rightScripts []RightScript
	var recipes []Recipe
	for i := 0; i < len(runnableBindings); i++ {
		if runnableBindings[i].Recipe != "" {
			recipes = append(recipes, extractRecipe(runnableBindings[i].Recipe, cookbooks))
		} else {
			rightScripts = append(rightScripts, extractRightScript(client, extractHref(runnableBindings[i].Links, "right_script")))
		}
	}
	return rightScripts, recipes
}

func serversRetrieve(client *cm15.Api, serversLocator string) []Server {
	serverLocator := client.ServerLocator(serversLocator)
	servers, err := serverLocator.Index(rsapi.ApiParams{"view": "instance_detail"})
	if err != nil {
		fmt.Println("failed to find servers: %s", err)
	}
	var serverList = make([]Server, len(servers))
	for i := 0; i < len(servers); i++ {
		nextInstanceLocator := extractHref(servers[i].Links, "next_instance")
		currentInstanceLocator := extractHref(servers[i].Links, "current_instance")
		s := Server{Name: servers[i].Name, Locked: false}
		nextInstance := instanceRetrieve(client, nextInstanceLocator)
		templateLocator := extractHref(nextInstance.Links, "server_template")
		template := templateRetrieve(client, templateLocator)
		s.Template = template.Name
		templates[templateLocator] = template.Name
		s.NextInstance = inputsRetrieve(client, extractHref(nextInstance.Links, "inputs"))
		if currentInstanceLocator != "" {
			templateLocator := instanceRetrieve(client, currentInstanceLocator)
			s.CurrentInstance = inputsRetrieve(client, extractHref(templateLocator.Links, "inputs"))
			s.Locked = templateLocator.Locked
		}
		serverList[i] = s
	}
	return serverList
}

func serverArraysRetrieve(client *cm15.Api, serverArraysLocator string) []ServerArray {
	arrayLocator := client.ServerArrayLocator(serverArraysLocator)
	serverArrays, err := arrayLocator.Index(rsapi.ApiParams{"view": "instance_detail"})
	if err != nil {
		fmt.Println("failed to find servers: %s", err)
	}
	var serverArrayList = make([]ServerArray, len(serverArrays))
	for i := 0; i < len(serverArrays); i++ {
		nextInstanceLocator := extractHref(serverArrays[i].Links, "next_instance")
		currentInstancesLocator := extractHref(serverArrays[i].Links, "current_instances")
		sa := ServerArray{Name: serverArrays[i].Name, Locked: false}
		nextInstance := instanceRetrieve(client, nextInstanceLocator)
		templateLocator := extractHref(nextInstance.Links, "server_template")
		template := templateRetrieve(client, templateLocator)
		sa.Template = template.Name
		templates[templateLocator] = template.Name
		sa.NextInstance = inputsRetrieve(client, extractHref(nextInstance.Links, "inputs"))
		instanceLocator := client.InstanceLocator(currentInstancesLocator)
		instances, err := instanceLocator.Index(rsapi.ApiParams{})
		if err != nil {
			fmt.Println("failed to find instances: %s", err)
		}
		if len(instances) != 0 {
			currentInstanceLocator := extractHref(instances[0].Links, "self")
			currentInstance := instanceRetrieve(client, currentInstanceLocator)
			sa.CurrentInstance = inputsRetrieve(client, extractHref(currentInstance.Links, "inputs"))
			sa.Locked = currentInstance.Locked
		}
		serverArrayList[i] = sa
	}
	return serverArrayList
}

func main() {
	// Retrieve login and endpoint information
	email := flag.String("e", "", "Login email")
	pwd := flag.String("p", "", "Login password")
	account := flag.Int("a", 0, "Account id")
	host := flag.String("h", "us-3.rightscale.com", "RightScale API host")
	insecure := flag.Bool("insecure", false, "Use HTTP instead of HTTPS - used for testing")
	deploymentId := flag.String("d", "", "Deployment id")
	flag.Parse()
	if *email == "" {
		fmt.Println("Login email required")
	}
	if *pwd == "" {
		fmt.Println("Login password required")
	}
	if *account == 0 {
		fmt.Println("Account id required")
	}
	if *host == "" {
		fmt.Println("Host required")
	}
	if *deploymentId == "" {
		fmt.Println("Deployment required")
	}

	// Setup client using basic auth
	auth := rsapi.NewBasicAuthenticator(*email, *pwd, *account)
	client := cm15.New(*host, auth, nil, nil)
	if *insecure {
		client.Insecure()
	}
	if err := client.CanAuthenticate(); err != nil {
		fmt.Println("invalid credentials: %s", err)
	}

	// Deployment show
	deploymentLocator := client.DeploymentLocator("/api/deployments/" + *deploymentId)
	deployment, err := deploymentLocator.Show(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find deployment: %s", err)
	}
	// Initialize the template maps to store only unique server templates
	templates = make(map[string]string)
	serversLocator := extractHref(deployment.Links, "servers")
	servers := serversRetrieve(client, serversLocator)
	serverArraysLocator := extractHref(deployment.Links, "server_arrays")
	serverArrays := serverArraysRetrieve(client, serverArraysLocator)
	var serverTemplates = make([]ServerTemplate, len(templates))
	i := 0
	for key, _ := range templates {
		template := templateRetrieve(client, key)
		st := ServerTemplate{
			Name:     template.Name,
			Revision: template.Revision,
		}
		runnableBindings := runnableBindingsRetrieve(client, extractHref(template.Links, "runnable_bindings"))
		cookbookAttachments := cookbookAttachmentsRetrieve(client, extractHref(template.Links, "cookbook_attachments"))
		cookbooks := extractCookbooks(client, cookbookAttachments)
		st.RightScripts, st.Recipes = extractAttachmentsInfo(client, runnableBindings, cookbooks)
		serverTemplates[i] = st
		i++
	}
	deploymentStruct := Deployment{
		Name:               deployment.Name,
		Inputs:             inputsRetrieve(client, extractHref(deployment.Links, "inputs")),
		Servers:            servers,
		ServersNumber:      len(servers),
		ServerArrays:       serverArrays,
		ServerArraysNumber: len(serverArrays),
		ServerTemplates:    serverTemplates,
	}
	jsonBody, err := json.MarshalIndent(deploymentStruct, "", "    ")
	if err == nil {
		fmt.Println(string(jsonBody))
	}
}
