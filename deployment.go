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
	for _, link_map := range links {
		if link_map["rel"] == rel {
			return link_map["href"]
		} else {
			continue
		}
	}
	return ""
}

func inputs_retrieve(client *cm15.Api, inputs_locator string) []Input {
	inpl := client.InputLocator(inputs_locator)
	inputs, err := inpl.Index(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find inputs: %s", err)
	}
	var inputsRetrieved = make([]Input, len(inputs))
	for index, inp := range inputs {
		inputsRetrieved[index] = Input{Name: inp.Name, Value: inp.Value}
	}
	return inputsRetrieved
}

func template_retrieve(client *cm15.Api, template_locator string) *cm15.ServerTemplate {
	tl := client.ServerTemplateLocator(template_locator)
	template, err := tl.Show(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find server template: %s", err)
	}
	return template
}

func cookbooks_retrieve(client *cm15.Api, cookbook_locator string) *cm15.Cookbook {
	cbl := client.CookbookLocator(cookbook_locator)
	cookbook, err := cbl.Show(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find cookbook: %s", err)
	}
	return cookbook
}

func cookbook_attachments_retrieve(client *cm15.Api, cookbook_attachments_locator string) []*cm15.CookbookAttachment {
	cookbookAttachmentLocator := client.CookbookAttachmentLocator(cookbook_attachments_locator)
	cookbookAttachments, err := cookbookAttachmentLocator.Index(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find cookbook attachments: %s", err)
	}
	return cookbookAttachments
}

func extractCookbooks(client *cm15.Api, cookbook_attachments []*cm15.CookbookAttachment) []*cm15.Cookbook {
	cookbooks := make([]*cm15.Cookbook, len(cookbook_attachments))
	for i := 0; i < len(cookbook_attachments); i++ {
		cookbooks[i] = cookbooks_retrieve(client, extractHref(cookbook_attachments[i].Links, "cookbook"))
	}
	return cookbooks
}

