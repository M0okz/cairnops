package zabbix

import (
	"regexp"
	"strings"
)

// Ces identités appartiennent aux prototypes Linux officiels Zabbix 7.0,
// agent passif et actif. Vérifier aussi la condition protège les templates
// modifiés qui auraient conservé leur UUID. Le nom et la description ne
// participent jamais à la reconnaissance.
// Références : templates/os/linux et templates/os/linux_active dans
// https://github.com/zabbix/zabbix/tree/release/7.0/templates/os
func standardTriggerNature(trigger remoteTrigger) string {
	switch strings.ToLower(trigger.UUID) {
	case "eb6230f786d04b658ce62c30a9309a34", "fd5732c3cf5249f9a05e3b6cedc2d2fd":
		if storageLatencyCondition(trigger) {
			return "storage.latency"
		}
	}
	return ""
}

var latencyFunction = regexp.MustCompile(`min\(/[^/]+/vfs\.dev\.(read|write)\.await\[\{#DEVNAME\}\],15m\)`)

func storageLatencyCondition(trigger remoteTrigger) bool {
	expression := strings.Join(strings.Fields(trigger.Expression), "")
	// Les versions qui rendent des identifiants de fonctions sont traduites
	// uniquement avec les items et fonctions effectivement décrits par l'API.
	for _, function := range trigger.Functions {
		if function.FunctionID == "" || function.Function != "min" || (function.Parameter != "15m" && function.Parameter != "$,15m") {
			continue
		}
		for _, item := range trigger.Items {
			if item.ItemID != function.ItemID {
				continue
			}
			for _, direction := range []string{"read", "write"} {
				if item.Key == "vfs.dev."+direction+".await[{#DEVNAME}]" {
					expression = strings.ReplaceAll(expression, "{"+function.FunctionID+"}", "min("+direction+",15m)")
				}
			}
		}
	}
	expression = latencyFunction.ReplaceAllString(expression, "min(${1},15m)")
	return expression == `min(read,15m)>{$VFS.DEV.READ.AWAIT.WARN:"{#DEVNAME}"}ormin(write,15m)>{$VFS.DEV.WRITE.AWAIT.WARN:"{#DEVNAME}"}`
}
