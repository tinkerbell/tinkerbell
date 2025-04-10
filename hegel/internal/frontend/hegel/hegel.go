//go:build ignore

package hegel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/packethost/pkg/log"
)

type metadataJSON struct {
	Interfaces []K8sNetworkInterface `json:"interfaces,omitempty"`
	Disks      []K8sHardwareDisk     `json:"disks,omitempty"`
	SSHKeys    []string              `json:"ssh_keys,omitempty"`
	Hostname   string                `json:"hostname,omitempty"`
	Gateway    string                `json:"gateway,omitempty"`
}

func v0HegelMetadataHandler(logger log.Logger, client Client, rg *gin.RouterGroup) {
	userdata := rg.Group("/user-data")
	userdata.GET("", userdataHandler(logger, client))

	vendordata := rg.Group("/vendor-data")
	vendordata.GET("", vendordataHandler(logger, client))

	metadata := rg.Group("/meta-data")
	metadata.GET("", metadataHandler(logger, client))

	metadata.GET("/disks", diskHandler(logger, client))
	metadata.GET("/disks/:index", diskIndexHandler(logger, client))

	metadata.GET("/ssh-public-keys", sshHandler(logger, client))
	metadata.GET("/ssh-public-keys/:index", sshIndexHandler(logger, client))

	metadata.GET("/hostname", hostnameHandler(logger, client))
	metadata.GET("/gateway", gatewayHandler(logger, client))

	metadata.GET("/:mac", macHandler(logger, client))
	metadata.GET("/:mac/ipv4", ipv4Handler(logger, client))
	metadata.GET("/:mac/ipv4/:index", ipv4IndexHandler(logger, client))
	metadata.GET("/:mac/ipv4/:index/ip", ipv4IPHandler(logger, client))
	metadata.GET("/:mac/ipv4/:index/netmask", ipv4NetmaskHandler(logger, client))
	metadata.GET("/:mac/ipv6", ipv6Handler(logger, client))
	metadata.GET("/:mac/ipv6/:index", ipv6IndexHandler(logger, client))
	metadata.GET("/:mac/ipv6/:index/ip", ipv6IPHandler(logger, client))
	metadata.GET("/:mac/ipv6/:index/netmask", ipv6NetmaskHandler(logger, client))
}

func getHardware(ctx context.Context, client Client, ip string) (K8sHardware, error) {
	hw, err := client.ByIP(ctx, ip)
	if err != nil {
		return K8sHardware{}, err
	}

	ehw, err := hw.Export()
	if err != nil {
		return K8sHardware{}, err
	}

	var reversed K8sHardware
	if err := json.Unmarshal(ehw, &reversed); err != nil {
		return K8sHardware{}, err
	}
	return reversed, nil
}

func userdataHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		data := hardwareData.Metadata.Userdata
		if data == nil {
			c.String(http.StatusOK, "")
		} else {
			c.String(http.StatusOK, *data+"\n")
		}
	}
	return gin.HandlerFunc(fn)
}

func vendordataHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		data := hardwareData.Metadata.Vendordata
		if data == nil {
			c.String(http.StatusOK, "")
		} else {
			c.String(http.StatusOK, *data+"\n")
		}
	}
	return gin.HandlerFunc(fn)
}

func metadataHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		var acceptJSON bool
		for _, header := range c.Request.Header["Accept"] {
			if header == "application/json" {
				acceptJSON = true
			}
		}
		if acceptJSON {
			hardwareData, err := getHardware(c, client, c.ClientIP())
			if err != nil {
				logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
				c.JSON(http.StatusNotFound, nil)
				return
			}
			c.Header("Content-Type", "application/json")
			var jsonResponse metadataJSON
			jsonResponse.Disks = hardwareData.Metadata.Instance.Disks
			jsonResponse.Gateway = hardwareData.Metadata.Gateway
			jsonResponse.Hostname = hardwareData.Metadata.Instance.Hostname
			jsonResponse.SSHKeys = hardwareData.Metadata.Instance.SSHKeys
			jsonResponse.Interfaces = hardwareData.Metadata.Interfaces
			c.IndentedJSON(http.StatusOK, jsonResponse)
		} else {
			c.String(http.StatusOK, "disks\nssh-public-keys\ngateway\nhostname\n:mac\n")
		}
	}
	return gin.HandlerFunc(fn)
}

func diskHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		disk := hardwareData.Metadata.Instance.Disks
		for i := 0; i < len(disk); i++ {
			c.String(http.StatusOK, fmt.Sprintln(i))
		}
	}
	return gin.HandlerFunc(fn)
}

func diskIndexHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		index, err := strconv.Atoi(c.Param("index"))
		if err != nil {
			logger.With("error", err).Info("disk interface index is not a valid number")
			c.JSON(http.StatusBadRequest, nil)
			return
		}
		disksArray := hardwareData.Metadata.Instance.Disks
		if index >= 0 && index < len(disksArray) {
			disk := hardwareData.Metadata.Instance.Disks[index].Device
			c.String(http.StatusOK, disk+"\n")
		} else {
			c.JSON(http.StatusBadRequest, nil)
		}
	}
	return gin.HandlerFunc(fn)
}

func sshHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		sshKeys := hardwareData.Metadata.Instance.SSHKeys
		for i := 0; i < len(sshKeys); i++ {
			c.String(http.StatusOK, fmt.Sprintln(i))
		}
	}
	return gin.HandlerFunc(fn)
}

func sshIndexHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		index, err := strconv.Atoi(c.Param("index"))
		if err != nil {
			logger.With("error", err).Info("disk interface index is not a valid number")
			c.JSON(http.StatusBadRequest, nil)
			return
		}
		sshKeys := hardwareData.Metadata.Instance.SSHKeys
		if index >= 0 && index < len(sshKeys) {
			ssh := hardwareData.Metadata.Instance.SSHKeys[index]
			c.String(http.StatusOK, ssh+"\n")
		} else {
			c.String(http.StatusBadRequest, "")
		}
	}
	return gin.HandlerFunc(fn)
}

func hostnameHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		hostname := hardwareData.Metadata.Instance.Hostname
		c.String(http.StatusOK, hostname+"\n")
	}
	return gin.HandlerFunc(fn)
}

func gatewayHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		gateway := hardwareData.Metadata.Gateway
		c.String(http.StatusOK, gateway+"\n")
	}
	return gin.HandlerFunc(fn)
}

func macHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		hardwareData, err := getHardware(c, client, c.ClientIP())
		if err != nil {
			logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
			c.JSON(http.StatusNotFound, nil)
			return
		}
		mac := c.Param("mac")
		networkInterfaces := hardwareData.Metadata.Interfaces
		var validInterface *K8sNetworkInterface
		for i := range networkInterfaces {
			if mac == networkInterfaces[i].MAC {
				validInterface = &networkInterfaces[i]
			}
		}
		if validInterface == nil {
			c.String(http.StatusNoContent, "")
		} else {
			c.String(http.StatusOK, "ipv4\nipv6\n")
		}
	}
	return gin.HandlerFunc(fn)
}

func ipv4Handler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		validInterfaces := getValidNetworkInterfaces(logger, client, c)
		if len(validInterfaces) == 0 {
			c.String(http.StatusNoContent, "")
		} else {
			i := 0
			for _, v := range validInterfaces {
				if v.Family == 4 {
					c.String(http.StatusOK, fmt.Sprintln(i))
					i++
				}
			}
			if i == 0 {
				c.String(http.StatusNoContent, "")
			}
		}
	}
	return gin.HandlerFunc(fn)
}

func ipv4IndexHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		validInterfaces := getValidNetworkInterfaces(logger, client, c)
		if len(validInterfaces) == 0 {
			c.String(http.StatusNoContent, "")
		} else {
			ipv4Networks := getValidAddressFamilyInterfaces(4, validInterfaces)
			if len(ipv4Networks) == 0 {
				c.String(http.StatusNoContent, "")
			} else {
				c.String(http.StatusOK, "ip\nnetmask\n")
			}
		}
	}
	return gin.HandlerFunc(fn)
}

func ipv4IPHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		validInterfaces := getValidNetworkInterfaces(logger, client, c)
		if len(validInterfaces) == 0 {
			c.String(http.StatusNoContent, "")
		} else {
			index, err := strconv.Atoi(c.Param("index"))
			if err != nil {
				logger.With("error", err).Info("ipv4 interface index is not a valid number")
				c.JSON(http.StatusBadRequest, nil)
				return
			}
			ipv4Networks := getValidAddressFamilyInterfaces(4, validInterfaces)
			if len(ipv4Networks) == 0 || index < 0 || index >= len(ipv4Networks) {
				c.String(http.StatusNoContent, "")
			} else {
				ip := ipv4Networks[index].Address
				c.String(http.StatusOK, ip+"\n")
			}
		}
	}
	return gin.HandlerFunc(fn)
}

