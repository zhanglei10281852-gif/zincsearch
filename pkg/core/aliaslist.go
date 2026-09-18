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
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/zincsearch/zincsearch/pkg/metadata"
	"github.com/zincsearch/zincsearch/pkg/zutils"
)

var ZINC_INDEX_ALIAS_LIST AliasList

type AliasList struct {
	lock    sync.RWMutex
	Aliases map[string][]string
}

func NewAliasList() *AliasList {
	return &AliasList{Aliases: map[string][]string{}}
}

// cloneAliasMap returns a deep copy of the alias map. Every mutation is
// prepared on a copy so a failed metadata write can never partially alter
// the previously visible state.
func cloneAliasMap(src map[string][]string) map[string][]string {
	dst := make(map[string][]string, len(src))
	for alias, indexes := range src {
		cp := make([]string, len(indexes))
		copy(cp, indexes)
		dst[alias] = cp
	}
	return dst
}

// publishLocked persists next and, only on success, publishes it as the
// visible state. The caller must hold al.lock. When persistence fails the
// previously visible state is kept, so the original request stays retryable
// against unchanged state.
func (al *AliasList) publishLocked(next map[string][]string) error {
	if err := metadata.Alias.Set(next); err != nil {
		log.Err(err).Msg("failed to save alias in metadata")
		return err
	}
	al.Aliases = next
	return nil
}

// withWriteLock runs fn while holding the alias write lock. Index deletion
// uses it to serialize against alias updates: concurrent alias changes and
// index deletion can only commit one after another, producing a single
// committed target set instead of interleaved half states.
func (al *AliasList) withWriteLock(fn func() error) error {
	al.lock.Lock()
	defer al.lock.Unlock()
	return fn()
}

func (al *AliasList) AddIndexesToAlias(alias string, indexes []string) error {
	al.lock.Lock()
	defer al.lock.Unlock()

	next := cloneAliasMap(al.Aliases)
	current := append([]string{}, next[alias]...)
	changed := false
	for _, index := range indexes {
		// an alias member is unique, so a retried add never duplicates it
		if zutils.SliceExists(current, index) {
			continue
		}
		current = append(current, index)
		changed = true
	}
	if !changed {
		return nil
	}
	next[alias] = current

	return al.publishLocked(next)
}

func (al *AliasList) RemoveIndexesFromAlias(alias string, removeIndexes []string) error {
	al.lock.Lock()
	defer al.lock.Unlock()

	current, ok := al.Aliases[alias]
	if !ok {
		return nil
	}

	removeIndexesMap := make(map[string]struct{}, len(removeIndexes))
	for _, index := range removeIndexes {
		removeIndexesMap[index] = struct{}{}
	}

	next := cloneAliasMap(al.Aliases)
	kept := make([]string, 0, len(current))
	changed := false
	for _, index := range next[alias] {
		if _, ok := removeIndexesMap[index]; ok {
			changed = true
			continue
		}
		kept = append(kept, index)
	}
	if !changed {
		return nil
	}

	// removing the last member removes the alias, so an alias never
	// describes an index set that is not searchable
	if len(kept) == 0 {
		delete(next, alias)
	} else {
		next[alias] = kept
	}

	return al.publishLocked(next)
}

// RemoveIndex detaches indexName from every alias and drops aliases left
// without members. The change is persisted as one transaction; on
// persistence failure the previous state is restored. It returns true when
// at least one alias referenced the index.
func (al *AliasList) RemoveIndex(indexName string) (bool, error) {
	al.lock.Lock()
	defer al.lock.Unlock()
	return al.removeIndexLocked(indexName)
}

// removeIndexLocked is RemoveIndex for callers that already hold the alias
// write lock (e.g. DeleteIndex, which must commit index removal and alias
// detachment as one serialized change).
func (al *AliasList) removeIndexLocked(indexName string) (bool, error) {
	next := cloneAliasMap(al.Aliases)
	changed := false
	for aliasName, indexes := range next {
		kept := make([]string, 0, len(indexes))
		removed := false
		for _, index := range indexes {
			if index == indexName {
				removed = true
				continue
			}
			kept = append(kept, index)
		}
		if !removed {
			continue
		}
		changed = true
		if len(kept) == 0 {
			delete(next, aliasName)
		} else {
			next[aliasName] = kept
		}
	}
	if !changed {
		return false, nil
	}

	if err := al.publishLocked(next); err != nil {
		return false, err
	}
	return true, nil
}

