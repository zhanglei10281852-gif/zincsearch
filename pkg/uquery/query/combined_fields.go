/* Copyright 2022 Zinc Labs Inc. and Contributors
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package query

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/vcaesar/riot"
	"github.com/vcaesar/riot/analysis"

	"github.com/zincsearch/zincsearch/pkg/errors"
	"github.com/zincsearch/zincsearch/pkg/meta"
	zincanalysis "github.com/zincsearch/zincsearch/pkg/uquery/analysis"
	zincanalyzer "github.com/zincsearch/zincsearch/pkg/uquery/analysis/analyzer"
	"github.com/zincsearch/zincsearch/pkg/zutils"
)

// CombinedFieldsQuery builds a combined_fields query: the query text is analyzed
// once with one search analyzer and every term may match in ANY of the mapped
// text fields, as if the field contents were indexed into a single field.
//
// Field compatibility is validated against the passed mappings (which belong to
// one index): mapped fields must be of type text and, unless an analyzer is
// forced explicitly, they must share one search analyzer. Fields that do not
// exist in the mappings are ignored, so an index without any of the requested
// fields deterministically becomes a match_none query instead of producing
// indeterminate results when readers are merged.
func CombinedFieldsQuery(query map[string]interface{}, mappings *meta.Mappings, analyzers map[string]*analysis.Analyzer) (riot.Query, error) {
	value := new(meta.CombinedFieldsQuery)
	value.Boost = -1.0

	hasQuery := false
	hasFields := false
	var minimumShouldMatch interface{}
	zeroTermsQuery := "none"

	for k, v := range query {
		k := strings.ToLower(k)
		switch k {
		case "query":
			s, ok := v.(string)
			if !ok {
				return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] query must be a string")
			}
			value.Query = s
			hasQuery = true
		case "fields":
			vv, ok := v.([]interface{})
			if !ok {
				return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] fields must be an array of strings")
			}
			for _, vvv := range vv {
				s, ok := vvv.(string)
				if !ok {
					return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] fields must be an array of strings")
				}
				value.Fields = append(value.Fields, s)
			}
			hasFields = true
		case "analyzer":
			s, ok := v.(string)
			if !ok {
				return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] analyzer must be a string")
			}
			value.Analyzer = s
		case "operator":
			s, ok := v.(string)
			if !ok {
				return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] operator must be a string")
			}
			value.Operator = s
		case "minimum_should_match":
			minimumShouldMatch = v
		case "boost":
			f, err := zutils.ToFloat64(v)
			if err != nil {
				return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] boost must be a number")
			}
			value.Boost = f
		case "zero_terms_query":
			s, ok := v.(string)
			if !ok {
				return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] zero_terms_query must be a string")
			}
			s = strings.ToLower(strings.TrimSpace(s))
			if s != "none" && s != "all" {
				return nil, errors.New(errors.ErrorTypeIllegalArgumentException,
					fmt.Sprintf("[combined_fields] illegal zero_terms_query value [%s], expected [none] or [all]", s))
			}
			zeroTermsQuery = s
		case "_name", "name":
			// accepted but not enforced, compatible with ES query naming
		default:
			// unknown options are ignored, same convention as match/multi_match
		}
	}

	if !hasQuery {
		return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] requires field [query]")
	}
	if !hasFields || len(value.Fields) == 0 {
		return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] requires field [fields] with at least one entry")
	}

	// only OR/AND are legal operators, reject anything else up front
	operator := riot.MatchQueryOperatorOr
	if value.Operator != "" {
		switch strings.ToUpper(strings.TrimSpace(value.Operator)) {
		case "OR":
			operator = riot.MatchQueryOperatorOr
		case "AND":
			operator = riot.MatchQueryOperatorAnd
		default:
			return nil, errors.New(errors.ErrorTypeIllegalArgumentException,
				fmt.Sprintf("[combined_fields] illegal operator value [%s], expected [OR] or [AND]", value.Operator))
		}
	}

	// parse field entries, supporting the "field^boost" notation; the field
	// order is preserved so the built query is stable across shards/indexes.
	fieldOrder := make([]string, 0, len(value.Fields))
	fieldBoosts := make(map[string]float64)
	for _, field := range value.Fields {
		name := field
		boost := 1.0
		if idx := strings.LastIndex(field, "^"); idx >= 0 {
			name = field[:idx]
			b, err := strconv.ParseFloat(field[idx+1:], 64)
			if err != nil || b <= 0 {
				return nil, errors.New(errors.ErrorTypeXContentParseException,
					fmt.Sprintf("[combined_fields] illegal field boost [%s]", field))
			}
			boost = b
		}
		if strings.TrimSpace(name) == "" {
			return nil, errors.New(errors.ErrorTypeXContentParseException, "[combined_fields] field name must not be empty")
		}
		if prev, ok := fieldBoosts[name]; ok {
			if prev != boost {
				return nil, errors.New(errors.ErrorTypeIllegalArgumentException,
					fmt.Sprintf("[combined_fields] field [%s] is listed more than once with different boosts", name))
			}
			continue
		}
		fieldBoosts[name] = boost
		fieldOrder = append(fieldOrder, name)
	}

	// mapping validation: mapped fields have to be text fields; fields that
	// this index does not map are skipped rather than matching nothing useful.
	participating := make([]string, 0, len(fieldOrder))
	for _, field := range fieldOrder {
		prop, ok := meta.Property{}, false
		if mappings != nil {
			prop, ok = mappings.GetProperty(field)
		}
		if !ok {
			continue
		}
		if prop.Type != "text" {
			return nil, errors.New(errors.ErrorTypeIllegalArgumentException,
				fmt.Sprintf("[combined_fields] field [%s] is of type [%s], but only text fields are supported", field, prop.Type))
		}
		participating = append(participating, field)
	}

	// none of the requested fields exists in this index: stable empty result
	if len(participating) == 0 {
		return riot.NewMatchNoneQuery(), nil
	}

	// resolve the single analyzer shared by every participating field
	var zer *analysis.Analyzer
	if value.Analyzer != "" {
		a, err := zincanalysis.QueryAnalyzer(analyzers, value.Analyzer)
		if err != nil {
			return nil, err
		}
		zer = a
	} else {
		analyzerNames := make(map[string]struct{})
		for _, field := range participating {
			prop, _ := mappings.GetProperty(field)
			name := prop.SearchAnalyzer
			if name == "" {
				name = prop.Analyzer
			}
			analyzerNames[name] = struct{}{}
		}
		if len(analyzerNames) > 1 {
			names := make([]string, 0, len(analyzerNames))
			for name := range analyzerNames {
				names = append(names, name)
			}
			sort.Strings(names)
			return nil, errors.New(errors.ErrorTypeIllegalArgumentException,
				fmt.Sprintf("[combined_fields] fields must all have the same search analyzer, but found [%s]", strings.Join(names, ", ")))
		}
		var commonName string
		for name := range analyzerNames {
			commonName = name
		}
		if commonName != "" {
			a, err := zincanalysis.QueryAnalyzer(analyzers, commonName)
			if err != nil {
				return nil, err
			}
			zer = a
		}
	}

	// analyze the query text exactly once with the common analyzer; a nil
	// analyzer falls back to the standard analyzer, which is also the index
	// time default for text fields without a configured analyzer.
	ana := zer
	if ana == nil {
		ana, _ = zincanalyzer.NewStandardAnalyzer(nil)
	}
	tokenStream := ana.Analyze([]byte(value.Query))

	// dedupe terms but keep first-seen order so scoring/minimum math is stable
	terms := make([]string, 0, len(tokenStream))
	seen := make(map[string]struct{}, len(tokenStream))
	for _, token := range tokenStream {
		term := string(token.Term)
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}

	if len(terms) == 0 {
		if zeroTermsQuery == "all" {
			subq := riot.NewMatchAllQuery()
			if value.Boost >= 0 {
				subq.SetBoost(value.Boost)
			}
			return subq, nil
		}
		return riot.NewMatchNoneQuery(), nil
	}

	// per-term clause: the term matches when it occurs in ANY participating field
	termQueries := make([]riot.Query, 0, len(terms))
	for _, term := range terms {
		termQuery := riot.NewBooleanQuery()
		for _, field := range participating {
			tq := riot.NewTermQuery(term)
			tq.SetField(field)
			tq.SetBoost(fieldBoosts[field])
			termQuery.AddShould(tq)
		}
		termQuery.SetMinShould(1)
		termQueries = append(termQueries, termQuery)
	}

	// combine terms with one deterministic boolean semantics over ALL fields
	subq := riot.NewBooleanQuery()
	switch operator {
	case riot.MatchQueryOperatorAnd:
		// every term must occur in at least one of the participating fields
		subq.AddMust(termQueries...)
	default:
		minShould := 1
		if minimumShouldMatch != nil {
			m, err := zutils.CalculateMin(len(terms), minimumShouldMatch)
			if err != nil {
				return nil, errors.New(errors.ErrorTypeXContentParseException,
					fmt.Sprintf("[combined_fields] unsupported minimum_should_match value: %v", err))
			}
			minShould = m
		}
		subq.AddShould(termQueries...)
		subq.SetMinShould(minShould)
	}
	if value.Boost >= 0 {
		subq.SetBoost(value.Boost)
	}

	return subq, nil
}
