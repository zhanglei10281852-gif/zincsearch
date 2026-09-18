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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vcaesar/riot"

	"github.com/zincsearch/zincsearch/pkg/meta"
)

func combinedFieldsMappings() *meta.Mappings {
	m := meta.NewMappings()
	m.SetProperty("f1", meta.NewProperty("text"))
	m.SetProperty("f2", meta.NewProperty("text"))
	std := meta.NewProperty("text")
	std.Analyzer = "standard"
	m.SetProperty("f_std", std)
	m.SetProperty("f_kw", meta.NewProperty("keyword"))
	m.SetProperty("f_num", meta.NewProperty("numeric"))
	return m
}

// assertTermClauses checks that a per-term clause is a should boolean query
// with one term query per participating field.
func assertTermClauses(t *testing.T, clause riot.Query, fields map[string]float64, term string) {
	bq := clause.(*riot.BooleanQuery)
	assert.Equal(t, 1, bq.MinShould())
	assert.Len(t, bq.Shoulds(), len(fields))
	for _, sub := range bq.Shoulds() {
		tq := sub.(*riot.TermQuery)
		assert.Equal(t, term, tq.Term())
		assert.Equal(t, fields[tq.Field()], tq.Boost())
	}
}

func TestCombinedFieldsQuery(t *testing.T) {
	mappings := combinedFieldsMappings()

	t.Run("default OR combines terms across all fields", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello world", "fields": []interface{}{"f1", "f2"},
		}, mappings, nil)
		assert.NoError(t, err)
		bq := q.(*riot.BooleanQuery)
		assert.Len(t, bq.Shoulds(), 2)
		assert.Equal(t, 1, bq.MinShould())
		assert.Equal(t, 1.0, bq.Boost())
		assertTermClauses(t, bq.Shoulds()[0], map[string]float64{"f1": 1, "f2": 1}, "hello")
		assertTermClauses(t, bq.Shoulds()[1], map[string]float64{"f1": 1, "f2": 1}, "world")
		assert.Empty(t, bq.Musts())
	})

	t.Run("AND requires every term in at least one field", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello world", "fields": []interface{}{"f1", "f2"}, "operator": "and",
		}, mappings, nil)
		assert.NoError(t, err)
		bq := q.(*riot.BooleanQuery)
		assert.Len(t, bq.Musts(), 2)
		assert.Empty(t, bq.Shoulds())
		assertTermClauses(t, bq.Musts()[0], map[string]float64{"f1": 1, "f2": 1}, "hello")
		assertTermClauses(t, bq.Musts()[1], map[string]float64{"f1": 1, "f2": 1}, "world")
	})

	t.Run("minimum_should_match as number", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "a b c", "fields": []interface{}{"f1"}, "minimum_should_match": 2.0,
		}, mappings, nil)
		assert.NoError(t, err)
		bq := q.(*riot.BooleanQuery)
		assert.Len(t, bq.Shoulds(), 3)
		assert.Equal(t, 2, bq.MinShould())
	})

	t.Run("minimum_should_match percentage", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "a b c d", "fields": []interface{}{"f1"}, "minimum_should_match": "75%",
		}, mappings, nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, q.(*riot.BooleanQuery).MinShould())
	})

	t.Run("minimum_should_match invalid", func(t *testing.T) {
		_, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "a b", "fields": []interface{}{"f1"}, "minimum_should_match": "bad",
		}, mappings, nil)
		assert.ErrorContains(t, err, "minimum_should_match")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "a b", "fields": []interface{}{"f1"}, "minimum_should_match": true,
		}, mappings, nil)
		assert.Error(t, err)
	})

	t.Run("minimum_should_match ignored with AND", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "a b", "fields": []interface{}{"f1"}, "minimum_should_match": 2.0, "operator": "AND",
		}, mappings, nil)
		assert.NoError(t, err)
		bq := q.(*riot.BooleanQuery)
		assert.Len(t, bq.Musts(), 2)
	})

	t.Run("field boost notation", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1^3", "f2^1.5"},
		}, mappings, nil)
		assert.NoError(t, err)
		assertTermClauses(t, q.(*riot.BooleanQuery).Shoulds()[0],
			map[string]float64{"f1": 3, "f2": 1.5}, "hello")
	})

	t.Run("illegal field boost", func(t *testing.T) {
		_, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1^x"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "illegal field boost")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1^0"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "illegal field boost")
	})

	t.Run("duplicate fields", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1", "f1^1"},
		}, mappings, nil)
		assert.NoError(t, err)
		assertTermClauses(t, q.(*riot.BooleanQuery).Shoulds()[0],
			map[string]float64{"f1": 1}, "hello")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1", "f1^2"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "different boosts")
	})

	t.Run("missing fields are ignored", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1", "unknown"},
		}, mappings, nil)
		assert.NoError(t, err)
		assertTermClauses(t, q.(*riot.BooleanQuery).Shoulds()[0],
			map[string]float64{"f1": 1}, "hello")
	})

	t.Run("all fields missing is match_none", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"a", "b"},
		}, mappings, nil)
		assert.NoError(t, err)
		assert.IsType(t, &riot.MatchNoneQuery{}, q)
	})

	t.Run("non-text mapped fields are rejected", func(t *testing.T) {
		_, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f_kw"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "only text fields are supported")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f_num"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "only text fields are supported")
	})

	t.Run("fields must share one search analyzer", func(t *testing.T) {
		_, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1", "f_std"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "same search analyzer")
	})

	t.Run("explicit analyzer forces compatibility", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1", "f_std"}, "analyzer": "standard",
		}, mappings, nil)
		assert.NoError(t, err)
		assertTermClauses(t, q.(*riot.BooleanQuery).Shoulds()[0],
			map[string]float64{"f1": 1, "f_std": 1}, "hello")
	})

	t.Run("unknown analyzer is rejected", func(t *testing.T) {
		_, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1"}, "analyzer": "nope",
		}, mappings, nil)
		assert.ErrorContains(t, err, "unknown analyzer")
	})

	t.Run("zero terms", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "", "fields": []interface{}{"f1"},
		}, mappings, nil)
		assert.NoError(t, err)
		assert.IsType(t, &riot.MatchNoneQuery{}, q)

		q, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "", "fields": []interface{}{"f1"}, "zero_terms_query": "all",
		}, mappings, nil)
		assert.NoError(t, err)
		assert.IsType(t, &riot.MatchAllQuery{}, q)

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "", "fields": []interface{}{"f1"}, "zero_terms_query": "maybe",
		}, mappings, nil)
		assert.ErrorContains(t, err, "zero_terms_query")
	})

	t.Run("terms are normalized and deduped", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "Hello HELLO hello", "fields": []interface{}{"f1"},
		}, mappings, nil)
		assert.NoError(t, err)
		bq := q.(*riot.BooleanQuery)
		assert.Len(t, bq.Shoulds(), 1)
		assertTermClauses(t, bq.Shoulds()[0], map[string]float64{"f1": 1}, "hello")
	})

	t.Run("top level boost", func(t *testing.T) {
		q, err := CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"f1"}, "boost": 2.5,
		}, mappings, nil)
		assert.NoError(t, err)
		assert.Equal(t, 2.5, q.(*riot.BooleanQuery).Boost())
	})

	t.Run("malformed requests are rejected", func(t *testing.T) {
		_, err := CombinedFieldsQuery(map[string]interface{}{
			"fields": []interface{}{"f1"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "requires field [query]")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "hello",
		}, mappings, nil)
		assert.ErrorContains(t, err, "requires field [fields]")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{},
		}, mappings, nil)
		assert.ErrorContains(t, err, "requires field [fields]")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": 1, "fields": []interface{}{"f1"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "query must be a string")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": "f1",
		}, mappings, nil)
		assert.ErrorContains(t, err, "fields must be an array")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{1},
		}, mappings, nil)
		assert.ErrorContains(t, err, "fields must be an array of strings")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "hello", "fields": []interface{}{"^2"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "field name must not be empty")

		_, err = CombinedFieldsQuery(map[string]interface{}{
			"query": "x", "fields": []interface{}{"f1"}, "operator": "xor",
		}, mappings, nil)
		assert.ErrorContains(t, err, "illegal operator")
	})

	t.Run("dispatch wraps parse errors", func(t *testing.T) {
		_, err := Query(map[string]interface{}{
			"combined_fields": map[string]interface{}{"query": "x", "fields": []interface{}{"f1"}, "operator": "xor"},
		}, mappings, nil)
		assert.ErrorContains(t, err, "[combined_fields] failed to parse field")
	})
}