// ApplyMembers atomically applies a batch of alias adds and removes: all
// adds are merged first, then all removes, matching Elasticsearch
// _aliases semantics. An add only commits indexes that currently exist, so
// an index deleted concurrently (serialized by the same write lock) can
// never become a committed alias target. Members are unique and an alias
// without members is removed. Either the whole batch is committed or the
// previous state stays visible and the error is returned for retry.
func (al *AliasList) ApplyMembers(addMembers, removeMembers map[string][]string) error {
	al.lock.Lock()
	defer al.lock.Unlock()

	next := cloneAliasMap(al.Aliases)
	changed := false

	for alias, indexes := range addMembers {
		current := append([]string{}, next[alias]...)
		for _, index := range indexes {
			if _, ok := ZINC_INDEX_LIST.Get(index); !ok {
				// index does not exist (possibly deleted concurrently); skip it
				continue
			}
			if zutils.SliceExists(current, index) {
				continue
			}
			current = append(current, index)
			changed = true
		}
		if len(current) > 0 {
			next[alias] = current
		}
	}

	for alias, indexes := range removeMembers {
		current, ok := next[alias]
		if !ok {
			continue
		}

		removeIndexesMap := make(map[string]struct{}, len(indexes))
		for _, index := range indexes {
			removeIndexesMap[index] = struct{}{}
		}

		kept := make([]string, 0, len(current))
		for _, index := range current {
			if _, ok := removeIndexesMap[index]; ok {
				changed = true
				continue
			}
			kept = append(kept, index)
		}
		if len(kept) == 0 {
			delete(next, alias)
		} else {
			next[alias] = kept
		}
	}

	if !changed {
		return nil
	}
	return al.publishLocked(next)
}

// PruneInvalidAliases removes alias members referencing indexes that are not
// loaded and deletes aliases without members. It runs at startup so an alias
// can never resolve to an index that a previous version or an interrupted
// deletion left behind; after a restart alias search only sees targets that
// actually exist. The in-memory state is always pruned, and the pruned map
// is persisted best-effort.
func (al *AliasList) PruneInvalidAliases() error {
	al.lock.Lock()
	defer al.lock.Unlock()

	next := cloneAliasMap(al.Aliases)
	changed := false
	for aliasName, indexes := range next {
		kept := make([]string, 0, len(indexes))
		removed := false
		for _, index := range indexes {
			if _, ok := ZINC_INDEX_LIST.Get(index); !ok {
				removed = true
				continue
			}
			kept = append(kept, index)
		}
		if !removed {
			continue
		}
		changed = true
		if len(kept) == 0 {
			delete(next, aliasName)
		} else {
			next[aliasName] = kept
		}
	}

	if !changed {
		return nil
	}

	// publish the pruned state even when persistence fails: for the running
	// process no dangling target is visible; the next successful alias
	// update writes the converged map. Report the error so callers can log it.
	if err := metadata.Alias.Set(next); err != nil {
		log.Err(err).Msg("failed to persist pruned aliases at startup, serving pruned state in memory")
		al.Aliases = next
		return err
	}
	al.Aliases = next
	return nil
}

func (al *AliasList) GetIndexesForAlias(aliasName string) ([]string, bool) {
	al.lock.RLock()
	idx, ok := al.Aliases[aliasName]
	if !ok {
		al.lock.RUnlock()
		return nil, false
	}

	v := make([]string, len(idx))
	copy(v, idx)

	al.lock.RUnlock()
	return v, ok
}

func (al *AliasList) GetAliasesForIndex(indexName string) []string {
	al.lock.RLock()
	var aliases []string
	for alias, indexes := range al.Aliases {
		if zutils.SliceExists(indexes, indexName) {
			aliases = append(aliases, alias)
		}
	}

	al.lock.RUnlock()
	return aliases
}

type M map[string]interface{}

// GetAliasMap returns an ES compatible map of indexes to their aliases
// In the form:
//
//	{"gitea_issues":{"aliases":{}},"gitea_codes.v1":{"aliases":{"gitea_codes":{}}}}
func (al *AliasList) GetAliasMap(targetIndexes, targetAliases []string) M {
	al.lock.RLock()
	top := M{}

outerLoop:
	for alias, indexes := range al.Aliases {
		if len(targetAliases) > 0 && !zutils.SliceExists(targetAliases, alias) { // check if this is one of the aliased we're looking for
			continue outerLoop
		}

		innerLoop:
		for _, index := range indexes {
			if len(targetIndexes) > 0 && !zutils.SliceExists(targetIndexes, index) { // check if this is one of the indexes we're looking for
				continue innerLoop
			}

			indexMap, _ := top[index].(M)
			if indexMap == nil {
				indexMap = M{}
				top[index] = indexMap
			}

			aliases, _ := indexMap["aliases"].(M)
			if aliases == nil {
				aliases = M{}
				indexMap["aliases"] = aliases
			}

			aliases[alias] = struct{}{}
		}
	}

	al.lock.RUnlock()
	return top
}
