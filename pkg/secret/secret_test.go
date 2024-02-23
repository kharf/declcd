package secret_test

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/secret"
	_ "github.com/kharf/declcd/test/workingdir"
	"go.uber.org/goleak"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
	)
}

var allInOneOmittedContent = `package secrets

import (
	"github.com/kharf/declcd/api/v1"
	corev1 "k8s.io/api/core/v1"
)

#Namespace: {
	_name!: string
	v1.#Component & {
		content: v1.#Manifest & {
			apiVersion: "v1"
			kind:       "Namespace"
			metadata: {
				name: _name
			}
		}
	}
}

ns: #Namespace & {
	_name: "mynamespace"
}

#Secret: {
	_name!: string
	data: {[string]: bytes}
	stringData: {[string]: string}
	corev1.#Secret & {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      _name
			namespace: ns.content.metadata.name
		}
	}
}

b: #Secret & {
	_name: "b"
	stringData: {
		foo: _bSecret
	}
}

c: #Secret & {
	_name: "c"
	data: {
		foo: _cSecret
	}
}

data: #Secret & {
	_name: "data"
	data: {
		foo: '(enc;value omitted)'
	}
}

stringData: #Secret & {
	_name: "stringData"
	stringData: {
		foo: "(enc;value omitted)"
	}
}

both: #Secret & {
	_name: "both"
	stringData: {
		foo: "(enc;value omitted)"
	}
	data: {
		foo: '(enc;value omitted)'
	}
}

multiLine: #Secret & {
	_name: "multiLine"
	stringData: {
		foo: """
(enc;value omitted)
			"""
	}
	data: {
		foo: '''
(enc;value omitted)
			'''
	}
}

none: #Secret & {
	_name: "none"
}
`

var allInOneDecryptedContent = `package secrets

import (
	"github.com/kharf/declcd/api/v1"
	corev1 "k8s.io/api/core/v1"
)

#Namespace: {
	_name!: string
	v1.#Component & {
		content: v1.#Manifest & {
			apiVersion: "v1"
			kind:       "Namespace"
			metadata: {
				name: _name
			}
		}
	}
}

ns: #Namespace & {
	_name: "mynamespace"
}

#Secret: {
	_name!: string
	data: {[string]: bytes}
	stringData: {[string]: string}
	corev1.#Secret & {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      _name
			namespace: ns.content.metadata.name
		}
	}
}

b: #Secret & {
	_name: "b"
	stringData: {
		foo: _bSecret
	}
}

c: #Secret & {
	_name: "c"
	data: {
		foo: _cSecret
	}
}

data: #Secret & {
	_name: "data"
	data: {
		foo: 'bar'
	}
}

stringData: #Secret & {
	_name: "stringData"
	stringData: {
		foo: "bar"
	}
}

both: #Secret & {
	_name: "both"
	stringData: {
		foo: "bar"
	}
	data: {
		foo: 'bar'
	}
}

multiLine: #Secret & {
	_name: "multiLine"
	stringData: {
		foo: """
				bar
				bar
				bar
			"""
	}
	data: {
		foo: '''
				bar
				bar
				bar
			'''
	}
}

none: #Secret & {
	_name: "none"
}
`

var aOmittedContent = `package secrets

_fooSecret: '(enc;value omitted)'
a: #Secret & {
	_name: "a"
	data: {
		foo: _fooSecret
	}
}
`

var aDecryptedContent = `package secrets

_fooSecret: 'bar'
a: #Secret & {
	_name: "a"
	data: {
		foo: _fooSecret
	}
}
`

var bOmittedContent = `package secrets

_bSecret: "(enc;value omitted)"
`

var bDecryptedContent = `package secrets

_bSecret: "bar"
`

var cOmittedContent = `package secrets

_cSecretFar: '(enc;value omitted)'
`

var cDecryptedContent = `package secrets

_cSecretFar: 'bar'
`