func ipv4NetmaskHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		validInterfaces := getValidNetworkInterfaces(logger, client, c)
		if len(validInterfaces) == 0 {
			c.String(http.StatusNoContent, "")
		} else {
			index, err := strconv.Atoi(c.Param("index"))
			if err != nil {
				logger.With("error", err).Info("ipv4 interface index is not a valid number")
				c.JSON(http.StatusBadRequest, nil)
				return
			}
			ipv4Networks := getValidAddressFamilyInterfaces(4, validInterfaces)
			if len(ipv4Networks) == 0 || index < 0 || index >= len(ipv4Networks) {
				c.String(http.StatusNoContent, "")
			} else {
				netmask := ipv4Networks[index].Netmask
				c.String(http.StatusOK, netmask+"\n")
			}
		}
	}
	return gin.HandlerFunc(fn)
}

func ipv6Handler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		validInterfaces := getValidNetworkInterfaces(logger, client, c)
		if len(validInterfaces) == 0 {
			c.String(http.StatusNoContent, "")
		} else {
			i := 0
			for _, v := range validInterfaces {
				if v.Family == 6 {
					c.String(http.StatusOK, fmt.Sprintln(i))
					i++
				}
			}
			if i == 0 {
				c.String(http.StatusNoContent, "")
			}
		}
	}
	return gin.HandlerFunc(fn)
}

func ipv6IndexHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		validInterfaces := getValidNetworkInterfaces(logger, client, c)
		if len(validInterfaces) == 0 {
			c.String(http.StatusNoContent, "")
		} else {
			ipv6Networks := getValidAddressFamilyInterfaces(6, validInterfaces)
			if len(ipv6Networks) == 0 {
				c.String(http.StatusNoContent, "")
			} else {
				c.String(http.StatusOK, "ip\nnetmask\n")
			}
		}
	}
	return gin.HandlerFunc(fn)
}

func ipv6IPHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		validInterfaces := getValidNetworkInterfaces(logger, client, c)
		if len(validInterfaces) == 0 {
			c.String(http.StatusNoContent, "")
		} else {
			index, err := strconv.Atoi(c.Param("index"))
			if err != nil {
				logger.With("error", err).Info("ipv6 interface index is not a valid number")
				c.JSON(http.StatusBadRequest, nil)
				return
			}
			ipv6Networks := getValidAddressFamilyInterfaces(6, validInterfaces)
			if len(ipv6Networks) == 0 || index < 0 || index >= len(ipv6Networks) {
				c.String(http.StatusNoContent, "")
			} else {
				ip := ipv6Networks[index].Address
				c.String(http.StatusOK, ip+"\n")
			}
		}
	}
	return gin.HandlerFunc(fn)
}

func ipv6NetmaskHandler(logger log.Logger, client Client) gin.HandlerFunc {
	fn := func(c *gin.Context) {
		validInterfaces := getValidNetworkInterfaces(logger, client, c)
		if len(validInterfaces) == 0 {
			c.String(http.StatusNoContent, "")
		} else {
			index, err := strconv.Atoi(c.Param("index"))
			if err != nil {
				logger.With("error", err).Info("ipv6 interface index is not a valid number")
				c.JSON(http.StatusBadRequest, nil)
				return
			}
			ipv6Networks := getValidAddressFamilyInterfaces(6, validInterfaces)
			if len(ipv6Networks) == 0 || index < 0 || index >= len(ipv6Networks) {
				c.String(http.StatusNoContent, "")
			} else {
				netmask := ipv6Networks[index].Netmask
				c.String(http.StatusOK, netmask+"\n")
			}
		}
	}
	return gin.HandlerFunc(fn)
}

func getValidNetworkInterfaces(logger log.Logger, client Client, c *gin.Context) []K8sNetworkInterface {
	hardwareData, err := getHardware(c, client, c.ClientIP())
	if err != nil {
		logger.With("error", err).Info("failed to get hardware in v0 metadata handler")
		c.JSON(http.StatusNotFound, nil)
		return nil
	}
	mac := c.Param("mac")
	networkInterfaces := hardwareData.Metadata.Interfaces
	var validInterfaces []K8sNetworkInterface
	for _, networkInterface := range networkInterfaces {
		if mac == networkInterface.MAC {
			validInterfaces = append(validInterfaces, networkInterface)
		}
	}
	return validInterfaces
}

func getValidAddressFamilyInterfaces(addressFamily int64, validInterfaces []K8sNetworkInterface) []K8sNetworkInterface {
	var ipvNetworks []K8sNetworkInterface
	for _, v := range validInterfaces {
		if v.Family == addressFamily {
			ipvNetworks = append(ipvNetworks, v)
		}
	}
	return ipvNetworks
}
