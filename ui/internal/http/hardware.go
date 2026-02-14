package webhttp

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	"sigs.k8s.io/yaml"
)

const (
	nameSingularHardware = "Hardware"
	namePluralHardware   = "Hardware"
)

// GetHardwareList fetches and returns paginated hardware data.
func GetHardwareList(c *gin.Context, log logr.Logger) ([]string, templates.HardwarePageData) {
	ctx := c.Request.Context()
	client, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.V(1).Info("Failed to get Kubernetes client from context", "error", err)
		if HandleAuthError(c, err, log) {
			return nil, templates.HardwarePageData{}
		}
		return nil, templates.HardwarePageData{}
	}
	namespaces := GetKubeNamespaces(ctx, c, client, log)
	selectedNamespace := GetSelectedNamespace(c, namespaces)

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	itemsPerPageStr := c.DefaultQuery("per_page", strconv.Itoa(DefaultItemsPerPage))
	itemsPerPage, _ := strconv.Atoi(itemsPerPageStr)
	itemsPerPage = ValidateItemsPerPage(itemsPerPage)

	var hardware []templates.Hardware

	hardwareList, err := client.ListHardware(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list "+namePluralHardware, "error", err)
		if HandleAuthError(c, err, log) {
			return namespaces, templates.HardwarePageData{}
		}
		return namespaces, GetPaginatedHardware(hardware, page, itemsPerPage)
	}

	for _, hw := range hardwareList.Items {
		mac := "N/A"
		ip := "N/A"
		if len(hw.Spec.Interfaces) > 0 && hw.Spec.Interfaces[0].DHCP != nil {
			mac = hw.Spec.Interfaces[0].DHCP.MAC
			if hw.Spec.Interfaces[0].DHCP.IP != nil {
				ip = hw.Spec.Interfaces[0].DHCP.IP.Address
			}
		}
		webHW := templates.Hardware{
			Name:        hw.Name,
			Namespace:   hw.Namespace,
			MAC:         mac,
			IPv4Address: ip,
			CreatedAt:   hw.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
		}
		hardware = append(hardware, webHW)
	}

	return namespaces, GetPaginatedHardware(hardware, page, itemsPerPage)
}

