package composition

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tarent/go-log-middleware/v2/logging"

	"golang.org/x/net/html"
)

const (
	LayoutFragmentName = "layout"
	FragmentSeparater  = "#"
	DefaultBufferSize  = 1024 * 100
)

// ContentMerge is a helper type for creation of a combined html document
// out of multiple Content pages.
type ContentMerge struct {
	MetaJSON       map[string]interface{}
	Head           []Fragment
	BodyAttrs      []Fragment
	BodyAttrsArray [][]html.Attribute

	// Aggregator for the Body Fragments of the results.
	// Each fragment is insertes twice with full name and local name,
	// The full name only ends with a FragmentSeparater ('#'), if the local name is not empty
	// and the local name is always prefixed with FragmentSeparater ('#').
	Body map[string]Fragment

	// Aggregator for the Tail Fragments of the results.
	Tail     []Fragment
	Buffered bool

	// merge priorities for the content objects
	// no entry means priority == 0
	priorities map[Content]int

	// all stylesheets contained in used fragments
	stylesheets [][]html.Attribute

	// strategy to prevent duplicacte <link rel="stylesheet"> tags
	stylesheetDeduplicationStrategy StylesheetDeduplicationStrategy
}

// NewContentMerge creates a new buffered ContentMerge
func NewContentMerge(metaJSON map[string]interface{}) *ContentMerge {
	cntx := &ContentMerge{
		MetaJSON:   metaJSON,
		Head:       make([]Fragment, 0, 0),
		Body:       make(map[string]Fragment),
		Tail:       make([]Fragment, 0, 0),
		Buffered:   true,
		priorities: make(map[Content]int),
	}
	return cntx
}

func (cntx *ContentMerge) SetDeduplicationStrategy(strategy StylesheetDeduplicationStrategy) {
	cntx.stylesheetDeduplicationStrategy = strategy
}

func (cntx *ContentMerge) collectStylesheets(f Fragment) {
	cntx.stylesheets = append(cntx.stylesheets, f.Stylesheets()...)
}

func (cntx *ContentMerge) deduplicateStylesheets() {
	if cntx.stylesheetDeduplicationStrategy != nil {
		cntx.stylesheets = cntx.stylesheetDeduplicationStrategy.Deduplicate(cntx.stylesheets)
	}
}

func (cntx *ContentMerge) writeStylesheets(w io.Writer) {

	// first make sure, stylesheets are deduplicated
	cntx.deduplicateStylesheets()

	for _, attrs := range cntx.stylesheets {
		joinedAttr := joinAttrs(attrs)
		stylesheet := fmt.Sprintf("\n    <link %s>", joinedAttr)
		io.WriteString(w, stylesheet)
	}
}

func generateExecutionFunction(cntx *ContentMerge, w io.Writer) (executeFragment func(fragmentName string) error) {
	executeFragment = func(fragmentName string) error {
		f, exist := cntx.GetBodyFragmentByName(fragmentName)
		if !exist {
			missingFragmentString := generateMissingFragmentString(cntx.Body, fragmentName)
			return errors.New(missingFragmentString)
		}
		cntx.collectStylesheets(f)
		return f.Execute(w, cntx.MetaJSON, executeFragment)
	}
	return executeFragment
}

func collectBodyAttrs(bodyAttrs [][]html.Attribute) string {
	var result map[string]string = make(map[string]string)
	for i := range bodyAttrs {
		for j := range bodyAttrs[i] {
			attr := &bodyAttrs[i][j]
			val, exists := result[attr.Key]
			if strings.ToLower(attr.Key) == "class" {
				// aggregate all class attributes
				var newVal string
				if exists {
					newVal = val + " "
				}
				newVal = newVal + attr.Val
				result[attr.Key] = newVal
			} else {
				// but overwrite others
				result[attr.Key] = attr.Val
			}
		}
	}

	var sResult string
	for k, v := range result {
		sResult = sResult + fmt.Sprintf(` %s="%s"`, k, v)
	}

	return sResult
}

