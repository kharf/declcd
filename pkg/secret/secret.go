package secret

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/otiai10/copy"
	"golang.org/x/sync/errgroup"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"filippo.io/age"
	"filippo.io/age/armor"
	internalCue "github.com/kharf/declcd/internal/cue"
	"github.com/kharf/declcd/pkg/kube"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	SecretsStatePackage           = "secrets"
	SecretsStateFileName          = "secrets.cue"
	SecretsStateValue             = "secrets"
	SecretsStateRecipientFileName = "recipients.cue"
	SecretsStateRecipientValue    = "recipient"
	K8sSecretName                 = "dec-key"
	K8sSecretDataKey              = "priv"
)

var (
	omittedValueText = "(enc;value omitted)"
	ErrKeyNotFound   = errors.New("Decryption key not found")
)

// Manager is capable of encrypting and decrypting secrets for a declcd gitops project.
// See [Decrypter] and [Encrpyter].
// Its main purpose is to maintain the encryption/decryption keys.
type Manager struct {
	Encrypter
	Decrypter
	// Namespace of the decryption key secret.
	namespace string
}

func NewManager(
	projectRoot string,
	namespace string,
	kubeClient kube.Client[unstructured.Unstructured],
	workerPoolSize int,
) Manager {
	return Manager{
		Encrypter: NewEncrypter(projectRoot),
		Decrypter: NewDecrypter(namespace, kubeClient, workerPoolSize),
		namespace: namespace,
	}
}

// Namespace of the decryption key secret.
func (manager Manager) Namespace() string {
	return manager.namespace
}

// CreateKeyIfNotExists creates the public/private key pair to encrypt and decrypt secrets of a declcd gitops project
// if the corresponding Kubernetes secret is not found.
// On creation it completely rewrites the secret/recipients.cue and secret/secrets.cue files
// and applies the decryption key as a Kubernetes secret.
func (manager Manager) CreateKeyIfNotExists(ctx context.Context, fieldManager string) error {
	sec, err := manager.getSecret(ctx)
	if err != nil {
		if k8sErrors.ReasonForError(err) != metav1.StatusReasonNotFound {
			return err
		}
	}
	if sec != nil {
		return nil
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return err
	}
	privKey := identity.String()
	unstr := &unstructured.Unstructured{}
	unstr.SetName(K8sSecretName)
	unstr.SetNamespace(manager.namespace)
	unstr.SetKind("Secret")
	unstr.SetAPIVersion("v1")
	unstr.Object["data"] = map[string][]byte{
		K8sSecretDataKey: []byte(privKey),
	}
	err = manager.kubeClient.Apply(ctx, unstr, fieldManager)
	if err != nil {
		return err
	}
	if err := manager.writeRecipientFile(identity.Recipient().String(), manager.projectRoot); err != nil {
		return err
	}
	if err := manager.writeSecretsStateFile(make([]encryptedInstance, 0)); err != nil {
		return err
	}
	return nil
}

func (manager Manager) writeRecipientFile(recipient string, projectRoot string) error {
	recipientFile := RecipientFile{
		Recipient: recipient,
	}
	recipientFileCueValue := cuecontext.New().Encode(recipientFile)
	if err := recipientFileCueValue.Err(); err != nil {
		return err
	}
	secretsDir := filepath.Join(projectRoot, SecretsStatePackage)
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(secretsDir, SecretsStateRecipientFileName))
	if err != nil {
		return err
	}
	defer file.Close()
	recipientFileCueDef, err := format.Node(recipientFileCueValue.Syntax())
	if err != nil {
		return err
	}
	_, err = file.WriteString(
		fmt.Sprintf("%s %s\n\n%s", "package", SecretsStatePackage, string(recipientFileCueDef)),
	)
	if err != nil {
		return err
	}
	return nil
}

type state struct {
	publicKey   string
	secretsFile SecretsStateFile
}

type SecretsStateFile struct {
	Secrets map[string]string `json:"secrets"`
}

type RecipientFile struct {
	Recipient string `json:"recipient"`
}

type file string

// Encrypter reads the public encryption key from the secret/recipients.cue file and uses it
// to encrypt every secret found in the declcd gitops repository.
type Encrypter struct {
	projectRoot string
}

func NewEncrypter(projectRoot string) Encrypter {
	return Encrypter{
		projectRoot: projectRoot,
	}
}

