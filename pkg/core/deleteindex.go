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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/zincsearch/zincsearch/pkg/config"
	zincErrors "github.com/zincsearch/zincsearch/pkg/errors"
	"github.com/zincsearch/zincsearch/pkg/metadata"
)

// removeAll removes the index data directory. It is a package variable so
// tests can simulate a physical deletion failure.
var removeAll = os.RemoveAll

// removeIndexRetryInterval / removeIndexRetryAttempts bound the wait for OS
// file handles to be released after the index writers are closed (on Windows
// the delete can briefly race the closing process).
var (
	removeIndexRetryInterval = 100 * time.Millisecond
	removeIndexRetryAttempts = 10
)

func indexDataPath(name string) string {
	return filepath.Join(config.Global.DataPath, name)
}

// removeIndexDirectory deletes the data directory, retrying transient
// "still in use" failures after the writers were closed. It returns the
// concrete OS error only after the retries are exhausted, leaving a
// recoverable leftover directory.
func removeIndexDirectory(dataPath string) error {
	var err error
	for attempt := 0; attempt < removeIndexRetryAttempts; attempt++ {
		if err = removeAll(dataPath); err == nil {
			return nil
		}
		if _, statErr := os.Stat(dataPath); statErr != nil {
			return nil // directory is already gone despite the reported error
		}
		time.Sleep(removeIndexRetryInterval)
	}
	return err
}

// DeleteIndex deletes an index and detaches it from every alias.
//
// The metadata writes for the index and its aliases are serialized with all
// other alias updates under the alias write lock, so a concurrent alias
// change and index deletion can only produce one committed target set.
//
// Commit order inside the lock:
//  1. delete the index metadata key (commit point); on failure nothing was
//     touched and the index stays fully searchable;
//  2. persist the alias map without this index; on failure the deleted
//     index metadata is restored as a compensating action and the whole
//     request stays retryable;
//  3. close and evict the in-memory index, after which neither direct nor
//     alias search can reach it in the running process.
//
// The physical data directory is removed only after that commit. If removal
// fails, metadata/cache/aliases already agree the index is gone (a restart
// cannot resurrect it or any dangling alias target); the concrete error is
// returned and retrying DeleteIndex cleans the leftover directory up.
func DeleteIndex(name string) error {
	// Fast path without the coordination lock. If the index is not cached it
	// may still have a leftover data directory from a failed physical removal
	// of an already committed deletion; make that cleanup idempotent and
	// retryable instead of reporting the index as existing.
	if _, exists := GetIndex(name); !exists {
		return deleteOrphanedIndex(name)
	}

	dataPath := ""
	err := ZINC_INDEX_ALIAS_LIST.withWriteLock(func() error {
		// re-check under the lock, a concurrent deletion may have committed
		index, exists := ZINC_INDEX_LIST.Get(name)
		if !exists {
			return errors.New("index " + name + " does not exists")
		}

		// keep a serialized copy of the index metadata so an alias
		// persistence failure can fully restore the committed deletion
		indexData, err := index.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal index[%s] before deletion: %w", name, err)
		}

		// 1. commit the index metadata deletion first; a restart must never
		// resurrect the index nor any alias pointing at it
		if err := metadata.Index.Delete(name); err != nil {
			return fmt.Errorf("failed to delete index[%s] metadata: %w", name, err)
		}

		// 2. persist aliases without this index; roll the index metadata
		// deletion back if the alias commit fails, so no half state is
		// published and the original request can be retried as-is
		if _, err := ZINC_INDEX_ALIAS_LIST.removeIndexLocked(name); err != nil {
			if restoreErr := metadata.Index.Set(name, indexData); restoreErr != nil {
				log.Error().
					Err(restoreErr).
					Str("index", name).
					Msg("failed to restore index metadata after alias persistence failure; manual recovery required")
				return fmt.Errorf("delete index[%s] failed: alias metadata error: %w; metadata restore error: %s", name, err, restoreErr.Error())
			}
			return fmt.Errorf("failed to detach index[%s] from aliases, deletion rolled back: %w", name, err)
		}

		// 3. close and evict the cached index; it is no longer searchable in
		// this process, and concurrent alias adds can no longer target it
		ZINC_INDEX_LIST.Delete(name)

		dataPath = indexDataPath(index.GetName())
		return nil
	})
	if err != nil {
		return err
	}

	// 4. physical removal happens after the commit, outside the alias lock so
	// large directories never block alias searches
	if err := removeIndexDirectory(dataPath); err != nil {
		log.Error().
			Err(err).
			Str("index", name).
			Str("path", dataPath).
			Msg("index deleted from metadata and cache but failed to remove data path; retry the deletion to clean it up")
		return fmt.Errorf("index[%s] deleted but failed to remove data path[%s]: %w", name, dataPath, err)
	}

	return nil
}

// deleteOrphanedIndex handles DeleteIndex for a name that is not cached: if
// the index metadata is gone as well but its data directory remains, the
// deletion was committed before and only the physical cleanup failed, so
// retry the cleanup; otherwise the index simply does not exist.
func deleteOrphanedIndex(name string) error {
	dataPath := indexDataPath(name)
	if _, statErr := os.Stat(dataPath); statErr != nil {
		return errors.New("index " + name + " does not exists")
	}

	if _, err := metadata.Index.Get(name); !errors.Is(err, zincErrors.ErrKeyNotFound) {
		// metadata still (unexpectedly) references the index; do not touch it
		return errors.New("index " + name + " does not exists")
	}

	if err := removeIndexDirectory(dataPath); err != nil {
		return fmt.Errorf("failed to remove leftover data path[%s] of deleted index[%s]: %w", dataPath, name, err)
	}

	log.Info().Str("index", name).Str("path", dataPath).Msg("cleaned leftover data path of deleted index")
	return nil
}