// HandleHome handles the home page route.
func HandleHome(c *gin.Context, log logr.Logger) {
	namespaces, hardwarePageData := GetHardwareList(c, log)

	if IsHTMXRequest(c) {
		component := templates.HardwareTableContent(hardwarePageData)
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.Homepage(cfg, hardwarePageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleHardwareList handles the hardware list page route.
func HandleHardwareList(c *gin.Context, log logr.Logger) {
	namespaces, hardwarePageData := GetHardwareList(c, log)

	if IsHTMXRequest(c) {
		component := templates.HardwareTableContent(hardwarePageData)
		c.Header("Content-Type", "text/html")
		RenderComponent(c.Request.Context(), c.Writer, component, log)
		return
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.Homepage(cfg, hardwarePageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleHardwareData handles the hardware data endpoint (HTMX partial).
func HandleHardwareData(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	selectedNamespace := c.Query("namespace")

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	itemsPerPageStr := c.DefaultQuery("per_page", strconv.Itoa(DefaultItemsPerPage))
	itemsPerPage, _ := strconv.Atoi(itemsPerPageStr)
	itemsPerPage = ValidateItemsPerPage(itemsPerPage)

	var hardware []templates.Hardware

	client, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.V(1).Info("Failed to get Kubernetes client from context", "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		c.Status(500)
		return
	}

	hardwareList, err := client.ListHardware(ctx, selectedNamespace)
	if err != nil {
		log.V(1).Info("Failed to list "+namePluralHardware, "error", err)
	} else {
		for _, hw := range hardwareList.Items {
			mac := "N/A"
			ip := "N/A"
			if len(hw.Spec.Interfaces) > 0 {
				if hw.Spec.Interfaces[0].DHCP != nil {
					mac = hw.Spec.Interfaces[0].DHCP.MAC
					if hw.Spec.Interfaces[0].DHCP.IP != nil {
						ip = hw.Spec.Interfaces[0].DHCP.IP.Address
					}
				}
			}
			webHW := templates.Hardware{
				Name:        hw.Name,
				Namespace:   hw.Namespace,
				MAC:         mac,
				IPv4Address: ip,
				CreatedAt:   hw.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
			}
			hardware = append(hardware, webHW)
		}
	}

	hardwarePageData := GetPaginatedHardware(hardware, page, itemsPerPage)

	component := templates.HardwareTableContent(hardwarePageData)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}

// HandleHardwareDetail handles the hardware detail page route.
func HandleHardwareDetail(c *gin.Context, log logr.Logger) {
	ctx := c.Request.Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	client, err := GetKubeClientFromGinContext(c)
	if err != nil {
		log.V(1).Info("Failed to get Kubernetes client from context", "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		c.Status(500)
		return
	}
	namespaces := GetKubeNamespaces(ctx, c, client, log)

	hw, err := client.GetHardware(ctx, namespace, name)
	if err != nil {
		log.V(1).Info("Failed to fetch "+nameSingularHardware, "namespace", namespace, "name", name, "error", err)
		if HandleAuthError(c, err, log) {
			return
		}
		cfg := templates.PageConfig{
			BaseURL:    GetBaseURL(c),
			Namespaces: namespaces,
		}
		component := templates.NotFoundPage(cfg, nameSingularHardware, name, namespace, "/hardware", namePluralHardware, fmt.Sprintf("This %s may have been deleted.", nameSingularHardware))
		c.Header("Content-Type", "text/html")
		c.Status(404)
		RenderComponent(ctx, c.Writer, component, log)
		return
	}

	yamlBytes, err := yaml.Marshal(hw)
	if err != nil {
		log.V(1).Info("Failed to marshal "+nameSingularHardware+" to YAML", "namespace", namespace, "name", name, "error", err)
		c.Status(500)
		return
	}
	specYAML, err := yaml.Marshal(&hw.Spec)
	if err != nil {
		log.V(1).Info("Failed to marshal "+nameSingularHardware+" spec to YAML", "namespace", namespace, "name", name, "error", err)
		c.Status(500)
		return
	}
	statusYAML, err := yaml.Marshal(&hw.Status)
	if err != nil {
		log.V(1).Info("Failed to marshal "+nameSingularHardware+" status to YAML", "namespace", namespace, "name", name, "error", err)
		c.Status(500)
		return
	}

	var agentAttrs *templates.AgentAttributes
	if attrJSON, ok := hw.Annotations["tinkerbell.org/agent-attributes"]; ok && attrJSON != "" {
		agentAttrs = &templates.AgentAttributes{}
		if err := json.Unmarshal([]byte(attrJSON), agentAttrs); err != nil {
			log.Error(err, "Failed to parse agent-attributes", "namespace", namespace, "name", name)
			agentAttrs = nil
		}
	}

	hwDetail := templates.HardwareDetail{
		Name:            hw.Name,
		Namespace:       hw.Namespace,
		Interfaces:      GetHardwareInterfaces(*hw),
		Status:          GetHardwareStatus(*hw),
		CreatedAt:       hw.GetCreationTimestamp().Format("2006-01-02 15:04:05"),
		Labels:          hw.Labels,
		Annotations:     hw.Annotations,
		AgentAttributes: agentAttrs,
		SpecYAML:        string(specYAML),
		StatusYAML:      string(statusYAML),
		YAML:            string(yamlBytes),
	}

	cfg := templates.PageConfig{
		BaseURL:    GetBaseURL(c),
		Namespaces: namespaces,
	}
	component := templates.HardwareDetailPage(cfg, hwDetail)
	c.Header("Content-Type", "text/html")
	RenderComponent(c.Request.Context(), c.Writer, component, log)
}
