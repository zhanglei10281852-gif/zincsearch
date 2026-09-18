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

package index

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zincsearch/zincsearch/pkg/core"
	"github.com/zincsearch/zincsearch/pkg/meta"
	"github.com/zincsearch/zincsearch/pkg/metadata"
	"github.com/zincsearch/zincsearch/pkg/zutils/json"
	"github.com/zincsearch/zincsearch/test/utils"
)

func TestAddOrRemoveESAlias(t *testing.T) {
	indexName := "TestAddOrRemoveESAlias.index_1"
	type args struct {
		data   string
		params map[string]string
		result string
	}
	tests := []struct {
		name        string
		nFn         func(*core.Index)
		args        args
		wantErr     bool
		wantCode    int
		wantAliases []string
	}{
		{
			name: "should_fail_for_empty_request_body",
			args: args{
				data:   "",
				result: `{"error":"invalid character '\u0000' looking for beginning of value"}`,
			},
			wantCode:    http.StatusBadRequest,
			wantErr:     false,
			wantAliases: []string{},
		},
		{
			name: "should_add_es_alias",
			args: args{
				data:   `{"actions": [{"add": {"index": "TestAddOrRemoveESAlias.index_1","alias": "test_alias_1"}}]}`,
				result: `{"acknowledged":true}`,
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"test_alias_1"},
		},
		{
			name: "should_add_es_alias_with_wildcard_index_name",
			args: args{
				data:   `{"actions": [{"add": {"index": "TestAdd*.index_1","alias": "test_alias_2"}}]}`,
				result: `{"acknowledged":true}`,
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"test_alias_2"},
		},
		{
			name: "should_add_es_alias_with_aliases_field",
			args: args{
				data:   `{"actions": [{"add": {"index": "TestAddOrRemoveESAlias.index_1","aliases": ["test_alias_3","test_alias_4"]}}]}`,
				result: `{"acknowledged":true}`,
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias", []string{index.GetName()}))
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"existing_alias", "test_alias_3", "test_alias_4"},
		},
		{
			name: "should_add_es_alias_with_indices_field",
			args: args{
				data:   `{"actions": [{"add": {"indices": ["TestAddOrRemoveESAlias.index_1"],"aliases": ["test_alias_3","test_alias_4"]}}]}`,
				result: `{"acknowledged":true}`,
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias", []string{index.GetName()}))
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"existing_alias", "test_alias_3", "test_alias_4"},
		},
		{
			name: "should_add_es_alias_with_indices_field_using_*",
			args: args{
				data:   `{"actions": [{"add": {"indices": ["*index_1"],"aliases": ["test_alias_3","test_alias_4"]}}]}`,
				result: `{"acknowledged":true}`,
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias", []string{index.GetName()}))
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"existing_alias", "test_alias_3", "test_alias_4"},
		},
		{
			name: "should_remove_es_alias",
			args: args{
				data:   `{"actions": [{"remove": {"index": "TestAddOrRemoveESAlias.index_1","alias": "existing_alias_2"}}]}`,
				result: `{"acknowledged":true}`,
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_2", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_1", []string{index.GetName()}))
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"existing_alias_1"},
		},
		{
			name: "should_remove_es_alias_with_aliases_field",
			args: args{
				data:   `{"actions": [{"remove": {"index": "TestAddOrRemoveESAlias.index_1","aliases": ["existing_alias_1","existing_alias_2"]}}]}`,
				result: `{"acknowledged":true}`,
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_1", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_2", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_3", []string{index.GetName()}))
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"existing_alias_3"},
		},
		{
			name: "should_remove_es_alias_with_indices_field",
			args: args{
				data:   `{"actions": [{"remove": {"indices": ["TestAddOrRemoveESAlias.index_1"],"aliases": ["existing_alias_1","existing_alias_2"]}}]}`,
				result: `{"acknowledged":true}`,
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_1", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_2", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_3", []string{index.GetName()}))
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"existing_alias_3"},
		},
		{
			name: "should_remove_es_alias_with_indices_field",
			args: args{
				data:   `{"actions": [{"remove": {"indices": ["*index_1"],"aliases": ["existing_alias_1","existing_alias_2"]}}]}`,
				result: `{"acknowledged":true}`,
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_1", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_2", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_3", []string{index.GetName()}))
			},
			wantCode:    http.StatusOK,
			wantErr:     false,
			wantAliases: []string{"existing_alias_3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, closeFn := newIndex(t, indexName)
			defer closeFn()

			if tt.nFn != nil {
				tt.nFn(index)
			}

			c, w := utils.NewGinContext()
			utils.SetGinRequestData(c, tt.args.data)
			utils.SetGinRequestParams(c, tt.args.params)
			AddOrRemoveESAlias(c)

			require.Equal(t, tt.wantCode, w.Code)
			require.Equal(t, tt.args.result, w.Body.String())

			als := core.ZINC_INDEX_ALIAS_LIST.GetAliasesForIndex(indexName)
			require.ElementsMatch(t, tt.wantAliases, als)
		})
	}
}

