package render

import (
	"container/list"
	"reflect"
	"sync"
	"text/template"
)

// templateCacheLimit bounds the parsed templates kept. A deployment has a
// handful of client templates; the admin preview renders arbitrary text,
// which must not grow the cache without bound.
const templateCacheLimit = 64

// templateCache keeps parsed client templates by their text, so a
// subscription fetch executes a parsed template instead of parsing the
// admin's template again. An edited template is new text and parses anew; the
// least recently used template leaves first. A parsed template is safe for
// concurrent execution.
type templateCache struct {
	mu      sync.Mutex
	limit   int
	entries map[string]*list.Element
	order   *list.List // most recently used first
}

type cachedTemplate struct {
	text string
	tmpl *template.Template
}

func newTemplateCache(limit int) *templateCache {
	return &templateCache{limit: limit, entries: make(map[string]*list.Element), order: list.New()}
}

var parsedTemplates = newTemplateCache(templateCacheLimit)

// get returns the parsed template for text; a template that fails to parse
// is not cached.
func (c *templateCache) get(text string) (*template.Template, error) {
	if tmpl, ok := c.lookup(text); ok {
		return tmpl, nil
	}
	tmpl, err := template.New("client").Funcs(templateFuncs).Parse(text)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[text]; ok {
		// Parsed concurrently: keep the first.
		c.order.MoveToFront(element)
		return element.Value.(*cachedTemplate).tmpl, nil
	}
	c.entries[text] = c.order.PushFront(&cachedTemplate{text: text, tmpl: tmpl})
	for c.order.Len() > c.limit {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*cachedTemplate).text)
	}
	return tmpl, nil
}

func (c *templateCache) lookup(text string) (*template.Template, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[text]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(element)
	return element.Value.(*cachedTemplate).tmpl, true
}

// proxyFieldNames are the names a template reads a proxy's fields by,
// resolved once instead of per proxy and fetch.
var proxyFieldNames = func() []string {
	t := reflect.TypeOf(Proxy{})
	names := make([]string, t.NumField())
	for i := range names {
		names[i] = t.Field(i).Name
	}
	return names
}()

// templateData is the proxy as the templates see it: its fields by name.
func (p Proxy) templateData() map[string]any {
	v := reflect.ValueOf(p)
	data := make(map[string]any, len(proxyFieldNames))
	for i, name := range proxyFieldNames {
		data[name] = v.Field(i).Interface()
	}
	return data
}
