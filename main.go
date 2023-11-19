package main

/**
To use this, make a new markdown file. For convention name the file the same as the slug. The md file should be in /markdown

To see more detailed instructions, see my blog at  https://fluxsec.red, or reach out to me
on twitter https://twitter.com/0xfluxsec.

The .md file must then have the following attributes, including the 3 lines ---
those lines separate the tags from the content:

Title: Page title, and title in left sidebar
Slug: slug-of-url
Parent: The name you wish the parent series to be called
Order: number in terms of parent order
Description: Small strap-line description which appears under the title
MetaPropertyTitle: Title for social sharing
MetaDescription: Description ~ 150 - 200 words of the page for SEO.
MetaPropertyDescription: SHORT description for social media sharing.
MetaOgURL: https://www.fluxsec.red/slug-of-url
---
Content goes here

Additional downloads:
* FontAwesome free, into /static/

*/

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type BlogPost struct {
	Title                   string
	Slug                    string
	Parent                  string
	Content                 template.HTML
	Description             string
	Order                   int
	Headers                 []string // these are the in page h2 tags
	MetaDescription         string
	MetaPropertyTitle       string
	MetaPropertyDescription string
	MetaOgURL               string
}

type SidebarData struct {
	Categories []Category
}

type Category struct {
	Name  string
	Pages []BlogPost
	Order int
}

var BaseURL = "http://localhost:8080"

func main() {
	gin.SetMode(gin.ReleaseMode)

	r := gin.Default()

	// sidebar data
	sidebarData, err := loadSidebarData("./markdown")
	if err != nil {
		log.Fatal(err) // I think this one is ok
	}

	// register the sidebar template as a partial
	r.SetFuncMap(template.FuncMap{
		"loadSidebar": func() SidebarData {
			return sidebarData
		},
		"dict": dict,
	})

	// load in the templates
	r.LoadHTMLGlob("templates/*")

	// serve static assets
	r.Static("/static", "./static")

	// load and parse markdown files
	posts, err := loadMarkdownPosts("./markdown")
	if err != nil {
		log.Fatal(err) // i think this fatal is ok
	}

	// single route for the home page
	r.GET("/", func(c *gin.Context) {
		indexPath := "./markdown/index.md"
		indexContent, err := os.ReadFile(indexPath)
		if err != nil {
			log.Printf("Error occurred during operation: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			return
		}

		post, err := parseMarkdownFile(indexContent)
		if err != nil {
			log.Printf("Error occurred during operation: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			return
		}

		sidebarLinks := createSidebarLinks(post.Headers)

		c.HTML(http.StatusOK, "index.html", gin.H{
			"Title":                   post.Title,
			"Content":                 post.Content,
			"SidebarData":             sidebarData,
			"Headers":                 post.Headers,
			"SidebarLinks":            sidebarLinks,
			"CurrentSlug":             post.Slug,
			"MetaDescription":         post.MetaDescription,
			"MetaPropertyTitle":       post.MetaPropertyTitle,
			"MetaPropertyDescription": post.MetaPropertyDescription,
			"MetaOgURL":               post.MetaOgURL,
		})
	})

	// routes for each blog post, based of of Slug following the /
	for _, post := range posts {
		localPost := post
		if localPost.Slug != "" {
			sidebarLinks := createSidebarLinks(localPost.Headers)
			r.GET("/"+localPost.Slug, func(c *gin.Context) {
				c.HTML(http.StatusOK, "layout.html", gin.H{
					"Title":                   localPost.Title,
					"Content":                 localPost.Content,
					"SidebarData":             sidebarData,
					"Headers":                 localPost.Headers,
					"Description":             localPost.Description,
					"SidebarLinks":            sidebarLinks,
					"CurrentSlug":             localPost.Slug,
					"MetaDescription":         localPost.MetaDescription,
					"MetaPropertyTitle":       localPost.MetaPropertyTitle,
					"MetaPropertyDescription": localPost.MetaPropertyDescription,
					"MetaOgURL":               localPost.MetaOgURL,
				})
			})
		} else {
			log.Printf("Warning: Post titled '%s' has an empty slug and will not be accessible via a unique URL.\n", localPost.Title)
		}
	}

	r.NoRoute(func(c *gin.Context) {
		c.HTML(http.StatusNotFound, "404.html", gin.H{
			"Title": "Page Not Found",
		})
	})

	r.Run()
}

func loadMarkdownPosts(dir string) ([]BlogPost, error) {
	var posts []BlogPost
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".md") {
			path := dir + "/" + file.Name()
			content, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}

			post, err := parseMarkdownFile(content)
			if err != nil {
				return nil, err
			}

			posts = append(posts, post)
		}
	}

	return posts, nil
}