// EncryptPackage reads the public encryption key from the secret/recipients.cue file and uses it
// to encrypt every secret found in the cue declcd/package and stores the encrypted files in secret/secrets.cue.
func (enc Encrypter) EncryptPackage(pkg string) error {
	packageValue, err := internalCue.BuildPackage(pkg, enc.projectRoot)
	if err != nil {
		return err
	}
	state, err := lookupState(enc.projectRoot)
	if err != nil {
		return err
	}
	secretInstances, err := locateSecrets(*packageValue)
	if err != nil {
		return err
	}
	encryptedInstances, err := encrypt(secretInstances, state.publicKey)
	if err != nil {
		return err
	}
	if err := enc.writeSecretsStateFile(encryptedInstances); err != nil {
		return err
	}
	return nil
}

func (enc Encrypter) writeSecretsStateFile(encryptedInstances []encryptedInstance) error {
	secretsStateFile := SecretsStateFile{
		Secrets: make(map[string]string, len(encryptedInstances)),
	}
	for _, encryptedInstance := range encryptedInstances {
		path, _ := strings.CutPrefix(encryptedInstance.file, enc.projectRoot)
		secretsStateFile.Secrets[path] = encryptedInstance.content
	}
	secretsStateFileCueValue := cuecontext.New().Encode(secretsStateFile)
	if err := secretsStateFileCueValue.Err(); err != nil {
		return err
	}
	secretsDir := filepath.Join(enc.projectRoot, SecretsStatePackage)
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(secretsDir, SecretsStateFileName))
	if err != nil {
		return err
	}
	defer file.Close()
	secretsStateFileCueDef, err := format.Node(secretsStateFileCueValue.Syntax())
	if err != nil {
		return err
	}
	_, err = file.WriteString(
		fmt.Sprintf("%s %s\n\n%s", "package", SecretsStatePackage, string(secretsStateFileCueDef)),
	)
	if err != nil {
		return err
	}
	return nil
}

func lookupState(projectRoot string) (*state, error) {
	secretsPackageValue, err := internalCue.BuildPackage(
		SecretsStatePackage,
		projectRoot,
	)
	if err != nil {
		return nil, err
	}
	recipientsValue := secretsPackageValue.LookupPath(cue.ParsePath(SecretsStateRecipientValue))
	if err := recipientsValue.Err(); err != nil {
		return nil, err
	}
	publicKey, err := recipientsValue.String()
	if err != nil {
		return nil, err
	}
	encryptedSecretsValue := secretsPackageValue.LookupPath(cue.ParsePath(SecretsStateValue))
	if err := encryptedSecretsValue.Err(); err != nil {
		return nil, err
	}
	var encryptedFilesMap map[string]string
	if err = encryptedSecretsValue.Decode(&encryptedFilesMap); err != nil {
		return nil, err
	}
	return &state{
		publicKey:   publicKey,
		secretsFile: SecretsStateFile{Secrets: encryptedFilesMap},
	}, nil
}

type lineValueType int

const (
	multiLineQuotesBegin lineValueType = iota
	multiLineQuotesEnd
	multiLineSensitiveContent
	sensitiveString
	sensitiveBytes
)

type lineValue struct {
	content string
	lineValueType
	offset int
}

type positionalSecretData map[file]positionValueMap

func (dst positionalSecretData) add(secretFile file, pos int, value lineValue) {
	if _, ok := dst[secretFile]; !ok {
		dst[secretFile] = positionValueMap{
			pos: value,
		}
	} else {
		dst[secretFile][pos] = value
	}
}

type positionValueMap map[int]lineValue

type instance struct {
	file             string
	positionValueMap positionValueMap
}

