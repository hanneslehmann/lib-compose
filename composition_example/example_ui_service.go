package main

import (
	"fmt"
	"net/http"
	"os"

	gorilla "github.com/gorilla/handlers"
	"github.com/tarent/lib-compose/v2/composition"
)

var host = "127.0.0.1:8080"

func main() {
	panic(http.ListenAndServe(host, handler()))
}

func handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", staticHandler())
	mux.HandleFunc("/teaser", sidebarHandler)
	mux.Handle("/", compositionHandler())
	return gorilla.LoggingHandler(os.Stdout, mux)
}

func compositionHandler() http.Handler {
	contentFetcherFactory := func(r *http.Request) composition.FetchResultSupplier {
		defaultMetaJSON := map[string]interface{}{
			"header_text": "Hello World!",
			"request":     composition.MetadataForRequest(r),
		}

		fetcher := composition.NewContentFetcher(defaultMetaJSON)

		// defines the 'teaser' fd for lazy fetching
		fetcher.SetFetchDefinitionFactory(NewLazyFdFactory(r).getFetchDefinitions)

		baseUrl := "http://" + r.Host
		fetcher.AddFetchJob(composition.NewFetchDefinition(baseUrl + "/static/layout.html").WithName("layout"))
		fetcher.AddFetchJob(composition.NewFetchDefinition(baseUrl + "/static/lorem.html").WithName("content"))
		fetcher.AddFetchJob(composition.NewFetchDefinition(baseUrl + "/static/styles.html"))

		return fetcher
	}
	factory := func() composition.StylesheetDeduplicationStrategy {
		return new(composition.SimpleDeduplicationStrategy)
	}
	return composition.NewCompositionHandler(contentFetcherFactory).WithDeduplicationStrategyFactory(factory)
}

func staticHandler() http.Handler {
	return http.FileServer(http.Dir("./"))
}

func sidebarHandler(w http.ResponseWriter, r *http.Request) {
	template := `<html><body><div class="teaser">This is a dynamic teaser for id %v</div></body></html>`
	teaserId := r.URL.Query().Get("teaser-id")
	fmt.Fprintf(w, template, teaserId)
}
