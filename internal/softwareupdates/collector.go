package softwareupdates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

func NormalizeSource(s Source) (Source, error) {
	u, err := publicURL(s.URL)
	if err != nil {
		return Source{}, err
	}
	s.Software = strings.TrimSpace(s.Software)
	if s.Software == "" || len(s.Software) > 160 {
		return Source{}, fmt.Errorf("%w: software name required", ErrInvalid)
	}
	s.URL = strings.TrimSuffix(u.String(), "/")
	switch s.Kind {
	case "github", "forgejo", "gitlab":
		if s.Kind == "github" && u.Hostname() != "github.com" {
			return Source{}, fmt.Errorf("%w: github.com repository required", ErrInvalid)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return Source{}, fmt.Errorf("%w: repository URL required", ErrInvalid)
		}
		if s.Kind != "gitlab" {
			u.Path = "/" + strings.Join(parts[:2], "/")
			s.URL = strings.TrimSuffix(u.String(), ".git")
		}
	case "changelog":
	default:
		return Source{}, fmt.Errorf("%w: unsupported source kind", ErrInvalid)
	}
	return s, nil
}
func Suggest(raw string) *Source {
	u, err := publicURL(raw)
	if err != nil {
		return nil
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Argus accepte l'adresse d'API d'un dépôt GitHub (…/repos/o/r/releases/latest) :
	// elle désigne le dépôt, pas une page de notes.
	if strings.EqualFold(u.Hostname(), "api.github.com") && len(p) >= 3 && p[0] == "repos" {
		u.Host, p = "github.com", p[1:]
	}
	if len(p) < 2 {
		return nil
	}
	kind := "changelog"
	if u.Hostname() == "github.com" {
		kind = "github"
	} else if strings.Contains(u.Hostname(), "gitlab") {
		kind = "gitlab"
	} else if u.Hostname() == "code.vikunja.io" || strings.Contains(u.Hostname(), "codeberg") {
		kind = "forgejo"
	}
	if kind != "changelog" {
		u.Path = "/" + strings.Join(p[:2], "/")
	}
	s, err := NormalizeSource(Source{Kind: kind, URL: u.String(), Software: p[1]})
	if err != nil {
		return nil
	}
	return &s
}
func Collect(ctx context.Context, client *http.Client, s Source, installed, target string) (Collection, error) {
	c := Collection{Installed: installed, Target: target, Notes: []Note{}}
	cmp, ok := compareVersions(installed, target)
	if !ok {
		return c, fmt.Errorf("versions_not_comparable")
	}
	if cmp >= 0 {
		return c, nil
	}
	var all []Note
	var err error
	if s.Kind == "changelog" {
		all, c.Incomplete, err = collectChangelog(ctx, client, s, installed, target)
	} else {
		all, c.Incomplete, err = collectRepository(ctx, client, s, installed)
	}
	if err != nil {
		return c, err
	}
	seen := map[string]int{}
	targetSeen := false
	for _, n := range all {
		if !inRange(n.Version, installed, target) {
			continue
		}
		v := strings.TrimPrefix(n.Version, "v")
		if index, exists := seen[v]; exists {
			if c.Notes[index].Missing && strings.TrimSpace(n.Body) != "" {
				c.Notes[index] = n
			}
			continue
		}
		seen[v] = len(c.Notes)
		if equal, ok := compareVersions(n.Version, target); ok && equal == 0 {
			targetSeen = true
		}
		n.Missing = strings.TrimSpace(n.Body) == ""
		c.Notes = append(c.Notes, n)
	}
	if !targetSeen {
		c.Notes = append(c.Notes, Note{Version: target, URL: s.URL, Missing: true})
		c.Incomplete = true
	}
	sort.Slice(c.Notes, func(i, j int) bool { v, _ := compareVersions(c.Notes[i].Version, c.Notes[j].Version); return v > 0 })
	return c, nil
}

// olderPage indique qu'une page triée de la plus récente à la plus ancienne ne
// contient que des versions antérieures à l'installation : les pages suivantes
// ne peuvent plus rien apporter. Un ordre inattendu ne permet pas de conclure.
func olderPage(page []string, installed string) bool {
	if len(page) == 0 {
		return false
	}
	for index, version := range page {
		order, ok := compareVersions(version, installed)
		if !ok || order >= 0 {
			return false
		}
		if index > 0 {
			if order, ok := compareVersions(page[index-1], version); !ok || order < 0 {
				return false
			}
		}
	}
	return true
}

func collectRepository(ctx context.Context, client *http.Client, s Source, installed string) ([]Note, bool, error) {
	u, _ := url.Parse(s.URL)
	repo := strings.Trim(u.Path, "/")
	base := "https://" + u.Host
	switch s.Kind {
	case "github":
		base = "https://api.github.com/repos/" + repo
	case "forgejo":
		base += "/api/v1/repos/" + repo
	case "gitlab":
		base += "/api/v4/projects/" + url.PathEscape(repo) + "/repository"
	}
	notes := []Note{}
	incomplete := false
	for _, resource := range []string{"releases", "tags"} {
		// Smaller page sizes divide their predecessor, preserving the offset
		// when a later page exceeds the bounded HTTP response size.
		sizes := []int{100, 50, 25, 5, 1}
		sizeIndex, offset := 0, 0
		for attempt := 0; attempt < 20; attempt++ {
			size := sizes[sizeIndex]
			page := offset/size + 1
			endpoint := base + "/" + resource
			if s.Kind == "gitlab" && resource == "releases" {
				endpoint = strings.TrimSuffix(base, "/repository") + "/releases"
			}
			b, err := request(ctx, client, "GET", fmt.Sprintf("%s?per_page=%d&limit=%d&page=%d", endpoint, size, size, page), "", nil)
			if err != nil {
				if errors.Is(err, errResponseTooLarge) && sizeIndex < len(sizes)-1 && attempt < 19 {
					sizeIndex++
					continue
				}
				if len(notes) == 0 {
					return nil, false, err
				}
				incomplete = true
				break
			}
			var rows []struct {
				Tag         string `json:"tag_name"`
				Name        string `json:"name"`
				URL         string `json:"html_url"`
				Body        string `json:"body"`
				Description string `json:"description"`
				Draft       bool   `json:"draft"`
			}
			if err = json.Unmarshal(b, &rows); err != nil {
				return nil, false, fmt.Errorf("invalid release catalogue")
			}
			listed := make([]string, 0, len(rows))
			for _, r := range rows {
				if r.Draft {
					continue
				}
				v := r.Tag
				if v == "" {
					v = r.Name
				}
				if versionPattern.FindString(v) == "" {
					continue
				}
				listed = append(listed, v)
				body := r.Body
				if body == "" {
					body = r.Description
				}
				link := r.URL
				if link == "" {
					link = s.URL + "/releases/tag/" + url.PathEscape(v)
					if s.Kind == "gitlab" {
						link = s.URL + "/-/releases/" + url.PathEscape(v)
					}
				}
				if _, err := publicURL(link); err != nil {
					link = s.URL
				}
				notes = append(notes, Note{Version: v, URL: link, Body: body})
			}
			offset += size
			if len(rows) < size || olderPage(listed, installed) {
				break
			}
			if attempt == 19 {
				incomplete = true
			}
		}
	}
	return notes, incomplete, nil
}

var embeddedVersion = regexp.MustCompile(`\bv?[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-[0-9A-Za-z.]+)?\b`)
var markdownHeading = regexp.MustCompile(`(?m)^#{1,4}\s+.*$`)

// Changelogs structurés par titres et index HTML vers des notes individuelles.
func collectChangelog(ctx context.Context, client *http.Client, s Source, installed, target string) ([]Note, bool, error) {
	b, err := request(ctx, client, "GET", s.URL, "", nil)
	if err != nil {
		return nil, false, err
	}
	text := string(b)
	notes := []Note{}
	links := map[string]string{}
	if strings.Contains(strings.ToLower(text), "<html") || strings.Contains(strings.ToLower(text), "<!doctype") {
		root, e := html.Parse(strings.NewReader(text))
		if e != nil {
			return nil, false, fmt.Errorf("invalid changelog")
		}
		text = htmlText(root)
		base, _ := url.Parse(s.URL)
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "a" {
				for _, a := range n.Attr {
					if a.Key != "href" {
						continue
					}
					ref, e := url.Parse(a.Val)
					if e != nil {
						continue
					}
					link := base.ResolveReference(ref)
					if link.Host != base.Host {
						continue
					}
					link.Fragment = ""
					if _, e = publicURL(link.String()); e != nil {
						continue
					}
					v := embeddedVersion.FindString(htmlText(n))
					if v == "" {
						v = embeddedVersion.FindString(link.Path)
					}
					if inRange(v, installed, target) {
						links[v] = link.String()
					}
				}
			}
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				walk(ch)
			}
		}
		walk(root)
	}
	headings := markdownHeading.FindAllStringIndex(text, -1)
	for i, h := range headings {
		v := embeddedVersion.FindString(text[h[0]:h[1]])
		if v == "" {
			continue
		}
		end := len(text)
		for j := i + 1; j < len(headings); j++ {
			if embeddedVersion.FindString(text[headings[j][0]:headings[j][1]]) != "" {
				end = headings[j][0]
				break
			}
		}
		notes = append(notes, Note{Version: v, URL: s.URL, Body: strings.TrimSpace(text[h[1]:end])})
	}
	versions := []string{}
	for v := range links {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	incomplete := false
	for i, v := range versions {
		if i >= 100 {
			incomplete = true
			break
		}
		b, e := request(ctx, client, "GET", links[v], "", nil)
		n := Note{Version: v, URL: links[v]}
		if e == nil {
			root, e := html.Parse(strings.NewReader(string(b)))
			if e == nil {
				n.Body = htmlText(root)
			}
		}
		notes = append(notes, n)
	}
	if len(notes) == 0 {
		incomplete = true
	}
	return notes, incomplete, nil
}
func htmlText(root *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "nav", "header", "footer":
				return
			case "h1", "h2", "h3", "h4":
				b.WriteString("\n## ")
			case "p", "div", "li", "section", "article", "br":
				b.WriteString("\n")
			}
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			b.WriteString("\n")
		}
	}
	walk(root)
	return strings.TrimSpace(b.String())
}