func (instance instance) encrypt(recipient *age.X25519Recipient) (*encryptedInstance, error) {
	tmpFile, err := os.CreateTemp("", "*.cue")
	if err != nil {
		return nil, err
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()
	file, err := os.Open(instance.file)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	bufferedFileReader := bufio.NewReader(file)
	var out strings.Builder
	encryptWriter, err := newBufferedEncryptWriter(&out, recipient)
	if err != nil {
		return nil, err
	}
	defer encryptWriter.close()
	omitWriter := bufio.NewWriter(tmpFile)
	for pos := 1; ; pos++ {
		lineStr, err := bufferedFileReader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if value, exists := instance.positionValueMap[pos]; exists {
			switch value.lineValueType {
			case sensitiveString:
				_, err = omitWriter.WriteString(
					strings.Replace(
						lineStr,
						value.content,
						fmt.Sprintf("\"%s\"", omittedValueText),
						1,
					),
				)
			case sensitiveBytes:
				_, err = omitWriter.WriteString(
					strings.Replace(
						lineStr,
						value.content,
						fmt.Sprintf("'%s'", omittedValueText),
						1,
					),
				)
			case multiLineQuotesBegin, multiLineQuotesEnd:
				_, err = omitWriter.WriteString(lineStr)
			case multiLineSensitiveContent:
				// only replace the first sensitive line and omit others
				if value.offset == 1 {
					_, err = omitWriter.WriteString(fmt.Sprintf("%s\n", omittedValueText))
				}
			}
		} else {
			_, err = omitWriter.WriteString(lineStr)
		}
		if err != nil {
			return nil, err
		}
		_, err = encryptWriter.WriteString(lineStr)
		if err != nil {
			return nil, err
		}
	}
	if err := encryptWriter.Flush(); err != nil {
		return nil, err
	}
	if err := omitWriter.Flush(); err != nil {
		return nil, err
	}
	if err := encryptWriter.Close(); err != nil {
		return nil, err
	}
	src, err := os.Open(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	defer src.Close()
	if err := replaceFile(file.Name(), src); err != nil {
		return nil, err
	}
	return &encryptedInstance{
		file:    instance.file,
		content: strings.TrimSpace(out.String()),
	}, nil
}

func replaceFile(dstFileName string, src io.Reader) error {
	dst, err := os.Create(dstFileName)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func locateSecrets(value cue.Value) ([]instance, error) {
	var err error
	positionalSecretData := positionalSecretData{}
	before := func(value cue.Value) bool {
		if err != nil {
			return false
		}
		kindValue := value.LookupPath(cue.ParsePath("kind"))
		if kindValue.Err() != nil {
			return true
		}
		var kind string
		kind, err = kindValue.String()
		if err != nil {
			return false
		}
		if kind != "Secret" {
			return true
		}
		dataValue := value.LookupPath(cue.ParsePath("data"))
		stringDataValue := value.LookupPath(cue.ParsePath("stringData"))
		if dataValue.Err() != nil && stringDataValue.Err() != nil {
			return true
		}
		if err = dereferenceData(dataValue, positionalSecretData); err != nil {
			return false
		}
		if err = dereferenceData(stringDataValue, positionalSecretData); err != nil {
			return false
		}
		return true
	}
	value.Walk(before, func(v cue.Value) {})
	if err != nil {
		return nil, err
	}
	instances := make([]instance, 0, len(positionalSecretData))
	for f, v := range positionalSecretData {
		instances = append(instances, instance{
			file:             string(f),
			positionValueMap: v,
		})
	}
	return instances, nil
}

func dereferenceData(dataValue cue.Value, dst positionalSecretData) error {
	fIter, err := dataValue.Fields()
	if err != nil {
		return err
	}
	for fIter.Next() {
		secretValue := fIter.Value()
		secretFile := file(secretValue.Pos().Filename())
		pos := secretValue.Pos().Line()
		switch secretValue.Syntax().(type) {
		case *ast.BasicLit:
			basicLit := secretValue.Syntax().(*ast.BasicLit)
			dereferenceLiteral(basicLit, dst, secretFile, pos)
		case *ast.BinaryExpr:
			binaryExpr := secretValue.Syntax().(*ast.BinaryExpr)
			switch binaryExpr.X.(type) {
			case *ast.BasicLit:
				basicLit := binaryExpr.X.(*ast.BasicLit)
				dereferenceLiteral(basicLit, dst, secretFile, pos)
			}
		}
	}
	return nil
}

func dereferenceLiteral(
	basicLit *ast.BasicLit,
	dst positionalSecretData,
	secretFile file,
	pos int,
) {
	var lines []string
	if isCueMultiLineString(basicLit.Value) {
		lines = strings.Split(basicLit.Value, "\n")
		length := len(lines)
		for i, v := range lines {
			lineValueType := multiLineSensitiveContent
			if i == 0 {
				lineValueType = multiLineQuotesBegin
			} else if i == length-1 {
				lineValueType = multiLineQuotesEnd
			}
			dst.add(secretFile, pos, lineValue{
				lineValueType: lineValueType,
				content:       v,
				offset:        i,
			})
			pos += 1
		}
	} else {
		lineValueType := sensitiveBytes
		value := basicLit.Value
		if isCueString(value) {
			lineValueType = sensitiveString
		}
		dst.add(secretFile, pos, lineValue{
			lineValueType: lineValueType,
			content:       value,
		})
	}
}

func isCueString(str string) bool {
	return strings.HasPrefix(str, `"`) && strings.HasSuffix(str, `"`)
}

func isCueMultiLineString(str string) bool {
	// https://cuelang.org/docs/tutorials/tour/types/stringlit/
	// The opening quote must be followed by a newline. The closing quote must also be on a newline
	return strings.HasPrefix(str, `"""`) && strings.HasSuffix(str, `"""`) ||
		strings.HasPrefix(str, `'''`) && strings.HasSuffix(str, "'''")
}

type encryptedInstance struct {
	file    string
	content string
}

func encrypt(instances []instance, publicKey string) ([]encryptedInstance, error) {
	encryptedInstances := make([]encryptedInstance, 0, len(instances))
	recipient, err := age.ParseX25519Recipient(publicKey)
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		encryptedInstance, err := instance.encrypt(recipient)
		if err != nil {
			return nil, err
		}
		encryptedInstances = append(encryptedInstances, *encryptedInstance)
	}
	return encryptedInstances, nil
}

type encryptWriter struct {
	*bufio.Writer
	close func() error
}

func newBufferedEncryptWriter(
	out io.Writer,
	recipient *age.X25519Recipient,
) (*encryptWriter, error) {
	armorWriter := armor.NewWriter(out)
	ageWriter, err := age.Encrypt(armorWriter, recipient)
	if err != nil {
		return nil, err
	}
	return &encryptWriter{
		Writer: bufio.NewWriter(ageWriter),
		close: func() error {
			if err := ageWriter.Close(); err != nil {
				return err
			}
			if err := armorWriter.Close(); err != nil {
				return err
			}
			return nil
		},
	}, nil
}

var _ io.WriteCloser = (*encryptWriter)(nil)

func (w encryptWriter) Close() error {
	return w.close()
}

// Decrypter reads the private decryption key from a Kubernetes secret and uses it
// to decrypt every encrypted secret found in the secrets/secrets.cue file in the declcd gitops repository.
type Decrypter struct {
	namespace  string
	kubeClient kube.Client[unstructured.Unstructured]
	// The amount of worker goroutines to be used to decrypt the files concurrently.
	workerPoolSize int
}

func NewDecrypter(
	namespace string,
	kubeClient kube.Client[unstructured.Unstructured],
	workerPoolSize int,
) Decrypter {
	return Decrypter{
		namespace:      namespace,
		kubeClient:     kubeClient,
		workerPoolSize: workerPoolSize,
	}
}

// Decrypt reads the private decryption key from a Kubernetes secret and uses it
// to decrypt every encrypted secret found in the secrets/secrets.cue file in the declcd gitops repository.
// It returns the path to the decrypted declcd project.
func (dec Decrypter) Decrypt(
	ctx context.Context,
	projectRoot string,
) (string, error) {
	decryptedProjectPath := fmt.Sprintf("%s-%s", projectRoot, "dec")
	eg := &errgroup.Group{}
	eg.SetLimit(dec.workerPoolSize)
	stateChan := make(chan *state, 1)
	identityChan := make(chan *age.X25519Identity, 1)
	eg.Go(func() error {
		if err := os.RemoveAll(decryptedProjectPath); err != nil {
			return err
		}
		if err := os.MkdirAll(decryptedProjectPath, 0700); err != nil {
			return err
		}
		if err := copy.Copy(projectRoot, decryptedProjectPath); err != nil {
			return err
		}
		state, err := lookupState(decryptedProjectPath)
		if err != nil {
			return err
		}
		stateChan <- state
		return nil
	})
	eg.Go(func() error {
		unstrSec, err := dec.getSecret(ctx)
		if err != nil {
			return err
		}
		var sec v1.Secret
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstrSec.Object, &sec)
		if err != nil {
			return err
		}
		privKey := sec.Data[K8sSecretDataKey]
		identity, err := age.ParseX25519Identity(string(privKey))
		if err != nil {
			return err
		}
		identityChan <- identity
		return nil
	})
	if err := eg.Wait(); err != nil {
		return "", err
	}
	state := <-stateChan
	identity := <-identityChan
	for fileName, secret := range state.secretsFile.Secrets {
		eg.Go(func() error {
			return decrpyt(fileName, secret, decryptedProjectPath, identity)
		})
	}
	if err := eg.Wait(); err != nil {
		return "", err
	}
	return decryptedProjectPath, nil
}

func decrpyt(
	fileName string,
	secret string,
	decryptedProjectPath string,
	identity age.Identity,
) error {
	armorReader := armor.NewReader(strings.NewReader(secret))
	ageReader, err := age.Decrypt(armorReader, identity)
	if err != nil {
		return err
	}
	err = replaceFile(filepath.Join(decryptedProjectPath, fileName), ageReader)
	if err != nil {
		return err
	}
	return nil
}

func (dec Decrypter) getSecret(ctx context.Context) (*unstructured.Unstructured, error) {
	unstr := &unstructured.Unstructured{}
	unstr.SetName(K8sSecretName)
	unstr.SetNamespace(dec.namespace)
	unstr.SetKind("Secret")
	unstr.SetAPIVersion("v1")
	return dec.kubeClient.Get(ctx, unstr)
}