func parseMarkdownFile(content []byte) (BlogPost, error) {
	sections := strings.SplitN(string(content), "---", 2)
	if len(sections) < 2 {
		return BlogPost{}, errors.New("invalid markdown format")
	}

	metadata := sections[0]
	mdContent := sections[1]

	// deal with rogue \r's
	metadata = strings.ReplaceAll(metadata, "\r", "")
	mdContent = strings.ReplaceAll(mdContent, "\r", "")

	title, slug, parent, description, order, metaDescriptionStr,
		metaPropertyTitleStr, metaPropertyDescriptionStr,
		metaOgURLStr := parseMetadata(metadata)

	htmlContent := mdToHTML([]byte(mdContent))
	headers := extractHeaders([]byte(mdContent))

	return BlogPost{
		Title:                   title,
		Slug:                    slug,
		Parent:                  parent,
		Description:             description,
		Content:                 template.HTML(htmlContent),
		Headers:                 headers,
		Order:                   order,
		MetaDescription:         metaDescriptionStr,
		MetaPropertyTitle:       metaPropertyTitleStr,
		MetaPropertyDescription: metaPropertyDescriptionStr,
		MetaOgURL:               metaOgURLStr,
	}, nil
}

func extractHeaders(content []byte) []string {
	var headers []string
	//match only level 2 markdown headers
	re := regexp.MustCompile(`(?m)^##\s+(.*)`)
	matches := re.FindAllSubmatch(content, -1)

	for _, match := range matches {
		// match[1] contains header text without the '##'
		headers = append(headers, string(match[1]))
	}

	return headers
}

func mdToHTML(md []byte) []byte {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	parser := parser.NewWithExtensions(extensions)

	opts := html.RendererOptions{
		Flags: html.CommonFlags | html.HrefTargetBlank,
	}
	renderer := html.NewRenderer(opts)

	doc := parser.Parse(md)

	output := markdown.Render(doc, renderer)

	return output
}

func parseMetadata(metadata string) (
	title string,
	slug string,
	parent string,
	description string,
	order int,
	metaDescription string,
	metaPropertyTitle string,
	metaPropertyDescription string,
	metaOgURL string,
) {
	re := regexp.MustCompile(`(?m)^(\w+):\s*(.+)`)
	matches := re.FindAllStringSubmatch(metadata, -1)

	metaDataMap := make(map[string]string)
	for _, match := range matches {
		if len(match) == 3 {
			metaDataMap[match[1]] = match[2]
		}
	}

	title = metaDataMap["Title"]
	slug = metaDataMap["Slug"]
	parent = metaDataMap["Parent"]
	description = metaDataMap["Description"]
	orderStr := metaDataMap["Order"]
	metaDescriptionStr := metaDataMap["MetaDescription"]
	metaPropertyTitleStr := metaDataMap["MetaPropertyTitle"]
	metaPropertyDescriptionStr := metaDataMap["MetaPropertyDescription"]
	metaOgURLStr := metaDataMap["MetaOgURL"]

	orderStr = strings.TrimSpace(orderStr)
	order, err := strconv.Atoi(orderStr)
	if err != nil {
		log.Printf("Error converting order from string: %v", err)
		order = 9999
	}

	return title, slug, parent, description, order, metaDescriptionStr,
		metaPropertyTitleStr, metaPropertyDescriptionStr, metaOgURLStr
}

func loadSidebarData(dir string) (SidebarData, error) {
	var sidebar SidebarData
	categoriesMap := make(map[string]*Category)

	posts, err := loadMarkdownPosts(dir)
	if err != nil {
		return sidebar, err
	}

	for _, post := range posts {
		if post.Parent != "" {
			if _, exists := categoriesMap[post.Parent]; !exists {
				categoriesMap[post.Parent] = &Category{
					Name:  post.Parent,
					Pages: []BlogPost{post},
					Order: post.Order,
				}
			} else {
				categoriesMap[post.Parent].Pages = append(categoriesMap[post.Parent].Pages, post)
			}
		}
	}

	// convert map to slice
	for _, cat := range categoriesMap {
		sidebar.Categories = append(sidebar.Categories, *cat)
	}

	// sort categories by order
	sort.Slice(sidebar.Categories, func(i, j int) bool {
		return sidebar.Categories[i].Order < sidebar.Categories[j].Order
	})

	return sidebar, nil
}

func createSidebarLinks(headers []string) template.HTML {
	var linksHTML string
	for _, header := range headers {
		sanitizedHeader := sanitizeHeaderForID(header)
		link := fmt.Sprintf(`<li><a href="#%s">%s</a></li>`, sanitizedHeader, header)
		linksHTML += link
	}
	return template.HTML(linksHTML)
}

func sanitizeHeaderForID(header string) string {
	// lowercase
	header = strings.ToLower(header)

	// replace spaces with hyphens
	header = strings.ReplaceAll(header, " ", "-")

	// remove any characters that are not alphanumeric or hyphens
	header = regexp.MustCompile(`[^a-z0-9\-]`).ReplaceAllString(header, "")

	return header
}

func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("invalid dict call")
	}
	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}
		dict[key] = values[i+1]
	}
	return dict, nil
}
