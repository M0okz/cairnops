package httpapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Le contrat HTTP se lit dans docs/api/openapi.yaml. Rien ne l'exécute : aucun
// générateur, aucun client, aucune étape de CI ne l'ouvrait. Une route pouvait
// donc vivre des mois sans y figurer — c'est arrivé à GET /api/v1/version, la
// route même dont la procédure de déploiement se sert pour prouver qu'un SHA
// attendu est bien celui qui répond.
//
// Ce test rend cette dérive impossible dans les deux sens : une route ajoutée
// sans documentation échoue, une documentation décrivant une route inexistante
// échoue aussi. Il compare les chemins ET les noms de paramètres, parce qu'un
// {targetID} documenté en {id} trompe un lecteur autant qu'un oubli.

var specPathPattern = regexp.MustCompile(`^  (/\S+):\s*$`)
var specMethodPattern = regexp.MustCompile(`^    (get|post|put|patch|delete):`)

func TestOpenAPIDescribesExactlyTheRegisteredRoutes(t *testing.T) {
	registered := registeredRoutes(t)
	documented := documentedRoutes(t)

	// Un analyseur cassé ne doit pas produire une comparaison vide qui passe.
	if len(registered) < 50 {
		t.Fatalf("lecture des routes suspecte : %d trouvées, la lecture du source a dû échouer", len(registered))
	}
	if len(documented) < 50 {
		t.Fatalf("lecture de la spécification suspecte : %d routes trouvées, la lecture du YAML a dû échouer", len(documented))
	}

	for _, route := range registered {
		if !slices.Contains(documented, route) {
			t.Errorf("route servie mais absente de docs/api/openapi.yaml : %s", route)
		}
	}
	for _, route := range documented {
		if !slices.Contains(registered, route) {
			t.Errorf("route documentée dans docs/api/openapi.yaml mais jamais servie : %s", route)
		}
	}
}

// registeredRoutes lit les littéraux confiés au multiplexeur dans le source du
// paquet, plutôt que d'interroger le http.ServeMux : celui-ci n'expose pas sa
// table, et seul le source dit ce que le serveur promet.
func registeredRoutes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("lecture du répertoire du paquet : %v", err)
	}
	fileSet := token.NewFileSet()
	var routes []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, name, nil, 0)
		if err != nil {
			t.Fatalf("lecture de %s : %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Handle" && selector.Sel.Name != "HandleFunc") {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			pattern, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			method, path, found := strings.Cut(pattern, " ")
			if !found || !strings.HasPrefix(path, "/api/") {
				return true
			}
			switch method {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
				route := method + " " + path
				if !slices.Contains(routes, route) {
					routes = append(routes, route)
				}
			}
			return true
		})
	}
	slices.Sort(routes)
	return routes
}

// documentedRoutes s'appuie sur l'indentation de la spécification : un chemin
// tient deux espaces, ses méthodes quatre. Un document reformaté fera échouer
// le contrôle de vraisemblance plus haut au lieu de passer en silence.
func documentedRoutes(t *testing.T) []string {
	t.Helper()
	content, err := os.ReadFile("../../docs/api/openapi.yaml")
	if err != nil {
		t.Fatalf("lecture de la spécification : %v", err)
	}
	var routes []string
	var path string
	inPaths := false
	for _, line := range strings.Split(string(content), "\n") {
		if line == "paths:" {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		if match := specPathPattern.FindStringSubmatch(line); match != nil {
			path = match[1]
			continue
		}
		if match := specMethodPattern.FindStringSubmatch(line); match != nil && path != "" {
			routes = append(routes, strings.ToUpper(match[1])+" "+path)
		}
	}
	slices.Sort(routes)
	return routes
}
