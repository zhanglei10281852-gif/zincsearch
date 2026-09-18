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

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zincsearch/zincsearch/pkg/meta"
)

func TestIndex_CombinedFieldsSearch(t *testing.T) {
	indexName := "TestIndex_CombinedFieldsSearch.index_1"
	index, err := NewIndex(indexName, "disk", 3)
	assert.NoError(t, err)
	assert.NotNil(t, index)
	assert.NoError(t, StoreIndex(index))

	for _, field := range []string{"title", "summary", "tags"} {
		index.GetMappings().SetProperty(field, meta.Property{Type: "text", Index: true})
	}

	docs := []map[string]interface{}{
		{"title": "quick brown fox", "summary": "forest animals guide", "tags": "nature"},
		{"title": "lazy dog", "summary": "pet care basics", "tags": "animals"},
		{"title": "brown bear", "summary": "wildlife stories", "tags": "forest nature"},
		{"title": "fox tales", "summary": "story collection", "tags": "fiction animals"},
	}

	t.Run("index documents across shards", func(t *testing.T) {
		for i, d := range docs {
			assert.NoError(t, index.CreateDocument(string(rune('a'+i)), d, false))
		}
		waitForDocs(t, index, len(docs))

		// documents must land on more than one shard reader for this test
		// to actually exercise shard merging
		readers, err := index.GetReaders(0, 0)
		assert.NoError(t, err)
		assert.Greater(t, len(readers), 1, "expected documents on multiple shard readers")
		for _, r := range readers {
			r.Close()
		}
	})

	newQuery := func(q string, operator string, msm interface{}) *meta.ZincQuery {
		return &meta.ZincQuery{
			Size: 10,
			Query: map[string]interface{}{
				"combined_fields": map[string]interface{}{
					"query":   q,
					"fields":  []interface{}{"title", "summary", "tags"},
					"operator": operator,
					"minimum_should_match": msm,
				},
			},
		}
	}

	hitIDs := func(resp *meta.SearchResponse) map[string]struct{} {
		ids := make(map[string]struct{})
		for _, h := range resp.Hits.Hits {
			ids[h.ID] = struct{}{}
		}
		return ids
	}

	t.Run("OR matches a term in any field", func(t *testing.T) {
		resp, err := index.Search(newQuery("brown", "", nil))
		assert.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"a": {}, "c": {}}, hitIDs(resp))
	})

	t.Run("AND combines terms across different fields", func(t *testing.T) {
		// "brown" is only in title, "nature" only in tags; a per-field AND
		// would fail, combined_fields must hit on both documents
		resp, err := index.Search(newQuery("brown nature", "AND", nil))
		assert.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"a": {}, "c": {}}, hitIDs(resp))

		resp, err = index.Search(newQuery("fox fiction", "AND", nil))
		assert.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"d": {}}, hitIDs(resp))
	})

	t.Run("AND misses when a term is absent", func(t *testing.T) {
		resp, err := index.Search(newQuery("fox elephant", "AND", nil))
		assert.NoError(t, err)
		assert.Equal(t, 0, resp.Hits.Total.Value)
	})

	t.Run("minimum_should_match is deterministic over terms", func(t *testing.T) {
		resp, err := index.Search(newQuery("quick brown fox", "", 2.0))
		assert.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"a": {}}, hitIDs(resp))

		resp, err = index.Search(newQuery("quick brown fox", "", "67%"))
		assert.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"a": {}}, hitIDs(resp))
	})

	t.Run("repeated searches return the same hit set and ranking", func(t *testing.T) {
		first, err := index.Search(newQuery("brown animals forest", "", nil))
		assert.NoError(t, err)
		second, err := index.Search(newQuery("brown animals forest", "", nil))
		assert.NoError(t, err)
		assert.Equal(t, hitIDs(first), hitIDs(second))
		assert.Equal(t, len(first.Hits.Hits), len(second.Hits.Hits))
		for i := range first.Hits.Hits {
			assert.Equal(t, first.Hits.Hits[i].ID, second.Hits.Hits[i].ID)
			assert.Equal(t, first.Hits.Hits[i].Score, second.Hits.Hits[i].Score)
		}
	})

	t.Run("illegal operator and minimum_should_match reject only this request", func(t *testing.T) {
		_, err := index.Search(newQuery("brown", "xor", nil))
		assert.ErrorContains(t, err, "illegal operator")

		_, err = index.Search(newQuery("brown", "", "bad"))
		assert.ErrorContains(t, err, "minimum_should_match")

		// a following valid query still works
		resp, err := index.Search(newQuery("brown", "", nil))
		assert.NoError(t, err)
		assert.Equal(t, 2, resp.Hits.Total.Value)
	})

	t.Run("cleanup", func(t *testing.T) {
		assert.NoError(t, DeleteIndex(indexName))
	})
}