func TestEncrypter_EncryptPackage(t *testing.T) {
	env := projecttest.StartProjectEnv(t,
		projecttest.WithProjectSource("secret"),
		projecttest.WithKubernetes(kubetest.WithKubernetesDisabled()),
	)
	defer env.Stop()
	privKey := "AGE-SECRET-KEY-1EYUZS82HMQXK0S83AKAP6NJ7HPW6KMV70DHHMH4TS66S3NURTWWS034Q34"
	identity, err := age.ParseX25519Identity(privKey)
	assert.NilError(t, err)
	err = secret.NewEncrypter(env.TestProject).EncryptPackage("infra/secrets")
	assert.NilError(t, err)
	secretsFile := readSecretsFile(t, env.TestProject)
	assert.Equal(t, len(secretsFile.Secrets), 4)
	testCases := []struct {
		name                     string
		expectedPath             string
		expectedOmittedContent   string
		expedtedDecryptedContent string
	}{
		{
			name:                     "AllInOneFile",
			expectedPath:             "/infra/secrets/component.cue",
			expectedOmittedContent:   allInOneOmittedContent,
			expedtedDecryptedContent: allInOneDecryptedContent,
		},
		{
			name:                     "A",
			expectedPath:             "/infra/secrets/a.cue",
			expectedOmittedContent:   aOmittedContent,
			expedtedDecryptedContent: aDecryptedContent,
		},
		{
			name:                     "B",
			expectedPath:             "/infra/secrets/bsecret.cue",
			expectedOmittedContent:   bOmittedContent,
			expedtedDecryptedContent: bDecryptedContent,
		},
		{
			name:                     "C",
			expectedPath:             "/infra/secrets/csecretfar.cue",
			expectedOmittedContent:   cOmittedContent,
			expedtedDecryptedContent: cDecryptedContent,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := os.Open(filepath.Join(env.TestProject, tc.expectedPath))
			assert.NilError(t, err)
			defer result.Close()
			assertCue(t, result, tc.expectedOmittedContent)
			encryptedContent, found := secretsFile.Secrets[tc.expectedPath]
			assert.Assert(t, found)
			encReader := strings.NewReader(encryptedContent)
			ar := armor.NewReader(encReader)
			reader, err := age.Decrypt(ar, identity)
			assert.NilError(t, err)
			out := &strings.Builder{}
			_, err = io.Copy(out, reader)
			assert.NilError(t, err)
			assert.Equal(t, out.String(), tc.expedtedDecryptedContent)
		})
	}
}

func readSecretsFile(t *testing.T, testProject string) secret.SecretsStateFile {
	secrets, err := os.ReadFile(filepath.Join(testProject, "secrets/secrets.cue"))
	assert.NilError(t, err)
	cueCtx := cuecontext.New()
	secretsValue := cueCtx.CompileBytes(secrets)
	assert.NilError(t, secretsValue.Err())
	var instance secret.SecretsStateFile
	assert.NilError(t, secretsValue.Decode(&instance))
	return instance
}

func assertCue(t *testing.T, result *os.File, expectedResult string) {
	r := bufio.NewReader(result)
	expectedResultScanner := bufio.NewScanner(strings.NewReader(expectedResult))
	for {
		isEOF := expectedResultScanner.Scan()
		line, err := r.ReadString('\n')
		if err == io.EOF {
			assert.Assert(t, !isEOF)
			assert.NilError(t, expectedResultScanner.Err())
			break
		}
		assert.NilError(t, err)
		assert.NilError(t, expectedResultScanner.Err())
		assert.Assert(t, isEOF)
		expectedLine := expectedResultScanner.Text()
		assert.Equal(t, strings.TrimSpace(line), strings.TrimSpace(expectedLine))
	}
}

func TestDecrypter_Decrypt(t *testing.T) {
	env := projecttest.StartProjectEnv(t,
		projecttest.WithProjectSource("secret"),
		projecttest.WithKubernetes(
			kubetest.WithHelm(false, false),
			kubetest.WithDecryptionKeyCreated(),
		),
	)
	defer env.Stop()
	err := secret.NewEncrypter(env.TestProject).EncryptPackage("infra/secrets")
	assert.NilError(t, err)
	newProjectRoot, err := secret.NewDecrypter(
		env.KubetestEnv.SecretManager.Namespace(), env.DynamicTestKubeClient, runtime.GOMAXPROCS(0),
	).Decrypt(env.Ctx, env.TestProject)
	assert.NilError(t, err)
	result, err := os.Open(filepath.Join(newProjectRoot, "infra/secrets/component.cue"))
	assert.NilError(t, err)
	assertCue(t, result, allInOneDecryptedContent)
	assert.Assert(t, env.TestProject != newProjectRoot)
}

