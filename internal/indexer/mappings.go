package indexer

import (
	"fmt"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/lang/ar"
	"github.com/blevesearch/bleve/v2/analysis/lang/da"
	"github.com/blevesearch/bleve/v2/analysis/lang/de"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/lang/es"
	"github.com/blevesearch/bleve/v2/analysis/lang/fa"
	"github.com/blevesearch/bleve/v2/analysis/lang/fi"
	"github.com/blevesearch/bleve/v2/analysis/lang/fr"
	"github.com/blevesearch/bleve/v2/analysis/lang/hu"
	"github.com/blevesearch/bleve/v2/analysis/lang/it"
	"github.com/blevesearch/bleve/v2/analysis/lang/nl"
	"github.com/blevesearch/bleve/v2/analysis/lang/pt"
	"github.com/blevesearch/bleve/v2/analysis/lang/ro"
	"github.com/blevesearch/bleve/v2/analysis/lang/ru"
	"github.com/blevesearch/bleve/v2/analysis/lang/sv"
	"github.com/blevesearch/bleve/v2/analysis/lang/tr"
	"github.com/blevesearch/bleve/v2/mapping"

	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"

	"github.com/abadojack/whatlanggo"
)

var (
	SupportedLanguages = map[string]whatlanggo.Lang{
		en.AnalyzerName: whatlanggo.Eng,
		ar.AnalyzerName: whatlanggo.Arb,
		da.AnalyzerName: whatlanggo.Dan,
		nl.AnalyzerName: whatlanggo.Nld,
		fi.AnalyzerName: whatlanggo.Fin,
		fr.AnalyzerName: whatlanggo.Fra,
		hu.AnalyzerName: whatlanggo.Hun,
		it.AnalyzerName: whatlanggo.Ita,
		de.AnalyzerName: whatlanggo.Deu,
		fa.AnalyzerName: whatlanggo.Pes,
		pt.AnalyzerName: whatlanggo.Por,
		ro.AnalyzerName: whatlanggo.Ron,
		ru.AnalyzerName: whatlanggo.Rus,
		es.AnalyzerName: whatlanggo.Spa,
		sv.AnalyzerName: whatlanggo.Swe,
		tr.AnalyzerName: whatlanggo.Tur,
	}
)

func BuildIndexMapping() (mapping.IndexMapping, error) {
	rootTextFieldMapping := bleve.NewTextFieldMapping()

	keywordFieldMapping := bleve.NewTextFieldMapping()
	keywordFieldMapping.Analyzer = keyword.Name

	bookmarkMapping := bleve.NewDocumentMapping()

	for k, _ := range SupportedLanguages {
		textFieldMapping := bleve.NewTextFieldMapping()
		textFieldMapping.Analyzer = k

		bookmarkMapping.AddFieldMappingsAt(fmt.Sprintf("%s_title", k), textFieldMapping)
		bookmarkMapping.AddFieldMappingsAt(fmt.Sprintf("%s_text", k), textFieldMapping)
		bookmarkMapping.AddFieldMappingsAt(fmt.Sprintf("%s_excerpt", k), textFieldMapping)
	}

	bookmarkMapping.AddFieldMappingsAt("folder", rootTextFieldMapping)
	bookmarkMapping.AddFieldMappingsAt("url", rootTextFieldMapping)

	bookmarkMapping.AddFieldMappingsAt("author", keywordFieldMapping)
	bookmarkMapping.AddFieldMappingsAt("lang", keywordFieldMapping)
	bookmarkMapping.AddFieldMappingsAt("siteName", keywordFieldMapping)

	indexMapping := bleve.NewIndexMapping()
	indexMapping.AddDocumentMapping("bookmark", bookmarkMapping)

	indexMapping.TypeField = "type"
	indexMapping.DefaultAnalyzer = "en"

	return indexMapping, nil
}