func extractRightScript(client *cm15.Api, rightscript_locator string) RightScript {
	var rs RightScript
	rightScriptLocator := client.RightScriptLocator(rightscript_locator)
	rightScript, err := rightScriptLocator.Show()
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

func extractRecipe(recipe_name string, cookbooks []*cm15.Cookbook) Recipe {
	var recipe Recipe
	cookbook_name := strings.Split(recipe_name, "::")[0]
	i := 0
	for ; i < len(cookbooks); i++ {
		if cookbooks[i].Name == cookbook_name {
			break
		}
	}
	if i != len(cookbooks) {
		recipe = Recipe{
			Name:     recipe_name,
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

func instance_retrieve(client *cm15.Api, instance_locator string) *cm15.Instance {
	il := client.InstanceLocator(instance_locator)
	instance, err := il.Show(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find instance: %s", err)
	}
	return instance
}

func runnable_binding_retieve(client *cm15.Api, runnable_binding_locator string) []*cm15.RunnableBinding {
	rbl := client.RunnableBindingLocator(runnable_binding_locator)
	runnablebindings, err := rbl.Index(rsapi.ApiParams{})
	if err != nil {
		fmt.Printf("failed to find runnable bindings: %s", err)
	}
	return runnablebindings
}

func extractAttachmentsInfo(client *cm15.Api, runnable_bindings []*cm15.RunnableBinding, cookbooks []*cm15.Cookbook) ([]RightScript, []Recipe) {
	rightScriptCount, recipeCount := 0, 0
	for i := 0; i < len(runnable_bindings); i++ {
		if runnable_bindings[i].Recipe != "" {
			recipeCount++
		} else {
			rightScriptCount++
		}
	}
	rightScripts := make([]RightScript, rightScriptCount)
	recipes := make([]Recipe, recipeCount)
	rightScriptCount, recipeCount = 0, 0
	for i := 0; i < len(runnable_bindings); i++ {
		if runnable_bindings[i].Recipe != "" {
			recipes[recipeCount] = extractRecipe(runnable_bindings[i].Recipe, cookbooks)
			recipeCount++
		} else {
			rightScripts[rightScriptCount] = extractRightScript(client, extractHref(runnable_bindings[i].Links, "right_script"))
			rightScriptCount++
		}
	}
	return rightScripts, recipes
}

func servers_retrieve(client *cm15.Api, servers_locator string) []Server {
	sl := client.ServerLocator(servers_locator)
	servers, err := sl.Index(rsapi.ApiParams{"view": "instance_detail"})
	if err != nil {
		fmt.Println("failed to find servers: %s", err)
	}
	var server_list = make([]Server, len(servers))
	for i := 0; i < len(servers); i++ {
		next_instance_locator := extractHref(servers[i].Links, "next_instance")
		current_instance_locator := extractHref(servers[i].Links, "current_instance")
		s := Server{Name: servers[i].Name, Locked: false}
		next_instance := instance_retrieve(client, next_instance_locator)
		template_locator := extractHref(next_instance.Links, "server_template")
		template := template_retrieve(client, template_locator)
		s.Template = template.Name
		templates[template_locator] = template.Name
		s.NextInstance = inputs_retrieve(client, extractHref(next_instance.Links, "inputs"))
		if current_instance_locator != "" {
			current_instance := instance_retrieve(client, current_instance_locator)
			s.CurrentInstance = inputs_retrieve(client, extractHref(current_instance.Links, "inputs"))
			s.Locked = current_instance.Locked
		}
		server_list[i] = s
	}
	return server_list
}

func server_arrays_retrieve(client *cm15.Api, servers_locator string) []ServerArray {
	sal := client.ServerArrayLocator(servers_locator)
	serverarrays, err := sal.Index(rsapi.ApiParams{"view": "instance_detail"})
	if err != nil {
		fmt.Println("failed to find servers: %s", err)
	}
	var server_array_list = make([]ServerArray, len(serverarrays))
	for i := 0; i < len(serverarrays); i++ {
		next_instance_locator := extractHref(serverarrays[i].Links, "next_instance")
		current_instances_locator := extractHref(serverarrays[i].Links, "current_instances")
		sa := ServerArray{Name: serverarrays[i].Name, Locked: false}
		next_instance := instance_retrieve(client, next_instance_locator)
		template_locator := extractHref(next_instance.Links, "server_template")
		template := template_retrieve(client, template_locator)
		sa.Template = template.Name
		templates[template_locator] = template.Name
		sa.NextInstance = inputs_retrieve(client, extractHref(next_instance.Links, "inputs"))
		il := client.InstanceLocator(current_instances_locator)
		instances, err := il.Index(rsapi.ApiParams{})
		if err != nil {
			fmt.Println("failed to find instances: %s", err)
		}
		if len(instances) != 0 {
			current_instance_locator := extractHref(instances[0].Links, "self")
			current_instance := instance_retrieve(client, current_instance_locator)
			sa.CurrentInstance = inputs_retrieve(client, extractHref(current_instance.Links, "inputs"))
			sa.Locked = current_instance.Locked
		}
		server_array_list[i] = sa
	}
	return server_array_list
}

func main() {
	// 1. Retrieve login and endpoint information
	email := flag.String("e", "", "Login email")
	pwd := flag.String("p", "", "Login password")
	account := flag.Int("a", 0, "Account id")
	host := flag.String("h", "us-3.rightscale.com", "RightScale API host")
	insecure := flag.Bool("insecure", false, "Use HTTP instead of HTTPS - used for testing")
	deployment_id := flag.String("d", "", "Deployment id")
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
	if *deployment_id == "" {
		fmt.Println("Deployment required")
	}

	// 2. Setup client using basic auth
	auth := rsapi.NewBasicAuthenticator(*email, *pwd, *account)
	client := cm15.New(*host, auth, nil, nil)
	if *insecure {
		client.Insecure()
	}
	if err := client.CanAuthenticate(); err != nil {
		fmt.Println("invalid credentials: %s", err)
	}

	//3. Deployment show
	// 3. Make cloud index call using extended view
	d := client.DeploymentLocator("/api/deployments/" + *deployment_id)
	deplymnt, err := d.Show(rsapi.ApiParams{})
	if err != nil {
		fmt.Println("failed to find deployment: %s", err)
	}
	templates = make(map[string]string)
	servers_locator := extractHref(deplymnt.Links, "servers")
	srvs := servers_retrieve(client, servers_locator)
	server_arrays_locator := extractHref(deplymnt.Links, "server_arrays")
	sarrays := server_arrays_retrieve(client, server_arrays_locator)
	var server_templates = make([]ServerTemplate, len(templates))
	i := 0
	for key, _ := range templates {
		tmplt := template_retrieve(client, key)
		st := ServerTemplate{
			Name:     tmplt.Name,
			Revision: tmplt.Revision,
		}
		rbindings := runnable_binding_retieve(client, extractHref(tmplt.Links, "runnable_bindings"))
		cookbook_attachments := cookbook_attachments_retrieve(client, extractHref(tmplt.Links, "cookbook_attachments"))
		cookbooks := extractCookbooks(client, cookbook_attachments)
		st.RightScripts, st.Recipes = extractAttachmentsInfo(client, rbindings, cookbooks)
		server_templates[i] = st
		i++
	}
	deployment_struct := &Deployment{
		Name:               deplymnt.Name,
		Inputs:             inputs_retrieve(client, extractHref(deplymnt.Links, "inputs")),
		Servers:            srvs,
		ServersNumber:      len(srvs),
		ServerArrays:       sarrays,
		ServerArraysNumber: len(sarrays),
		ServerTemplates:    server_templates,
	}
	b, err := json.MarshalIndent(deployment_struct, "", "    ")
	if err == nil {
		fmt.Println(string(b))
	}
}