func TestGetESAliases(t *testing.T) {
	indexName := "TestAddOrRemoveESAlias.index_1"
	type args struct {
		result string
	}
	tests := []struct {
		name     string
		nFn      func(*core.Index)
		params   map[string]string
		args     args
		wantErr  bool
		wantCode int
	}{
		{
			name: "should_get_es_alias",
			args: args{
				result: `{"TestAddOrRemoveESAlias.index_1":{"aliases":{"existing_alias_1":{},"existing_alias_2":{},"existing_alias_3":{}}}}`,
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_1", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_2", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_3", []string{index.GetName()}))
			},
			wantCode: http.StatusOK,
			wantErr:  false,
		},
		{
			name: "should_get_es_alias_of_target_index",
			args: args{
				result: `{"TestAddOrRemoveESAlias.index_1":{"aliases":{"existing_alias_1":{},"existing_alias_2":{},"existing_alias_3":{}}}}`,
			},
			params: map[string]string{
				"target": "TestAddOrRemoveESAlias.index_1",
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_1", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_2", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_3", []string{index.GetName()}))
			},
			wantCode: http.StatusOK,
			wantErr:  false,
		},
		{
			name: "should_get_es_alias_of_target_alias",
			args: args{
				result: `{"TestAddOrRemoveESAlias.index_1":{"aliases":{"existing_alias_1":{}}}}`,
			},
			params: map[string]string{
				"target_alias": "existing_alias_1",
			},
			nFn: func(index *core.Index) {
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_1", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_2", []string{index.GetName()}))
				require.NoError(t, core.ZINC_INDEX_ALIAS_LIST.AddIndexesToAlias("existing_alias_3", []string{index.GetName()}))
			},
			wantCode: http.StatusOK,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, closeFn := newIndex(t, indexName)
			defer closeFn()

			if tt.nFn != nil {
				tt.nFn(index)
			}

			c, w := utils.NewGinContext()
			utils.SetGinRequestParams(c, tt.params)
			GetESAliases(c)

			require.Equal(t, tt.wantCode, w.Code)
			require.Equal(t, tt.args.result, w.Body.String())
		})
	}
}

// a persistence failure must surface as an error, leave the previously
// visible alias state untouched, and allow the same request to be retried.
func TestAddOrRemoveESAlias_PersistFailureRollbackAndRetry(t *testing.T) {
	indexName := "TestAddOrRemoveESAlias.rollback"
	index, closeFn := newIndex(t, indexName)
	defer closeFn()
	require.Equal(t, indexName, index.GetName())

	body := `{"actions": [{"add": {"index": "` + indexName + `","alias": "rollback_alias_1"}}]}`

	// metadata backend refuses writes
	mem := utils.NewMemStorage()
	prev := metadata.SetDB(mem)
	mem.FailSet = func(string) error { return errors.New("metadata unavailable") }

	c, w := utils.NewGinContext()
	utils.SetGinRequestData(c, body)
	AddOrRemoveESAlias(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var errResp meta.HTTPResponseError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	require.Contains(t, errResp.Error, "metadata unavailable")
	require.Empty(t, core.ZINC_INDEX_ALIAS_LIST.GetAliasesForIndex(indexName))

	// recover storage and retry the very same request
	mem.FailSet = nil
	metadata.SetDB(prev)

	c2, w2 := utils.NewGinContext()
	utils.SetGinRequestData(c2, body)
	AddOrRemoveESAlias(c2)

	require.Equal(t, http.StatusOK, w2.Code)
	require.Equal(t, `{"acknowledged":true}`, w2.Body.String())
	require.ElementsMatch(t, []string{"rollback_alias_1"},
		core.ZINC_INDEX_ALIAS_LIST.GetAliasesForIndex(indexName))
}

func newIndex(t *testing.T, indexName string) (*core.Index, func()) {
	index, err := core.NewIndex(indexName, "disk", 2)
	require.NoError(t, err)

	err = core.StoreIndex(index)
	require.NoError(t, err)

	return index, func() {
		require.NoError(t, metadata.Alias.Set(map[string][]string{}))
		core.ZINC_INDEX_ALIAS_LIST = *core.NewAliasList()
		require.NoError(t, core.DeleteIndex(indexName))
	}
}