func (cntx *ContentMerge) GetHtml() ([]byte, error) {

	if len(cntx.priorities) > 0 {
		cntx.processMetaPriorityParsing()
	}

	// start header, but don't close it. We will add stylsheets later on
	header := bytes.NewBuffer(make([]byte, 0, DefaultBufferSize))
	io.WriteString(header, "<!DOCTYPE html>\n<html>\n  <head>\n    ")

	for _, f := range cntx.Head {
		cntx.collectStylesheets(f)
		executeFragment := generateExecutionFunction(cntx, header)
		if err := f.Execute(header, cntx.MetaJSON, executeFragment); err != nil {
			return nil, err
		}
	}

	// open body tag
	body := bytes.NewBuffer(make([]byte, 0, DefaultBufferSize))
	io.WriteString(body, "\n  <body")
	io.WriteString(body, collectBodyAttrs(cntx.BodyAttrsArray))
	io.WriteString(body, ">\n    ")

	startFragmentName := ""
	if _, exist := cntx.GetBodyFragmentByName(LayoutFragmentName); exist {
		startFragmentName = LayoutFragmentName
	}

	// recursively process body fragments
	executeFragment := generateExecutionFunction(cntx, body)
	if err := executeFragment(startFragmentName); err != nil {
		return nil, err
	}

	for _, f := range cntx.Tail {
		cntx.collectStylesheets(f)
		if err := f.Execute(body, cntx.MetaJSON, executeFragment); err != nil {
			return nil, err
		}
	}
	io.WriteString(body, "\n  </body>\n</html>\n")

	// write the collected stylesheets to the header and close it
	cntx.writeStylesheets(header)
	io.WriteString(header, "\n  </head>")

	// return concatenated header and body
	html := append(header.Bytes(), body.Bytes()...)
	return html, nil
}

// GetBodyFragmentByName returns a fragment by ists name.
// If the name does not contain a FragmentSeparater ('#'), and no such fragment is found.
// also a lookup for '#name' is done, to check, if there is a local name matching.
// The bool return value indicates, if the fragment was found.
func (cntx *ContentMerge) GetBodyFragmentByName(name string) (Fragment, bool) {
	f, found := cntx.Body[name]

	// Normalize: e.g. main# -> main
	if !found && strings.HasSuffix(name, FragmentSeparater) {
		f, found = cntx.Body[name[0:len(name)-1]]
	}

	// search also for local fragment if nothing else found
	if !found && !strings.Contains(name, FragmentSeparater) {
		f, found = cntx.Body[FragmentSeparater+name]
	}

	return f, found
}

func (cntx *ContentMerge) AddContent(c Content, priority int) {
	cntx.addHead(c.Head())
	contentV2, ok := c.(ContentV2)
	if ok {
		cntx.addBodyAttributesArray(contentV2.BodyAttributesArray())
	} else {
		logging.Logger.Warnf("This body-content will not be rendered. Change type of c to ContentV2")
	}
	cntx.addBody(c)
	cntx.addTail(c.Tail())
	if priority > 0 {
		cntx.priorities[c] = priority
	}
}

func (cntx *ContentMerge) addHead(f Fragment) {
	if f != nil {
		cntx.Head = append(cntx.Head, f)
	}
}

func (cntx *ContentMerge) addBodyAttributesArray(a []html.Attribute) {
	if a != nil {
		cntx.BodyAttrsArray = append(cntx.BodyAttrsArray, a)
	}
}

func (cntx *ContentMerge) addBody(c Content) {

	for localName, f := range c.Body() {
		// add twice: local and full qualified name
		cntx.Body[FragmentSeparater+localName] = f
		fqn := c.Name()
		if localName != "" {
			fqn += FragmentSeparater + localName
		}
		cntx.Body[fqn] = f
	}
}

func (cntx *ContentMerge) addTail(f Fragment) {
	if f != nil {
		cntx.Tail = append(cntx.Tail, f)
	}
}

// Generates String for the missing Fragment error message. It adds all existing fragments from the body
func generateMissingFragmentString(body map[string]Fragment, fragmentName string) string {
	text := "Fragment does not exist: " + fragmentName + ". Existing fragments: "
	index := 0
	for k, _ := range body {
		if index == 0 {
			text += `"` + k + `"`
		} else {
			text += `, "` + k + `"`
		}
		index++
	}
	return text
}

// Processes all heads to remove duplicate meta and title tags, respecting the priority of head fragments
func (cntx *ContentMerge) processMetaPriorityParsing() {
	headPropertyMap := make(map[string]string)

	for i := len(cntx.Head) - 1; i >= 0; i-- {
		var currentHead interface{} = cntx.Head[i]
		if currentHead != nil {
			currentStringFragment := currentHead.(*StringFragment)
			ParseHeadFragment(currentStringFragment, headPropertyMap)
			cntx.Head[i] = currentStringFragment
		}
	}
}