func TestManager_CreateKeyIfNotExists(t *testing.T) {
	env := projecttest.StartProjectEnv(t, projecttest.WithProjectSource("empty"))
	defer env.Stop()
	nsStr := "test"
	t.Run("GetError", func(t *testing.T) {
		expectedErr := errors.New("error")
		err := secret.NewManager(env.TestProject, nsStr, &kubetest.FakeDynamicClient{
			Err: expectedErr,
		}, runtime.GOMAXPROCS(0)).CreateKeyIfNotExists(env.Ctx, "manager")
		assert.ErrorIs(t, err, expectedErr)
		var sec corev1.Secret
		err = env.TestKubeClient.Get(
			env.Ctx,
			types.NamespacedName{Namespace: nsStr, Name: secret.K8sSecretName},
			&sec,
		)
		assert.Assert(t, k8sErrors.ReasonForError(err) == v1.StatusReasonNotFound)
		_, err = os.Open(filepath.Join(env.TestProject, "secrets/recipients.cue"))
		assert.Assert(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("Existing", func(t *testing.T) {
		manager := secret.NewManager(
			env.TestProject,
			nsStr,
			env.DynamicTestKubeClient,
			runtime.GOMAXPROCS(0),
		)
		err := manager.CreateKeyIfNotExists(env.Ctx, "manager")
		assert.NilError(t, err)
		recipientFile := readRecipientFile(t, env.TestProject)
		assert.Assert(t, recipientFile.Recipient != "")
		secretsFile := readSecretsFile(t, env.TestProject)
		assert.Assert(t, len(secretsFile.Secrets) == 0)
		var sec corev1.Secret
		err = env.TestKubeClient.Get(
			env.Ctx,
			types.NamespacedName{Namespace: nsStr, Name: secret.K8sSecretName},
			&sec,
		)
		assert.NilError(t, err)
		key, found := sec.Data[secret.K8sSecretDataKey]
		assert.Assert(t, found)
		assert.Assert(t, strings.HasPrefix(string(key), "AGE-SECRET-KEY-"))
		err = manager.CreateKeyIfNotExists(env.Ctx, "manager")
		assert.NilError(t, err)
		recipientFile2 := readRecipientFile(t, env.TestProject)
		assert.Assert(t, recipientFile.Recipient != "")
		secretsFile2 := readSecretsFile(t, env.TestProject)
		assert.Assert(t, len(secretsFile.Secrets) == 0)
		var sec2 corev1.Secret
		err = env.TestKubeClient.Get(
			env.Ctx,
			types.NamespacedName{Namespace: nsStr, Name: secret.K8sSecretName},
			&sec2,
		)
		assert.NilError(t, err)
		key2, found := sec.Data[secret.K8sSecretDataKey]
		assert.Equal(t, string(key2), string(key))
		assert.Assert(t, maps.Equal(secretsFile2.Secrets, secretsFile.Secrets))
		assert.Equal(t, recipientFile2.Recipient, recipientFile.Recipient)
	})
	err := secret.NewManager(env.TestProject, nsStr, env.DynamicTestKubeClient, runtime.GOMAXPROCS(0)).
		CreateKeyIfNotExists(env.Ctx, "manager")
	assert.NilError(t, err)
	recipientFile := readRecipientFile(t, env.TestProject)
	assert.Assert(t, recipientFile.Recipient != "")
	secretsFile := readSecretsFile(t, env.TestProject)
	assert.Assert(t, len(secretsFile.Secrets) == 0)
	var sec corev1.Secret
	err = env.TestKubeClient.Get(
		env.Ctx,
		types.NamespacedName{Namespace: nsStr, Name: secret.K8sSecretName},
		&sec,
	)
	assert.NilError(t, err)
	key, found := sec.Data[secret.K8sSecretDataKey]
	assert.Assert(t, found)
	assert.Assert(t, strings.HasPrefix(string(key), "AGE-SECRET-KEY-"))
}

func readRecipientFile(t *testing.T, testProject string) secret.RecipientFile {
	recipient, err := os.ReadFile(filepath.Join(testProject, "secrets/recipients.cue"))
	assert.NilError(t, err)
	cueCtx := cuecontext.New()
	recipientValue := cueCtx.CompileBytes(recipient)
	assert.NilError(t, recipientValue.Err())
	var instance secret.RecipientFile
	assert.NilError(t, recipientValue.Decode(&instance))
	return instance
}

var decryptResult string

func BenchmarkDecrypter_Decrypt(b *testing.B) {
	env := projecttest.StartProjectEnv(b,
		projecttest.WithProjectSource("secret"),
		projecttest.WithKubernetes(
			kubetest.WithHelm(false, false),
			kubetest.WithDecryptionKeyCreated(),
		),
	)
	defer env.Stop()
	err := secret.NewEncrypter(env.TestProject).EncryptPackage("infra/secrets")
	assert.NilError(b, err)
	workerPoolSize := runtime.GOMAXPROCS(0)
	b.ResetTimer()
	var newProjectRoot string
	for n := 0; n < b.N; n++ {
		newProjectRoot, err = secret.NewDecrypter(
			env.KubetestEnv.SecretManager.Namespace(),
			env.DynamicTestKubeClient,
			workerPoolSize,
		).Decrypt(env.Ctx, env.TestProject)
	}
	decryptResult = newProjectRoot
}
