package zabbix

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// triggerEvaluationWindow décrit l'horizon temporel de la condition active,
// indépendamment de la fréquence à laquelle CairnOps la collecte. Seules les
// agrégations à période explicite de l'expression problème sont admissibles.
// Une macro non résolue, un nombre d'échantillons ou un décalage historique ne
// sont jamais interprétés comme une durée. Le cycle applique ensuite sa borne.
func triggerEvaluationWindow(trigger remoteTrigger) time.Duration {
	var window time.Duration
	for _, function := range trigger.Functions {
		if function.FunctionID == "" || !strings.Contains(trigger.Expression, "{"+function.FunctionID+"}") {
			continue
		}
		switch function.Function {
		case "min", "max", "avg", "sum", "count":
		default:
			continue
		}
		parameter := strings.TrimPrefix(function.Parameter, "$,")
		period, _, _ := strings.Cut(parameter, ",")
		window = max(window, literalPeriod(period))
	}
	return window
}

func literalPeriod(value string) time.Duration {
	if value == "" {
		return 0
	}
	unit := time.Second
	switch value[len(value)-1] {
	case 's':
		value = value[:len(value)-1]
	case 'm':
		unit, value = time.Minute, value[:len(value)-1]
	case 'h':
		unit, value = time.Hour, value[:len(value)-1]
	case 'd':
		unit, value = 24*time.Hour, value[:len(value)-1]
	case 'w':
		unit, value = 7*24*time.Hour, value[:len(value)-1]
	}
	count, err := strconv.ParseUint(value, 10, 64)
	if err != nil || count > uint64(math.MaxInt64)/uint64(unit) {
		return 0
	}
	return time.Duration(count) * unit
}
