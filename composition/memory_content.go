package composition

import (
	"io"
	"net/http"

	"golang.org/x/net/html"
)

type MemoryContent struct {
	name            string
	requiredContent map[string]*FetchDefinition // key ist the url
	dependencies    map[string]Params
	meta            map[string]interface{}
	head            Fragment
	body            map[string]Fragment
	tail            Fragment
	bodyAttributes  []html.Attribute
	reader          io.ReadCloser
	httpHeader      http.Header
	httpStatusCode  int
}

func NewMemoryContent() *MemoryContent {
	return &MemoryContent{
		requiredContent: make(map[string]*FetchDefinition),
		dependencies:    make(map[string]Params),
		meta:            make(map[string]interface{}),
		body:            make(map[string]Fragment),
	}
}

func (c *MemoryContent) MemorySize() int {
	// We estimate the size for caching, here
	// so a rougth esitmation is enough
	i := len(c.meta)*20 + len(c.httpHeader)*20

	if c.head != nil {
		i += c.head.MemorySize()
	}
	if c.tail != nil {
		i += c.tail.MemorySize()
	}
	for _, f := range c.body {
		i += f.MemorySize()
	}
	return i
}

func (c *MemoryContent) Name() string {
	return c.name
}

func (c *MemoryContent) RequiredContent() []*FetchDefinition {
	deps := make([]*FetchDefinition, 0, len(c.requiredContent))
	for _, dep := range c.requiredContent {
		deps = append(deps, dep)
	}
	return deps
}

func (c *MemoryContent) Dependencies() map[string]Params {
	return c.dependencies
}

func (c *MemoryContent) Meta() map[string]interface{} {
	return c.meta
}

func (c *MemoryContent) Head() Fragment {
	return c.head
}

func (c *MemoryContent) Body() map[string]Fragment {
	return c.body
}

func (c *MemoryContent) Tail() Fragment {
	return c.tail
}

// Deprecated: This method is deprecated
func (c *MemoryContent) BodyAttributes() Fragment {
	return NewStringFragment(joinAttrs(c.bodyAttributes))
}

func (c *MemoryContent) BodyAttributesArray() []html.Attribute {
	return c.bodyAttributes
}

func (c *MemoryContent) Reader() io.ReadCloser {
	return c.reader
}

func (c *MemoryContent) HttpHeader() http.Header {
	return c.httpHeader
}

func (c *MemoryContent) HttpStatusCode() int {
	return c.httpStatusCode
}
