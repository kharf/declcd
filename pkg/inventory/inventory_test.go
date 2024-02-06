package inventory_test

import (
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
				inventory.NewManifestItem(
					metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					},
					"test",
					"a",
					"",
				),
				inventory.NewHelmReleaseItem(
					"test", "test", "test",
				),
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
				err := manager.StoreItem(item, nil)
				assert.NilError(t, err)
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
		item := inventory.NewManifestItem(
			metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			strconv.Itoa(i),
			"name",
			"namespace",
		)
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
