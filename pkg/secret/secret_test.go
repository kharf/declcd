package secret_test

import (
	"bufio"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/secret"
	_ "github.com/kharf/declcd/test/workingdir"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var expectedEncryptedContent = `package secrets

import "k8s.io/api/core/v1"

#data: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	data: {
		foo: '(enc;value omitted)'
	}
}

#stringData: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	stringData: {
		foo: "(enc;value omitted)"
	}
}

#both: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	stringData: {
		foo: "(enc;value omitted)"
	}
	data: {
		foo: '(enc;value omitted)'
	}
}

#multiLine: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
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

#none: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
}
`

var expectedDecryptedContent = `package secrets

import "k8s.io/api/core/v1"

#data: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	data: {
		foo: 'bar'
	}
}

#stringData: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	stringData: {
		foo: "bar"
	}
}

#both: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	stringData: {
		foo: "bar"
	}
	data: {
		foo: 'bar'
	}
}

#multiLine: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
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

#none: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
}
`

func TestEncrypter_EncryptComponent(t *testing.T) {
	env := projecttest.StartProjectEnv(t,
		projecttest.WithProjectSource("secret"),
		projecttest.WithKubernetes(kubetest.WithKubernetesDisabled()),
	)
	defer env.Stop()
	privKey := "AGE-SECRET-KEY-1EYUZS82HMQXK0S83AKAP6NJ7HPW6KMV70DHHMH4TS66S3NURTWWS034Q34"
	identity, err := age.ParseX25519Identity(privKey)
	assert.NilError(t, err)
	err = secret.NewEncrypter(env.TestProject).EncryptComponent("infra/secrets")
	assert.NilError(t, err)
	result, err := os.Open(filepath.Join(env.TestProject, "infra/secrets/secrets.cue"))
	assert.NilError(t, err)
	defer result.Close()
	assertCue(t, result, expectedEncryptedContent)
	secretsFile := readSecretsFile(t, env.TestProject)
	assert.Equal(t, len(secretsFile.Secrets), 1)
	for k, v := range secretsFile.Secrets {
		assert.Equal(t, k, "/infra/secrets/secrets.cue")
		encReader := strings.NewReader(v)
		ar := armor.NewReader(encReader)
		reader, err := age.Decrypt(ar, identity)
		assert.NilError(t, err)
		out := &strings.Builder{}
		_, err = io.Copy(out, reader)
		assert.NilError(t, err)
		assert.Equal(t, out.String(), expectedDecryptedContent)
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
	err := secret.NewEncrypter(env.TestProject).EncryptComponent("infra/secrets")
	assert.NilError(t, err)
	newProjectRoot, err := secret.NewDecrypter(
		env.KubetestEnv.SecretManager.Namespace(), env.DynamicTestKubeClient,
	).Decrypt(env.Ctx, env.TestProject)
	assert.NilError(t, err)
	result, err := os.Open(filepath.Join(newProjectRoot, "infra/secrets/secrets.cue"))
	assert.NilError(t, err)
	assertCue(t, result, expectedDecryptedContent)
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
		}).CreateKeyIfNotExists(env.Ctx, "manager")
		assert.ErrorIs(t, err, expectedErr)
		var sec corev1.Secret
		err = env.TestKubeClient.Get(env.Ctx, types.NamespacedName{Namespace: nsStr, Name: secret.K8sSecretName}, &sec)
		assert.Assert(t, k8sErrors.ReasonForError(err) == v1.StatusReasonNotFound)
		_, err = os.Open(filepath.Join(env.TestProject, "secrets/recipients.cue"))
		assert.Assert(t, os.IsNotExist(err))
	})
	t.Run("Existing", func(t *testing.T) {
		manager := secret.NewManager(env.TestProject, nsStr, env.DynamicTestKubeClient)
		err := manager.CreateKeyIfNotExists(env.Ctx, "manager")
		assert.NilError(t, err)
		recipientFile := readRecipientFile(t, env.TestProject)
		assert.Assert(t, recipientFile.Recipient != "")
		secretsFile := readSecretsFile(t, env.TestProject)
		assert.Assert(t, len(secretsFile.Secrets) == 0)
		var sec corev1.Secret
		err = env.TestKubeClient.Get(env.Ctx, types.NamespacedName{Namespace: nsStr, Name: secret.K8sSecretName}, &sec)
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
		err = env.TestKubeClient.Get(env.Ctx, types.NamespacedName{Namespace: nsStr, Name: secret.K8sSecretName}, &sec2)
		assert.NilError(t, err)
		key2, found := sec.Data[secret.K8sSecretDataKey]
		assert.Equal(t, string(key2), string(key))
		assert.Assert(t, maps.Equal(secretsFile2.Secrets, secretsFile.Secrets))
		assert.Equal(t, recipientFile2.Recipient, recipientFile.Recipient)
	})
	err := secret.NewManager(env.TestProject, nsStr, env.DynamicTestKubeClient).CreateKeyIfNotExists(env.Ctx, "manager")
	assert.NilError(t, err)
	recipientFile := readRecipientFile(t, env.TestProject)
	assert.Assert(t, recipientFile.Recipient != "")
	secretsFile := readSecretsFile(t, env.TestProject)
	assert.Assert(t, len(secretsFile.Secrets) == 0)
	var sec corev1.Secret
	err = env.TestKubeClient.Get(env.Ctx, types.NamespacedName{Namespace: nsStr, Name: secret.K8sSecretName}, &sec)
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
