// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package inventory_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/inventory"
	"go.uber.org/goleak"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
	)
}

func setUp() logr.Logger {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{"stdout"}
	logOpts := ctrlZap.Options{
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))
	return log
}

func TestManager_Load(t *testing.T) {
	logger := setUp()
	testCases := []struct {
		name  string
		items []inventory.Item
	}{
		{
			name: "Mixed",
			items: []inventory.Item{
				&inventory.ManifestItem{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					},
					Name:      "a",
					Namespace: "",
					ID:        "a___Namespace",
				},
				&inventory.HelmReleaseItem{
					Name:      "test",
					Namespace: "test",
					ID:        "test_test_HelmRelease",
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path, err := os.MkdirTemp("", "")
			assert.NilError(t, err)
			manager := inventory.Manager{
				Log:  logger,
				Path: path,
			}
			for _, item := range tc.items {
				switch item := item.(type) {
				case *inventory.ManifestItem:
					unstr := map[string]interface{}{
						"apiVersion": item.TypeMeta.APIVersion,
						"kind":       item.TypeMeta.Kind,
						"metadata": map[string]interface{}{
							"name":      item.Name,
							"Namespace": item.Namespace,
						},
					}
					buf := &bytes.Buffer{}
					err := json.NewEncoder(buf).Encode(&unstr)
					assert.NilError(t, err)
					err = manager.StoreItem(item, buf)
					assert.NilError(t, err)
				case *inventory.HelmReleaseItem:
					err := manager.StoreItem(item, nil)
					assert.NilError(t, err)
				}
			}
			storage, err := manager.Load()
			assert.NilError(t, err)
			for _, item := range tc.items {
				assert.Assert(t, storage.HasItem(item))
			}
		})
	}
}

var storageResult *inventory.Storage

func BenchmarkManager_Load(b *testing.B) {
	logger := setUp()
	path, err := os.MkdirTemp("", "")
	assert.NilError(b, err)
	manager := inventory.Manager{
		Log:  logger,
		Path: path,
	}
	for i := 0; i < 1000; i++ {
		item := &inventory.ManifestItem{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			Name:      strconv.Itoa(i),
			Namespace: "namespace",
		}
		err := manager.StoreItem(item, nil)
		assert.NilError(b, err)
	}
	var storage *inventory.Storage
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		storage, err = manager.Load()
	}
	storageResult = storage
}
